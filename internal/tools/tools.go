// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package tools

import (
	"github.com/juju/juju/core/semversion"
)

// Tools represents the location and version of a tools tarball.
type Tools struct {
	Version semversion.Binary `json:"version"`
	URL     string            `json:"url"`
	SHA256  string            `json:"sha256,omitempty"`
	Size    int64             `json:"size"`
}

// AgentVersion is the version of Juju being represented.
func (t *Tools) AgentVersion() semversion.Number {
	return t.Version.Number
}
