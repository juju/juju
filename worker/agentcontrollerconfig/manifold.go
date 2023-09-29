// Copyright 2023 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package agentcontrollerconfig

import (
	"github.com/juju/errors"
	"github.com/juju/worker/v3"
	"github.com/juju/worker/v3/dependency"
)

// Logger represents the logging methods called.
type Logger interface {
	Errorf(message string, args ...interface{})
	Infof(message string, args ...interface{})
	Debugf(message string, args ...interface{})
}

// ManifoldConfig defines the configuration for the agent controller config
// manifold.
type ManifoldConfig struct {
	Logger Logger
}

// Validate validates the manifold configuration.
func (cfg ManifoldConfig) Validate() error {
	if cfg.Logger == nil {
		return errors.NotValidf("nil Logger")
	}
	return nil
}

// Manifold returns a dependency manifold that runs the trace worker.
func Manifold(config ManifoldConfig) dependency.Manifold {
	return dependency.Manifold{
		Inputs: []string{},
		Output: configOutput,
		Start: func(context dependency.Context) (worker.Worker, error) {
			if err := config.Validate(); err != nil {
				return nil, errors.Trace(err)
			}

			w, err := NewWorker(WorkerConfig{
				Logger: config.Logger,
			})
			if err != nil {
				return nil, errors.Trace(err)
			}

			return w, nil
		},
	}
}

func configOutput(in worker.Worker, out any) error {
	return nil
}
