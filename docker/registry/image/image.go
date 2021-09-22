// Copyright 2021 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package image

import (
	"github.com/juju/version/v2"

	"github.com/juju/juju/tools"
)

type imageInfo struct {
	version version.Number
}

func (info imageInfo) AgentVersion() version.Number {
	return info.version
}

func (info imageInfo) String() string {
	return info.version.String()
}

// NewImageInfo creates an imageInfo.
func NewImageInfo(ver version.Number) tools.HasVersion {
	return &imageInfo{version: ver}
}
