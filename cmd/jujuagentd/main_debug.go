// Copyright 2012, 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

//go:build debug

package main

import (
	"fmt"
	"os"
	"slices"

	"github.com/juju/juju/internal/dlv"
)

func main() {
	args := os.Args

	// enable debug except for command `version` which relies on command output. Debugging server
	// will mess up with that command.
	if slices.Contains(args, "version") {
		os.Exit(Main(args))
	}

	// Start the delve runner against a socket.
	debugMain := dlv.NewDlvRunner(
		dlv.WithLogger(logger.Child("dlv")),
		dlv.Headless(),
		dlv.NoWait(),
		dlv.WithApiVersion(2),
		dlv.WithSocket(fmt.Sprintf("%s.%s.socketd", args[0], args[1])),
	)(Main)

	os.Exit(debugMain(args))
}
