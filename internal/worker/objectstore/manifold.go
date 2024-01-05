// Copyright 2023 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package objectstore

import (
	stdcontext "context"
	"time"

	"github.com/juju/clock"
	"github.com/juju/errors"
	"github.com/juju/mgo/v3"
	"github.com/juju/worker/v3"
	"github.com/juju/worker/v3/dependency"

	"github.com/juju/juju/agent"
	"github.com/juju/juju/controller"
	"github.com/juju/juju/core/lease"
	coreobjectstore "github.com/juju/juju/core/objectstore"
	"github.com/juju/juju/internal/objectstore"
	"github.com/juju/juju/internal/servicefactory"
	"github.com/juju/juju/internal/worker/common"
	"github.com/juju/juju/internal/worker/state"
	"github.com/juju/juju/internal/worker/trace"
	jujustate "github.com/juju/juju/state"
)

// Logger represents the logging methods called.
type Logger interface {
	Errorf(message string, args ...any)
	Warningf(message string, args ...any)
	Infof(message string, args ...any)
	Debugf(message string, args ...any)
	Tracef(message string, args ...any)

	IsTraceEnabled() bool
}

// ObjectStoreGetter is the interface that is used to get a object store.
type ObjectStoreGetter interface {
	// GetObjectStore returns a object store for the given namespace.
	GetObjectStore(stdcontext.Context, string) (coreobjectstore.ObjectStore, error)
}

// StatePool is the interface to retrieve the mongo session from.
// Deprecated: is only here for backwards compatibility.
type StatePool interface {
	// Get returns a PooledState for a given model, creating a new State instance
	// if required.
	// If the State has been marked for removal, an error is returned.
	Get(string) (MongoSession, error)
	SystemState() (MongoSession, error)
}

// MongoSession is the interface that is used to get a mongo session.
// Deprecated: is only here for backwards compatibility.
type MongoSession interface {
	MongoSession() *mgo.Session
}

// MetadataServiceGetter is the interface that is used to get the
// MetadataService for a given model UUID.
type MetadataServiceGetter interface {
	// For returns the MetadataService for the given model UUID.
	ForModelUUID(string) MetadataService
}

// ModelClaimGetter is the interface that is used to get a model claimer.
type ModelClaimGetter interface {
	ForModelUUID(string) (objectstore.Claimer, error)
}

// MetadataService is the interface that is used to get a object store.
type MetadataService interface {
	ObjectStore() coreobjectstore.ObjectStoreMetadata
}

// ManifoldConfig defines the configuration for the trace manifold.
type ManifoldConfig struct {
	AgentName          string
	TraceName          string
	ServiceFactoryName string
	LeaseManagerName   string

	Clock                clock.Clock
	Logger               Logger
	NewObjectStoreWorker objectstore.ObjectStoreWorkerFunc
	GetObjectStoreType   func(ControllerConfigService) (coreobjectstore.BackendType, error)

	// StateName is only here for backwards compatibility. Once we have
	// the right abstractions in place, and we have a replacement, we can
	// remove this.
	StateName string
}

// Validate validates the manifold configuration.
func (cfg ManifoldConfig) Validate() error {
	if cfg.AgentName == "" {
		return errors.NotValidf("empty AgentName")
	}
	if cfg.TraceName == "" {
		return errors.NotValidf("empty TraceName")
	}
	if cfg.ServiceFactoryName == "" {
		return errors.NotValidf("empty ServiceFactoryName")
	}
	if cfg.LeaseManagerName == "" {
		return errors.NotValidf("empty LeaseManagerName")
	}
	if cfg.Clock == nil {
		return errors.NotValidf("nil Clock")
	}
	if cfg.Logger == nil {
		return errors.NotValidf("nil Logger")
	}
	if cfg.NewObjectStoreWorker == nil {
		return errors.NotValidf("nil NewObjectStoreWorker")
	}
	return nil
}

// Manifold returns a dependency manifold that runs the trace worker.
func Manifold(config ManifoldConfig) dependency.Manifold {
	return dependency.Manifold{
		Inputs: []string{
			config.AgentName,
			config.TraceName,
			config.StateName,
			config.ServiceFactoryName,
		},
		Output: output,
		Start: func(context dependency.Context) (worker.Worker, error) {
			if err := config.Validate(); err != nil {
				return nil, errors.Trace(err)
			}

			var a agent.Agent
			if err := context.Get(config.AgentName, &a); err != nil {
				return nil, err
			}

			var tracerGetter trace.TracerGetter
			if err := context.Get(config.TraceName, &tracerGetter); err != nil {
				return nil, errors.Trace(err)
			}

			// Ensure we can support the object store type, before continuing.
			var controllerServiceFactory servicefactory.ControllerServiceFactory
			if err := context.Get(config.ServiceFactoryName, &controllerServiceFactory); err != nil {
				return nil, errors.Trace(err)
			}
			objectStoreType, err := config.GetObjectStoreType(controllerServiceFactory.ControllerConfig())
			if err != nil {
				return nil, errors.Trace(err)
			}

			var leaseManager lease.Manager
			if err := context.Get(config.LeaseManagerName, &leaseManager); err != nil {
				return nil, errors.Trace(err)
			}

			var modelServiceFactoryGetter servicefactory.ServiceFactoryGetter
			if err := context.Get(config.ServiceFactoryName, &modelServiceFactoryGetter); err != nil {
				return nil, errors.Trace(err)
			}

			var stTracker state.StateTracker
			if err := context.Get(config.StateName, &stTracker); err != nil {
				return nil, errors.Trace(err)
			}

			// Get the state pool after grabbing dependencies so we don't need
			// to remember to call Done on it if they're not running yet.
			statePool, _, err := stTracker.Use()
			if err != nil {
				return nil, errors.Trace(err)
			}

			w, err := NewWorker(WorkerConfig{
				TracerGetter:               tracerGetter,
				RootDir:                    a.CurrentConfig().DataDir(),
				Clock:                      config.Clock,
				Logger:                     config.Logger,
				NewObjectStoreWorker:       config.NewObjectStoreWorker,
				ObjectStoreType:            objectStoreType,
				ControllerMetadataService:  controllerMetadataService{factory: controllerServiceFactory},
				ModelMetadataServiceGetter: modelMetadataServiceGetter{factoryGetter: modelServiceFactoryGetter},
				ModelClaimGetter:           modelClaimGetter{manager: leaseManager},

				// StatePool is only here for backwards compatibility. Once we
				// have the right abstractions in place, and we have a
				// replacement, we can remove this.
				StatePool: shimStatePool{statePool: statePool},
			})
			if err != nil {
				_ = stTracker.Done()
				return nil, errors.Trace(err)
			}

			return common.NewCleanupWorker(w, func() {
				// Ensure we clean up the state pool.
				_ = stTracker.Done()
			}), nil
		},
	}
}

// ControllerConfigService is the interface that is used to get the controller
// config.
type ControllerConfigService interface {
	ControllerConfig(stdcontext.Context) (controller.Config, error)
}

// GetObjectStoreType returns the object store type from the controller config
// service.
// In reality this is a work around from the fact that we're dealing with
// a real concrete controller config service, and not an interface.
func GetObjectStoreType(controllerConfigService ControllerConfigService) (coreobjectstore.BackendType, error) {
	controllerConfig, err := controllerConfigService.ControllerConfig(stdcontext.TODO())
	if err != nil {
		return coreobjectstore.BackendType(""), errors.Trace(err)
	}

	return controllerConfig.ObjectStoreType(), nil
}

func output(in worker.Worker, out any) error {
	if w, ok := in.(*common.CleanupWorker); ok {
		in = w.Worker
	}
	w, ok := in.(*objectStoreWorker)
	if !ok {
		return errors.Errorf("expected input of objectStoreWorker, got %T", in)
	}

	switch out := out.(type) {
	case *ObjectStoreGetter:
		var target ObjectStoreGetter = w
		*out = target
	default:
		return errors.Errorf("expected output of ObjectStore, got %T", out)
	}
	return nil
}

type shimStatePool struct {
	statePool *jujustate.StatePool
}

// Get returns a PooledState for a given model, creating a new State instance
// if required.
// If the State has been marked for removal, an error is returned.
func (s shimStatePool) Get(namespace string) (MongoSession, error) {
	return s.statePool.Get(namespace)
}

func (s shimStatePool) SystemState() (MongoSession, error) {
	return s.statePool.SystemState()
}

type controllerMetadataService struct {
	factory servicefactory.ControllerServiceFactory
}

// ObjectStore returns the object store metadata for the controller model.
// This is the global object store.
func (s controllerMetadataService) ObjectStore() coreobjectstore.ObjectStoreMetadata {
	return s.factory.AgentObjectStore()
}

type modelMetadataServiceGetter struct {
	factoryGetter servicefactory.ServiceFactoryGetter
}

// ForModelUUID returns the MetadataService for the given model UUID.
func (s modelMetadataServiceGetter) ForModelUUID(modelUUID string) MetadataService {
	return modelMetadataService{factory: s.factoryGetter.FactoryForModel(modelUUID)}
}

type modelMetadataService struct {
	factory servicefactory.ServiceFactory
}

// ObjectStore returns the object store metadata for the given model UUID
func (s modelMetadataService) ObjectStore() coreobjectstore.ObjectStoreMetadata {
	return s.factory.ObjectStore()
}

type modelClaimGetter struct {
	manager lease.Manager
}

// ForModelUUID returns the Locker for the given model UUID.
func (s modelClaimGetter) ForModelUUID(modelUUID string) (objectstore.Claimer, error) {
	leaseClaimer, err := s.manager.Claimer(lease.ObjectStoreNamespace, modelUUID)
	if err != nil {
		return nil, errors.Trace(err)
	}
	leaseRevoker, err := s.manager.Revoker(lease.ObjectStoreNamespace, modelUUID)
	if err != nil {
		return nil, errors.Trace(err)
	}

	return claimer{
		claimer: leaseClaimer,
		revoker: leaseRevoker,
	}, nil
}

const (
	defaultLockDuration = time.Minute
	defaultHolderName   = "objectstore"
)

type claimer struct {
	claimer lease.Claimer
	revoker lease.Revoker
}

// Lock locks the given hash for the default duration.
func (l claimer) Claim(ctx stdcontext.Context, hash string) (objectstore.ClaimExtender, error) {
	if err := l.claimer.Claim(hash, defaultHolderName, defaultLockDuration); err != nil {
		return nil, errors.Trace(err)
	}

	return claimExtender{
		claimer: l.claimer,
		hash:    hash,
	}, nil
}

// Unlock unlocks the given hash.
func (l claimer) Release(ctx stdcontext.Context, hash string) error {
	return l.revoker.Revoke(hash, defaultHolderName)
}

type claimExtender struct {
	claimer lease.Claimer
	hash    string
}

// Extend extends the lock for the given hash.
func (l claimExtender) Extend(ctx stdcontext.Context) error {
	return l.claimer.Claim(l.hash, defaultHolderName, defaultLockDuration)
}

// Duration returns the duration of the lock.
func (l claimExtender) Duration() time.Duration {
	return defaultLockDuration / 2
}
