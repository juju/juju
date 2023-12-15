// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package meterstatus

import (
	"github.com/juju/charm/v12/hooks"
	"github.com/juju/errors"
	"github.com/juju/names/v4"

	"github.com/juju/juju/agent"
	"github.com/juju/juju/core/machinelock"
	"github.com/juju/juju/internal/worker/common/charmrunner"
	"github.com/juju/juju/internal/worker/uniter"
	"github.com/juju/juju/internal/worker/uniter/runner"
)

// HookRunner implements the functionality necessary to run a meter-status-changed hook.
type HookRunner interface {
	RunHook(string, string, <-chan struct{})
}

// hookRunner implements functionality for running a hook.
type hookRunner struct {
	machineLock machinelock.Lock
	config      agent.Config
	tag         names.UnitTag
	clock       Clock
	logger      Logger
}

// HookRunnerConfig is just an argument struct for NewHookRunner.
type HookRunnerConfig struct {
	MachineLock machinelock.Lock
	AgentConfig agent.Config
	Tag         names.UnitTag
	Clock       Clock
	Logger      Logger
}

func NewHookRunner(config HookRunnerConfig) HookRunner {
	return &hookRunner{
		tag:         config.Tag,
		machineLock: config.MachineLock,
		config:      config.AgentConfig,
		clock:       config.Clock,
		logger:      config.Logger,
	}
}

// acquireExecutionLock acquires the machine-level execution lock and returns a function to be used
// to unlock it.
func (w *hookRunner) acquireExecutionLock(action string, interrupt <-chan struct{}) (func(), error) {
	spec := machinelock.Spec{
		Cancel:  interrupt,
		Worker:  "meterstatus",
		Comment: action,
	}
	releaser, err := w.machineLock.Acquire(spec)
	if err != nil {
		return nil, errors.Trace(err)
	}
	return releaser, nil
}

func (w *hookRunner) RunHook(code, info string, interrupt <-chan struct{}) {
	unitTag := w.tag
	ctx := newLimitedContext(hookConfig{
		unitName: unitTag.String(),
		clock:    w.clock,
		logger:   w.logger,
	})
	ctx.SetEnvVars(map[string]string{
		"JUJU_METER_STATUS": code,
		"JUJU_METER_INFO":   info,
	})
	paths := uniter.NewPaths(w.config.DataDir(), unitTag, nil)
	r := runner.NewRunner(ctx, paths, nil)
	releaser, err := w.acquireExecutionLock(string(hooks.MeterStatusChanged), interrupt)
	if err != nil {
		w.logger.Errorf("failed to acquire machine lock: %v", err)
		return
	}
	defer releaser()
	handlerType, err := r.RunHook(string(hooks.MeterStatusChanged))
	cause := errors.Cause(err)
	switch {
	case charmrunner.IsMissingHookError(cause):
		w.logger.Infof("skipped %q hook (missing)", string(hooks.MeterStatusChanged))
	case cause == runner.ErrTerminated:
		w.logger.Warningf("%q hook was terminated", hooks.MeterStatusChanged)
	case err != nil:
		w.logger.Errorf("error running %q: %v", hooks.MeterStatusChanged, err)
	default:
		w.logger.Infof("ran %q hook (via %s)", hooks.MeterStatusChanged, handlerType)
	}
}
