// Copyright 2020 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package caasmodeloperator

import (
	"context"

	"github.com/juju/errors"
	"github.com/juju/names/v6"
	"github.com/juju/worker/v4/catacomb"

	"github.com/juju/juju/agent"
	"github.com/juju/juju/api/controller/caasmodeloperator"
	"github.com/juju/juju/caas"
	"github.com/juju/juju/core/logger"
	"github.com/juju/juju/core/watcher"
	"github.com/juju/juju/internal/password"
	"github.com/juju/juju/internal/version"
)

type ModelOperatorAPI interface {
	SetPassword(ctx context.Context, password string) error
	ModelOperatorProvisioningInfo(context.Context) (caasmodeloperator.ModelOperatorProvisioningInfo, error)
	WatchModelOperatorProvisioningInfo(context.Context) (watcher.NotifyWatcher, error)
}

// ModelOperatorBroker describes the caas broker interface needed for installing
// a ModelOperator into Kubernetes
type ModelOperatorBroker interface {
	EnsureModelOperator(context.Context, string, string, *caas.ModelOperatorConfig) error
	ModelOperator(ctx context.Context) (*caas.ModelOperatorConfig, error)
	ModelOperatorExists(ctx context.Context) (bool, error)
}

// ModelOperatorManager defines the worker used for managing model operators in
// caas
type ModelOperatorManager struct {
	agentConfig agent.Config
	api         ModelOperatorAPI
	broker      ModelOperatorBroker
	catacomb    catacomb.Catacomb
	logger      logger.Logger
	modelUUID   string
}

const (
	// DefaultModelOperatorPort is the default port used for the api server on
	// the model operator
	DefaultModelOperatorPort = 17071
)

// Kill implements worker kill method
func (m *ModelOperatorManager) Kill() {
	m.catacomb.Kill(nil)
}

// Wait implements worker Wait method
func (m *ModelOperatorManager) Wait() error {
	return m.catacomb.Wait()
}

func (m *ModelOperatorManager) loop() error {
	ctx, cancel := m.scopedContext()
	defer cancel()

	watcher, err := m.api.WatchModelOperatorProvisioningInfo(ctx)
	if err != nil {
		return errors.Annotate(err, "cannot watch model operator provisioning info")
	}
	err = m.catacomb.Add(watcher)
	if err != nil {
		return errors.Trace(err)
	}

	for {
		select {
		case <-m.catacomb.Dying():
			return m.catacomb.ErrDying()
		case <-watcher.Changes():
			err := m.update(ctx)
			if err != nil {
				return errors.Annotate(err, "failed to update model operator")
			}
		}
	}
}

func (m *ModelOperatorManager) scopedContext() (context.Context, context.CancelFunc) {
	ctx, cancel := context.WithCancel(context.Background())
	return m.catacomb.Context(ctx), cancel
}

func (m *ModelOperatorManager) update(ctx context.Context) error {
	m.logger.Debugf(ctx, "gathering model operator provisioning information for model %s", m.modelUUID)
	info, err := m.api.ModelOperatorProvisioningInfo(ctx)
	if err != nil {
		return errors.Trace(err)
	}

	exists, err := m.broker.ModelOperatorExists(ctx)
	if err != nil {
		return errors.Trace(err)
	}

	setPassword := true
	password, err := password.RandomPassword()
	if err != nil {
		return errors.Trace(err)
	}
	if exists {
		mo, err := m.broker.ModelOperator(ctx)
		if err != nil {
			return errors.Trace(err)
		}
		prevConf, err := agent.ParseConfigData(mo.AgentConf)
		if err != nil {
			return errors.Annotate(err, "cannot parse model operator agent config: ")
		}
		// reuse old password
		if prevInfo, ok := prevConf.APIInfo(); ok && prevInfo.Password != "" {
			password = prevInfo.Password
			setPassword = false
		} else if prevConf.OldPassword() != "" {
			password = prevConf.OldPassword()
			setPassword = false
		}
	}
	if setPassword {
		err := m.api.SetPassword(ctx, password)
		if err != nil {
			return errors.Annotate(err, "failed to set model api passwords")
		}
	}

	agentConf, err := m.updateAgentConf(info.APIAddresses, password, info.Version)
	if err != nil {
		return errors.Trace(err)
	}
	agentConfBuf, err := agentConf.Render()
	if err != nil {
		return errors.Trace(err)
	}

	m.logger.Debugf(ctx, "ensuring model operator deployment in kubernetes for model %s", m.modelUUID)
	err = m.broker.EnsureModelOperator(
		ctx,
		m.modelUUID,
		m.agentConfig.DataDir(),
		&caas.ModelOperatorConfig{
			AgentConf:    agentConfBuf,
			ImageDetails: info.ImageDetails,
			Port:         DefaultModelOperatorPort,
		},
	)
	if err != nil {
		return errors.Annotate(err, "deploying model operator")
	}

	return nil
}

// NewModelOperatorManager constructs a new model operator manager worker
func NewModelOperatorManager(
	logger logger.Logger,
	api ModelOperatorAPI,
	broker ModelOperatorBroker,
	modelUUID string,
	agentConfig agent.Config,
) (*ModelOperatorManager, error) {
	m := &ModelOperatorManager{
		agentConfig: agentConfig,
		api:         api,
		broker:      broker,
		logger:      logger,
		modelUUID:   modelUUID,
	}

	if err := catacomb.Invoke(catacomb.Plan{
		Site: &m.catacomb,
		Work: m.loop,
	}); err != nil {
		return m, errors.Trace(err)
	}

	return m, nil
}

func (m *ModelOperatorManager) updateAgentConf(
	apiAddresses []string,
	password string,
	ver version.Number,
) (agent.ConfigSetterWriter, error) {
	modelTag := names.NewModelTag(m.modelUUID)
	conf, err := agent.NewAgentConfig(
		agent.AgentConfigParams{
			Paths: agent.Paths{
				DataDir: m.agentConfig.DataDir(),
				LogDir:  m.agentConfig.LogDir(),
			},
			Tag:          modelTag,
			Controller:   m.agentConfig.Controller(),
			Model:        modelTag,
			APIAddresses: apiAddresses,
			CACert:       m.agentConfig.CACert(),
			Password:     password,

			// UpgradedToVersion is mandatory but not used by
			// caas operator agents as they are not upgraded insitu.
			UpgradedToVersion: ver,
		},
	)
	if err != nil {
		return nil, errors.Annotatef(err, "creating new agent config")
	}

	return conf, nil
}
