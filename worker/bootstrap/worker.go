// Copyright 2023 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package bootstrap

import (
	"context"
	"fmt"
	"os"
	"path/filepath"

	"github.com/juju/errors"
	"gopkg.in/tomb.v2"

	"github.com/juju/juju/agent"
	"github.com/juju/juju/core/flags"
	"github.com/juju/juju/core/objectstore"
	"github.com/juju/juju/internal/cloudconfig"
	"github.com/juju/juju/internal/cloudconfig/instancecfg"
	"github.com/juju/juju/worker/gate"
)

const (
	// States which report the state of the worker.
	stateStarted   = "started"
	stateCompleted = "completed"
)

// WorkerConfig encapsulates the configuration options for the
// bootstrap worker.
type WorkerConfig struct {
	Agent                   agent.Agent
	ObjectStoreGetter       ObjectStoreGetter
	ControllerConfigService ControllerConfigService
	FlagService             FlagService
	BootstrapUnlocker       gate.Unlocker
	AgentBinaryUploader     AgentBinaryBootstrapFunc
	ControllerCharmDeployer ControllerCharmDeployerFunc
	PopulateControllerCharm PopulateControllerCharmFunc
	CharmhubHTTPClient      HTTPClient
	UnitPassword            string

	// Deprecated: This is only here, until we can remove the state layer.
	SystemState SystemState

	LoggerFactory LoggerFactory
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
	if c.FlagService == nil {
		return errors.NotValidf("nil FlagService")
	}
	if c.ControllerCharmDeployer == nil {
		return errors.NotValidf("nil ControllerCharmDeployer")
	}
	if c.PopulateControllerCharm == nil {
		return errors.NotValidf("nil PopulateControllerCharm")
	}
	if c.CharmhubHTTPClient == nil {
		return errors.NotValidf("nil CharmhubHTTPClient")
	}
	if c.LoggerFactory == nil {
		return errors.NotValidf("nil LoggerFactory")
	}
	if c.SystemState == nil {
		return errors.NotValidf("nil SystemState")
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

	// Seed the controller charm to the object store.
	if err := w.seedControllerCharm(ctx, dataDir); err != nil {
		return errors.Trace(err)
	}

	// Set the bootstrap flag, to indicate that the bootstrap has completed.
	if err := w.cfg.FlagService.SetFlag(ctx, flags.BootstrapFlag, true); err != nil {
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
	objectStore, err := w.cfg.ObjectStoreGetter.GetObjectStore(ctx, w.cfg.SystemState.ControllerModelUUID())
	if err != nil {
		return fmt.Errorf("failed to get object store: %v", err)
	}

	// Agent binary seeder will populate the tools for the agent.
	agentStorage := agentStorageShim{State: w.cfg.SystemState}
	if err := w.cfg.AgentBinaryUploader(ctx, dataDir, agentStorage, objectStore, w.cfg.LoggerFactory.Child("agentbinary")); err != nil {
		return errors.Trace(err)
	}

	return nil
}

func (w *bootstrapWorker) seedControllerCharm(ctx context.Context, dataDir string) error {
	args, err := w.bootstrapParams(ctx, dataDir)
	if err != nil {
		return errors.Annotatef(err, "getting bootstrap params")
	}

	controllerConfig, err := w.cfg.ControllerConfigService.ControllerConfig(ctx)
	if err != nil {
		return errors.Trace(err)
	}

	objectStore, err := w.cfg.ObjectStoreGetter.GetObjectStore(ctx, w.cfg.SystemState.ControllerModelUUID())
	if err != nil {
		return fmt.Errorf("failed to get object store: %v", err)
	}

	// Controller charm seeder will populate the charm for the controller.
	deployer, err := w.cfg.ControllerCharmDeployer(ControllerCharmDeployerConfig{
		StateBackend:                w.cfg.SystemState,
		ObjectStore:                 objectStore,
		ControllerConfig:            controllerConfig,
		DataDir:                     dataDir,
		BootstrapMachineConstraints: args.BootstrapMachineConstraints,
		ControllerCharmName:         args.ControllerCharmPath,
		ControllerCharmChannel:      args.ControllerCharmChannel,
		CharmhubHTTPClient:          w.cfg.CharmhubHTTPClient,
		UnitPassword:                w.cfg.UnitPassword,
		LoggerFactory:               w.cfg.LoggerFactory,
	})
	if err != nil {
		return errors.Trace(err)
	}

	return w.cfg.PopulateControllerCharm(ctx, deployer)
}

func (w *bootstrapWorker) bootstrapParams(ctx context.Context, dataDir string) (instancecfg.StateInitializationParams, error) {
	bootstrapParamsData, err := os.ReadFile(filepath.Join(dataDir, cloudconfig.FileNameBootstrapParams))
	if err != nil {
		return instancecfg.StateInitializationParams{}, errors.Annotate(err, "reading bootstrap params file")
	}
	var args instancecfg.StateInitializationParams
	if err := args.Unmarshal(bootstrapParamsData); err != nil {
		return instancecfg.StateInitializationParams{}, errors.Trace(err)
	}
	return args, nil
}

type agentStorageShim struct {
	State SystemState
}

// AgentBinaryStorage returns the interface for the BinaryAgentStorage.
// This is currently a shim wrapper around the tools storage. That will be
// renamed once we re-implement the tools storage in dqlite.
func (s agentStorageShim) AgentBinaryStorage(objectStore objectstore.ObjectStore) (BinaryAgentStorage, error) {
	return s.State.ToolsStorage(objectStore)
}
