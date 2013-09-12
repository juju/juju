// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package tools

import (
	"launchpad.net/loggo"

	"launchpad.net/juju-core/version"
)

var logger = loggo.GetLogger("juju.tools")

// Tools represents the location and version of a tools tarball.
type Tools struct {
	Version version.Binary
	URL     string
	SHA256  string
	Size    int64
}

// HasTools instances can be asked for a tools list.
type HasTools interface {
	Tools() List
}
