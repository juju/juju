// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package statushistory

import (
	"github.com/juju/juju/api/base"
	"github.com/juju/juju/apiserver/params"
)

const apiName = "StatusHistory"

// Facade allows calls to "StatusHistory" endpoints
type Facade struct {
	facade base.FacadeCaller
}

// NewFacade returns a status "StatusHistory" Facade.
func NewFacade(caller base.APICaller) *Facade {
	facadeCaller := base.NewFacadeCaller(caller, apiName)
	return &Facade{facadeCaller}
}

// Prune calls "StatusHistory.Prune"
func (s *Facade) Prune(maxLogsPerEntity int) error {
	p := params.StatusHistoryPruneArgs{
		MaxLogsPerEntity: maxLogsPerEntity,
	}
	return s.facade.FacadeCall("Prune", p, nil)
}
