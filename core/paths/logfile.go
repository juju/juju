// Copyright 2019 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.
//go:build !windows

package paths

import (
	"os"
	"os/user"
	"strconv"

	"github.com/juju/juju/internal/errors"
)

// LogfilePermission is the file mode to use for log files.
const LogfilePermission = os.FileMode(0640)

// SetSyslogOwner sets the owner and group of the file to be the appropriate
// syslog users as defined by the SyslogUserGroup method.
func SetSyslogOwner(filename string) error {
	user, group := SyslogUserGroup()
	return SetOwnership(filename, user, group)
}

// SetOwnership sets the ownership of a given file from a path.
// Searches for the corresponding id's from user, group and uses them to chown.
func SetOwnership(filePath string, wantedUser string, wantedGroup string) error {
	group, err := user.LookupGroup(wantedGroup)
	if err != nil {
		return errors.Capture(err)
	}
	gid, err := strconv.Atoi(group.Gid)
	if err != nil {
		return errors.Capture(err)
	}
	usr, err := user.Lookup(wantedUser)
	if err != nil {
		return errors.Capture(err)
	}
	uid, err := strconv.Atoi(usr.Uid)
	if err != nil {
		return errors.Capture(err)
	}
	return Chown(filePath, uid, gid)
}

// PrimeLogFile ensures that the given log file is created with the
// correct mode and ownership.
func PrimeLogFile(path string) error {
	f, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY, LogfilePermission)
	if err != nil {
		return errors.Capture(err)
	}
	if err := f.Close(); err != nil {
		return errors.Capture(err)
	}
	return SetSyslogOwner(path)
}

// SyslogUserGroup returns the names of the user and group that own the log files.
func SyslogUserGroup() (string, string) {
	return "syslog", "adm"
}
