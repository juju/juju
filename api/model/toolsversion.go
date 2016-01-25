// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package model

import (
	"github.com/juju/juju/api/base"
)

// ToolsVersionUpdater allows api calls to update available tool version.
type ToolsVersionUpdater struct {
	facade base.FacadeCaller
}

// NewToolsVersionUpdater returns a new ToolsVersionUpdater pointer.
func NewToolsVersionUpdater(facade base.FacadeCaller) *ToolsVersionUpdater {
	return &ToolsVersionUpdater{facade}
}

// UpdateToolsVersion calls UpdateToolsAvailable in the server with
// the provided version.
func (t *ToolsVersionUpdater) UpdateToolsVersion() error {
	return t.facade.FacadeCall("UpdateToolsAvailable", nil, nil)
}
