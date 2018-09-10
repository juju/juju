// Copyright 2017 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package caasoperatorprovisioner

import (
	"strings"

	"github.com/juju/errors"
	"github.com/juju/loggo"
	"github.com/juju/utils"
	"gopkg.in/juju/names.v2"
	"gopkg.in/juju/worker.v1"
	"gopkg.in/juju/worker.v1/catacomb"

	"github.com/juju/juju/agent"
	apicaasprovisioner "github.com/juju/juju/api/caasoperatorprovisioner"
	"github.com/juju/juju/apiserver/params"
	"github.com/juju/juju/caas"
	"github.com/juju/juju/core/life"
	"github.com/juju/juju/watcher"
)

var logger = loggo.GetLogger("juju.workers.caasprovisioner")

// CAASProvisionerFacade exposes CAAS provisioning functionality to a worker.
type CAASProvisionerFacade interface {
	OperatorProvisioningInfo() (apicaasprovisioner.OperatorProvisioningInfo, error)
	WatchApplications() (watcher.StringsWatcher, error)
	SetPasswords([]apicaasprovisioner.ApplicationPassword) (params.ErrorResults, error)
	Life(string) (life.Value, error)
	WatchAPIHostPorts() (watcher.NotifyWatcher, error)
	APIAddresses() ([]string, error)
}

// Config defines the operation of a Worker.
type Config struct {
	Facade      CAASProvisionerFacade
	Broker      caas.Broker
	ModelTag    names.ModelTag
	AgentConfig agent.Config
}

// NewProvisionerWorker starts and returns a new CAAS provisioner worker.
func NewProvisionerWorker(config Config) (worker.Worker, error) {
	p := &provisioner{
		provisionerFacade: config.Facade,
		broker:            config.Broker,
		modelTag:          config.ModelTag,
		agentConfig:       config.AgentConfig,
		appPasswords:      make(map[string]string),
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

	appPasswords map[string]string
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

	appWatcher, err := p.provisionerFacade.WatchApplications()
	if err != nil {
		return errors.Trace(err)
	}
	if err := p.catacomb.Add(appWatcher); err != nil {
		return errors.Trace(err)
	}

	apiAddressWatcher, err := p.provisionerFacade.WatchAPIHostPorts()
	if err != nil {
		return errors.Trace(err)
	}
	if err := p.catacomb.Add(apiAddressWatcher); err != nil {
		return errors.Trace(err)
	}

	var apiAddressChanged watcher.NotifyChannel
	for {
		select {
		case <-p.catacomb.Dying():
			return p.catacomb.ErrDying()

		// API addresses have changed so we need to update
		// each operator pod so it has the new addresses.
		case _, ok := <-apiAddressChanged:
			if !ok {
				return errors.New("api watcher closed channel")
			}
			for app, password := range p.appPasswords {
				if err := p.ensureOperator(app, password); err != nil {
					return errors.Annotatef(err, "updating operator for %q with new api addresses", app)
				}
			}

		// CAAS applications changed so either create or remove pods as appropriate.
		case apps, ok := <-appWatcher.Changes():
			if !ok {
				return errors.New("app watcher closed channel")
			}
			var newApps []apicaasprovisioner.ApplicationPassword
			for _, app := range apps {
				appLife, err := p.provisionerFacade.Life(app)
				if errors.IsNotFound(err) || appLife == life.Dead {
					logger.Debugf("deleting operator for %q", app)
					if err := p.broker.DeleteOperator(app); err != nil {
						return errors.Annotatef(err, "failed to stop operator for %q", app)
					}
					delete(p.appPasswords, app)
					continue
				}
				if appLife != life.Alive {
					continue
				}

				password, err := utils.RandomPassword()
				if err != nil {
					return errors.Trace(err)
				}
				newApps = append(newApps, apicaasprovisioner.ApplicationPassword{Name: app, Password: password})
			}
			if len(newApps) == 0 {
				continue
			}
			if err := p.ensureOperators(newApps); err != nil {
				return errors.Trace(err)
			}
			// Store the apps we have just added.
			for _, ap := range newApps {
				p.appPasswords[ap.Name] = ap.Password
			}

			// Now we have been through all the applications at least once, we can
			// listen for api address changes.
			apiAddressChanged = apiAddressWatcher.Changes()
		}
	}
}

// ensureOperators creates operator pods for the specified app names -> api passwords.
func (p *provisioner) ensureOperators(appPasswords []apicaasprovisioner.ApplicationPassword) error {
	errorResults, err := p.provisionerFacade.SetPasswords(appPasswords)
	if err != nil {
		return errors.Annotate(err, "failed to set application api passwords")
	}
	var errorStrings []string
	for i, r := range errorResults.Results {
		if r.Error != nil {
			errorStrings = append(errorStrings, r.Error.Error())
			continue
		}
		if err := p.ensureOperator(appPasswords[i].Name, appPasswords[i].Password); err != nil {
			return errors.Trace(err)
		}
	}
	if errorStrings != nil {
		err := errors.New(strings.Join(errorStrings, "\n"))
		return errors.Annotate(err, "failed to set application api passwords")
	}
	return nil
}

func (p *provisioner) ensureOperator(app, password string) error {
	config, err := p.newOperatorConfig(app, password)
	if err != nil {
		return errors.Trace(err)
	}
	if err := p.broker.EnsureOperator(app, p.agentConfig.DataDir(), config); err != nil {
		return errors.Annotatef(err, "failed to start operator for %q", app)
	}
	logger.Infof("started operator for application %q", app)
	return nil
}

func (p *provisioner) newOperatorConfig(appName string, password string) (*caas.OperatorConfig, error) {
	appTag := names.NewApplicationTag(appName)
	apiAddrs, err := p.provisionerFacade.APIAddresses()
	if err != nil {
		return nil, errors.Trace(err)
	}

	info, err := p.provisionerFacade.OperatorProvisioningInfo()
	if err != nil {
		return nil, errors.Trace(err)
	}
	conf, err := agent.NewAgentConfig(
		agent.AgentConfigParams{
			Paths: agent.Paths{
				DataDir: p.agentConfig.DataDir(),
				LogDir:  p.agentConfig.LogDir(),
			},
			UpgradedToVersion: info.Version,
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

	logger.Debugf("using caas operator info %+v", info)
	return &caas.OperatorConfig{
		AgentConf:         confBytes,
		OperatorImagePath: info.ImagePath,
		Version:           info.Version,
	}, nil
}
