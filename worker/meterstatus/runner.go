// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package meterstatus

import (
	"time"

	"github.com/juju/errors"
	"github.com/juju/mutex"
	"github.com/juju/utils/clock"
	"gopkg.in/juju/charm.v6-unstable/hooks"
	"gopkg.in/juju/names.v2"

	"github.com/juju/juju/agent"
	"github.com/juju/juju/worker/uniter"
	"github.com/juju/juju/worker/uniter/runner"
)

// HookRunner implements the functionality necessary to run a meter-status-changed hook.
type HookRunner interface {
	RunHook(string, string, <-chan struct{}) error
}

// hookRunner implements functionality for running a hook.
type hookRunner struct {
	machineLockName string
	config          agent.Config
	tag             names.UnitTag
	clock           clock.Clock
}

func NewHookRunner(tag names.UnitTag, lockName string, config agent.Config, clock clock.Clock) HookRunner {
	return &hookRunner{
		tag:             tag,
		machineLockName: lockName,
		config:          config,
		clock:           clock,
	}
}

// acquireExecutionLock acquires the machine-level execution lock and returns a function to be used
// to unlock it.
func (w *hookRunner) acquireExecutionLock(interrupt <-chan struct{}) (mutex.Releaser, error) {
	spec := mutex.Spec{
		Name:   w.machineLockName,
		Clock:  w.clock,
		Delay:  250 * time.Millisecond,
		Cancel: interrupt,
	}
	logger.Debugf("acquire lock %q for meter status hook execution", w.machineLockName)
	releaser, err := mutex.Acquire(spec)
	if err != nil {
		return nil, errors.Trace(err)
	}
	logger.Debugf("lock %q acquired", w.machineLockName)
	return releaser, nil
}

func (w *hookRunner) RunHook(code, info string, interrupt <-chan struct{}) (runErr error) {
	unitTag := w.tag
	paths := uniter.NewPaths(w.config.DataPath(), unitTag)
	ctx := NewLimitedContext(unitTag.String())
	ctx.SetEnvVars(map[string]string{
		"JUJU_METER_STATUS": code,
		"JUJU_METER_INFO":   info,
	})
	r := runner.NewRunner(ctx, paths)
	releaser, err := w.acquireExecutionLock(interrupt)
	if err != nil {
		return errors.Annotate(err, "failed to acquire machine lock")
	}
	// Defer the logging first so it is executed after the Release. LIFO.
	defer logger.Debugf("release lock %q for meter status hook execution", w.machineLockName)
	defer releaser.Release()
	return r.RunHook(string(hooks.MeterStatusChanged))
}
