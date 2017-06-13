// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package cmdtest

import (
	"github.com/juju/cmd"
	"github.com/juju/cmd/cmdtesting"

	"github.com/juju/juju/provider/dummy"
)

// RunCommandWithDummyProvider runs the command and returns channels holding the
// command's operations and errors.
func RunCommandWithDummyProvider(ctx *cmd.Context, com cmd.Command, args ...string) (opc chan dummy.Operation, errc chan error) {
	if ctx == nil {
		panic("ctx == nil")
	}
	errc = make(chan error, 1)
	opc = make(chan dummy.Operation, 200)
	dummy.Listen(opc)
	go func() {
		defer func() {
			// signal that we're done with this ops channel.
			dummy.Listen(nil)
			// now that dummy is no longer going to send ops on
			// this channel, close it to signal to test cases
			// that we are done.
			close(opc)
		}()

		if err := cmdtesting.InitCommand(com, args); err != nil {
			errc <- err
			return
		}

		errc <- com.Run(ctx)
	}()
	return
}
