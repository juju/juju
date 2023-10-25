// Copyright 2017 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package action

import (
	"context"
	"time"

	"github.com/juju/juju/api/base"
	"github.com/juju/juju/api/common"
	"github.com/juju/juju/rpc/params"
)

const apiName = "ActionPruner"

// Facade allows calls to "ActionPruner" endpoints.
type Facade struct {
	facade base.FacadeCaller
	*common.ModelWatcher
}

// NewPruner builds a facade for the action pruner endpoints.
func NewPruner(caller base.APICaller) *Facade {
	facadeCaller := base.NewFacadeCaller(caller, apiName)
	return &Facade{facade: facadeCaller, ModelWatcher: common.NewModelWatcher(facadeCaller)}
}

// Prune prunes action entries by specified age and size.
func (s *Facade) Prune(maxHistoryTime time.Duration, maxHistoryMB int) error {
	p := params.ActionPruneArgs{
		MaxHistoryTime: maxHistoryTime,
		MaxHistoryMB:   maxHistoryMB,
	}
	return s.facade.FacadeCall(context.TODO(), "Prune", p, nil)
}
