// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.
package certupdater

import (
	"github.com/juju/errors"
	"github.com/juju/names"

	"github.com/juju/juju/agent"
	"github.com/juju/juju/apiserver/params"
	"github.com/juju/juju/state"
	"github.com/juju/juju/worker"
	apiserverworker "github.com/juju/juju/worker/apiserver"
	"github.com/juju/juju/worker/dependency"
	workerstate "github.com/juju/juju/worker/state"
	"github.com/juju/juju/worker/util"
)

// ManifoldConfig defines a certupdater's dependencies.
type ManifoldConfig struct {
	util.PostUpgradeManifoldConfig
	APIServerName string
	StateName     string
	ChangeConfig  func(mutate agent.ConfigMutator) error
}

// Manifold returns a dependency manifold that runs a certupdater worker,
// using the resource names defined in the supplied config.
func Manifold(config ManifoldConfig) dependency.Manifold {
	return dependency.Manifold{
		Inputs: []string{config.AgentName, config.StateName, config.APIServerName, config.UpgradeWaiterName},
		Start: func(getResource dependency.GetResourceFunc) (worker.Worker, error) {

			var a agent.Agent
			if err := getResource(config.AgentName, &a); err != nil {
				return nil, err
			}

			cfg := a.CurrentConfig()

			// Grab the tag and ensure that it's for a machine.
			tag, ok := cfg.Tag().(names.MachineTag)
			if !ok {
				return nil, errors.New("agent's tag is not a machine tag")
			}

			var stTracker workerstate.StateTracker
			if err := getResource(config.StateName, &stTracker); err != nil {
				return nil, err
			}

			st, err := stTracker.Use()
			if err != nil {
				return nil, errors.Annotate(err, "acquiring state")
			}

			e, err := st.FindEntity(tag)
			if err != nil {
				return nil, err
			}
			m := e.(*state.Machine)

			var upgradesDone bool
			if err := getResource(config.UpgradeWaiterName, &upgradesDone); err != nil {
				return nil, err
			}
			if !upgradesDone {
				return nil, dependency.ErrMissing
			}

			var certChanger apiserverworker.CertChanger
			if err := getResource(config.APIServerName, &certChanger); err != nil {
				return nil, err
			}
			certChangedChan := certChanger.CertChangedChan()

			// newWorker trivially wraps NewCertificateUpdater for use in a util.PostUpgradeManifold.
			//
			// TODO(waigani) - It's not tested at the moment, because the scaffolding
			// necessary is too unwieldy/distracting to introduce at this point.
			var stateServingSetter StateServingInfoSetter = func(info params.StateServingInfo, done <-chan struct{}) error {
				return config.ChangeConfig(func(cfg agent.ConfigSetter) error {
					cfg.SetStateServingInfo(info)
					logger.Infof("update apiserver worker with new certificate")
					select {
					case certChangedChan <- info:
						return nil
					case <-done:
						return nil
					}
				})
			}

			w := NewCertificateUpdater(m, a.CurrentConfig(), st, st, stateServingSetter)

			// When the state workers are done, indicate that we no
			// longer need the State.
			go func() {
				w.Wait()
				stTracker.Done()
			}()
			return w, nil

		},
	}
}
