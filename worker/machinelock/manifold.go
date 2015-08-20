// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package machinelock

import (
	"github.com/juju/errors"

	"github.com/juju/juju/agent"
	cmdutil "github.com/juju/juju/cmd/jujud/util"
	"github.com/juju/juju/worker"
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
	manifold.Output = util.ValueWorkerOutput
	return manifold
}

// newWorker creates a degenerate worker that provides access to an fslock.
func newWorker(a agent.Agent) (worker.Worker, error) {
	dataDir := a.CurrentConfig().DataDir()
	lock, err := createLock(dataDir)
	if err != nil {
		return nil, errors.Trace(err)
	}
	return util.NewValueWorker(lock)
}
