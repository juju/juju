// Copyright 2020 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package caasmodeloperator

import (
	"context"

	"github.com/juju/errors"
	"github.com/juju/names/v6"
	"github.com/juju/worker/v5"
	"github.com/juju/worker/v5/dependency"

	"github.com/juju/juju/caas"
	"github.com/juju/juju/core/logger"
	tracingservice "github.com/juju/juju/domain/tracing/service"
	"github.com/juju/juju/internal/services"
)

// ConfigProvider provides dynamic values that can change during a
// worker's lifetime. The provider re-reads the backing config on
// each method call so that bounced workers observe current values.
// Static values that never change (DataDir, LogDir, ControllerTag)
// are passed as direct ManifoldConfig fields, not through the
// provider.
type ConfigProvider interface {
	CACert() (string, error)
}

// TracingService provides access to workload tracing configuration.
type TracingService interface {
	// GetWorkloadTracingConfig returns the current workload tracing
	// configuration. The worker uses it when building the model
	// operator agent config so that tracing settings are applied to
	// the running operator.
	GetWorkloadTracingConfig(ctx context.Context) (tracingservice.WorkloadTracingConfig, error)
}

// ManifoldConfig describes the resources used by the CAASModelOperatorWorker
type ManifoldConfig struct {
	// BrokerName is the name of the broker dependency to fetch
	BrokerName string
	// DomainServicesName is the name of the model domain services dependency.
	DomainServicesName string
	// ConfigProvider provides dynamic values that may change during a
	// worker's lifetime, such as CACert.
	ConfigProvider ConfigProvider
	// Logger to use in this worker
	Logger logger.Logger
	// ModelUUID is the id of the model this worker is operating on
	ModelUUID string
	// DataDir is the directory for agent data
	DataDir string
	// LogDir is the directory for agent logs
	LogDir string
	// ControllerTag identifies the controller
	ControllerTag names.ControllerTag
}

// Manifold returns a Manifold that encapsulates a Kubernetes model operator.
func Manifold(config ManifoldConfig) dependency.Manifold {
	return dependency.Manifold{
		Inputs: []string{
			config.BrokerName,
			config.DomainServicesName,
		},
		Output: nil,
		Start:  config.Start,
	}
}

// Start is used to start the manifold an extract a worker from the supplied
// configuration
func (m ManifoldConfig) Start(context context.Context, getter dependency.Getter) (worker.Worker, error) {
	if err := m.Validate(); err != nil {
		return nil, errors.Trace(err)
	}

	var broker caas.Broker
	if err := getter.Get(m.BrokerName, &broker); err != nil {
		return nil, errors.Trace(err)
	}

	var domainServices services.DomainServices
	if err := getter.Get(m.DomainServicesName, &domainServices); err != nil {
		return nil, errors.Trace(err)
	}

	api := &modelOperatorAPIAdapter{
		ctrlConfigSvc:  domainServices.ControllerConfig(),
		modelConfigSvc: domainServices.Config(),
		ctrlNodeSvc:    domainServices.ControllerNode(),
		ctrlSvc:        domainServices.Controller(),
		agentPwdSvc:    domainServices.AgentPassword(),
	}

	return NewModelOperatorManager(
		m.Logger,
		api,
		broker,
		m.ModelUUID,
		m.DataDir,
		m.LogDir,
		m.ControllerTag,
		m.ConfigProvider,
		domainServices.Tracing(),
	)
}

// Validate checks all the config fields are valid for the Manifold to start
func (m ManifoldConfig) Validate() error {
	if m.BrokerName == "" {
		return errors.NotValidf("empty BrokerName")
	}
	if m.DomainServicesName == "" {
		return errors.NotValidf("empty DomainServicesName")
	}
	if m.ConfigProvider == nil {
		return errors.NotValidf("nil ConfigProvider")
	}
	if m.Logger == nil {
		return errors.NotValidf("nil Logger")
	}
	if m.ModelUUID == "" {
		return errors.NotValidf("empty ModelUUID")
	}
	if m.DataDir == "" {
		return errors.NotValidf("empty DataDir")
	}
	if m.LogDir == "" {
		return errors.NotValidf("empty LogDir")
	}
	if m.ControllerTag.Id() == "" {
		return errors.NotValidf("empty ControllerTag")
	}
	return nil
}
