// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package agent

import (
	"github.com/juju/names"
	"launchpad.net/tomb"

	"github.com/juju/juju/agent"
	"github.com/juju/juju/worker"
	"github.com/juju/juju/worker/dependency"
)

type Agent interface {
	Tag() names.Tag
	CurrentConfig() agent.Config
	ChangeConfig(agent.ConfigMutator) error
}

func Manifold(agent Agent) dependency.Manifold {
	return dependency.Manifold{
		Start: func(_ dependency.GetResourceFunc) (worker.Worker, error) {
			w := &agentWorker{agent: agent}
			go func() {
				defer w.tomb.Done()
				<-w.tomb.Dying()
			}()
			return w, nil
		},
		Output: func(in worker.Worker, out interface{}) bool {
			inWorker, _ := in.(*agentWorker)
			outPointer, _ := out.(*Agent)
			if inWorker == nil || outPointer == nil {
				return false
			}
			*outPointer = inWorker.agent
			return true
		},
	}
}

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
