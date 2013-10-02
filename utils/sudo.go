// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package utils

import (
	"fmt"
	"os"
	"strconv"
)

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
