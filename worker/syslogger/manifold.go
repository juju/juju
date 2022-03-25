// Copyright 2022 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package syslogger

import (
	"io"
	"log/syslog"

	"github.com/juju/errors"
	"github.com/juju/juju/state"
	"github.com/juju/worker/v3"
	"github.com/juju/worker/v3/dependency"
)

// ManifoldConfig defines the names of the manifolds on which a Manifold will
// depend.
type ManifoldConfig struct {
	NewWorker func(WorkerConfig) (worker.Worker, error)
	NewLogger NewLogger
}

// Validate validates the manifold configuration.
func (config ManifoldConfig) Validate() error {
	if config.NewWorker == nil {
		return errors.NotValidf("nil NewWorker")
	}
	if config.NewLogger == nil {
		return errors.NotValidf("nil NewLogger")
	}
	return nil
}

// Manifold returns a dependency manifold that runs the dbaccessor
// worker, using the resource names defined in the supplied config.
func Manifold(config ManifoldConfig) dependency.Manifold {
	return dependency.Manifold{
		Inputs: []string{},
		Output: output,
		Start: func(context dependency.Context) (worker.Worker, error) {
			if err := config.Validate(); err != nil {
				return nil, errors.Trace(err)
			}

			w, err := config.NewWorker(WorkerConfig{
				NewLogger: config.NewLogger,
			})
			if err != nil {
				return nil, errors.Trace(err)
			}
			return w, nil
		},
	}
}

type withOutputs interface {
	Log([]state.LogRecord) error
}

func output(in worker.Worker, out interface{}) error {
	w, ok := in.(withOutputs)
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

func NewSyslog(priority syslog.Priority, tag string) (io.WriteCloser, error) {
	return syslog.New(priority, tag)
}
