// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package agent

import (
	"github.com/juju/errors"
	"github.com/juju/names"
	"launchpad.net/tomb"

	"github.com/juju/juju/agent"
	"github.com/juju/juju/network"
	"github.com/juju/juju/worker"
	"github.com/juju/juju/worker/dependency"
)

// Agent is the interface exposed to workers that depend upon an agent's
// representation in its dependency.Engine.
type Agent interface {
	Tag() names.Tag
	CurrentConfig() agent.Config
	ChangeConfig(agent.ConfigMutator) error
	SetAPIHostPorts([][]network.HostPort) error
}

// Manifold returns a manifold that starts a worker proxying the supplied Agent
// for use by other workers.
func Manifold(agent Agent) dependency.Manifold {
	return dependency.Manifold{
		Start:  startFunc(agent),
		Output: outputFunc,
	}
}

// startFunc returns a StartFunc that starts a worker holding a reference to
// the supplied Agent.
func startFunc(agent Agent) dependency.StartFunc {
	return func(_ dependency.GetResourceFunc) (worker.Worker, error) {
		w := &agentWorker{agent: agent}
		go func() {
			defer w.tomb.Done()
			<-w.tomb.Dying()
		}()
		return w, nil
	}
}

// outputFunc extracts an Agent from its *agentWorker.
func outputFunc(in worker.Worker, out interface{}) error {
	inWorker, _ := in.(*agentWorker)
	outPointer, _ := out.(*Agent)
	if inWorker == nil || outPointer == nil {
		return errors.Errorf("expected %T->%T; got %T->%T", inWorker, outPointer, in, out)
	}
	*outPointer = inWorker.agent
	return nil
}

// agentWorker exists only to make an Agent accessible via the manifold's Output.
type agentWorker struct {
	tomb  tomb.Tomb
	agent Agent
}

func (w *agentWorker) Kill() {
	w.tomb.Kill(nil)
}

func (w *agentWorker) Wait() error {
	return w.tomb.Wait()
}
