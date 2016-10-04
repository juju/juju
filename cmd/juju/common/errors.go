// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package common

import (
	"fmt"

	"github.com/juju/errors"
)

var permMsg = "You do not have permission to %s."
var grantMsg = `You may ask an administrator to grant you access with "juju grant".`

func PermissionsError(err error, command string) error {
	if command == "" {
		command = "complete this operation"
	}
	permMsg := fmt.Sprintf(permMsg, command)
	return errors.Errorf("%v\n\n%s\n%s\n", err, permMsg, grantMsg)
}
