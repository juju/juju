// Copyright 2022 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package stateconverter

import (
	"github.com/juju/errors"
	"github.com/juju/names/v5"
	"github.com/juju/worker/v3"
	"github.com/juju/worker/v3/dependency"

	"github.com/juju/juju/agent"
	"github.com/juju/juju/api"
	apimachiner "github.com/juju/juju/api/agent/machiner"
	"github.com/juju/juju/api/base"
	"github.com/juju/juju/core/watcher"
)

// ManifoldConfig provides the dependencies for the
// stateconverter manifold.
type ManifoldConfig struct {
	AgentName     string
	APICallerName string
	Logger        Logger

	// A constructor for the machiner API which can be overridden
	// during testing. If omitted, the default client for the machiner
	// facade will be automatically used.
	NewMachinerAPI func(base.APICaller) Machiner
}

// Manifold returns a Manifold that encapsulates the stateconverter worker.
func Manifold(cfg ManifoldConfig) dependency.Manifold {
	return dependency.Manifold{
		Inputs: []string{
			cfg.AgentName,
			cfg.APICallerName,
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
	if cfg.Logger == nil {
		return errors.NotValidf("nil Logger")
	}
	return nil
}

// start is a StartFunc for a Worker manifold.
func (cfg ManifoldConfig) start(context dependency.Context) (worker.Worker, error) {
	if err := cfg.Validate(); err != nil {
		return nil, errors.Trace(err)
	}

	var a agent.Agent
	if err := context.Get(cfg.AgentName, &a); err != nil {
		return nil, errors.Trace(err)
	}

	tag := a.CurrentConfig().Tag()
	mTag, ok := tag.(names.MachineTag)
	if !ok {
		return nil, errors.NotValidf("%q machine tag", a)
	}

	machiner, err := cfg.newMachiner(context)
	if err != nil {
		return nil, errors.Trace(err)
	}

	cfg.Logger.Tracef("starting NotifyWorker for %s", mTag)
	handlerCfg := config{
		machineTag: mTag,
		machiner:   machiner,
		logger:     cfg.Logger,
	}
	w, err := watcher.NewNotifyWorker(watcher.NotifyConfig{
		Handler: NewConverter(handlerCfg),
	})
	if err != nil {
		return nil, errors.Annotate(err, "cannot start controller promoter worker")
	}
	return w, nil
}

func (cfg ManifoldConfig) newMachiner(context dependency.Context) (Machiner, error) {
	if cfg.NewMachinerAPI != nil {
		machiner := cfg.NewMachinerAPI(nil)
		return machiner, nil
	}
	var apiConn api.Connection
	if err := context.Get(cfg.APICallerName, &apiConn); err != nil {
		return nil, errors.Trace(err)
	}
	return wrapper{m: apimachiner.NewClient(apiConn)}, nil
}
