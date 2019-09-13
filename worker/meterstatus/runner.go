// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package meterstatus

import (
	"github.com/juju/clock"
	"github.com/juju/errors"
	"gopkg.in/juju/charm.v6/hooks"
	"gopkg.in/juju/names.v3"

	"github.com/juju/juju/agent"
	"github.com/juju/juju/core/machinelock"
	"github.com/juju/juju/worker/uniter"
	"github.com/juju/juju/worker/uniter/runner"
)

// HookRunner implements the functionality necessary to run a meter-status-changed hook.
type HookRunner interface {
	RunHook(string, string, <-chan struct{}) error
}

// hookRunner implements functionality for running a hook.
type hookRunner struct {
	machineLock machinelock.Lock
	config      agent.Config
	tag         names.UnitTag
	clock       clock.Clock
}

func NewHookRunner(tag names.UnitTag, machineLock machinelock.Lock, config agent.Config, clock clock.Clock) HookRunner {
	return &hookRunner{
		tag:         tag,
		machineLock: machineLock,
		config:      config,
		clock:       clock,
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

func (w *hookRunner) RunHook(code, info string, interrupt <-chan struct{}) (runErr error) {
	unitTag := w.tag
	ctx := NewLimitedContext(unitTag.String())
	ctx.SetEnvVars(map[string]string{
		"JUJU_METER_STATUS": code,
		"JUJU_METER_INFO":   info,
	})
	paths := uniter.NewPaths(w.config.DataDir(), unitTag, nil)
	r := runner.NewRunner(ctx, paths, nil)
	releaser, err := w.acquireExecutionLock(string(hooks.MeterStatusChanged), interrupt)
	if err != nil {
		return errors.Annotate(err, "failed to acquire machine lock")
	}
	defer releaser()
	return r.RunHook(string(hooks.MeterStatusChanged))
}
