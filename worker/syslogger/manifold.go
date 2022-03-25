// Copyright 2022 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package syslogger

import (
	"github.com/juju/errors"
	"github.com/juju/juju/worker/common"
	"github.com/juju/worker/v3"
	"github.com/juju/worker/v3/dependency"
)

// ManifoldConfig defines the names of the manifolds on which a Manifold will
// depend.
type ManifoldConfig struct{}

// Manifold returns a dependency manifold that runs the dbaccessor
// worker, using the resource names defined in the supplied config.
func Manifold(config ManifoldConfig) dependency.Manifold {
	return dependency.Manifold{
		Inputs: []string{},
		Output: sysloggerOutput,
		Start: func(context dependency.Context) (worker.Worker, error) {
			cfg := WorkerConfig{}

			w, err := NewWorker(cfg)
			if err != nil {
				return nil, errors.Trace(err)
			}
			return w, nil
		},
	}
}

func sysloggerOutput(in worker.Worker, out interface{}) error {
	if w, ok := in.(*common.CleanupWorker); ok {
		in = w.Worker
	}
	w, ok := in.(*syslogWorker)
	if !ok {
		return errors.Errorf("expected input of type syslogWorker, got %T", in)
	}

	switch out := out.(type) {
	case *SysLogger:
		var target SysLogger = w
		*out = target
	default:
		return errors.Errorf("expected output of *syslogger.SysLogger, got %T", out)
	}
	return nil
}
