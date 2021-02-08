// Copyright 2021 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package logpruner

import (
	"time"

	"github.com/juju/errors"

	"github.com/juju/juju/api/base"
	"github.com/juju/juju/api/common"
	"github.com/juju/juju/apiserver/params"
	"github.com/juju/juju/core/watcher"
	"github.com/juju/juju/environs/config"
)

const apiName = "LogPruner"

// Facade allows calls to "LogPruner" endpoints.
type Facade struct {
	facade base.FacadeCaller
	*common.ControllerConfigAPI
}

// NewFacade returns a "LogPruner" Facade.
func NewFacade(caller base.APICaller) *Facade {
	facadeCaller := base.NewFacadeCaller(caller, apiName)
	return &Facade{facade: facadeCaller,
		ControllerConfigAPI: common.NewControllerConfig(facadeCaller),
	}
}

// Prune calls "LogPruner.Prune".
func (s *Facade) Prune(maxLogTime time.Duration, maxLogMB int) error {
	p := params.LogPruneArgs{
		MaxLogTime: maxLogTime,
		MaxLogMB:   maxLogMB,
	}
	return s.facade.FacadeCall("Prune", p, nil)
}

// WatchForModelConfigChanges implements worker.pruner.Facade but is not used for logs.
func (s *Facade) WatchForModelConfigChanges() (watcher.NotifyWatcher, error) {
	return nil, errors.NotSupportedf("WatchForModelConfigChanges")
}

// ModelConfig implements worker.pruner.Facade but is not used for logs.
func (s *Facade) ModelConfig() (*config.Config, error) {
	return nil, errors.NotSupportedf("ModelConfig")
}
