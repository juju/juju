// Copyright 2019 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.
//go:build !windows

package paths

import (
	"os"

	"github.com/juju/juju/internal/errors"
)

const LogfilePermission = os.FileMode(0640)

// PrimeLogFile ensures that the given log file is created with the
// correct mode.
func PrimeLogFile(path string) error {
	f, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY, LogfilePermission)
	if err != nil {
		return errors.Capture(err)
	}
	return errors.Capture(f.Close())
}

// SyslogUserGroup returns the names of the user and group that own the log files.
func SyslogUserGroup() (string, string) {
	return "syslog", "adm"
}
