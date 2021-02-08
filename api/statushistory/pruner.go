// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package statushistory

import (
	"time"

	"github.com/juju/errors"

	"github.com/juju/juju/api/base"
	"github.com/juju/juju/api/common"
	"github.com/juju/juju/apiserver/params"
	"github.com/juju/juju/controller"
	"github.com/juju/juju/core/watcher"
)

const apiName = "StatusHistory"

// Facade allows calls to "StatusHistory" endpoints.
type Facade struct {
	facade base.FacadeCaller
	*common.ModelWatcher
}

// NewFacade returns a status "StatusHistory" Facade.
func NewFacade(caller base.APICaller) *Facade {
	facadeCaller := base.NewFacadeCaller(caller, apiName)
	return &Facade{facade: facadeCaller, ModelWatcher: common.NewModelWatcher(facadeCaller)}
}

// Prune calls "StatusHistory.Prune"
func (s *Facade) Prune(maxHistoryTime time.Duration, maxHistoryMB int) error {
	p := params.StatusHistoryPruneArgs{
		MaxHistoryTime: maxHistoryTime,
		MaxHistoryMB:   maxHistoryMB,
	}
	return s.facade.FacadeCall("Prune", p, nil)
}

// WatchForControllerConfigChanges implements worker.pruner.Facade but is not used for status.
func (s *Facade) WatchForControllerConfigChanges() (watcher.NotifyWatcher, error) {
	return nil, errors.NotSupportedf("WatchForControllerConfigChanges")
}

// ControllerConfig implements worker.pruner.Facade but is not used for status.
func (s *Facade) ControllerConfig() (controller.Config, error) {
	return nil, errors.NotSupportedf("ControllerConfig")
}
