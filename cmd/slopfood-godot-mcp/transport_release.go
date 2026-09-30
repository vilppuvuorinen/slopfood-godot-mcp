//go:build !devtransport

package main

import "flag"

func registerTransportFlag(_ *flag.FlagSet, _ *options) {}
