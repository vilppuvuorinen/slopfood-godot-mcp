package godot

import (
	"bufio"
	"strings"
	"testing"
)

func TestParseReadySkipsBanner(t *testing.T) {
	for _, line := range []string{
		"Godot Engine v4.7.2.stable.official.ed1daf0bf - https://godotengine.org\n",
		"some engine diagnostic\n",
		"{\"ready\":false}\n",
	} {
		info, err := parseReady(line)
		if err != nil {
			t.Errorf("parseReady(%q) error = %v, want nil", line, err)
		}
		if info != nil {
			t.Errorf("parseReady(%q) = %+v, want nil", line, info)
		}
	}
}

func TestParseReadyObject(t *testing.T) {
	line := `{"ready":true,"transport":"uds","path":"/tmp/p.engine.sock","token":"ab"}` + "\n"
	info, err := parseReady(line)
	if err != nil {
		t.Fatalf("parseReady: %v", err)
	}
	if info == nil {
		t.Fatal("parseReady returned nil, want the readiness object")
	}
	if !info.Ready || info.Transport != "uds" || info.Path != "/tmp/p.engine.sock" || info.Token != "ab" {
		t.Errorf("readiness = %+v", info)
	}
}

// TestParseReadyTCPObject asserts a TCP readiness object carries its host and
// port instead of a path.
func TestParseReadyTCPObject(t *testing.T) {
	line := `{"ready":true,"transport":"tcp","host":"127.0.0.1","port":44444,"token":"cd"}` + "\n"
	info, err := parseReady(line)
	if err != nil {
		t.Fatalf("parseReady: %v", err)
	}
	if info == nil {
		t.Fatal("parseReady returned nil, want the readiness object")
	}
	if !info.Ready || info.Transport != "tcp" || info.Host != "127.0.0.1" || info.Port != 44444 || info.Token != "cd" {
		t.Errorf("readiness = %+v", info)
	}
}

func TestParseReadyMalformed(t *testing.T) {
	if _, err := parseReady(`{"ready":` + "\n"); err == nil {
		t.Fatal("parseReady accepted malformed JSON, want an error")
	}
}

func TestReadReadinessSkipsBanner(t *testing.T) {
	lines := "Godot Engine v4.7.2\nready\n" + `{"ready":true,"transport":"uds","path":"p","token":"t"}` + "\n"
	info, err := readReadiness(bufio.NewReader(strings.NewReader(lines)), func() {})
	if err != nil {
		t.Fatalf("readReadiness: %v", err)
	}
	if info == nil || info.Token != "t" {
		t.Fatalf("readiness = %+v", info)
	}
}

func TestReadReadinessMalformed(t *testing.T) {
	if _, err := readReadiness(bufio.NewReader(strings.NewReader(`{"ready":`+"\n")), func() {}); err == nil {
		t.Fatal("readReadiness accepted a malformed readiness line, want an error")
	}
}
