// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.
package certupdater

import (
	"github.com/juju/errors"
	"github.com/juju/juju/agent"
	"github.com/juju/juju/api/base"
	"github.com/juju/juju/apiserver/params"
	"github.com/juju/juju/state"
	"github.com/juju/juju/worker"
	"github.com/juju/juju/worker/dependency"
	"github.com/juju/juju/worker/util"
)

// ManifoldConfig defines a certupdater's dependencies.
type ManifoldConfig struct {
	util.PostUpgradeManifoldConfig
	APIServerName   string
	OpenState       func() (_ *state.State, _ *state.Machine, err error)
	CertChangedChan chan params.StateServingInfo
	ChangeConfig    func(mutate agent.ConfigMutator) error
}

// Manifold returns a dependency manifold that runs a certupdater worker,
// using the resource names defined in the supplied config.
func Manifold(config ManifoldConfig) dependency.Manifold {

	// newWorker trivially wraps NewCertificateUpdater for use in a util.PostUpgradeManifold.
	//
	// TODO(waigani) - It's not tested at the moment, because the scaffolding
	// necessary is too unwieldy/distracting to introduce at this point.
	newWorker := func(a agent.Agent, apiCaller base.APICaller) (worker.Worker, error) {
		var stateServingSetter StateServingInfoSetter = func(info params.StateServingInfo, done <-chan struct{}) error {
			return config.ChangeConfig(func(cfg agent.ConfigSetter) error {
				cfg.SetStateServingInfo(info)
				logger.Infof("update apiserver worker with new certificate")
				select {
				case config.CertChangedChan <- info:
					return nil
				case <-done:
					return nil
				}
			})
		}

		st, m, err := config.OpenState()
		if err != nil {
			return nil, errors.Trace(err)
		}
		defer st.Close()

		return NewCertificateUpdater(m, a.CurrentConfig(), st, st, stateServingSetter), nil
	}

	return util.PostUpgradeManifold(config.PostUpgradeManifoldConfig, newWorker)
}
