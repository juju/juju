// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package provider

import (
	"os/exec"
	"strings"

	"github.com/juju/errors"
	"github.com/juju/loggo"
)

var logger = loggo.GetLogger("juju.storage.provider")

// runCommandFunc is a function type used for running commands
// on the local machine. We use this rather than os/exec directly
// for testing purposes.
type runCommandFunc func(cmd string, args ...string) (string, error)

// logAndExec logs the specified command and arguments, executes
// them, and returns the combined stdout/stderr and an error if
// the command fails.
func logAndExec(cmd string, args ...string) (string, error) {
	logger.Debugf("running: %s %s", cmd, strings.Join(args, " "))
	c := exec.Command(cmd, args...)
	output, err := c.CombinedOutput()
	if err != nil {
		output := strings.TrimSpace(string(output))
		if len(output) > 0 {
			err = errors.Annotate(err, output)
		}
	}
	return string(output), err
}
