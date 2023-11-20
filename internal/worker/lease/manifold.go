// Copyright 2018 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package lease

import (
	stdcontext "context"
	"time"

	"github.com/juju/clock"
	"github.com/juju/errors"
	"github.com/juju/worker/v3"
	"github.com/juju/worker/v3/dependency"
	"github.com/prometheus/client_golang/prometheus"

	"github.com/juju/juju/agent"
	"github.com/juju/juju/core/database"
	"github.com/juju/juju/core/lease"
	coretrace "github.com/juju/juju/core/trace"
	"github.com/juju/juju/domain/lease/service"
	"github.com/juju/juju/domain/lease/state"
	"github.com/juju/juju/internal/worker/common"
	"github.com/juju/juju/internal/worker/trace"
)

const (
	// MaxSleep is the longest the manager will sleep before checking
	// whether any leases should be expired. If it can see a lease
	// expiring sooner than that it will still wake up earlier.
	MaxSleep = time.Minute
)

// ManifoldConfig holds the resources needed to start the lease
// manager in a dependency engine.
type ManifoldConfig struct {
	AgentName      string
	ClockName      string
	DBAccessorName string
	TraceName      string

	Logger               Logger
	LogDir               string
	PrometheusRegisterer prometheus.Registerer
	NewWorker            func(ManagerConfig) (worker.Worker, error)
	NewStore             func(database.DBGetter, Logger) lease.Store
}

// Validate checks that the config has all the required values.
func (c ManifoldConfig) Validate() error {
	if c.AgentName == "" {
		return errors.NotValidf("empty AgentName")
	}
	if c.ClockName == "" {
		return errors.NotValidf("empty ClockName")
	}
	if c.DBAccessorName == "" {
		return errors.NotValidf("empty DBAccessor")
	}
	if c.TraceName == "" {
		return errors.NotValidf("empty TraceName")
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

	var dbGetter database.DBGetter
	if err := context.Get(s.config.DBAccessorName, &dbGetter); err != nil {
		return nil, errors.Trace(err)
	}

	var tracerGetter trace.TracerGetter
	if err := context.Get(s.config.TraceName, &tracerGetter); err != nil {
		return nil, errors.Trace(err)
	}

	currentConfig := agent.CurrentConfig()

	tracer, err := tracerGetter.GetTracer(stdcontext.TODO(), coretrace.Namespace("leaseexpiry", currentConfig.Model().Id()))
	if err != nil {
		tracer = coretrace.NoopTracer{}
	}

	store := s.config.NewStore(dbGetter, s.config.Logger)

	controllerUUID := currentConfig.Controller().Id()
	w, err := s.config.NewWorker(ManagerConfig{
		Secretary:            SecretaryFinder(controllerUUID),
		Store:                store,
		Tracer:               tracer,
		Clock:                clock,
		Logger:               s.config.Logger,
		MaxSleep:             MaxSleep,
		EntityUUID:           controllerUUID,
		LogDir:               s.config.LogDir,
		PrometheusRegisterer: s.config.PrometheusRegisterer,
	})
	return w, errors.Trace(err)
}

func (s *manifoldState) output(in worker.Worker, out interface{}) error {
	if w, ok := in.(*common.CleanupWorker); ok {
		in = w.Worker
	}
	manager, ok := in.(*Manager)
	if !ok {
		return errors.Errorf("expected input of type *worker/Manager, got %T", in)
	}
	switch out := out.(type) {
	case *lease.Manager:
		*out = manager
		return nil
	default:
		return errors.Errorf("expected output of type *core/Manager, got %T", out)
	}
}

// Manifold builds a dependency.Manifold for running a lease manager.
func Manifold(config ManifoldConfig) dependency.Manifold {
	s := manifoldState{config: config}
	return dependency.Manifold{
		Inputs: []string{
			config.AgentName,
			config.ClockName,
			config.DBAccessorName,
			config.TraceName,
		},
		Start:  s.start,
		Output: s.output,
	}
}

// NewWorker wraps NewManager to return worker.Worker for testability.
func NewWorker(config ManagerConfig) (worker.Worker, error) {
	return NewManager(config)
}

// NewStore returns a new lease store based on the input config.
func NewStore(dbGetter database.DBGetter, logger Logger) lease.Store {
	factory := database.NewTxnRunnerFactoryForNamespace(dbGetter.GetDB, database.ControllerNS)
	return service.NewService(state.NewState(factory, logger))
}
