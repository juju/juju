// Copyright 2023 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package bootstrap

import (
	"context"
	"fmt"

	"github.com/juju/errors"
	"gopkg.in/tomb.v2"

	"github.com/juju/juju/agent"
	"github.com/juju/juju/controller"
	"github.com/juju/juju/core/objectstore"
	"github.com/juju/juju/state/binarystorage"
	"github.com/juju/juju/worker/gate"
)

const (
	// States which report the state of the worker.
	stateStarted   = "started"
	stateCompleted = "completed"
)

// ControllerConfigService is the interface that is used to get the
// controller configuration.
type ControllerConfigService interface {
	ControllerConfig(context.Context) (controller.Config, error)
}

// ObjectStoreGetter is the interface that is used to get a object store.
type ObjectStoreGetter interface {
	// GetObjectStore returns a object store for the given namespace.
	GetObjectStore(context.Context, string) (objectstore.ObjectStore, error)
}

// LegacyState is the interface that is used to get the legacy state (mongo).
type LegacyState interface {
	// ControllerModelUUID returns the UUID of the model that was
	// bootstrapped.  This is the only model that can have controller
	// machines.  The owner of this model is also considered "special", in
	// that they are the only user that is able to create other users
	// (until we have more fine grained permissions), and they cannot be
	// disabled.
	ControllerModelUUID() string
	// ToolsStorage returns a new binarystorage.StorageCloser that stores tools
	// metadata in the "juju" database "toolsmetadata" collection.
	ToolsStorage(store objectstore.ObjectStore) (binarystorage.StorageCloser, error)
}

// WorkerConfig encapsulates the configuration options for the
// bootstrap worker.
type WorkerConfig struct {
	Agent                     agent.Agent
	ObjectStoreGetter         ObjectStoreGetter
	ControllerConfigService   ControllerConfigService
	BootstrapUnlocker         gate.Unlocker
	AgentBinaryUploader       AgentBinaryBootstrapFunc
	RemoveBootstrapParamsFile RemoveBootstrapParamsFileFunc

	// Deprecated: This is only here, until we can remove the state layer.
	State LegacyState

	Logger Logger
}

// Validate ensures that the config values are valid.
func (c *WorkerConfig) Validate() error {
	if c.Agent == nil {
		return errors.NotValidf("nil Agent")
	}
	if c.ObjectStoreGetter == nil {
		return errors.NotValidf("nil ObjectStoreGetter")
	}
	if c.ControllerConfigService == nil {
		return errors.NotValidf("nil ControllerConfigService")
	}
	if c.BootstrapUnlocker == nil {
		return errors.NotValidf("nil BootstrapUnlocker")
	}
	if c.AgentBinaryUploader == nil {
		return errors.NotValidf("nil AgentBinaryUploader")
	}
	if c.RemoveBootstrapParamsFile == nil {
		return errors.NotValidf("nil RemoveBootstrapParamsFile")
	}
	if c.Logger == nil {
		return errors.NotValidf("nil Logger")
	}
	if c.State == nil {
		return errors.NotValidf("nil State")
	}
	return nil
}

type bootstrapWorker struct {
	internalStates chan string
	cfg            WorkerConfig
	tomb           tomb.Tomb
}

// NewWorker creates a new bootstrap worker.
func NewWorker(cfg WorkerConfig) (*bootstrapWorker, error) {
	return newWorker(cfg, nil)
}

func newWorker(cfg WorkerConfig, internalStates chan string) (*bootstrapWorker, error) {
	var err error
	if err = cfg.Validate(); err != nil {
		return nil, errors.Trace(err)
	}

	w := &bootstrapWorker{
		internalStates: internalStates,
		cfg:            cfg,
	}
	w.tomb.Go(w.loop)
	return w, nil
}

// Kill stops the worker.
func (w *bootstrapWorker) Kill() {
	w.tomb.Kill(nil)
}

// Wait waits for the worker to stop and then returns the reason it was
// killed.
func (w *bootstrapWorker) Wait() error {
	return w.tomb.Wait()
}

func (w *bootstrapWorker) loop() error {
	// Report the initial started state.
	w.reportInternalState(stateStarted)

	ctx, cancel := w.scopedContext()
	defer cancel()

	agentConfig := w.cfg.Agent.CurrentConfig()
	dataDir := agentConfig.DataDir()

	// Seed the agent binary to the object store.
	if err := w.seedAgentBinary(ctx, dataDir); err != nil {
		return errors.Trace(err)
	}

	// Complete the bootstrap, only after this is complete do we unlock the
	// bootstrap gate.
	if err := w.cfg.RemoveBootstrapParamsFile(agentConfig); err != nil {
		return errors.Trace(err)
	}

	w.reportInternalState(stateCompleted)

	w.cfg.BootstrapUnlocker.Unlock()
	return nil
}

func (w *bootstrapWorker) reportInternalState(state string) {
	select {
	case <-w.tomb.Dying():
	case w.internalStates <- state:
	default:
	}
}

// scopedContext returns a context that is in the scope of the worker lifetime.
// It returns a cancellable context that is cancelled when the action has
// completed.
func (w *bootstrapWorker) scopedContext() (context.Context, context.CancelFunc) {
	ctx, cancel := context.WithCancel(context.Background())
	return w.tomb.Context(ctx), cancel
}

func (w *bootstrapWorker) seedAgentBinary(ctx context.Context, dataDir string) error {
	objectStore, err := w.cfg.ObjectStoreGetter.GetObjectStore(ctx, w.cfg.State.ControllerModelUUID())
	if err != nil {
		return fmt.Errorf("failed to get object store: %v", err)
	}

	// Agent binary seeder will populate the tools for the agent.
	agentStorage := agentStorageShim{State: w.cfg.State}
	if err := w.cfg.AgentBinaryUploader(ctx, dataDir, agentStorage, objectStore, w.cfg.Logger); err != nil {
		return errors.Trace(err)
	}

	return nil
}

type agentStorageShim struct {
	State LegacyState
}

// AgentBinaryStorage returns the interface for the BinaryAgentStorage.
// This is currently a shim wrapper around the tools storage. That will be
// renamed once we re-implement the tools storage in dqlite.
func (s agentStorageShim) AgentBinaryStorage(objectStore objectstore.ObjectStore) (BinaryAgentStorage, error) {
	return s.State.ToolsStorage(objectStore)
}
