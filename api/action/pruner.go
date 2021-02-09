// Copyright 2017 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package action

import (
	"time"

	"github.com/juju/errors"

	"github.com/juju/juju/api/base"
	"github.com/juju/juju/api/common"
	"github.com/juju/juju/apiserver/params"
	"github.com/juju/juju/controller"
	"github.com/juju/juju/core/watcher"
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
	return s.facade.FacadeCall("Prune", p, nil)
}

// WatchForControllerConfigChanges implements worker.pruner.Facade but is not used for actions.
func (s *Facade) WatchForControllerConfigChanges() (watcher.NotifyWatcher, error) {
	return nil, errors.NotSupportedf("WatchForControllerConfigChanges")
}

// ControllerConfig implements worker.pruner.Facade but is not used for actions.
func (s *Facade) ControllerConfig() (controller.Config, error) {
	return nil, errors.NotSupportedf("ControllerConfig")
}
