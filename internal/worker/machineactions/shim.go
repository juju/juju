// Copyright 2016 Canonical Ltd.
// Copyright 2016 Cloudbase Solutions SRL
// Licensed under the AGPLv3, see LICENCE file for details.

package machineactions

import (
	"github.com/juju/juju/api/agent/machineactions"
	"github.com/juju/juju/api/base"
)

// NewFacade creates a Facade from a base.APICaller.
// It's a sensible value for ManifoldConfig.NewFacade.
func NewFacade(apiCaller base.APICaller) Facade {
	return machineactions.NewClient(apiCaller)
}
