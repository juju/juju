// Copyright 2020 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package caasadmission

import (
	"fmt"

	"github.com/juju/errors"
	"github.com/juju/worker/v2"
	"github.com/juju/worker/v2/dependency"
	admission "k8s.io/api/admissionregistration/v1beta1"

	"github.com/juju/juju/agent"
	"github.com/juju/juju/api/base"
	admissionapi "github.com/juju/juju/api/caasadmission"
	"github.com/juju/juju/apiserver/apiserverhttp"
	"github.com/juju/juju/pki"
	"github.com/juju/juju/worker/caasrbacmapper"
)

// K8sBroker describes a Kubernetes broker interface this worker needs to
// function.
type K8sBroker interface {
	// CurrentModel returns the current model the broker is targeting
	CurrentModel() string

	// GetCurrentNamespace returns the current namespace being targeted on the
	// broker
	GetCurrentNamespace() string

	// EnsureMutatingWebhookConfiguration make the supplied webhook config exist
	// inside the k8s cluster if it currently does not. Return values is a
	// cleanup function that will destroy the webhook configuration from k8s
	// when called and a subsequent error if there was a problem. If error is
	// not nil then no other return values should be considered valid.
	EnsureMutatingWebhookConfiguration(*admission.MutatingWebhookConfiguration) (func(), error)
}

// Logger represents the methods used by the worker to log details
type Logger interface {
	Debugf(string, ...interface{})
	Errorf(string, ...interface{})
	Infof(string, ...interface{})
}

// ManifoldConfig describes the resources used by the admission worker
type ManifoldConfig struct {
	AgentName      string
	APICallerName  string
	Authority      pki.Authority
	BrokerName     string
	Logger         Logger
	Mux            *apiserverhttp.Mux
	RBACMapperName string
}

// Manifold returns a Manifold that encapsulates a Kubernetes mutating admission
// controller. Manifold has no outputs.
func Manifold(config ManifoldConfig) dependency.Manifold {
	return dependency.Manifold{
		Inputs: []string{
			config.AgentName,
			config.APICallerName,
			config.BrokerName,
			config.RBACMapperName,
		},
		Output: nil,
		Start:  config.Start,
	}
}

// Start is used to start the manifold an extract a worker from the supplied
// configuration.
func (c ManifoldConfig) Start(context dependency.Context) (worker.Worker, error) {
	if err := c.Validate(); err != nil {
		return nil, errors.Trace(err)
	}

	var agent agent.Agent
	if err := context.Get(c.AgentName, &agent); err != nil {
		return nil, errors.Trace(err)
	}

	var apiCaller base.APICaller
	if err := context.Get(c.APICallerName, &apiCaller); err != nil {
		return nil, errors.Trace(err)
	}

	api, err := admissionapi.NewClient(apiCaller)
	if err != nil {
		return nil, errors.Trace(err)
	}

	var broker K8sBroker
	if err := context.Get(c.BrokerName, &broker); err != nil {
		return nil, errors.Trace(err)
	}

	var rbacMapper caasrbacmapper.Mapper
	if err := context.Get(c.RBACMapperName, &rbacMapper); err != nil {
		return nil, errors.Trace(err)
	}

	ctrlCfg, err := api.ControllerConfig()
	if err != nil {
		return nil, errors.Annotate(err, "fetching controller configuration for name")
	}

	currentConfig := agent.CurrentConfig()
	admissionPath := AdmissionPathForModel(currentConfig.Model().Id())
	port := int32(17070)
	admissionCreator, err := NewAdmissionCreator(c.Authority,
		broker.GetCurrentNamespace(), broker.CurrentModel(),
		broker.EnsureMutatingWebhookConfiguration,
		&admission.ServiceReference{
			Name:      "controller-service",
			Namespace: fmt.Sprintf("controller-%s", ctrlCfg.ControllerName()),
			Path:      &admissionPath,
			Port:      &port,
		},
	)
	if err != nil {
		return nil, errors.Trace(err)
	}

	return NewController(
		c.Logger,
		c.Mux,
		AdmissionPathForModel(currentConfig.Model().Id()),
		admissionCreator,
		rbacMapper)
}

// Validate is used to to establish if the configuration is valid for use when
// creating new workers.
func (c ManifoldConfig) Validate() error {
	if c.AgentName == "" {
		return errors.NotValidf("empty AgentName")
	}
	if c.APICallerName == "" {
		return errors.NotValidf("empty APICallerName ")
	}
	if c.Authority == nil {
		return errors.NotValidf("nil Authority")
	}
	if c.BrokerName == "" {
		return errors.NotValidf("empty BrokerName")
	}
	if c.Logger == nil {
		return errors.NotValidf("nil Logger")
	}
	if c.Mux == nil {
		return errors.NotValidf("nil apiserverhttp.Mux reference")
	}
	if c.RBACMapperName == "" {
		return errors.NotValidf("empty RBACMapperName")
	}
	return nil
}
