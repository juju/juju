// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package proxyupdater

import (
	"github.com/juju/errors"
	"github.com/juju/utils/proxy"
	worker "gopkg.in/juju/worker.v1"

	"github.com/juju/juju/agent"
	"github.com/juju/juju/api/base"
	"github.com/juju/juju/api/proxyupdater"
	"github.com/juju/juju/worker/dependency"
)

// ManifoldConfig defines the names of the manifolds on which a Manifold will depend.
type ManifoldConfig struct {
	AgentName       string
	APICallerName   string
	WorkerFunc      func(Config) (worker.Worker, error)
	ExternalUpdate  func(proxy.Settings) error
	InProcessUpdate func(proxy.Settings) error
}

// Manifold returns a dependency manifold that runs a proxy updater worker,
// using the api connection resource named in the supplied config.
func Manifold(config ManifoldConfig) dependency.Manifold {
	return dependency.Manifold{
		Inputs: []string{
			config.AgentName,
			config.APICallerName,
		},
		Start: func(context dependency.Context) (worker.Worker, error) {
			if config.WorkerFunc == nil {
				return nil, errors.NotValidf("missing WorkerFunc")
			}
			if config.InProcessUpdate == nil {
				return nil, errors.NotValidf("missing InProcessUpdate")
			}
			var agent agent.Agent
			if err := context.Get(config.AgentName, &agent); err != nil {
				return nil, err
			}
			var apiCaller base.APICaller
			if err := context.Get(config.APICallerName, &apiCaller); err != nil {
				return nil, err
			}

			agentConfig := agent.CurrentConfig()
			proxyAPI, err := proxyupdater.NewAPI(apiCaller, agentConfig.Tag())
			if err != nil {
				return nil, err
			}
			w, err := config.WorkerFunc(Config{
				SystemdFiles: []string{"/etc/systemd/system.conf.d/juju-proxy.conf",
					"/etc/systemd/user.conf.d/juju-proxy.conf"},
				EnvFiles:        []string{"/etc/juju-proxy.conf"},
				RegistryPath:    `HKCU:\Software\Microsoft\Windows\CurrentVersion\Internet Settings`,
				API:             proxyAPI,
				ExternalUpdate:  config.ExternalUpdate,
				InProcessUpdate: config.InProcessUpdate,
			})
			if err != nil {
				return nil, errors.Trace(err)
			}
			return w, nil
		},
	}
}
