// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package testbase

import (
	"os/exec"
)

// HookCommandOutput intercepts CommandOutput to a function that passes the
// actual command and it's output back via a channel, and returns the error
// passed into this function.  It also returns a cleanup function so you can
// restore the original function
func HookCommandOutput(
	outputFunc *func(cmd *exec.Cmd) ([]byte, error), output []byte, err error) (<-chan *exec.Cmd, func()) {

	cmdChan := make(chan *exec.Cmd, 1)
	origCommandOutput := *outputFunc
	cleanup := func() {
		close(cmdChan)
		*outputFunc = origCommandOutput
	}
	*outputFunc = func(cmd *exec.Cmd) ([]byte, error) {
		cmdChan <- cmd
		return output, err
	}
	return cmdChan, cleanup
}
