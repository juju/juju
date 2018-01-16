// Copyright 2017 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package caasoperatorprovisioner

import (
	"time"

	"github.com/juju/errors"
	"github.com/juju/loggo"
	"github.com/juju/utils"
	"github.com/juju/utils/clock"
	"gopkg.in/juju/names.v2"
	"gopkg.in/juju/worker.v1"

	"github.com/juju/juju/agent"
	apicaasprovisioner "github.com/juju/juju/api/caasoperatorprovisioner"
	"github.com/juju/juju/apiserver/params"
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
	UpdateUnits(arg params.UpdateApplicationUnits) error
}

// Config defines the operation of a Worker.
type Config struct {
	Facade      CAASProvisionerFacade
	Broker      caas.Broker
	ModelTag    names.ModelTag
	AgentConfig agent.Config
	Clock       clock.Clock
}

// NewProvisionerWorker starts and returns a new CAAS provisioner worker.
func NewProvisionerWorker(config Config) (worker.Worker, error) {
	p := &provisioner{
		provisionerFacade: config.Facade,
		broker:            config.Broker,
		modelTag:          config.ModelTag,
		agentConfig:       config.AgentConfig,
		runner: worker.NewRunner(worker.RunnerParams{
			Clock: config.Clock,

			// One of the application/unit workers failing should not
			// prevent the others from running.
			IsFatal: func(error) bool { return false },

			// For any failures, try again in 5 seconds.
			RestartDelay: 5 * time.Second,
		}),
	}
	err := catacomb.Invoke(catacomb.Plan{
		Site: &p.catacomb,
		Work: p.loop,
		Init: []worker.Worker{p.runner},
	})
	return p, err
}

type provisioner struct {
	catacomb          catacomb.Catacomb
	provisionerFacade CAASProvisionerFacade
	broker            caas.Broker

	runner *worker.Runner

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
				password, err := p.handleApplicationChange(app)
				if err != nil {
					return errors.Trace(err)
				}
				if password == "" {
					continue
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

func (p *provisioner) handleApplicationChange(app string) (string, error) {
	// TODO(caas) - cleanup when an application is deleted
	// For now, assume all changes are for new apps being created.
	logger.Debugf("Received change notification for app: %s", app)

	password, err := utils.RandomPassword()
	if err != nil {
		return "", errors.Trace(err)
	}
	config, err := p.newOperatorConfig(app, password)
	if err != nil {
		return "", errors.Trace(err)
	}
	if err := p.broker.EnsureOperator(app, p.agentConfig.DataDir(), config); err != nil {
		return "", errors.Annotatef(err, "failed to start operator for %q", app)
	}

	if _, err := p.runner.Worker(app, p.catacomb.Dying()); err == nil {
		// TODO(wallyworld): handle application dying or dead.
		// As of now, if the worker is already running, that's all we need.
		return "", nil
	}

	startFunc := func() (worker.Worker, error) {
		appWorker := &applicationWorker{
			applicationName: app,
			broker:          p.broker,
			facade:          p.provisionerFacade,
		}
		if err := catacomb.Invoke(catacomb.Plan{
			Site: &appWorker.catacomb,
			Work: appWorker.loop,
		}); err != nil {
			return nil, errors.Trace(err)
		}
		return appWorker, nil
	}

	logger.Debugf("starting unit watcher for application %q", app)
	if err := p.runner.StartWorker(app, startFunc); err != nil {
		return "", errors.Annotate(err, "error starting application worker")
	}

	return password, nil
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
