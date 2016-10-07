// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package common

import (
	"fmt"
	"io"
)

func PermissionsMessage(writer io.Writer, command string) {
	const (
		perm  = "You do not have permission to %s."
		grant = `You may ask an administrator to grant you access with "juju grant".`
	)

	if command == "" {
		command = "complete this operation"
	}
	fmt.Fprintf(writer, "\n%s\n%s\n\n", fmt.Sprintf(perm, command), grant)
}
