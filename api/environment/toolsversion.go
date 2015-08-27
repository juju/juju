// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package environment

import (
	"github.com/juju/juju/api/base"
)

type ToolsVersionUpdater struct {
	facade base.FacadeCaller
}

func NewToolsVersionUpdater(facade base.FacadeCaller) *ToolsVersionUpdater {
	return &ToolsVersionUpdater{facade}
}

// UpdateToolsVersion
func (t *ToolsVersionUpdater) UpdateToolsVersion() error {
	return t.facade.FacadeCall("UpdateToolsAvailable", nil, nil)
}
