// Copyright 2019 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package paths

import (
	"os"

	"github.com/juju/juju/internal/errors"
)

// LogfilePermission is the file mode to use for log files.
// Windows only uses the first byte, 0400 for read-only, 0600 for read/write.
const LogfilePermission = os.FileMode(0600)

// Windows doesn't have the same issues around ownership. In fact calling
// Chown on windows always fails.

// SetSyslogOwner is a no-op on windows.
func SetSyslogOwner(filename string) error {
	return nil
}

// SetOwnership is a no-op on windows.
func SetOwnership(filePath string, wantedUser string, wantedGroup string) error {
	return nil
}

// PrimeLogFile ensures that the given log file is created with the
// correct mode and ownership.
func PrimeLogFile(path string) error {
	f, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY, LogfilePermission)
	if err != nil {
		return errors.Capture(err)
	}
	return errors.Capture(f.Close())
}

// SyslogUserGroup returns the names of the user and group that own the log files.
func SyslogUserGroup() (string, string) {
	return "noone", "noone"
}
