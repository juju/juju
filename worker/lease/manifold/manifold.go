// Copyright 2018 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package manifold

// This needs to be in a different package from the lease manager
// because it uses state (to construct the raftlease store), but the
// lease manager also runs as a worker in state, so the state package
// depends on worker/lease. Having it in worker/lease produces an
// import cycle.

import (
	"fmt"
	"math/rand"
	"time"

	"github.com/juju/clock"
	"github.com/juju/errors"
	"github.com/juju/pubsub"
	"github.com/juju/worker/v2"
	"github.com/juju/worker/v2/dependency"
	"github.com/prometheus/client_golang/prometheus"

	"github.com/juju/juju/agent"
	"github.com/juju/juju/core/globalclock"
	corelease "github.com/juju/juju/core/lease"
	"github.com/juju/juju/core/raftlease"
	"github.com/juju/juju/worker/common"
	"github.com/juju/juju/worker/lease"
	workerstate "github.com/juju/juju/worker/state"
)

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

	source := rand.NewSource(clock.Now().UnixNano())
	runID := rand.New(source).Int31()

	s.store = s.config.NewStore(raftlease.StoreConfig{
		FSM:          s.config.FSM,
		Hub:          hub,
		Trapdoor:     st.LeaseTrapdoorFunc(),
		RequestTopic: s.config.RequestTopic,
		ResponseTopic: func(requestID uint64) string {
			return fmt.Sprintf("%s.%08x.%d", s.config.RequestTopic, runID, requestID)
		},
		Clock:          clock,
		ForwardTimeout: ForwardTimeout,
	})

	controllerUUID := agent.CurrentConfig().Controller().Id()
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
		stTracker.Done()
		return nil, errors.Trace(err)
	}
	return common.NewCleanupWorker(w, func() { stTracker.Done() }), nil
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
	case *globalclock.Updater:
		*out = s.store
		return nil
	case *corelease.Manager:
		*out = manager
		return nil
	default:
		return errors.Errorf("expected output of type *globalclock.Updater or *core/lease.Manager, got %T", out)
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
