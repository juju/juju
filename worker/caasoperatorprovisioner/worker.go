// Copyright 2017 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package caasoperatorprovisioner

import (
	"github.com/juju/errors"
	"github.com/juju/juju/apiserver/params"
	"github.com/juju/loggo"
	"github.com/juju/utils"
	"gopkg.in/juju/names.v2"
	"gopkg.in/juju/worker.v1"

	"github.com/juju/juju/agent"
	apicaasprovisioner "github.com/juju/juju/api/caasoperatorprovisioner"
	"github.com/juju/juju/caas"
	"github.com/juju/juju/version"
	"github.com/juju/juju/watcher"
	"github.com/juju/juju/worker/catacomb"
)

var logger = loggo.GetLogger("juju.workers.caasprovisioner")

// CAASProvisionerFacade exposes CAAS provisioning functionality to a worker.
type CAASProvisionerFacade interface {
	WatchApplications() (watcher.StringsWatcher, error)
	SetPasswords([]apicaasprovisioner.ApplicationPassword) (params.ErrorResults, error)
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

func (p *provisioner) loop() error {
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
			// TODO(caas) - cleanup when an application is deleted

			var appPasswords []apicaasprovisioner.ApplicationPassword
			for _, app := range apps {
				logger.Debugf("Received change notification for app: %s", app)
				password, err := utils.RandomPassword()
				if err != nil {
					return errors.Trace(err)
				}
				config, err := p.newOperatorConfig(app, password)
				if err != nil {
					return errors.Trace(err)
				}
				if err := p.broker.EnsureOperator(app, p.agentConfig.DataDir(), config); err != nil {
					return errors.Annotatef(err, "failed to start operator for %q", app)
				}
				appPasswords = append(appPasswords, apicaasprovisioner.ApplicationPassword{Name: app, Password: password})
			}
			errorResults, err := p.provisionerFacade.SetPasswords(appPasswords)
			if err != nil {
				return errors.Annotate(err, "failed to set application api passwords")
			}
			if err := errorResults.Combine(); err != nil {
				return errors.Annotate(err, "failed to set application api passwords")
			}
		}
	}
}

func (p *provisioner) newOperatorConfig(appName string, password string) (*caas.OperatorConfig, error) {
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
			Password:          password,
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
