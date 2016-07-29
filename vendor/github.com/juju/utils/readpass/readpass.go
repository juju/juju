// Copyright 2014 Canonical Ltd.
// Licensed under the LGPLv3, see LICENCE file for details.

package readpass

import (
	"os"

	"golang.org/x/crypto/ssh/terminal"
)

func ReadPassword() (string, error) {
	fd := os.Stdin.Fd()
	pass, err := terminal.ReadPassword(int(fd))
	return string(pass), err
}
