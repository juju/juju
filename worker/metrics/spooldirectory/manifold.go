// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

// Package spooldirectory contains the implementation of a degenerate
// worker that extracts the spool directory path from the agent
// config and makes it available to other workers that depend
// on it.
package spooldirectory

import (
	"github.com/juju/errors"
	"launchpad.net/tomb"

	"github.com/juju/juju/agent"
	"github.com/juju/juju/worker"
	"github.com/juju/juju/worker/dependency"
	"github.com/juju/juju/worker/util"
)

// ManifoldConfig specifies names a spooldirectory manifold should use to
// address its dependencies.
type ManifoldConfig util.AgentManifoldConfig

// Manifold returns a dependency.Manifold that extracts the metrics
// spool directory path from the agent.
func Manifold(config ManifoldConfig) dependency.Manifold {
	manifold := util.AgentManifold(util.AgentManifoldConfig(config), newWorker)
	manifold.Output = outputFunc
	return manifold
}

// newWorker creates a degenerate worker that provides access to the metrics
// spool directory path.
func newWorker(a agent.Agent) (worker.Worker, error) {
	metricsSpoolDir := a.CurrentConfig().MetricsSpoolDir()
	w := &metricsSpoolDirWorker{metricsSpoolDir: metricsSpoolDir}
	go func() {
		defer w.tomb.Done()
		<-w.tomb.Dying()
	}()
	return w, nil
}

// outputFunc extracts the metrics spool directory path from a *metricsSpoolDirWorker.
func outputFunc(in worker.Worker, out interface{}) error {
	inWorker, _ := in.(*metricsSpoolDirWorker)
	outPointer, _ := out.(*string)
	if inWorker == nil || outPointer == nil {
		return errors.Errorf("expected %T->%T; got %T->%T", inWorker, outPointer, in, out)
	}
	*outPointer = inWorker.metricsSpoolDir
	return nil
}

// metricsSpoolDirWorker is a degenerate worker that exists only to hold
// the metrics spool directory path.
type metricsSpoolDirWorker struct {
	tomb            tomb.Tomb
	metricsSpoolDir string
}

// Kill is part of the worker.Worker interface.
func (w *metricsSpoolDirWorker) Kill() {
	w.tomb.Kill(nil)
}

// Wait is part of the worker.Worker interface.
func (w *metricsSpoolDirWorker) Wait() error {
	return w.tomb.Wait()
}
