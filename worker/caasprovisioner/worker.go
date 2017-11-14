// Copyright 2017 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package caasprovisioner

import (
	"github.com/juju/errors"
	"github.com/juju/juju/watcher"
	"github.com/juju/loggo"
	"gopkg.in/juju/names.v2"
	"gopkg.in/juju/worker.v1"

	"github.com/juju/juju/agent"
	apicaasprovisioner "github.com/juju/juju/api/caasprovisioner"
	"github.com/juju/juju/version"
	"github.com/juju/juju/worker/catacomb"
)

var logger = loggo.GetLogger("juju.workers.caasprovisioner")

// CAASProvisionerFacade exposes CAAS provisioning functionality to a worker.
type CAASProvisionerFacade interface {
	WatchApplications() (watcher.StringsWatcher, error)
	ConnectionConfig() (*apicaasprovisioner.ConnectionConfig, error)
}

// CAASClient instances are used to control the CAAS cloud.
type CAASClient interface {
	EnsureOperator(appName, agentPath string, newConfig NewOperatorConfigFunc) error
}

// NewOperatorConfigFunc functions return the agent config to use for
// a CAAS jujud operator.
type NewOperatorConfigFunc func(appName string) ([]byte, error)

// NewCAASClientFunc functions return a client used to control the CAAS cloud.
type NewCAASClientFunc func(f CAASProvisionerFacade) (CAASClient, error)

// NewProvisionerWorker starts and returns a new CAAS provisioner worker.
func NewProvisionerWorker(
	facade CAASProvisionerFacade,
	newCAASClient NewCAASClientFunc,
	modelTag names.ModelTag,
	agentConfig agent.Config,
) (worker.Worker, error) {
	p := &provisioner{
		provisionerFacade: facade,
		newCAASClientFunc: newCAASClient,
		modelTag:          modelTag,
		agentConfig:       agentConfig,
	}
	err := catacomb.Invoke(catacomb.Plan{
		Site: &p.catacomb,
		Work: p.loop,
	})
	return p, err
}

// newCAASClient creates a new client object to talk to the CAAS cloud.
func newCAASClient(f CAASProvisionerFacade) (CAASClient, error) {
	// TODO(caas) - abstract out kubernetes specific functionality
	return newK8sClient(f)
}

type provisioner struct {
	catacomb          catacomb.Catacomb
	provisionerFacade CAASProvisionerFacade
	newCAASClientFunc func(f CAASProvisionerFacade) (CAASClient, error)

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
	client, err := p.newCAASClientFunc(p.provisionerFacade)
	if err != nil {
		return errors.Annotate(err, "creating k8s client")
	}

	newConfig := func(appName string) ([]byte, error) {
		return p.newOperatorConfig(appName)
	}

	// TODO(caas) -  this loop should also keep an eye on kubernetes and ensure
	// that the operator stays up, redeploying it if the pod goes
	// away. For some runtimes we *could* rely on the the runtime's
	// features to do this.

	// TODO(caas) - add watcher for cloud credential changes, create new client

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
				if err := client.EnsureOperator(app, p.agentConfig.DataDir(), newConfig); err != nil {
					return errors.Annotatef(err, "failed to start operator for %q", app)
				}
			}
		}
	}
}

func (p *provisioner) newOperatorConfig(appName string) ([]byte, error) {
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
	return confBytes, nil
}
