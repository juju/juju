// Copyright 2020 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package caasmodeloperator

import (
	"github.com/juju/errors"
	"github.com/juju/names/v4"
	"github.com/juju/utils/v2"
	"github.com/juju/version/v2"
	"github.com/juju/worker/v2/catacomb"

	"github.com/juju/juju/agent"
	"github.com/juju/juju/api/caasmodeloperator"
	"github.com/juju/juju/caas"
)

type ModelOperatorAPI interface {
	SetPassword(password string) error
	ModelOperatorProvisioningInfo() (caasmodeloperator.ModelOperatorProvisioningInfo, error)
}

// ModelOperatorBroker describes the caas broker interface needed for installing
// a ModelOperator into Kubernetes
type ModelOperatorBroker interface {
	EnsureModelOperator(string, string, *caas.ModelOperatorConfig) error
	ModelOperator() (*caas.ModelOperatorConfig, error)
	ModelOperatorExists() (bool, error)
}

// ModelOperatorManager defines the worker used for managing model operators in
// caas
type ModelOperatorManager struct {
	agentConfig agent.Config
	api         ModelOperatorAPI
	broker      ModelOperatorBroker
	catacomb    catacomb.Catacomb
	logger      Logger
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
	m.logger.Debugf("gathering model operator provisioning information for model %s", m.modelUUID)
	info, err := m.api.ModelOperatorProvisioningInfo()
	if err != nil {
		return errors.Trace(err)
	}

	exists, err := m.broker.ModelOperatorExists()
	if err != nil {
		return errors.Trace(err)
	}

	var agentConfBuf []byte
	if !exists {
		password, err := utils.RandomPassword()
		if err != nil {
			return errors.Trace(err)
		}
		err = m.api.SetPassword(password)
		if err != nil {
			return errors.Annotate(err, "failed to set model api passwords")
		}

		agentConf, err := m.updateAgentConf(info.APIAddresses, password, info.Version)
		if err != nil {
			return errors.Trace(err)
		}

		agentConfBuf, err = agentConf.Render()
		if err != nil {
			return errors.Trace(err)
		}
	} else {
		mo, err := m.broker.ModelOperator()
		if err != nil {
			return errors.Trace(err)
		}
		agentConfBuf = mo.AgentConf
	}

	m.logger.Debugf("ensuring model operator deployment in kubernetes for model %s", m.modelUUID)
	err = m.broker.EnsureModelOperator(
		m.modelUUID,
		m.agentConfig.DataDir(),
		&caas.ModelOperatorConfig{
			AgentConf:         agentConfBuf,
			OperatorImagePath: info.ImagePath,
			Port:              DefaultModelOperatorPort,
		},
	)

	if err != nil {
		return errors.Annotate(err, "deploying model operator")
	}

	select {
	case <-m.catacomb.Dying():
		return m.catacomb.ErrDying()
	}
}

// NewModelOperatorManager constructs a new model operator manager worker
func NewModelOperatorManager(
	logger Logger,
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
