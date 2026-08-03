// Copyright 2026 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package logsinkproxy

import (
	"context"

	"github.com/juju/errors"
	"github.com/juju/worker/v5"
	"github.com/juju/worker/v5/dependency"

	"github.com/juju/juju/agent/engine"
	corelogger "github.com/juju/juju/core/logger"
)

// ManifoldConfig defines the configuration for the log sink proxy manifold.
type ManifoldConfig struct {
	// ControllerFlagName is the name of the flag that indicates whether
	// the agent is running on a controller machine.
	ControllerFlagName string

	// ControllerLogSinkName is the name of the log sink manifold for
	// controller machines.
	ControllerLogSinkName string

	// NonControllerLogSinkName is the name of the log sink manifold for
	// non-controller machines.
	NonControllerLogSinkName string
}

// Validate validates the manifold configuration.
func (c ManifoldConfig) Validate() error {
	if c.ControllerFlagName == "" {
		return errors.NotValidf("empty ControllerFlagName")
	}
	if c.ControllerLogSinkName == "" {
		return errors.NotValidf("empty ControllerLogSinkName")
	}
	if c.NonControllerLogSinkName == "" {
		return errors.NotValidf("empty NonControllerLogSinkName")
	}
	return nil
}

// Manifold returns a dependency manifold that runs a log sink proxy
// worker, using the resource names defined in the supplied config.
func Manifold(config ManifoldConfig) dependency.Manifold {
	return dependency.Manifold{
		Inputs: []string{
			config.ControllerFlagName,
			config.ControllerLogSinkName,
			config.NonControllerLogSinkName,
		},
		Output: outputFunc,
		Start: func(ctx context.Context, getter dependency.Getter) (worker.Worker, error) {
			if err := config.Validate(); err != nil {
				return nil, err
			}

			var isController engine.Flag
			if err := getter.Get(config.ControllerFlagName, &isController); err != nil {
				return nil, err
			}

			name := config.NonControllerLogSinkName
			if isController.Check() {
				name = config.ControllerLogSinkName
			}

			var ml corelogger.ModelLogger
			if err := getter.Get(name, &ml); err != nil {
				return nil, err
			}
			var lcg corelogger.LoggerContextGetter
			if err := getter.Get(name, &lcg); err != nil {
				return nil, err
			}
			var msg corelogger.ModelLogSinkGetter
			if err := getter.Get(name, &msg); err != nil {
				return nil, err
			}

			return engine.NewValueWorker(logSinkProxy{
				ModelLogger:         ml,
				LoggerContextGetter: lcg,
				ModelLogSinkGetter:  msg,
			})
		},
	}
}

// logSinkProxy bundles the three interfaces exposed by the log-sink
// worker so that the manifold can forward them from the active branch
// (controller or non-controller).
type logSinkProxy struct {
	corelogger.ModelLogger
	corelogger.LoggerContextGetter
	corelogger.ModelLogSinkGetter
}

// outputFunc extracts the log sink interfaces from a value worker
// wrapping a logSinkProxy.
func outputFunc(in worker.Worker, out any) error {
	raw, err := engine.ExtractValue(in)
	if err != nil {
		return err
	}
	proxy := raw.(logSinkProxy)
	switch outPointer := out.(type) {
	case *corelogger.ModelLogger:
		*outPointer = proxy.ModelLogger
	case *corelogger.LoggerContextGetter:
		*outPointer = proxy.LoggerContextGetter
	case *corelogger.ModelLogSinkGetter:
		*outPointer = proxy.ModelLogSinkGetter
	default:
		return errors.Errorf("unexpected output type %T", out)
	}
	return nil
}
