//go:build devtransport

package main

import "flag"

// registerTransportFlag registers the dev-only --transport override. It is
// compiled only under the devtransport tag, so a shipped release binary has no
// switch to force a transport and always resolves auto.
func registerTransportFlag(fs *flag.FlagSet, opts *options) {
	fs.Var((*transportFlag)(opts), "transport", "dev: force the engine and daemon transport (auto, uds, tcp)")
}

// transportFlag adapts options to flag.Value and records that the flag was set,
// so daemonChildArgs can forward it only when the user actually chose one.
type transportFlag options

func (t *transportFlag) String() string { return (*options)(t).transport }

func (t *transportFlag) Set(value string) error {
	(*options)(t).transport = value
	(*options)(t).transportSet = true
	return nil
}
