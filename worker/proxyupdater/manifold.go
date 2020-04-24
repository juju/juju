// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package proxyupdater

import (
	"github.com/juju/errors"
	"github.com/juju/proxy"
	"github.com/juju/worker/v2"
	"github.com/juju/worker/v2/dependency"

	"github.com/juju/juju/agent"
	"github.com/juju/juju/api/base"
	"github.com/juju/juju/api/proxyupdater"
)

// Logger represents the methods used for logging messages.
type Logger interface {
	Errorf(string, ...interface{})
	Warningf(string, ...interface{})
	Infof(string, ...interface{})
	Debugf(string, ...interface{})
	Tracef(string, ...interface{})
}

// ManifoldConfig defines the names of the manifolds on which a Manifold will depend.
type ManifoldConfig struct {
	AgentName           string
	APICallerName       string
	Logger              Logger
	WorkerFunc          func(Config) (worker.Worker, error)
	SupportLegacyValues bool
	ExternalUpdate      func(proxy.Settings) error
	InProcessUpdate     func(proxy.Settings) error
	RunFunc             func(string, string, ...string) (string, error)
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
				SystemdFiles:        []string{"/etc/juju-proxy-systemd.conf"},
				EnvFiles:            []string{"/etc/juju-proxy.conf"},
				RegistryPath:        `HKCU:\Software\Microsoft\Windows\CurrentVersion\Internet Settings`,
				API:                 proxyAPI,
				SupportLegacyValues: config.SupportLegacyValues,
				ExternalUpdate:      config.ExternalUpdate,
				InProcessUpdate:     config.InProcessUpdate,
				Logger:              config.Logger,
				RunFunc:             config.RunFunc,
			})
			if err != nil {
				return nil, errors.Trace(err)
			}
			return w, nil
		},
	}
}
