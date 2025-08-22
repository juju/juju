// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package provider

import (
	"context"
	"os/exec"
	"strings"

	"github.com/juju/errors"

	internallogger "github.com/juju/juju/internal/logger"
)

var logger = internallogger.GetLogger("juju.storage.provider")

// RunCommandFunc is a function type used for running commands
// on the local machine. We use this rather than os/exec directly
// for testing purposes.
type RunCommandFunc func(cmd string, args ...string) (string, error)

// LogAndExec logs the specified command and arguments, executes
// them, and returns the combined stdout/stderr and an error if
// the command fails.
func LogAndExec(cmd string, args ...string) (string, error) {
	logger.Debugf(context.TODO(), "running: %s %s", cmd, strings.Join(args, " "))
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
