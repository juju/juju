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
	"github.com/juju/juju/worker/util"
)

// createLock exists to be patched out in export_test.go
var createLock = cmdutil.HookExecutionLock

// ManifoldConfig specifies the names a machinelock manifold should use to
// address its dependencies.
type ManifoldConfig util.AgentManifoldConfig

// Manifold returns a dependency.Manifold that governs the construction of
// and access to a machine-wide lock intended to prevent various operations
// from running concurrently and interfering with one another. Examples (are
// not limited to): hook executions, package installation, synchronisation
// of reboots.
// Clients can access the lock by passing a **fslock.Lock into the out param
// of their GetResourceFunc.
func Manifold(config ManifoldConfig) dependency.Manifold {
	manifold := util.AgentManifold(util.AgentManifoldConfig(config), newWorker)
	manifold.Output = outputFunc
	return manifold
}

// newWorker creates a degenerate worker that provides access to an fslock.
func newWorker(agent agent.Agent) (worker.Worker, error) {
	dataDir := agent.CurrentConfig().DataDir()
	lock, err := createLock(dataDir)
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
