// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package common

import (
	"fmt"
	"io"
)

var permMsg = "You do not have permission to %s."
var grantMsg = `You may ask an administrator to grant you access with "juju grant".`

func PermissionsError(err error, stdout io.Writer, command string) error {
	if command == "" {
		command = "complete this operation"
	}
	permMsg := fmt.Sprintf(permMsg, command)
	stdout.Write([]byte(
		fmt.Sprintf("\n%s\n%s\n\n", permMsg, grantMsg)),
	)
	return err
}
