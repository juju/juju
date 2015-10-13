// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

// Package meterstatus provides a worker that executes the meter-status-changed hook
// periodically.
package meterstatus

import (
	"fmt"
	"path"

	"github.com/juju/errors"
	"github.com/juju/loggo"
	"github.com/juju/names"
	"github.com/juju/utils/fslock"
	"gopkg.in/juju/charm.v6-unstable/hooks"
	"launchpad.net/tomb"

	"github.com/juju/juju/agent"
	"github.com/juju/juju/api/base"
	"github.com/juju/juju/api/meterstatus"
	"github.com/juju/juju/state/watcher"
	"github.com/juju/juju/worker"
	"github.com/juju/juju/worker/dependency"
	"github.com/juju/juju/worker/uniter"
	"github.com/juju/juju/worker/uniter/runner"
)

var (
	logger               = loggo.GetLogger("juju.worker.meterstatus")
	newMeterStatusClient = meterstatus.NewClient
	newRunner            = runner.NewRunner
)

// ManifoldConfig identifies the resource names upon which the status manifold depends.
type ManifoldConfig struct {
	AgentName       string
	APICallerName   string
	MachineLockName string
}

// Manifold returns a status manifold.
func Manifold(config ManifoldConfig) dependency.Manifold {
	return dependency.Manifold{
		Inputs: []string{
			config.AgentName,
			config.APICallerName,
			config.MachineLockName,
		},
		Start: func(getResource dependency.GetResourceFunc) (worker.Worker, error) {
			return newStatusWorker(config, getResource)
		},
	}
}

func newStatusWorker(config ManifoldConfig, getResource dependency.GetResourceFunc) (worker.Worker, error) {
	var agent agent.Agent
	if err := getResource(config.AgentName, &agent); err != nil {
		return nil, err
	}

	var machineLock *fslock.Lock
	if err := getResource(config.MachineLockName, &machineLock); err != nil {
		return nil, err
	}

	var apiCaller base.APICaller
	err := getResource(config.APICallerName, &apiCaller)
	if err != nil {
		return nil, err
	}

	return newActiveStatusWorker(agent, apiCaller, machineLock)

}

type activeStatusWorker struct {
	tomb tomb.Tomb

	status      meterstatus.MeterStatusClient
	stateFile   *StateFile
	config      agent.Config
	machineLock *fslock.Lock
	tag         names.UnitTag

	init func()
}

func newActiveStatusWorker(agent agent.Agent, apiCaller base.APICaller, machineLock *fslock.Lock) (worker.Worker, error) {
	tag := agent.CurrentConfig().Tag()
	unitTag, ok := tag.(names.UnitTag)
	if !ok {
		return nil, errors.Errorf("expected a unit tag, got %v", tag)
	}
	status := newMeterStatusClient(apiCaller, unitTag)

	config := agent.CurrentConfig()
	stateFile := NewStateFile(path.Join(config.DataDir(), "meter-status.yaml"))

	w := &activeStatusWorker{
		config:      config,
		status:      status,
		stateFile:   stateFile,
		machineLock: machineLock,
		tag:         unitTag,
	}
	go func() {
		defer w.tomb.Done()
		w.tomb.Kill(w.loop())
	}()
	return w, nil
}

func (w *activeStatusWorker) Kill() {
	w.tomb.Kill(nil)
}

func (w *activeStatusWorker) Wait() error {
	return w.tomb.Wait()
}

// acquireExecutionLock acquires the machine-level execution lock and returns a function to be used
// to unlock it.
func (w *activeStatusWorker) acquireExecutionLock() (func() error, error) {
	message := "running meter-status-changed hook"
	logger.Tracef("lock: %v", message)
	checkTomb := func() error {
		select {
		case <-w.tomb.Dying():
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

func (w *activeStatusWorker) loop() error {
	code, info, err := w.stateFile.Read()
	if err != nil {
		return errors.Trace(err)
	}

	// Check current meter status before entering loop.
	currentCode, currentInfo, err := w.status.MeterStatus()
	if err != nil {
		return errors.Trace(err)
	}
	if code != currentCode || info != currentInfo {
		err = w.runHook(currentCode, currentInfo)
		if err != nil {
			return errors.Trace(err)
		}
		code, info = currentCode, currentInfo
	}

	watch, err := w.status.WatchMeterStatus()
	if err != nil {
		return errors.Trace(err)
	}
	defer watcher.Stop(watch, &w.tomb)

	// This function is used in tests to signal entering the worker loop.
	if w.init != nil {
		w.init()
	}
	for {
		select {
		case _, ok := <-watch.Changes():
			logger.Debugf("got meter status change")
			if !ok {
				return watcher.EnsureErr(watch)
			}
			currentCode, currentInfo, err := w.status.MeterStatus()
			if err != nil {
				return errors.Trace(err)
			}
			if currentCode == code && currentInfo == info {
				continue
			}
			err = w.runHook(currentCode, currentInfo)
			if err != nil {
				return errors.Trace(err)
			}
			code, info = currentCode, currentInfo
		case <-w.tomb.Dying():
			return tomb.ErrDying
		}
	}
}

func (w *activeStatusWorker) runHook(code, info string) (runErr error) {
	unitTag := w.tag
	paths := uniter.NewPaths(w.config.DataDir(), unitTag)
	ctx := NewLimitedContext(unitTag.String())
	ctx.SetEnvVars(map[string]string{
		"JUJU_METER_STATUS": code,
		"JUJU_METER_INFO":   info,
	})
	r := newRunner(ctx, paths)
	unlock, err := w.acquireExecutionLock()
	if err != nil {
		return errors.Annotate(err, "failed to acquire machine lock")
	}
	defer func() {
		unlockErr := unlock()
		if unlockErr != nil {
			logger.Criticalf("hook run resulted in error %v; error overridden by unlock failure error", runErr)
			runErr = unlockErr
		}
	}()
	err = r.RunHook(string(hooks.MeterStatusChanged))
	if err != nil {
		return errors.Annotatef(err, "error running 'meter-status-changed' hook")
	}
	return errors.Trace(w.stateFile.Write(code, info))
}
