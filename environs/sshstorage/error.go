// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package sshstorage

import (
	"fmt"
)

type SSHStorageError struct {
	Output   string
	ExitCode int
}

func (e SSHStorageError) Error() string {
	if e.Output == "" {
		return fmt.Sprintf("exit code %d", e.ExitCode)
	}
	return e.Output
}
