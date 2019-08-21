// Copyright 2017 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package caasoperatorprovisioner

import (
	"strings"
	"time"

	"github.com/juju/clock"
	"github.com/juju/errors"
	"github.com/juju/loggo"
	"github.com/juju/retry"
	"github.com/juju/utils"
	"gopkg.in/juju/names.v3"
	"gopkg.in/juju/worker.v1"
	"gopkg.in/juju/worker.v1/catacomb"

	"github.com/juju/juju/agent"
	apicaasprovisioner "github.com/juju/juju/api/caasoperatorprovisioner"
	"github.com/juju/juju/apiserver/params"
	"github.com/juju/juju/caas"
	"github.com/juju/juju/caas/kubernetes/provider"
	"github.com/juju/juju/core/life"
	"github.com/juju/juju/core/watcher"
	"github.com/juju/juju/storage"
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
	Clock       clock.Clock
}

// NewProvisionerWorker starts and returns a new CAAS provisioner worker.
func NewProvisionerWorker(config Config) (worker.Worker, error) {
	p := &provisioner{
		provisionerFacade: config.Facade,
		broker:            config.Broker,
		modelTag:          config.ModelTag,
		agentConfig:       config.AgentConfig,
		clock:             config.Clock,
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
	clock             clock.Clock

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

	appWatcher, err := p.provisionerFacade.WatchApplications()
	if err != nil {
		return errors.Trace(err)
	}
	if err := p.catacomb.Add(appWatcher); err != nil {
		return errors.Trace(err)
	}

	for {
		select {
		case <-p.catacomb.Dying():
			return p.catacomb.ErrDying()

		// CAAS applications changed so either create or remove pods as appropriate.
		case apps, ok := <-appWatcher.Changes():
			if !ok {
				return errors.New("app watcher closed channel")
			}
			var newApps []string
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
				newApps = append(newApps, app)
			}
			if len(newApps) == 0 {
				continue
			}
			if err := p.ensureOperators(newApps); err != nil {
				return errors.Trace(err)
			}
		}
	}
}

func (p *provisioner) waitForOperatorTerminated(app string) error {
	tryAgain := errors.New("try again")
	existsFunc := func() error {
		opState, err := p.broker.OperatorExists(app)
		if err != nil {
			return errors.Trace(err)
		}
		if !opState.Exists {
			return nil
		}
		if opState.Exists && !opState.Terminating {
			return errors.Errorf("operator %q should be terminating but is now running", app)
		}
		return tryAgain
	}
	retryCallArgs := retry.CallArgs{
		Attempts:    60,
		Delay:       3 * time.Second,
		MaxDuration: 3 * time.Minute,
		Clock:       p.clock,
		Func:        existsFunc,
		IsFatalError: func(err error) bool {
			return err != tryAgain
		},
	}
	return errors.Trace(retry.Call(retryCallArgs))
}

// ensureOperators creates operator pods for the specified app names -> api passwords.
func (p *provisioner) ensureOperators(apps []string) error {
	var appPasswords []apicaasprovisioner.ApplicationPassword
	operatorConfig := make([]*caas.OperatorConfig, len(apps))
	for i, app := range apps {
		opState, err := p.broker.OperatorExists(app)
		if err != nil {
			return errors.Annotatef(err, "failed to find operator for %q", app)
		}
		if opState.Exists && opState.Terminating {
			// We can't deploy an app while a previous version is terminating.
			// TODO(caas) - the remove application process should block until app terminated
			// TODO(caas) - consider making this async, but ok for now as it's a corner case
			if err := p.waitForOperatorTerminated(app); err != nil {
				return errors.Annotatef(err, "operator for %q was terminating and there was an error waiting for it to stop", app)
			}
			opState.Exists = false
		}
		// If the operator does not exist already, we need to create an initial
		// password for it.
		var password string
		if !opState.Exists {
			if password, err = utils.RandomPassword(); err != nil {
				return errors.Trace(err)
			}
			appPasswords = append(appPasswords, apicaasprovisioner.ApplicationPassword{Name: app, Password: password})
		}

		config, err := p.makeOperatorConfig(app, password)
		if err != nil {
			return errors.Annotatef(err, "failed to generate operator config for %q", app)
		}
		operatorConfig[i] = config
	}
	// If we did create any passwords for new operators, first they need
	// to be saved so the agent can login when it starts up.
	if len(appPasswords) > 0 {
		errorResults, err := p.provisionerFacade.SetPasswords(appPasswords)
		if err != nil {
			return errors.Annotate(err, "failed to set application api passwords")
		}
		if err := errorResults.Combine(); err != nil {
			return errors.Annotate(err, "failed to set application api passwords")
		}
	}

	// Now that any new config/passwords are done, create or update
	// the operators themselves.
	var errorStrings []string
	for i, app := range apps {
		if err := p.ensureOperator(app, operatorConfig[i]); err != nil {
			errorStrings = append(errorStrings, err.Error())
			continue
		}
	}
	if errorStrings != nil {
		err := errors.New(strings.Join(errorStrings, "\n"))
		return errors.Annotate(err, "failed to provision all operators")
	}
	return nil
}

func (p *provisioner) ensureOperator(app string, config *caas.OperatorConfig) error {
	if err := p.broker.EnsureOperator(app, p.agentConfig.DataDir(), config); err != nil {
		return errors.Annotatef(err, "failed to start operator for %q", app)
	}
	logger.Infof("started operator for application %q", app)
	return nil
}

func (p *provisioner) makeOperatorConfig(appName, password string) (*caas.OperatorConfig, error) {
	appTag := names.NewApplicationTag(appName)
	info, err := p.provisionerFacade.OperatorProvisioningInfo()
	if err != nil {
		return nil, errors.Trace(err)
	}
	// All operators must have storage configured because charms
	// have persistent state which must be preserved between any
	// operator restarts.
	if info.CharmStorage.Provider != provider.K8s_ProviderType {
		if spType := info.CharmStorage.Provider; spType == "" {
			return nil, errors.NotValidf("missing operator storage provider")
		} else {
			return nil, errors.NotSupportedf("operator storage provider %q", spType)
		}
	}
	logger.Debugf("using caas operator info %+v", info)

	cfg := &caas.OperatorConfig{
		OperatorImagePath: info.ImagePath,
		Version:           info.Version,
		ResourceTags:      info.Tags,
		CharmStorage:      charmStorageParams(info.CharmStorage),
	}
	// If no password required, we leave the agent conf empty.
	if password == "" {
		return cfg, nil
	}

	conf, err := agent.NewAgentConfig(
		agent.AgentConfigParams{
			Paths: agent.Paths{
				DataDir: p.agentConfig.DataDir(),
				LogDir:  p.agentConfig.LogDir(),
			},
			Tag:          appTag,
			Controller:   p.agentConfig.Controller(),
			Model:        p.modelTag,
			APIAddresses: info.APIAddresses,
			CACert:       p.agentConfig.CACert(),
			Password:     password,

			// UpgradedToVersion is mandatory but not used by caas operator agents as they
			// are not upgraded insitu.
			UpgradedToVersion: info.Version,
		},
	)
	if err != nil {
		return nil, errors.Trace(err)
	}

	confBytes, err := conf.Render()
	if err != nil {
		return nil, errors.Trace(err)
	}
	cfg.AgentConf = confBytes
	return cfg, nil
}

func charmStorageParams(in storage.KubernetesFilesystemParams) caas.CharmStorageParams {
	return caas.CharmStorageParams{
		Provider:     in.Provider,
		Size:         in.Size,
		Attributes:   in.Attributes,
		ResourceTags: in.ResourceTags,
	}
}
