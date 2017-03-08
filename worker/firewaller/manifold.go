// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package firewaller

import (
	"github.com/juju/errors"

	"github.com/juju/juju/agent"
	"github.com/juju/juju/api"
	"github.com/juju/juju/api/base"
	"github.com/juju/juju/api/remoterelations"
	"github.com/juju/juju/environs"
	"github.com/juju/juju/environs/config"
	"github.com/juju/juju/worker"
	"github.com/juju/juju/worker/dependency"
)

// ManifoldConfig describes the resources used by the firewaller worker.
type ManifoldConfig struct {
	AgentName     string
	APICallerName string
	EnvironName   string

	NewAPIConnForModel       api.NewConnectionForModelFunc
	NewRemoteRelationsFacade func(base.APICaller) (*remoterelations.Client, error)
	NewFirewallerFacade      func(base.APICaller) (FirewallerAPI, error)
	NewFirewallerWorker      func(Config) (worker.Worker, error)
}

// Manifold returns a Manifold that encapsulates the firewaller worker.
func Manifold(cfg ManifoldConfig) dependency.Manifold {
	return dependency.Manifold{
		Inputs: []string{
			cfg.AgentName,
			cfg.APICallerName,
			cfg.EnvironName,
		},
		Start: cfg.start,
	}
}

// Validate is called by start to check for bad configuration.
func (cfg ManifoldConfig) Validate() error {
	if cfg.AgentName == "" {
		return errors.NotValidf("empty AgentName")
	}
	if cfg.APICallerName == "" {
		return errors.NotValidf("empty APICallerName")
	}
	if cfg.EnvironName == "" {
		return errors.NotValidf("empty EnvironName")
	}
	if cfg.NewAPIConnForModel == nil {
		return errors.NotValidf("nil NewAPIConnForModel")
	}
	if cfg.NewRemoteRelationsFacade == nil {
		return errors.NotValidf("nil NewRemoteRelationsFacade")
	}
	if cfg.NewFirewallerFacade == nil {
		return errors.NotValidf("nil NewFirewallerFacade")
	}
	if cfg.NewFirewallerWorker == nil {
		return errors.NotValidf("nil NewFirewallerWorker")
	}
	return nil
}

// start is a StartFunc for a Worker manifold.
func (cfg ManifoldConfig) start(context dependency.Context) (worker.Worker, error) {
	if err := cfg.Validate(); err != nil {
		return nil, errors.Trace(err)
	}

	var agent agent.Agent
	if err := context.Get(cfg.AgentName, &agent); err != nil {
		return nil, errors.Trace(err)
	}
	var apiConn api.Connection
	if err := context.Get(cfg.APICallerName, &apiConn); err != nil {
		return nil, errors.Trace(err)
	}

	var environ environs.Environ
	if err := context.Get(cfg.EnvironName, &environ); err != nil {
		return nil, errors.Trace(err)
	}
	mode := environ.Config().FirewallMode()
	if mode == config.FwNone {
		logger.Infof("stopping firewaller (not required)")
		return nil, dependency.ErrUninstall
	}

	agentConf := agent.CurrentConfig()
	apiInfo, ok := agentConf.APIInfo()
	if !ok {
		return nil, errors.New("no API connection details")
	}
	apiConnForModelFunc, err := cfg.NewAPIConnForModel(apiInfo)
	if err != nil {
		return nil, errors.Trace(err)
	}

	firewallerAPI, err := cfg.NewFirewallerFacade(apiConn)
	if err != nil {
		return nil, errors.Trace(err)
	}
	remoteRelationsAPI, err := cfg.NewRemoteRelationsFacade(apiConn)
	if err != nil {
		return nil, errors.Trace(err)
	}

	w, err := cfg.NewFirewallerWorker(Config{
		ModelUUID:          agent.CurrentConfig().Model().Id(),
		RemoteRelationsApi: remoteRelationsAPI,
		FirewallerAPI:      firewallerAPI,
		EnvironFirewaller:  environ,
		EnvironInstances:   environ,
		Mode:               mode,
		NewRemoteFirewallerAPIFunc: remoteFirewallerAPIFunc(apiConnForModelFunc),
	})
	if err != nil {
		return nil, errors.Trace(err)
	}
	return w, nil
}
