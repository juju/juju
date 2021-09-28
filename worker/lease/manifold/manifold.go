// Copyright 2018 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package manifold

// This needs to be in a different package from the lease manager
// because it uses state (to construct the raftlease store), but the
// lease manager also runs as a worker in state, so the state package
// depends on worker/lease. Having it in worker/lease produces an
// import cycle.

import (
	"math/rand"
	"time"

	"github.com/juju/clock"
	"github.com/juju/errors"
	"github.com/juju/pubsub/v2"
	"github.com/juju/worker/v2"
	"github.com/juju/worker/v2/dependency"
	"github.com/prometheus/client_golang/prometheus"

	"github.com/juju/juju/agent"
	"github.com/juju/juju/api"
	apiraftlease "github.com/juju/juju/api/raftlease"
	corelease "github.com/juju/juju/core/lease"
	"github.com/juju/juju/core/raftlease"
	"github.com/juju/juju/feature"
	"github.com/juju/juju/worker/common"
	"github.com/juju/juju/worker/lease"
	workerstate "github.com/juju/juju/worker/state"
)

type Logger interface {
	Errorf(string, ...interface{})
	Infof(string, ...interface{})
	Debugf(string, ...interface{})
	Tracef(string, ...interface{})
}

const (
	// MaxSleep is the longest the manager will sleep before checking
	// whether any leases should be expired. If it can see a lease
	// expiring sooner than that it will still wake up earlier.
	MaxSleep = time.Minute

	// ForwardTimeout is how long the store should wait for a response
	// after sending a lease operation over the hub before deciding a
	// a response is never coming back (for example if we send the
	// request during a raft-leadership election). This should be long
	// enough that we can be very confident the request was missed.
	ForwardTimeout = 5 * time.Second
)

// TODO(raftlease): This manifold does too much - split out a worker
// that holds the lease store and a manifold that creates it. Then
// make this one depend on that.

// ManifoldConfig holds the resources needed to start the lease
// manager in a dependency engine.
type ManifoldConfig struct {
	AgentName      string
	ClockName      string
	CentralHubName string
	StateName      string

	FSM                  *raftlease.FSM
	RequestTopic         string
	Logger               lease.Logger
	LogDir               string
	PrometheusRegisterer prometheus.Registerer
	NewWorker            func(lease.ManagerConfig) (worker.Worker, error)
	NewStore             func(raftlease.StoreConfig) *raftlease.Store
	NewClient            ClientFunc
}

// Validate checks that the config has all the required values.
func (c ManifoldConfig) Validate() error {
	if c.AgentName == "" {
		return errors.NotValidf("empty AgentName")
	}
	if c.ClockName == "" {
		return errors.NotValidf("empty ClockName")
	}
	if c.CentralHubName == "" {
		return errors.NotValidf("empty CentralHubName")
	}
	if c.StateName == "" {
		return errors.NotValidf("empty StateName")
	}
	if c.FSM == nil {
		return errors.NotValidf("nil FSM")
	}
	if c.RequestTopic == "" {
		return errors.NotValidf("empty RequestTopic")
	}
	if c.Logger == nil {
		return errors.NotValidf("nil Logger")
	}
	if c.PrometheusRegisterer == nil {
		return errors.NotValidf("nil PrometheusRegisterer")
	}
	if c.NewWorker == nil {
		return errors.NotValidf("nil NewWorker")
	}
	if c.NewStore == nil {
		return errors.NotValidf("nil NewStore")
	}
	if c.NewClient == nil {
		return errors.NotValidf("nil NewClient")
	}
	return nil
}

type manifoldState struct {
	config ManifoldConfig
	store  *raftlease.Store
}

func (s *manifoldState) start(context dependency.Context) (worker.Worker, error) {
	if err := s.config.Validate(); err != nil {
		return nil, errors.Trace(err)
	}

	var agent agent.Agent
	if err := context.Get(s.config.AgentName, &agent); err != nil {
		return nil, errors.Trace(err)
	}

	var clock clock.Clock
	if err := context.Get(s.config.ClockName, &clock); err != nil {
		return nil, errors.Trace(err)
	}

	var hub *pubsub.StructuredHub
	if err := context.Get(s.config.CentralHubName, &hub); err != nil {
		return nil, errors.Trace(err)
	}

	var stTracker workerstate.StateTracker
	if err := context.Get(s.config.StateName, &stTracker); err != nil {
		return nil, errors.Trace(err)
	}
	statePool, err := stTracker.Use()
	if err != nil {
		return nil, errors.Trace(err)
	}

	st := statePool.SystemState()

	// We require the controller config to get the
	controllerConfig, err := st.ControllerConfig()
	if err != nil {
		return nil, errors.Annotate(err, "cannot fetch the controller config")
	}

	currentConfig := agent.CurrentConfig()
	apiInfo, ok := currentConfig.APIInfo()
	if !ok {
		return nil, dependency.ErrMissing
	}

	clientType := PubsubClientType
	if controllerConfig.Features().Contains(feature.RaftAPILeases) {
		clientType = APIClientType
	}

	metrics := raftlease.NewOperationClientMetrics(clock)
	client, err := s.config.NewClient(
		clientType,
		apiInfo,
		hub,
		s.config.RequestTopic,
		clock,
		metrics,
		s.config.Logger,
	)
	if err != nil {
		return nil, errors.Trace(err)
	}

	s.store = s.config.NewStore(raftlease.StoreConfig{
		FSM:              s.config.FSM,
		Trapdoor:         st.LeaseTrapdoorFunc(),
		Client:           client,
		Clock:            clock,
		MetricsCollector: metrics,
	})

	controllerUUID := currentConfig.Controller().Id()
	w, err := s.config.NewWorker(lease.ManagerConfig{
		Secretary:            lease.SecretaryFinder(controllerUUID),
		Store:                s.store,
		Clock:                clock,
		Logger:               s.config.Logger,
		MaxSleep:             MaxSleep,
		EntityUUID:           controllerUUID,
		LogDir:               s.config.LogDir,
		PrometheusRegisterer: s.config.PrometheusRegisterer,
	})
	if err != nil {
		_ = stTracker.Done()
		return nil, errors.Trace(err)
	}
	return common.NewCleanupWorker(w, func() { _ = stTracker.Done() }), nil
}

func (s *manifoldState) output(in worker.Worker, out interface{}) error {
	if w, ok := in.(*common.CleanupWorker); ok {
		in = w.Worker
	}
	manager, ok := in.(*lease.Manager)
	if !ok {
		return errors.Errorf("expected input of type *worker/lease.Manager, got %T", in)
	}
	switch out := out.(type) {
	case *corelease.Manager:
		*out = manager
		return nil
	default:
		return errors.Errorf("expected output of type *core/lease.Manager, got %T", out)
	}
}

// Manifold builds a dependency.Manifold for running a lease manager.
func Manifold(config ManifoldConfig) dependency.Manifold {
	s := manifoldState{config: config}
	return dependency.Manifold{
		Inputs: []string{
			config.AgentName,
			config.ClockName,
			config.CentralHubName,
			config.StateName,
		},
		Start:  s.start,
		Output: s.output,
	}
}

// NewWorker wraps NewManager to return worker.Worker for testability.
func NewWorker(config lease.ManagerConfig) (worker.Worker, error) {
	return lease.NewManager(config)
}

// NewStore is a shim to make a raftlease.Store for testability.
func NewStore(config raftlease.StoreConfig) *raftlease.Store {
	return raftlease.NewStore(config)
}

// ClientType defines the type of client we want to create.
type ClientType string

const (
	// PubsubClientType will request a pubsub client.
	PubsubClientType ClientType = "pubsub"
	// APIClientType will request a API client.
	APIClientType ClientType = "api-client"
)

// ClientFunc only exists until we can just one of the clients. Until then
// we have to create this type.
// TODO (stickupkid): Remove this once API Client type is battle tested and
// we've deprecated pubsub client.
type ClientFunc = func(ClientType, *api.Info, *pubsub.StructuredHub, string, clock.Clock, *raftlease.OperationClientMetrics, Logger) (raftlease.Client, error)

// NewClientFunc returns a client depending on the type of feature flag
// enablement.
func NewClientFunc(clientType ClientType, apiInfo *api.Info,
	hub *pubsub.StructuredHub,
	requestTopic string,
	clock clock.Clock,
	metrics *raftlease.OperationClientMetrics,
	logger Logger) (raftlease.Client, error) {

	logger.Infof("Using lease client type %q for raft lease transport", clientType)

	switch clientType {
	case PubsubClientType:
		return raftlease.NewPubsubClient(raftlease.PubsubClientConfig{
			Hub:            hub,
			RequestTopic:   requestTopic,
			Clock:          clock,
			ForwardTimeout: ForwardTimeout,
			ClientMetrics:  metrics,
		}), nil
	case APIClientType:
		return apiraftlease.NewClient(apiraftlease.Config{
			APIInfo:        apiInfo,
			Hub:            hub,
			ForwardTimeout: ForwardTimeout,
			ClientMetrics:  metrics,
			Logger:         logger,
			NewRemote:      apiraftlease.NewRemote,
			Random:         rand.New(rand.NewSource(clock.Now().UnixNano())),
			Clock:          clock,
		})
	default:
		return nil, errors.Errorf("unknown client type: %v", clientType)
	}
}
