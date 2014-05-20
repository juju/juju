// Copyright 2012, 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package utils

import (
	"regexp"
)

var validName = regexp.MustCompile("^[a-zA-Z][a-zA-Z0-9.-]*[a-zA-Z0-9]$")

func IsUsernameValid(name string) bool {
	return validName.MatchString(name)
}
