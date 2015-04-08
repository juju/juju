// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package machinelock

import (
	"github.com/juju/errors"
	"github.com/juju/utils/fslock"
	"launchpad.net/tomb"

	cmdutil "github.com/juju/juju/cmd/jujud/util"
	"github.com/juju/juju/worker"
	"github.com/juju/juju/worker/agent"
	"github.com/juju/juju/worker/dependency"
)

// ManifoldConfig specifies the names a machinelock manifold should use to
// address its dependencies.
type ManifoldConfig struct {
	AgentName string
}

// Manifold returns a dependency.Manifold that governs the construction of
// and access to a machine-wide lock intended to prevent various operations
// from running concurrently and interfering with one another.
// Clients can access the lock by passing a **fslock.Lock into the out param
// of their GetResourceFunc.
func Manifold(config ManifoldConfig) dependency.Manifold {
	return dependency.Manifold{
		Inputs: []string{
			config.AgentName,
		},
		Start:  startFunc(config),
		Output: outputFunc,
	}
}

// startFunc returns a StartFunc that creates a worker based on the manifolds
// named in the supplied config.
func startFunc(config ManifoldConfig) dependency.StartFunc {
	return func(getResource dependency.GetResourceFunc) (worker.Worker, error) {
		var agent agent.Agent
		if err := getResource(config.AgentName, &agent); err != nil {
			return nil, err
		}
		dataDir := agent.CurrentConfig().DataDir()
		lock, err := cmdutil.HookExecutionLock(dataDir)
		if err != nil {
			return nil, errors.Trace(err)
		}
		w := &machineLockWorker{lock: lock}
		go func() {
			defer w.tomb.Done()
			<-w.tomb.Dying()
		}()
		return w, nil
	}
}

// outputFunc extracts a *fslock.Lock from a *machineLockWorker.
func outputFunc(in worker.Worker, out interface{}) error {
	inWorker, _ := in.(*machineLockWorker)
	outPointer, _ := out.(**fslock.Lock)
	if inWorker == nil || outPointer == nil {
		return errors.Errorf("expected %T->%T; got %T->%T", inWorker, outPointer, in, out)
	}
	*outPointer = inWorker.lock
	return nil
}

// machineLockWorker is a degenerate worker that exists only to hold a reference
// to its lock.
type machineLockWorker struct {
	tomb tomb.Tomb
	lock *fslock.Lock
}

// Kill is part of the worker.Worker interface.
func (w *machineLockWorker) Kill() {
	w.tomb.Kill(nil)
}

// Wait is part of the worker.Worker interface.
func (w *machineLockWorker) Wait() error {
	return w.tomb.Wait()
}
