// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package statushistorypruner

import (
	"time"

	"github.com/juju/errors"

	"github.com/juju/juju/api/base"
	"github.com/juju/juju/api/statushistory"
	"github.com/juju/juju/worker"
	"github.com/juju/juju/worker/dependency"
)

// ManifoldConfig describes the resources and configuration on which the
// statushistorypruner worker depends.
type ManifoldConfig struct {
	APICallerName    string
	MaxLogsPerEntity uint
	PruneInterval    time.Duration
	NewTimer         worker.NewTimerFunc
}

// Manifold returns a Manifold that encapsulates the statushistorypruner worker.
func Manifold(config ManifoldConfig) dependency.Manifold {
	return dependency.Manifold{
		Inputs: []string{config.APICallerName},
		Start: func(getResource dependency.GetResourceFunc) (worker.Worker, error) {
			var apiCaller base.APICaller
			if err := getResource(config.APICallerName, &apiCaller); err != nil {
				return nil, errors.Trace(err)
			}

			facade := statushistory.NewFacade(apiCaller)
			prunerConfig := Config{
				Facade:           facade,
				MaxLogsPerEntity: config.MaxLogsPerEntity,
				PruneInterval:    config.PruneInterval,
				NewTimer:         config.NewTimer,
			}
			w, err := New(prunerConfig)
			if err != nil {
				return nil, errors.Trace(err)
			}
			return w, nil
		},
	}
}
