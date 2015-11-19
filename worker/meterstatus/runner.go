// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package meterstatus

import (
	"fmt"

	"github.com/juju/errors"
	"github.com/juju/names"
	"github.com/juju/utils/fslock"
	"gopkg.in/juju/charm.v6-unstable/hooks"
	"launchpad.net/tomb"

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
	machineLock *fslock.Lock
	config      agent.Config
	tag         names.UnitTag
}

func NewHookRunner(tag names.UnitTag, lock *fslock.Lock, config agent.Config) HookRunner {
	return &hookRunner{
		tag:         tag,
		machineLock: lock,
		config:      config,
	}
}

// acquireExecutionLock acquires the machine-level execution lock and returns a function to be used
// to unlock it.
func (w *hookRunner) acquireExecutionLock(interrupt <-chan struct{}) (func() error, error) {
	message := "running meter-status-changed hook"
	logger.Tracef("lock: %v", message)
	checkTomb := func() error {
		select {
		case <-interrupt:
			return tomb.ErrDying
		default:
			return nil
		}
	}
	message = fmt.Sprintf("%s: %s", w.tag.String(), message)
	if err := w.machineLock.LockWithFunc(message, checkTomb); err != nil {
		return nil, err
	}
	return func() error {
		logger.Tracef("unlock: %v", message)
		return w.machineLock.Unlock()
	}, nil
}

func (w *hookRunner) RunHook(code, info string, interrupt <-chan struct{}) (runErr error) {
	unitTag := w.tag
	paths := uniter.NewPaths(w.config.DataDir(), unitTag)
	ctx := NewLimitedContext(unitTag.String())
	ctx.SetEnvVars(map[string]string{
		"JUJU_METER_STATUS": code,
		"JUJU_METER_INFO":   info,
	})
	r := runner.NewRunner(ctx, paths)
	unlock, err := w.acquireExecutionLock(interrupt)
	if err != nil {
		return errors.Annotate(err, "failed to acquire machine lock")
	}
	defer func() {
		unlockErr := unlock()
		if unlockErr != nil {
			logger.Criticalf("hook run resulted in error %v; unlock failure error: %v", runErr, unlockErr)
		}
	}()
	return r.RunHook(string(hooks.MeterStatusChanged))
}
