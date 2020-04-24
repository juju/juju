// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package agent

import (
	"github.com/juju/errors"
	"github.com/juju/worker/v2"
	"github.com/juju/worker/v2/dependency"
	"gopkg.in/tomb.v2"

	"github.com/juju/juju/agent"
	"github.com/juju/juju/version"
)

// Manifold returns a manifold that starts a worker proxying the supplied Agent
// for use by other workers.
func Manifold(a agent.Agent) dependency.Manifold {
	return dependency.Manifold{
		Start:  startFunc(a),
		Output: outputFunc,
	}
}

// startFunc returns a StartFunc that starts a worker holding a reference to
// the supplied Agent.
func startFunc(a agent.Agent) dependency.StartFunc {
	return func(_ dependency.Context) (worker.Worker, error) {
		w := &agentWorker{agent: a}
		w.tomb.Go(func() error {
			<-w.tomb.Dying()
			return nil
		})
		return w, nil
	}
}

// outputFunc extracts an Agent from its *agentWorker.
func outputFunc(in worker.Worker, out interface{}) error {
	inWorker, _ := in.(*agentWorker)
	outPointer, _ := out.(*agent.Agent)
	if inWorker == nil || outPointer == nil {
		return errors.Errorf("expected %T->%T; got %T->%T", inWorker, outPointer, in, out)
	}
	*outPointer = inWorker.agent
	return nil
}

// agentWorker is a degenerate worker.Worker that exists only to make an Agent
// accessible via the manifold's Output.
type agentWorker struct {
	tomb  tomb.Tomb
	agent agent.Agent
}

// Kill is part of the worker.Worker interface.
func (w *agentWorker) Kill() {
	w.tomb.Kill(nil)
}

// Wait is part of the worker.Worker interface.
func (w *agentWorker) Wait() error {
	return w.tomb.Wait()
}

// Report shows up in the dependency engine report.
func (w *agentWorker) Report() map[string]interface{} {
	cfg := w.agent.CurrentConfig()
	return map[string]interface{}{
		"agent":      cfg.Tag().String(),
		"model-uuid": cfg.Model().Id(),
		"version":    version.Current.String(),
	}
}
