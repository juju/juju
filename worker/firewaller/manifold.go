// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package firewaller

import (
	"github.com/juju/errors"
	"github.com/juju/worker/v2"
	"github.com/juju/worker/v2/dependency"

	"github.com/juju/juju/agent"
	"github.com/juju/juju/api"
	"github.com/juju/juju/api/base"
	"github.com/juju/juju/api/remoterelations"
	"github.com/juju/juju/environs"
	"github.com/juju/juju/environs/config"
	"github.com/juju/juju/worker/apicaller"
	"github.com/juju/juju/worker/common"
)

// logger is here to stop the desire of creating a package level logger.
// Don't do this, instead use the one passed as manifold config.
var logger interface{}

// Logger represents the methods used by the worker to log details.
type Logger interface {
	Tracef(string, ...interface{})
	Debugf(string, ...interface{})
	Infof(string, ...interface{})
	Errorf(string, ...interface{})
}

// ManifoldConfig describes the resources used by the firewaller worker.
type ManifoldConfig struct {
	AgentName     string
	APICallerName string
	EnvironName   string
	Logger        Logger

	NewControllerConnection      apicaller.NewExternalControllerConnectionFunc
	NewRemoteRelationsFacade     func(base.APICaller) (*remoterelations.Client, error)
	NewFirewallerFacade          func(base.APICaller) (FirewallerAPI, error)
	NewFirewallerWorker          func(Config) (worker.Worker, error)
	NewCredentialValidatorFacade func(base.APICaller) (common.CredentialAPI, error)
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
	if cfg.Logger == nil {
		return errors.NotValidf("nil Logger")
	}
	if cfg.NewControllerConnection == nil {
		return errors.NotValidf("nil NewControllerConnection")
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
	if cfg.NewCredentialValidatorFacade == nil {
		return errors.NotValidf("nil NewCredentialValidatorFacade")
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

	// Check if the env supports global firewalling.  If the
	// configured mode is instance, we can ignore fwEnv being a
	// nil value, as it won't be used.
	fwEnv, fwEnvOK := environ.(environs.Firewaller)

	mode := environ.Config().FirewallMode()
	if mode == config.FwNone {
		cfg.Logger.Infof("stopping firewaller (not required)")
		return nil, dependency.ErrUninstall
	} else if mode == config.FwGlobal {
		if !fwEnvOK {
			cfg.Logger.Infof("Firewall global mode set on provider with no support. stopping firewaller")
			return nil, dependency.ErrUninstall
		}
	}

	firewallerAPI, err := cfg.NewFirewallerFacade(apiConn)
	if err != nil {
		return nil, errors.Trace(err)
	}
	remoteRelationsAPI, err := cfg.NewRemoteRelationsFacade(apiConn)
	if err != nil {
		return nil, errors.Trace(err)
	}

	credentialAPI, err := cfg.NewCredentialValidatorFacade(apiConn)
	if err != nil {
		return nil, errors.Trace(err)
	}

	w, err := cfg.NewFirewallerWorker(Config{
		ModelUUID:               agent.CurrentConfig().Model().Id(),
		RemoteRelationsApi:      remoteRelationsAPI,
		FirewallerAPI:           firewallerAPI,
		EnvironFirewaller:       fwEnv,
		EnvironInstances:        environ,
		Mode:                    mode,
		NewCrossModelFacadeFunc: crossmodelFirewallerFacadeFunc(cfg.NewControllerConnection),
		CredentialAPI:           credentialAPI,
		Logger:                  cfg.Logger,
	})
	if err != nil {
		return nil, errors.Trace(err)
	}
	return w, nil
}
