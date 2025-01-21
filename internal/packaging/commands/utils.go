// Copyright 2015 Canonical Ltd.
// Copyright 2015 Cloudbase Solutions SRL
// Licensed under the LGPLv3, see LICENCE file for details.

package commands

import (
	"strings"
)

// buildCommand is a helper function which simply joins its attributes with a space.
func buildCommand(args ...string) string {
	return strings.Join(args, " ")
}
