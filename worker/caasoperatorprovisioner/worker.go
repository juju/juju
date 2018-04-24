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

	"github.com/juju/juju/agent"
	apicaasprovisioner "github.com/juju/juju/api/caasoperatorprovisioner"
	"github.com/juju/juju/apiserver/params"
	"github.com/juju/juju/caas"
	"github.com/juju/juju/core/life"
	"github.com/juju/juju/version"
	"github.com/juju/juju/watcher"
	"github.com/juju/juju/worker/catacomb"
)

var logger = loggo.GetLogger("juju.workers.caasprovisioner")

// CAASProvisionerFacade exposes CAAS provisioning functionality to a worker.
type CAASProvisionerFacade interface {
	OperatorProvisioningInfo() (apicaasprovisioner.OperatorProvisioningInfo, error)
	WatchApplications() (watcher.StringsWatcher, error)
	SetPasswords([]apicaasprovisioner.ApplicationPassword) (params.ErrorResults, error)
	Life(string) (life.Value, error)
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
			var appPasswords []apicaasprovisioner.ApplicationPassword
			for _, app := range apps {
				appLife, err := p.provisionerFacade.Life(app)
				if errors.IsNotFound(err) || appLife == life.Dead {
					logger.Debugf("deleting operator for %q", app)
					if err := p.broker.DeleteOperator(app); err != nil {
						return errors.Annotatef(err, "failed to stop operator for %q", app)
					}
					continue
				}
				if appLife != life.Alive {
					continue
				}

				password, err := utils.RandomPassword()
				if err != nil {
					return errors.Trace(err)
				}
				appPasswords = append(appPasswords, apicaasprovisioner.ApplicationPassword{Name: app, Password: password})
			}
			if len(appPasswords) == 0 {
				continue
			}
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
				if err := p.createOperator(appPasswords[i].Name, appPasswords[i].Password); err != nil {
					return errors.Trace(err)
				}
			}
			if errorStrings != nil {
				err := errors.New(strings.Join(errorStrings, "\n"))
				return errors.Annotate(err, "failed to set application api passwords")
			}
		}
	}
}

func (p *provisioner) createOperator(app, password string) error {
	logger.Debugf("creating operator for %q", app)
	config, err := p.newOperatorConfig(app, password)
	if err != nil {
		return errors.Trace(err)
	}
	if err := p.broker.EnsureOperator(app, p.agentConfig.DataDir(), config); err != nil {
		return errors.Annotatef(err, "failed to start operator for %q", app)
	}
	return nil
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

	info, err := p.provisionerFacade.OperatorProvisioningInfo()
	if err != nil {
		return nil, errors.Trace(err)
	}
	logger.Debugf("using caas operator info %+v", info)
	return &caas.OperatorConfig{AgentConf: confBytes, OperatorImagePath: info.ImagePath}, nil
}
