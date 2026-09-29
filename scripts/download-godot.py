#!/usr/bin/env python3
"""download-godot - fetch, verify, and unpack the Godot builds in a manifest.

Downloads the Linux x86_64 Godot builds listed in a manifest of
"<minor> [<version> <sha256> [<templates-sha256>]]" lines, verifies them
against the manifest checksums, and optionally installs the matching export
templates.

Usage:
    scripts/download-godot.py install [manifest]
    scripts/download-godot.py install-latest <minor>
    scripts/download-godot.py minors [--stable] [lower]

The manifest defaults to godot.versions next to this script; pass "-" to read
it from stdin. Each version is fetched from the godotengine/godot-builds GitHub
release, checked against its manifest checksum, and unpacked into
.godot/<minor>/ so point releases of the same minor line share a directory and
upgrade in place. With no subcommand, "install" runs with the default manifest.

An entry may be just a minor (e.g. "4.7"): the release to install is resolved
from the godot-builds release manifests. Within a minor line a stable release is
preferred when one exists; otherwise the release with the newest release_date
wins. A release whose manifest lacks the engine asset for the target GODOT_OS
(or the export templates when GODOT_TEMPLATES=1) is skipped, because a tag can
be published before every platform build finishes uploading. The manifests live
in the godot-builds repository, which is shallow-cloned
once into <builds-dir> (a few MB) and refreshed on later runs, so no GitHub API
and no token are involved. A checksum missing from the manifest is filled in from
the selected release manifest (releases/godot-<tag>.json), which lists the sha512
of every published asset, so verification works without a pinned manifest. This
is how CI chases the newest published build per supported minor:

    printf '%s\\n' 4.7 4.8 | \\
        GODOT_TEMPLATES=1 \\
        GODOT_TEMPLATE_FILES='templates/version.txt templates/linux_*' \\
        scripts/download-godot.py install -

The same manifests back the "minors" subcommand, which prints every minor line
at or above a lower bound that has a published release, so CI can build its
matrix from the script instead of the GitHub API. With --stable, only lines that
have a stable release are printed, so a caller can pick the newest stable line
without resolving each one:

    scripts/download-godot.py minors 4.7
    scripts/download-godot.py minors --stable 4.7

The "install-latest" subcommand is the single-entry form of that resolution: it
resolves one minor line and installs the winning release, honoring the same
GODOT_TEMPLATES/GODOT_VERIFY overrides as "install":

    scripts/download-godot.py install-latest 4.7

When GODOT_TEMPLATES=1, the export templates for every entry are fetched,
verified, and unpacked into <templates-dir>/<godot-version>/. A pinned
<templates-sha256>, when present, wins; otherwise the release manifest supplies
one. GODOT_TEMPLATE_FILES narrows the members unpacked from the .tpz
(space-separated glob patterns); unset unpacks the whole templates/ tree.

Apart from git, which is used only to clone the release manifests, the standard
library is used throughout, so this runs on any Ubuntu box or modern macOS
without installing anything. The engine assets stay Linux x86_64 regardless of
host; set GODOT_ARCH to pick a different Linux architecture.

Environment overrides:
  GODOT_DIR            output directory for engine builds
                       (default: <repo>/.godot)
  GODOT_BUILDS_DIR     checkout of the godot-builds release manifests
                       (default: <GODOT_DIR>/godot-builds)
  GODOT_OS             engine platform to download: linux (default), windows,
                       or macos; selects the asset name, binary name, and the
                       default GODOT_TEMPLATE_FILES
  GODOT_VERIFY         set to 0 to skip checksum verification (default: 1);
                       missing checksums are filled from the release manifest,
                       so verification needs no pinned manifest
  GODOT_TEMPLATES      set to 1 to also download export templates
                       (default: 0)
  GODOT_TEMPLATE_FILES space-separated unzip patterns selecting which .tpz
                       members to unpack; unset uses the OS default
  GODOT_TEMPLATES_DIR  export templates root, i.e. Godot's
                       "<data>/godot/export_templates"
                       (default: <GODOT_DIR>/godot/export_templates)
  GODOT_REPO           GitHub "owner/repo" to download from
                       (default: godotengine/godot-builds)
  GODOT_ARCH           Linux architecture (default: x86_64)
  GODOT_FORCE          set to 1 to redownload entries that are already
                       unpacked
"""

import argparse
import fnmatch
import hashlib
import json
import os
import re
import shutil
import subprocess
import sys
import tempfile
import urllib.error
import urllib.request
import zipfile
from dataclasses import dataclass
from pathlib import Path

SCRIPT_DIR = Path(__file__).resolve().parent
REPO_ROOT = SCRIPT_DIR.parent
DEFAULT_MANIFEST = SCRIPT_DIR / "godot.versions"
USER_AGENT = "slopfood-godot-mcp-download-godot"
BUILDS_READY = set()


def die(message):
    print("error: %s" % message, file=sys.stderr)
    sys.exit(1)


def info(message):
    print("==> %s" % message, file=sys.stderr)


@dataclass
class OSProfile:
    """Per-platform asset, binary, and template defaults."""

    # Asset-name platform fragment: "linux.<arch>" or "win64.exe"/"macos.universal".
    asset_platform: str
    # Binary-name platform fragment used in diagnostics.
    binary_platform: str
    # Glob matching the engine binary inside the unpacked minor directory.
    binary_glob: str
    # Glob matching the stale point releases to remove on upgrade.
    stale_glob: str
    # Default GODOT_TEMPLATE_FILES when the variable is unset.
    template_glob: str


OS_PROFILES = {
    "linux": OSProfile(
        asset_platform="linux.{arch}",
        binary_platform="linux.{arch}",
        binary_glob="Godot_v*_linux.*",
        stale_glob="Godot_v*_linux.{arch}",
        template_glob="templates/version.txt templates/linux_*",
    ),
    "windows": OSProfile(
        asset_platform="win64.exe",
        binary_platform="win64",
        # The console-subsystem build is preferred: its stdout survives a pipe,
        # which the readiness line, --version, and diagnostics depend on. The
        # GUI build is a fallback.
        binary_glob="Godot_v*_win64_console.exe",
        stale_glob="Godot_v*_win64*.exe",
        template_glob="templates/version.txt templates/windows_*",
    ),
    "macos": OSProfile(
        asset_platform="macos.universal",
        binary_platform="macos.universal",
        binary_glob="Godot_v*_macos.*",
        stale_glob="Godot_v*_macos.*",
        template_glob="templates/version.txt templates/macosx_* templates/macos_*",
    ),
}


def os_profile(config):
    """Return the OSProfile for config.os, or die on an unknown platform."""
    profile = OS_PROFILES.get(config.os)
    if profile is None:
        die("unsupported GODOT_OS '%s': want one of %s" % (config.os, ", ".join(sorted(OS_PROFILES))))
    return profile


@dataclass
class Config:
    manifest: str
    out_dir: Path
    repo: str
    arch: str
    os: str
    force: str
    verify: str
    download_templates: str
    template_files: str
    templates_dir: Path
    builds_dir: Path


def load_config(manifest=None):
    out_dir = Path(os.environ.get("GODOT_DIR") or (REPO_ROOT / ".godot"))
    templates_dir = os.environ.get("GODOT_TEMPLATES_DIR") or (
        out_dir / "godot" / "export_templates"
    )
    builds_dir = os.environ.get("GODOT_BUILDS_DIR") or (out_dir / "godot-builds")
    target_os = os.environ.get("GODOT_OS") or "linux"
    template_files = os.environ.get("GODOT_TEMPLATE_FILES")
    if template_files is None:
        # Default to the target OS's templates so a per-OS install stays small;
        # a caller that needs another platform's templates sets the variable
        # explicitly (the Windows e2e lane adds the Linux ones the export test
        # needs).
        profile = OS_PROFILES.get(target_os)
        template_files = profile.template_glob if profile else ""
    return Config(
        manifest=str(DEFAULT_MANIFEST) if manifest is None else manifest,
        out_dir=out_dir,
        repo=os.environ.get("GODOT_REPO") or "godotengine/godot-builds",
        arch=os.environ.get("GODOT_ARCH") or "x86_64",
        os=target_os,
        force=os.environ.get("GODOT_FORCE") or "0",
        verify=os.environ.get("GODOT_VERIFY") or "1",
        download_templates=os.environ.get("GODOT_TEMPLATES") or "0",
        template_files=template_files,
        templates_dir=Path(templates_dir),
        builds_dir=Path(builds_dir),
    )


def download(url, dest):
    """Stream url into dest, raising OSError/URLError on any failure."""
    request = urllib.request.Request(url, headers={"User-Agent": USER_AGENT})
    with urllib.request.urlopen(request) as response, open(dest, "wb") as handle:
        shutil.copyfileobj(response, handle)


def checksum_of(path, expected):
    """Hash path with the algorithm implied by the expected digest length.

    Pinned manifests carry sha256 (64 hex chars); the godot-builds release
    manifests carry sha512 (128 hex chars).
    """
    algorithm = "sha512" if len(expected) == 128 else "sha256"
    digest = hashlib.new(algorithm)
    with open(path, "rb") as handle:
        for chunk in iter(lambda: handle.read(1 << 20), b""):
            digest.update(chunk)
    return digest.hexdigest()


def git(args, cwd=None):
    """Run git, returning the completed process (a missing git is fatal)."""
    try:
        return subprocess.run(
            ["git"] + args, cwd=cwd, capture_output=True, text=True
        )
    except OSError as exc:
        die("git is required to read the godot-builds manifests: %s" % exc)


def ensure_builds_repo(config):
    """Shallow-clone or refresh godot-builds, returning its local path.

    The repo holds only JSON release manifests, so a depth-1 clone of the
    default branch (a few MB) is enough to list minors, order releases by
    release_date, and read checksums without touching the GitHub API.
    """
    path = config.builds_dir
    if path in BUILDS_READY:
        return path
    if (path / ".git").is_dir():
        fetched = git(["fetch", "--depth", "1", "origin"], path)
        if fetched.returncode == 0:
            reset = git(["reset", "--hard", "FETCH_HEAD"], path)
            if reset.returncode != 0:
                info("using cached %s: %s" % (path, reset.stderr.strip()))
        else:
            info("using cached %s: %s" % (path, fetched.stderr.strip()))
    else:
        url = "https://github.com/%s.git" % config.repo
        info("cloning %s (depth 1)" % config.repo)
        path.parent.mkdir(parents=True, exist_ok=True)
        cloned = git(["clone", "--depth", "1", "--single-branch", url, str(path)])
        if cloned.returncode != 0:
            die("failed to clone %s: %s" % (url, cloned.stderr.strip()))
    BUILDS_READY.add(path)
    return path


def release_tags(config):
    """Return the release tags named by the local godot-builds manifests."""
    ensure_builds_repo(config)
    prefix = "godot-"
    return [
        path.name[len(prefix) : -len(".json")]
        for path in config.builds_dir.glob("releases/godot-*.json")
    ]


def load_release_manifest(version, config):
    """Load the local release manifest for a tag, or None.

    Every published release carries releases/godot-<tag>.json listing the
    release_date and the sha512 of each asset, which is what lets the script
    order releases and verify a download without a pinned manifest.
    """
    ensure_builds_repo(config)
    path = config.builds_dir / "releases" / ("godot-%s.json" % version)
    try:
        with path.open() as handle:
            return json.load(handle)
    except (OSError, json.JSONDecodeError) as exc:
        info("no release manifest for %s: %s" % (version, exc))
        return None


def release_sort_key(tag):
    """Order tags by version first and release stage second.

    A stable build outranks an rc of the same version, an rc outranks a beta,
    and so on. Used as a fallback when release dates are unavailable.
    """
    version = re.sub(r"-.*$", "", tag)
    parts = [int(part) for part in version.split(".") if part != ""]
    if tag.endswith("-stable"):
        rank = 4
    elif re.search(r"-rc[0-9]+$", tag):
        rank = 3
    elif re.search(r"-beta[0-9]+$", tag):
        rank = 2
    elif re.search(r"-alpha[0-9]+$", tag):
        rank = 1
    else:
        rank = 0
    match = re.search(r"(?:dev|beta|rc|alpha)([0-9]+)$", tag)
    number = int(match.group(1)) if match else 0
    return (parts, rank, number)


def candidate_tags(tags, minor):
    """Return the release tags that belong to a minor line."""
    dotted = "%s." % minor
    dashed = "%s-" % minor
    return [
        tag
        for tag in tags
        if tag == minor or tag.startswith(dotted) or tag.startswith(dashed)
    ]


def release_has_assets(release, version, config):
    """Return whether a release manifest carries the assets an install needs.

    A tag's manifest is committed when the release is published, but the
    release can go public before every platform build finishes uploading, so
    the newest tag is not always installable. Require the engine asset, and the
    export templates when they were requested, so resolution can fall back to
    the newest release that can actually be installed. A missing manifest
    (offline, or an old tag without one) is treated as usable so pinned and
    offline flows keep working.
    """
    if release is None:
        return True
    if not manifest_checksum(release, engine_asset_name(version, config)):
        return False
    if config.download_templates == "1" and not manifest_checksum(
        release, templates_asset_name(version)
    ):
        return False
    return True


def select_latest_release(records):
    """Pick from (tag, release_date, stable, ...) records for a minor line.

    A stable release wins when the line has any; otherwise the newest release
    by date wins. A zero date (manifest unavailable) defers to the tag's
    version/stage ordering, so resolution still works offline.
    """
    stables = [record for record in records if record[2]]
    pool = stables or records
    return max(pool, key=lambda record: (record[1], release_sort_key(record[0])))[0]


def resolve_latest_release(minor, config):
    """Return the newest installable release tag for a minor line.

    Ordering comes from each release's release_date in the godot-builds
    release manifest, with a stable release preferred when the line has one.
    Releases missing the assets this install needs are dropped first, so a
    half-published tag never wins over a complete older one.
    """
    candidates = candidate_tags(release_tags(config), minor)
    if not candidates:
        die("no published release found for minor '%s' in %s" % (minor, config.repo))
    records = []
    for tag in candidates:
        manifest = load_release_manifest(tag, config)
        date = int(manifest.get("release_date") or 0) if manifest else 0
        records.append(
            (
                tag,
                date,
                tag.endswith("-stable"),
                release_has_assets(manifest, tag, config),
            )
        )
    installable = [record for record in records if record[3]]
    if not installable:
        needed = (
            "engine or export-template"
            if config.download_templates == "1"
            else "engine"
        )
        die(
            "no installable release for minor '%s' in %s: every candidate (%s) "
            "lacks the %s assets for GODOT_OS=%s"
            % (
                minor,
                config.repo,
                ", ".join(record[0] for record in records),
                needed,
                config.os,
            )
        )
    return select_latest_release(installable)


def list_minors(lower, config, stable_only=False):
    """Return every published minor line at or above lower, ascending.

    With stable_only, only lines that have at least one stable release are
    listed, so a caller can pick the newest stable line without resolving each
    candidate release.
    """
    match = re.match(r"^(\d+)\.(\d+)", lower)
    if match is None:
        die("invalid minor lower bound '%s'" % lower)
    floor = (int(match.group(1)), int(match.group(2)))
    minors = {}
    for tag in release_tags(config):
        found = re.match(r"^(\d+)\.(\d+)", tag)
        if found is None:
            continue
        key = (int(found.group(1)), int(found.group(2)))
        if key < floor:
            continue
        minors[key] = minors.get(key, False) or tag.endswith("-stable")
    return [
        "%d.%d" % key
        for key, stable in sorted(minors.items())
        if stable or not stable_only
    ]


def engine_asset_name(version, config):
    """Return the engine zip asset name for a release tag and target OS."""
    profile = os_profile(config)
    platform = profile.asset_platform.format(arch=config.arch)
    return "Godot_v%s_%s.zip" % (version, platform)


def engine_binary_glob(config):
    """Return the glob matching the engine binary inside the unpacked dir."""
    return os_profile(config).binary_glob


def templates_asset_name(version):
    """Return the export templates .tpz asset name for a release tag."""
    return "Godot_v%s_export_templates.tpz" % version


def manifest_checksum(manifest, filename):
    """Return the checksum recorded for filename in a release manifest, or ""."""
    for entry in manifest.get("files", []):
        if entry.get("filename") == filename:
            return (entry.get("checksum") or "").lower()
    return ""


def template_dir_name(tag):
    """Map a release tag to the directory Godot uses (VERSION_FULL_CONFIG).

    4.7.2-stable -> 4.7.2.stable and 4.8-dev6 -> 4.8.dev6.
    """
    if "-" in tag:
        version, stage = tag.split("-", 1)
        return "%s.%s" % (version, stage)
    return "%s.%s" % (tag, tag)


def install_export_templates(version, expected, config):
    asset = templates_asset_name(version)
    url = "https://github.com/%s/releases/download/%s/%s" % (
        config.repo,
        version,
        asset,
    )
    version_dir = template_dir_name(version)
    dest_dir = config.templates_dir / version_dir

    if config.force != "1" and (dest_dir / "version.txt").is_file():
        info("already installed: export templates %s" % version)
        return

    config.templates_dir.mkdir(parents=True, exist_ok=True)
    zip_path = config.templates_dir / ("%s.part" % asset)

    info("downloading %s (%s)" % (asset, version))
    try:
        download(url, zip_path)
    except (urllib.error.URLError, OSError) as exc:
        zip_path.unlink(missing_ok=True)
        die("download failed: %s (%s)" % (url, exc))

    if config.verify == "1":
        actual = checksum_of(zip_path, expected)
        if actual != expected:
            zip_path.unlink(missing_ok=True)
            die(
                "checksum mismatch for %s: expected %s, got %s"
                % (asset, expected, actual)
            )
        info("checksum ok: export templates %s" % version)
    else:
        info("skipping checksum: export templates %s" % version)

    # templates/version.txt is authoritative for the directory name; fall back
    # to the name derived from the release tag when it is missing.
    staging = Path(tempfile.mkdtemp(prefix=".tpz.", dir=str(config.templates_dir)))
    try:
        with zipfile.ZipFile(zip_path) as archive:
            try:
                packed = archive.read("templates/version.txt").decode(
                    "utf-8", "replace"
                )
            except KeyError:
                packed = ""
            packed = re.sub(r"\s+", "", packed)
            if packed:
                version_dir = packed
                dest_dir = config.templates_dir / version_dir

            # Optional member filter; unset means the whole templates/ tree.
            patterns = config.template_files.split()
            if patterns:
                members = [
                    name
                    for name in archive.namelist()
                    if any(fnmatch.fnmatch(name, pattern) for pattern in patterns)
                ]
            else:
                members = archive.namelist()
            for name in members:
                archive.extract(name, str(staging))
    except (zipfile.BadZipFile, OSError) as exc:
        shutil.rmtree(staging, ignore_errors=True)
        zip_path.unlink(missing_ok=True)
        die("unpack failed: %s: %s" % (asset, exc))
    zip_path.unlink(missing_ok=True)

    source = staging / "templates"
    if not source.is_dir():
        shutil.rmtree(staging, ignore_errors=True)
        die("expected templates/ directory in %s" % asset)

    if dest_dir.exists():
        shutil.rmtree(dest_dir)
    dest_dir.parent.mkdir(parents=True, exist_ok=True)
    shutil.move(str(source), str(dest_dir))
    shutil.rmtree(staging, ignore_errors=True)

    if not (dest_dir / "version.txt").is_file():
        die("expected version.txt in %s" % dest_dir)
    info("installed export templates %s -> %s" % (version, dest_dir))


def installed_binary(dest_dir, config):
    """Return the first existing engine binary in dest_dir, or the preferred name.

    The glob is OS-specific, so a Windows install resolves the console build
    ahead of the plain GUI one. Returns the matched path, or the preferred
    (first-glob) path when nothing exists yet.
    """
    profile = os_profile(config)
    matches = sorted(dest_dir.glob(profile.binary_glob))
    if matches:
        return matches[0]
    name = "Godot_v*_%s" % profile.binary_platform.format(arch=config.arch)
    return dest_dir / name


def install_build(minor, version, expected, config):
    asset = engine_asset_name(version, config)
    profile = os_profile(config)
    dest_dir = config.out_dir / minor
    url = "https://github.com/%s/releases/download/%s/%s" % (
        config.repo,
        version,
        asset,
    )

    # Existence is a regular file, not X_OK: the execute bit is meaningless on
    # Windows, where the engine binary is a .exe.
    if config.force != "1" and installed_binary(dest_dir, config).is_file():
        info("already unpacked: %s" % version)
        return

    dest_dir.mkdir(parents=True, exist_ok=True)
    zip_path = dest_dir / ("%s.part" % asset)

    info("downloading %s (%s)" % (asset, version))
    try:
        download(url, zip_path)
    except (urllib.error.URLError, OSError) as exc:
        zip_path.unlink(missing_ok=True)
        die("download failed: %s (%s)" % (url, exc))

    if config.verify == "1":
        actual = checksum_of(zip_path, expected)
        if actual != expected:
            zip_path.unlink(missing_ok=True)
            die(
                "checksum mismatch for %s: expected %s, got %s"
                % (version, expected, actual)
            )
        info("checksum ok: %s" % version)
    else:
        info("skipping checksum: %s" % version)

    # Drop any older point release already sitting in the shared minor dir. The
    # glob follows the target OS, so the Linux and Windows builds of a minor
    # line can coexist without one pruning the other.
    stale_glob = profile.stale_glob.format(arch=config.arch)
    keep = {installed_binary(dest_dir, config).name}
    for stale in dest_dir.glob(stale_glob):
        if stale.name not in keep and stale.is_file():
            stale.unlink()

    try:
        with zipfile.ZipFile(zip_path) as archive:
            archive.extractall(str(dest_dir))
    except (zipfile.BadZipFile, OSError) as exc:
        zip_path.unlink(missing_ok=True)
        die("unpack failed: %s: %s" % (asset, exc))
    zip_path.unlink(missing_ok=True)

    target = installed_binary(dest_dir, config)
    if not target.is_file():
        die("expected binary not found after unpack: %s" % target)
    if config.os != "windows":
        target.chmod(target.stat().st_mode | 0o111)
    info("installed %s -> %s" % (version, target))


def install_entry(minor, version, expected, templates_expected, config, source):
    """Install one resolved (minor, version) pair, filling missing checksums.

    Shared by the manifest-driven "install" and the single-entry
    "install-latest"; source names where the entry came from for diagnostics.
    """
    if not (
        version == minor
        or version.startswith(minor + ".")
        or version.startswith(minor + "-")
    ):
        die(
            "version '%s' does not belong to minor '%s' in %s"
            % (version, minor, source)
        )
    if config.verify == "1":
        expected = expected.lower()
    templates_expected = templates_expected.lower()

    # Fill checksums the manifest did not pin from the release manifest for the
    # tag, so a minor-only entry can still be verified.
    need_engine = config.verify == "1" and not expected
    need_templates = (
        config.verify == "1"
        and config.download_templates == "1"
        and not templates_expected
    )
    if need_engine or need_templates:
        release = load_release_manifest(version, config)
        if release is None:
            die(
                "missing checksum for version '%s' in %s and no "
                "release manifest for %s in %s (set GODOT_VERIFY=0 "
                "to skip)" % (version, source, version, config.repo)
            )
        if need_engine:
            expected = manifest_checksum(
                release, engine_asset_name(version, config)
            )
        if need_templates:
            templates_expected = manifest_checksum(
                release, templates_asset_name(version)
            )

    if config.verify == "1" and not expected:
        die(
            "missing checksum for version '%s' in %s "
            "(set GODOT_VERIFY=0 to skip)" % (version, source)
        )

    install_build(minor, version, expected, config)

    if config.download_templates == "1":
        if config.verify == "1" and not templates_expected:
            info("no export templates checksum for %s; skipping" % version)
        else:
            install_export_templates(version, templates_expected, config)


def cmd_install(manifest):
    config = load_config(manifest)
    manifest_name = config.manifest
    if config.manifest == "-":
        lines = sys.stdin
        manifest_name = "<stdin>"
        should_close = False
    else:
        path = Path(config.manifest)
        if not path.is_file():
            die("manifest not found: %s" % path)
        lines = path.open()
        should_close = True

    config.out_dir.mkdir(parents=True, exist_ok=True)
    try:
        for raw in lines:
            line = raw.split("#", 1)[0]
            fields = line.split()
            minor = fields[0] if len(fields) > 0 else ""
            version = fields[1] if len(fields) > 1 else ""
            expected = fields[2] if len(fields) > 2 else ""
            templates_expected = fields[3] if len(fields) > 3 else ""

            if not minor:
                continue
            if not re.match(r"^[0-9].*\.[0-9]", minor):
                die("invalid minor version '%s' in %s" % (minor, manifest_name))

            if not version:
                version = resolve_latest_release(minor, config)
                info("resolved %s -> %s" % (minor, version))

            install_entry(
                minor,
                version,
                expected,
                templates_expected,
                config,
                manifest_name,
            )
    finally:
        if should_close:
            lines.close()

    info("done; builds in %s" % config.out_dir)


def cmd_install_latest(minor):
    config = load_config(None)
    if not re.match(r"^[0-9].*\.[0-9]", minor):
        die("invalid minor version '%s'" % minor)
    config.out_dir.mkdir(parents=True, exist_ok=True)
    version = resolve_latest_release(minor, config)
    info("resolved %s -> %s" % (minor, version))
    install_entry(minor, version, "", "", config, "minor '%s'" % minor)
    info("done; builds in %s" % config.out_dir)


def build_parser():
    parser = argparse.ArgumentParser(
        prog="download-godot.py",
        description="Download, verify, and unpack the Godot builds in a manifest.",
        formatter_class=argparse.RawDescriptionHelpFormatter,
    )
    subparsers = parser.add_subparsers(
        dest="command", metavar="{install,install-latest,minors}"
    )
    install = subparsers.add_parser(
        "install",
        help="download, verify, and unpack the builds in a manifest (default)",
    )
    install.add_argument(
        "manifest",
        nargs="?",
        default=None,
        help="manifest path, or - for stdin (default: scripts/godot.versions)",
    )
    latest = subparsers.add_parser(
        "install-latest",
        help="resolve a minor line to its newest release and install it",
    )
    latest.add_argument("minor", help="minor line to resolve, e.g. 4.7")
    minors = subparsers.add_parser(
        "minors", help="print published minor lines at or above a lower bound"
    )
    minors.add_argument(
        "lower",
        nargs="?",
        default="0.0",
        help="lowest minor line to print, e.g. 4.7 (default: 0.0)",
    )
    minors.add_argument(
        "--stable",
        action="store_true",
        help="only print lines that have a stable release",
    )
    return parser


def main(argv=None):
    args = build_parser().parse_args(argv)
    if args.command in (None, "install"):
        cmd_install(getattr(args, "manifest", None))
    elif args.command == "install-latest":
        cmd_install_latest(args.minor)
    elif args.command == "minors":
        for minor in list_minors(args.lower, load_config(None), args.stable):
            print(minor)


if __name__ == "__main__":
    main()
