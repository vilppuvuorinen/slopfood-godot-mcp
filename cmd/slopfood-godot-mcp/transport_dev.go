//go:build devtransport

package main

import "flag"

func registerTransportFlag(fs *flag.FlagSet, opts *options) {
	fs.Var((*transportFlag)(opts), "transport", "dev: force the engine and daemon transport (auto, uds, tcp)")
}

type transportFlag options

func (t *transportFlag) String() string { return (*options)(t).transport }

func (t *transportFlag) Set(value string) error {
	(*options)(t).transport = value
	(*options)(t).transportSet = true
	return nil
}
