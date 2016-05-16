// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package statushistory

import (
	"time"

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
func (s *Facade) Prune(maxHistoryTime time.Duration, maxHistoryMB int) error {
	p := params.StatusHistoryPruneArgs{
		MaxHistoryTime: maxHistoryTime,
		MaxHistoryMB:   maxHistoryMB,
	}
	return s.facade.FacadeCall("Prune", p, nil)
}
