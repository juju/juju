// Copyright 2020 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package caasadmission

import (
	"github.com/juju/errors"
	"github.com/juju/worker/v3"
	"github.com/juju/worker/v3/dependency"
	admission "k8s.io/api/admissionregistration/v1"

	"github.com/juju/juju/agent"
	"github.com/juju/juju/apiserver/apiserverhttp"
	"github.com/juju/juju/caas/kubernetes/provider/constants"
	"github.com/juju/juju/internal/worker/caasrbacmapper"
	"github.com/juju/juju/internal/worker/muxhttpserver"
	"github.com/juju/juju/pki"
)

// K8sBroker describes a Kubernetes broker interface this worker needs to
// function.
type K8sBroker interface {
	// ModelName returns the model the broker is targeting
	ModelName() string

	// ModelUUID returns the model the broker is targeting
	ModelUUID() string

	// ControllerUUID returns the controller the broker is on
	ControllerUUID() string

	// Namespace returns the current namespace being targeted on the
	// broker
	Namespace() string

	// EnsureMutatingWebhookConfiguration make the supplied webhook config exist
	// inside the k8s cluster if it currently does not. Return values is a
	// cleanup function that will destroy the webhook configuration from k8s
	// when called and a subsequent error if there was a problem. If error is
	// not nil then no other return values should be considered valid.
	EnsureMutatingWebhookConfiguration(*admission.MutatingWebhookConfiguration) (func(), error)

	// LabelVersion reports if the k8s broker requires legacy labels to be
	// used for the broker model/namespace
	LabelVersion() constants.LabelVersion
}

// Logger represents the methods used by the worker to log details
type Logger interface {
	Debugf(string, ...interface{})
	Errorf(string, ...interface{})
	Infof(string, ...interface{})
}

// ManifoldConfig describes the resources used by the admission worker
type ManifoldConfig struct {
	AgentName        string
	AuthorityName    string
	Authority        pki.Authority
	BrokerName       string
	Logger           Logger
	MuxName          string
	RBACMapperName   string
	ServerInfoName   string
	ServiceName      string
	ServiceNamespace string
}

const (
	// DefaultModelOperatorPort
	DefaultModelOperatorPort = int32(17071)
)

// Manifold returns a Manifold that encapsulates a Kubernetes mutating admission
// controller. Manifold has no outputs.
func Manifold(config ManifoldConfig) dependency.Manifold {
	return dependency.Manifold{
		Inputs: []string{
			config.AgentName,
			config.AuthorityName,
			config.BrokerName,
			config.RBACMapperName,
			config.MuxName,
			config.ServerInfoName,
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

	var authority pki.Authority
	if err := context.Get(c.AuthorityName, &authority); err != nil {
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

	var mux *apiserverhttp.Mux
	if err := context.Get(c.MuxName, &mux); err != nil {
		return nil, errors.Trace(err)
	}

	var serverInfo muxhttpserver.ServerInfo
	if err := context.Get(c.ServerInfoName, &serverInfo); err != nil {
		return nil, errors.Trace(err)
	}

	port, err := serverInfo.PortInt()
	if err != nil {
		return nil, errors.Annotate(err, "fetching http server port as int")
	}

	svcPort := int32(port)
	currentConfig := agent.CurrentConfig()
	admissionPath := AdmissionPathForModel(currentConfig.Model().Id())
	admissionCreator, err := NewAdmissionCreator(authority,
		broker.Namespace(), broker.ModelName(),
		broker.ModelUUID(), broker.ControllerUUID(),
		broker.LabelVersion(),
		broker.EnsureMutatingWebhookConfiguration,
		&admission.ServiceReference{
			Name:      c.ServiceName,
			Namespace: c.ServiceNamespace,
			Path:      &admissionPath,
			Port:      &svcPort,
		},
	)
	if err != nil {
		return nil, errors.Trace(err)
	}

	return NewController(
		c.Logger,
		mux,
		AdmissionPathForModel(currentConfig.Model().Id()),
		broker.LabelVersion(),
		admissionCreator,
		rbacMapper,
		currentConfig.Controller().Id(),
		broker.ModelUUID(),
		broker.ModelName(),
	)
}

// Validate is used to to establish if the configuration is valid for use when
// creating new workers.
func (c ManifoldConfig) Validate() error {
	if c.AgentName == "" {
		return errors.NotValidf("empty AgentName")
	}
	if c.AuthorityName == "" {
		return errors.NotValidf("empty AuthorityName ")
	}
	if c.BrokerName == "" {
		return errors.NotValidf("empty BrokerName")
	}
	if c.Logger == nil {
		return errors.NotValidf("nil Logger")
	}
	if c.MuxName == "" {
		return errors.NotValidf("empty MuxName")
	}
	if c.RBACMapperName == "" {
		return errors.NotValidf("empty RBACMapperName")
	}
	if c.ServerInfoName == "" {
		return errors.NotValidf("empty ServerInfoName")
	}
	if c.ServiceName == "" {
		return errors.NotValidf("empty ServiceName")
	}
	if c.ServiceNamespace == "" {
		return errors.NotValidf("empty ServiceNamespace")
	}
	return nil
}
