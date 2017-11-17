// Copyright 2017 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package caasprovisioner

import (
	"github.com/juju/errors"
	"github.com/juju/juju/caas"
	"github.com/juju/juju/watcher"
	"github.com/juju/loggo"
	"gopkg.in/juju/names.v2"
	"gopkg.in/juju/worker.v1"

	"github.com/juju/juju/agent"
	"github.com/juju/juju/version"
	"github.com/juju/juju/worker/catacomb"
)

var logger = loggo.GetLogger("juju.workers.caasprovisioner")

// CAASProvisionerFacade exposes CAAS provisioning functionality to a worker.
type CAASProvisionerFacade interface {
	WatchApplications() (watcher.StringsWatcher, error)
}

// NewProvisionerWorker starts and returns a new CAAS provisioner worker.
func NewProvisionerWorker(
	facade CAASProvisionerFacade,
	broker caas.Broker,
	modelTag names.ModelTag,
	agentConfig agent.Config,
) (worker.Worker, error) {
	p := &provisioner{
		provisionerFacade: facade,
		broker:            broker,
		modelTag:          modelTag,
		agentConfig:       agentConfig,
	}
	err := catacomb.Invoke(catacomb.Plan{
		Site: &p.catacomb,
		Work: p.loop,
	})
	return p, err
}

type provisioner struct {
	catacomb          catacomb.Catacomb
	provisionerFacade CAASProvisionerFacade
	broker            caas.Broker

	modelTag    names.ModelTag
	agentConfig agent.Config
}

// Kill is part of the worker.Worker interface.
func (p *provisioner) Kill() {
	p.catacomb.Kill(nil)
}

// Wait is part of the worker.Worker interface.
func (p *provisioner) Wait() error {
	return p.catacomb.Wait()
}

// TODO(caas) - remove
type fakeFacade struct {
	watcher.CoreWatcher
}

func (f *fakeFacade) WatchApplications() (watcher.StringsWatcher, error) {
	return f, nil
}

func (f *fakeFacade) Changes() watcher.StringsChannel {
	return make(watcher.StringsChannel)
}

func (p *provisioner) loop() error {
	// TODO(caas) - remove
	if p.provisionerFacade == nil {
		logger.Criticalf("Started CAAS Provisioner worker with fake facade and broker %v", p.broker)
		p.provisionerFacade = &fakeFacade{}
	}

	newConfig := func(appName string) (*caas.OperatorConfig, error) {
		return p.newOperatorConfig(appName)
	}

	// TODO(caas) -  this loop should also keep an eye on kubernetes and ensure
	// that the operator stays up, redeploying it if the pod goes
	// away. For some runtimes we *could* rely on the the runtime's
	// features to do this.

	w, err := p.provisionerFacade.WatchApplications()
	if err != nil {
		return errors.Trace(err)
	}
	if err := p.catacomb.Add(w); err != nil {
		return errors.Trace(err)
	}

	for {
		select {
		case <-p.catacomb.Dying():
			return p.catacomb.ErrDying()
		case apps, ok := <-w.Changes():
			if !ok {
				return errors.New("watcher closed channel")
			}
			for _, app := range apps {
				logger.Debugf("Received change notification for app: %s", app)
				if err := p.broker.EnsureOperator(app, p.agentConfig.DataDir(), newConfig); err != nil {
					return errors.Annotatef(err, "failed to start operator for %q", app)
				}
			}
		}
	}
}

func (p *provisioner) newOperatorConfig(appName string) (*caas.OperatorConfig, error) {
	appTag := names.NewApplicationTag(appName)

	// TODO(caas) - restart operator when api addresses change
	apiAddrs, err := p.agentConfig.APIAddresses()
	if err != nil {
		return nil, errors.Trace(err)
	}

	conf, err := agent.NewAgentConfig(
		agent.AgentConfigParams{
			Paths: agent.Paths{
				DataDir: p.agentConfig.DataDir(),
				LogDir:  p.agentConfig.LogDir(),
			},
			// This isn't actually used but needs to be supplied.
			UpgradedToVersion: version.Current,
			Tag:               appTag,
			Password:          p.agentConfig.OldPassword(),
			Controller:        p.agentConfig.Controller(),
			Model:             p.modelTag,
			APIAddresses:      apiAddrs,
			CACert:            p.agentConfig.CACert(),
		},
	)
	if err != nil {
		return nil, errors.Trace(err)
	}

	confBytes, err := conf.Render()
	if err != nil {
		return nil, errors.Trace(err)
	}
	return &caas.OperatorConfig{AgentConf: confBytes}, nil
}
