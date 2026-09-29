//go:build !devtransport

package main

import "flag"

// registerTransportFlag is the release-build no-op: --transport is not
// registered, so a shipped binary always resolves auto and can never be forced
// onto another transport.
func registerTransportFlag(_ *flag.FlagSet, _ *options) {}
