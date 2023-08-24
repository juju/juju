// Copyright 2019 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package common

import (
	"os"
	"os/exec"

	"github.com/juju/errors"
)

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
