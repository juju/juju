// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package utils

import (
	"os"
	"os/exec"
	"strings"

	"github.com/juju/errors"
)

// RunCommand execs the provided command.
func RunCommand(cmd string, args ...string) error {
	command := exec.Command(cmd, args...)
	out, err := command.CombinedOutput()
	if err == nil {
		return nil
	}
	if _, ok := err.(*exec.ExitError); ok && len(out) > 0 {
		return errors.Errorf(
			"error executing %q: %s",
			cmd,
			strings.Replace(string(out), "\n", "; ", -1),
		)
	}
	return errors.Annotatef(err, "error executing %q", cmd)
}

// IsCmdNotFoundErr returns true if the provided error indicates that the
// command passed to exec.LookPath or exec.Command was not found.
func IsCmdNotFoundErr(err error) bool {
	err = errors.Cause(err)
	if os.IsNotExist(err) {
		// Executable could not be found, go 1.3 and later
		return true
	}
	if err == exec.ErrNotFound {
		return true
	}
	if execErr, ok := err.(*exec.Error); ok {
		// Executable could not be found, go 1.2
		if os.IsNotExist(execErr.Err) || execErr.Err == exec.ErrNotFound {
			return true
		}
	}
	return false
}
