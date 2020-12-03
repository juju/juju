// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package tools

import (
	"github.com/juju/version"
)

// Tools represents the location and version of a tools tarball.
type Tools struct {
	Version version.Binary `json:"version"`
	URL     string         `json:"url"`
	SHA256  string         `json:"sha256,omitempty"`
	Size    int64          `json:"size"`
}

// AgentVersion
func (t *Tools) AgentVersion() version.Number {
	return t.Version.Number
}

// DashboardArchive represents the location and version of a dashboard release archive.
type DashboardArchive struct {
	Version version.Number `json:"version"`
	URL     string         `json:"url"`
	SHA256  string         `json:"sha256,omitempty"`
	Size    int64          `json:"size"`
}
