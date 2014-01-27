// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package utils

import (
	"fmt"
	"os"
	"path/filepath"
	"strconv"
)

// CheckIfRoot is a simple function that we can use to determine if
// the ownership of files and directories we create.
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

// MkdirForUser will call down to os.Mkdir and if the user is running as root,
// the ownership will be changed to the sudo user.
func MkdirForUser(dir string, perm os.FileMode) error {
	if err := os.Mkdir(dir, perm); err != nil {
		return err
	}
	return ChownToUser(dir)
}

// MkdirAllForUser will call down to os.MkdirAll and if the user is running as
// root, the ownership will be changed to the sudo user for each directory
// that was created.
func MkdirAllForUser(dir string, perm os.FileMode) error {
	toCreate := []string{}
	path := dir
	for {
		_, err := os.Lstat(path)
		if os.IsNotExist(err) {
			toCreate = append(toCreate, path)
		} else {
			break
		}
		path = filepath.Dir(path)
	}

	if err := os.MkdirAll(dir, perm); err != nil {
		return err
	}
	return ChownToUser(toCreate...)
}

// ChownToUser will attempt to change the ownership of all the paths
// to the user returned by the SudoCallerIds method.  Ownership change
// will only be attempted if we are running as root.
func ChownToUser(paths ...string) error {
	if !CheckIfRoot() {
		return nil
	}
	uid, gid, err := SudoCallerIds()
	if err != nil {
		return err
	}
	for _, path := range paths {
		if err := os.Chown(path, uid, gid); err != nil {
			return err
		}
	}
	return nil
}
