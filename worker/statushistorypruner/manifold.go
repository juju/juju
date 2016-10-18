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
	APICallerName  string
	MaxHistoryTime time.Duration
	MaxHistoryMB   uint
	PruneInterval  time.Duration
	// TODO(fwereade): 2016-03-17 lp:1558657
	NewTimer worker.NewTimerFunc
}

// Manifold returns a Manifold that encapsulates the statushistorypruner worker.
func Manifold(config ManifoldConfig) dependency.Manifold {
	return dependency.Manifold{
		Inputs: []string{config.APICallerName},
		Start: func(context dependency.Context) (worker.Worker, error) {
			var apiCaller base.APICaller
			if err := context.Get(config.APICallerName, &apiCaller); err != nil {
				return nil, errors.Trace(err)
			}

			facade := statushistory.NewFacade(apiCaller)
			prunerConfig := Config{
				Facade:         facade,
				MaxHistoryTime: config.MaxHistoryTime,
				MaxHistoryMB:   config.MaxHistoryMB,
				PruneInterval:  config.PruneInterval,
				NewTimer:       config.NewTimer,
			}
			w, err := New(prunerConfig)
			if err != nil {
				return nil, errors.Trace(err)
			}
			return w, nil
		},
	}
}
