// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package tools

import (
	"launchpad.net/loggo"

	"launchpad.net/juju-core/version"
)

var logger = loggo.GetLogger("juju.tools")

type Tools struct {
	Version version.Binary
	URL     string
}
