// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package utils

import (
	"fmt"
	"os"
	"strconv"
)

var CheckIfRoot = func() bool {
	return os.Getuid() == 0
}

// SudoCallerIds returns the user id and group id of the SUDO caller.
// If either is unset, it returns zero for both values.
// An error is returned if the relevant environment variables
// are not valid integers.
func SudoCallerIds() (uid int, gid int, err error) {
	uidStr := os.Getenv("SUDO_UID")
	gidStr := os.Getenv("SUDO_GID")

	if uidStr == "" || gidStr == "" {
		return 0, 0, nil
	}
	uid, err = strconv.Atoi(uidStr)
	if err != nil {
		return 0, 0, fmt.Errorf("invalid value %q for SUDO_UID", uidStr)
	}
	gid, err = strconv.Atoi(gidStr)
	if err != nil {
		return 0, 0, fmt.Errorf("invalid value %q for SUDO_GID", gidStr)
	}
	return
}

// MkDirForUser will call down to os.MkDir and if the user is running as root,
// the ownership will be changed to the sudo user.  If there is an error
// getting the SudoCallerIds, the directory is removed and an error returned.
func MkDirForUser(dir string, perm os.FileMode) error {
	if err := os.MkdirAll(dir, perm); err != nil {
		return err
	}
	if CheckIfRoot() {
		uid, gid, err := SudoCallerIds()
		if err != nil {
			os.RemoveAll(dir)
			return err
		}
		if err := os.Chown(dir, uid, gid); err != nil {
			os.RemoveAll(dir)
			return err
		}
	}
	return nil
}
