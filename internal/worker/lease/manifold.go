// Copyright 2018 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package lease

import (
	"context"
	"time"

	"github.com/juju/clock"
	"github.com/juju/errors"
	"github.com/juju/worker/v5"
	"github.com/juju/worker/v5/dependency"
	"github.com/prometheus/client_golang/prometheus"

	"github.com/juju/juju/core/database"
	"github.com/juju/juju/core/lease"
	"github.com/juju/juju/core/logger"
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
	DBAccessorName string
	TraceName      string

	// ControllerUUID is the controller entity UUID used by the
	// secretary finder and passed as the lease entity UUID.
	ControllerUUID string

	// ControllerModelUUID is the controller model UUID used to build
	// the trace namespace for the lease manager.
	ControllerModelUUID string

	Clock                clock.Clock
	Logger               logger.Logger
	LogDir               string
	PrometheusRegisterer prometheus.Registerer
	NewWorker            func(ManagerConfig) (worker.Worker, error)
	NewStore             func(database.DBGetter, logger.Logger) lease.Store
	NewSecretaryFinder   func(string) lease.SecretaryFinder
}

// Validate checks that the config has all the required values.
func (c ManifoldConfig) Validate() error {
	if c.ControllerUUID == "" {
		return errors.NotValidf("empty ControllerUUID")
	}
	if c.ControllerModelUUID == "" {
		return errors.NotValidf("empty ControllerModelUUID")
	}
	if c.DBAccessorName == "" {
		return errors.NotValidf("empty DBAccessor")
	}
	if c.TraceName == "" {
		return errors.NotValidf("empty TraceName")
	}
	if c.Clock == nil {
		return errors.NotValidf("nil Clock")
	}
	if c.Logger == nil {
		return errors.NotValidf("nil Logger")
	}
	if c.LogDir == "" {
		return errors.NotValidf("empty LogDir")
	}
	if c.NewSecretaryFinder == nil {
		return errors.NotValidf("nil NewSecretaryFinder")
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

func (s *manifoldState) start(ctx context.Context, getter dependency.Getter) (worker.Worker, error) {
	if err := s.config.Validate(); err != nil {
		return nil, errors.Trace(err)
	}

	var dbGetter database.DBGetter
	if err := getter.Get(s.config.DBAccessorName, &dbGetter); err != nil {
		return nil, errors.Trace(err)
	}

	var tracerGetter trace.TracerGetter
	if err := getter.Get(s.config.TraceName, &tracerGetter); err != nil {
		return nil, errors.Trace(err)
	}

	tracer, err := tracerGetter.GetTracer(ctx, coretrace.Namespace("leaseexpiry", s.config.ControllerModelUUID))
	if err != nil {
		tracer = coretrace.NoopTracer{}
	}

	store := s.config.NewStore(dbGetter, s.config.Logger)

	controllerUUID := s.config.ControllerUUID
	w, err := s.config.NewWorker(ManagerConfig{
		SecretaryFinder:      s.config.NewSecretaryFinder(controllerUUID),
		Store:                store,
		Tracer:               tracer,
		Clock:                s.config.Clock,
		Logger:               s.config.Logger,
		MaxSleep:             MaxSleep,
		EntityUUID:           controllerUUID,
		LogDir:               s.config.LogDir,
		PrometheusRegisterer: s.config.PrometheusRegisterer,
	})
	return w, errors.Trace(err)
}

func (s *manifoldState) output(in worker.Worker, out any) error {
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
func NewStore(dbGetter database.DBGetter, logger logger.Logger) lease.Store {
	factory := database.NewTxnRunnerFactoryForNamespace(dbGetter.GetDB, database.ControllerNS)
	return service.NewService(state.NewState(factory, logger))
}
