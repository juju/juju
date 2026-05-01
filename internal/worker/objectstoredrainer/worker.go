// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package objectstoredrainer

import (
	"context"
	"io"
	"slices"
	"time"

	"github.com/juju/clock"
	"github.com/juju/worker/v5"
	"github.com/juju/worker/v5/catacomb"

	"github.com/juju/juju/agent"
	coreerrors "github.com/juju/juju/core/errors"
	"github.com/juju/juju/core/logger"
	"github.com/juju/juju/core/model"
	"github.com/juju/juju/core/objectstore"
	"github.com/juju/juju/core/watcher"
	objectstoreservice "github.com/juju/juju/domain/objectstore/service"
	"github.com/juju/juju/internal/errors"
	internalworker "github.com/juju/juju/internal/worker"
	"github.com/juju/juju/internal/worker/fortress"
)

const (
	// ErrWorkerInUnknownState is returned when the worker is in an unknown
	// state.
	ErrWorkerInUnknownState = errors.ConstError("object store drainer worker is in an unknown state")
)

// SelectFileHashFunc is a function that selects the file hash from the
// metadata.
type SelectFileHashFunc func(objectstore.Metadata) string

// HashFileSystemAccessor is the interface for reading and deleting files from
// the file system.
// The file system accessor is used for draining files from the file backed
// object store to the s3 object store. It should at no point be used for
// writing files to the file system.
type HashFileSystemAccessor interface {
	// HashExists checks if the file exists in the file backed object store.
	// Returns a NotFound error if the file doesn't exist.
	HashExists(ctx context.Context, hash string) error

	// GetByHash returns an io.ReadCloser for the file at the given hash.
	GetByHash(ctx context.Context, hash string) (io.ReadCloser, int64, error)

	// DeleteByHash deletes the file at the given hash.
	DeleteByHash(ctx context.Context, hash string) error
}

// NewHashFileSystemAccessorFunc is a function that creates a new
// HashFileSystemAccessor.
type NewHashFileSystemAccessorFunc func(namespace, rootDir string, logger logger.Logger) HashFileSystemAccessor

// DrainingService provides access to the object store for draining
// operations.
type DrainingService interface {
	// GetDrainingPhase returns the current active draining phase of the
	// object store.
	GetDrainingPhase(ctx context.Context) (objectstore.Phase, error)

	// GetDrainingPhaseInfo returns the current active draining phase info of
	// the object store.
	GetDrainingPhaseInfo(ctx context.Context) (objectstore.DrainingPhaseInfo, error)

	// SetDrainingPhase sets the phase of the object store to draining.
	SetDrainingPhase(ctx context.Context, phase objectstore.Phase) error

	// WatchDraining returns a watcher that watches the draining phase of the
	// object store.
	WatchDraining(ctx context.Context) (watcher.NotifyWatcher, error)

	// GetActiveObjectStoreBackend returns the active backend info for the
	// object store.
	GetActiveObjectStoreBackend(ctx context.Context) (objectstoreservice.BackendInfo, error)

	// GetObjectStoreBackend returns the backend info for the given backend
	// uuid.
	GetObjectStoreBackend(ctx context.Context, uuid objectstore.UUID) (objectstoreservice.BackendInfo, error)

	// MarkObjectStoreBackendAsDrained marks the object store backend as
	// drained, which will cause the controller to switch to the new backend
	// and update the agent configuration. This should only be called once the
	// draining process has completed successfully.
	MarkObjectStoreBackendAsDrained(ctx context.Context) error

	// WatchObjectStoreBackend returns a watcher that watches the object store
	// backend. The watcher emits the backend changes that either have been
	// added or removed.
	WatchObjectStoreBackend(ctx context.Context) (watcher.StringsWatcher, error)
}

// ControllerService provides access to the controller for draining
// operations.
type ControllerService interface {
	// GetModelNamespaces returns the model namespaces of all models in the
	// state.
	GetModelNamespaces(ctx context.Context) ([]string, error)
}

// Config holds the dependencies and configuration for a Worker.
type Config struct {
	Agent                        agent.Agent
	Guard                        fortress.Guard
	DrainingService              DrainingService
	ControllerService            ControllerService
	ControllerConfigService      ControllerConfigService
	ControllerObjectStoreService objectstore.ObjectStoreMetadata
	ObjectStoreServicesGetter    ObjectStoreServicesGetter
	ObjectStoreFlusher           objectstore.ObjectStoreFlusher
	ObjectStoreType              objectstore.BackendType
	NewHashFileSystemAccessor    NewHashFileSystemAccessorFunc
	NewDrainerWorker             NewDrainerWorkerFunc
	S3Client                     objectstore.Client
	SelectFileHash               SelectFileHashFunc
	RootDir                      string
	RootBucketName               string
	Logger                       logger.Logger
	Clock                        clock.Clock
}

// Validate returns an error if the config cannot be expected to
// drive a functional Worker.
func (config Config) Validate() error {
	if config.Agent == nil {
		return errors.Errorf("nil Agent").Add(coreerrors.NotValid)
	}
	if config.Guard == nil {
		return errors.Errorf("nil Guard").Add(coreerrors.NotValid)
	}
	if config.DrainingService == nil {
		return errors.Errorf("nil DrainingService").Add(coreerrors.NotValid)
	}
	if config.ControllerService == nil {
		return errors.Errorf("nil ControllerService").Add(coreerrors.NotValid)
	}
	if config.ControllerConfigService == nil {
		return errors.Errorf("nil ControllerConfigService").Add(coreerrors.NotValid)
	}
	if config.ControllerObjectStoreService == nil {
		return errors.Errorf("nil controllerObjectStoreService").Add(coreerrors.NotValid)
	}
	if config.ObjectStoreServicesGetter == nil {
		return errors.Errorf("nil ObjectStoreServicesGetter").Add(coreerrors.NotValid)
	}
	if config.ObjectStoreFlusher == nil {
		return errors.Errorf("nil ObjectStoreFlusher").Add(coreerrors.NotValid)
	}
	if config.NewHashFileSystemAccessor == nil {
		return errors.Errorf("nil NewHashFileSystemAccessor").Add(coreerrors.NotValid)
	}
	if config.NewDrainerWorker == nil {
		return errors.Errorf("nil NewDrainerWorker").Add(coreerrors.NotValid)
	}
	if config.S3Client == nil {
		return errors.Errorf("nil S3Client").Add(coreerrors.NotValid)
	}
	if config.SelectFileHash == nil {
		return errors.Errorf("nil SelectFileHash").Add(coreerrors.NotValid)
	}
	if config.RootDir == "" {
		return errors.Errorf("empty RootDir").Add(coreerrors.NotValid)
	}
	if config.RootBucketName == "" {
		return errors.Errorf("empty RootBucketName").Add(coreerrors.NotValid)
	}
	if config.Logger == nil {
		return errors.Errorf("nil Logger").Add(coreerrors.NotValid)
	}
	if config.Clock == nil {
		return errors.Errorf("nil Clock").Add(coreerrors.NotValid)
	}
	return nil
}

// NewWorker returns a Worker that tracks the result of the configured.
func NewWorker(config Config) (worker.Worker, error) {
	if err := config.Validate(); err != nil {
		return nil, errors.Capture(err)
	}

	runner, err := worker.NewRunner(worker.RunnerParams{
		Name: "objectstore-drainer",
		IsFatal: func(err error) bool {
			if errors.Is(err, ErrWorkerInUnknownState) {
				return true
			}
			return false
		},
		ShouldRestart: internalworker.ShouldRunnerRestart,
		RestartDelay:  time.Second * 10,
		Clock:         config.Clock,
		Logger:        internalworker.WrapLogger(config.Logger),
	})
	if err != nil {
		return nil, errors.Capture(err)
	}

	w := &Worker{
		runner: runner,

		agent: config.Agent,

		guard:           config.Guard,
		drainingService: config.DrainingService,

		controllerService:            config.ControllerService,
		controllerConfigService:      config.ControllerConfigService,
		controllerObjectStoreService: config.ControllerObjectStoreService,

		objectStoreServicesGetter: config.ObjectStoreServicesGetter,
		objectStoreFlusher:        config.ObjectStoreFlusher,
		objectStoreType:           config.ObjectStoreType,

		newDrainWorker: config.NewDrainerWorker,
		newFileSystem:  config.NewHashFileSystemAccessor,
		client:         config.S3Client,
		rootDir:        config.RootDir,
		rootBucketName: config.RootBucketName,

		selectFileHash: config.SelectFileHash,

		clock:  config.Clock,
		logger: config.Logger,
	}

	if err := catacomb.Invoke(catacomb.Plan{
		Name: "objectstoredrainer",
		Site: &w.catacomb,
		Work: w.loop,
		Init: []worker.Worker{
			runner,
		},
	}); err != nil {
		return nil, errors.Capture(err)
	}
	return w, nil
}

// Worker watches the object store service for changes to the draining
// phase. If the phase is draining, it locks the guard. If the phase is not
// draining, it unlocks the guard.
// The worker will manage the lifecycle of the watcher and will stop
// watching when the worker is killed or when the context is cancelled.
type Worker struct {
	catacomb catacomb.Catacomb
	runner   *worker.Runner

	agent agent.Agent

	guard           fortress.Guard
	drainingService DrainingService

	controllerService            ControllerService
	controllerConfigService      ControllerConfigService
	controllerObjectStoreService objectstore.ObjectStoreMetadata

	objectStoreServicesGetter ObjectStoreServicesGetter
	objectStoreFlusher        objectstore.ObjectStoreFlusher
	objectStoreType           objectstore.BackendType

	newFileSystem  NewHashFileSystemAccessorFunc
	newDrainWorker NewDrainerWorkerFunc
	client         objectstore.Client
	rootDir        string
	rootBucketName string

	selectFileHash SelectFileHashFunc

	clock  clock.Clock
	logger logger.Logger
}

// Kill kills the worker. It will cause the worker to stop if it is
// not already stopped. The worker will transition to the dying state.
func (w *Worker) Kill() {
	w.catacomb.Kill(nil)
}

// Wait waits for the worker to finish. It will cause the worker to
// stop if it is not already stopped. It will return an error if the
// worker was killed with an error.
func (w *Worker) Wait() error {
	return w.catacomb.Wait()
}

// Report returns a report of the worker's state. This is used for
// debugging and monitoring purposes.
func (w *Worker) Report(ctx context.Context) map[string]any {
	return w.runner.Report(ctx)
}

func (w *Worker) loop() error {
	ctx := w.catacomb.Context(context.Background())

	cfgWatcher, err := w.controllerConfigService.WatchControllerConfig(ctx)
	if err != nil {
		return errors.Capture(err)
	}

	if err := w.catacomb.Add(cfgWatcher); err != nil {
		return errors.Capture(err)
	}

	drainingWatcher, err := w.drainingService.WatchDraining(ctx)
	if err != nil {
		return errors.Capture(err)
	}
	if err := w.catacomb.Add(drainingWatcher); err != nil {
		return errors.Capture(err)
	}

	for {
		select {
		case <-w.catacomb.Dying():
			return w.catacomb.ErrDying()

		case <-cfgWatcher.Changes():
			if err := w.handleConfigChange(ctx); err != nil {
				return errors.Capture(err)
			}

		case <-drainingWatcher.Changes():
			phase, err := w.drainingService.GetDrainingPhase(ctx)
			if err != nil {
				return errors.Capture(err)
			}

			// We're not draining, so we can unlock the guard and wait
			// for the next change.
			if phase.IsNotStarted() || phase == objectstore.PhaseCompleted {
				w.logger.Infof(ctx, "object store is not draining, unlocking guard")

				if err := w.guard.Unlock(ctx); err != nil {
					return errors.Errorf("failed to update guard: %v", err)
				}
				continue
			} else if phase == objectstore.PhaseError {
				w.logger.Errorf(ctx, "object store is in an error state, manual intervention required")
				continue
			}

			w.logger.Infof(ctx, "object store is draining, locking guard")

			if err := w.guard.Lockdown(ctx); err != nil {
				return errors.Errorf("failed to update guard: %v", err)
			}

			// TODO (stickupkid): Support draining from one s3 object store to
			// another. For now, we just log that we're in the draining phase
			// from file to s3.

			// Drain the agent binary object store, then drain all the models.
			if err := w.drainAgentBinaries(ctx); err != nil {
				_ = w.drainingService.SetDrainingPhase(ctx, objectstore.PhaseError)
				return errors.Errorf("draining agent binaries: %w", err)
			}

			namespaces, err := w.controllerService.GetModelNamespaces(ctx)
			if err != nil {
				_ = w.drainingService.SetDrainingPhase(ctx, objectstore.PhaseError)
				return errors.Errorf("getting model namespaces: %w", err)
			}

			uniqueNamespaces := unique(namespaces)
			if len(uniqueNamespaces) == 0 {
				if err := w.completeDraining(ctx); err != nil {
					_ = w.drainingService.SetDrainingPhase(ctx, objectstore.PhaseError)
					return errors.Errorf("completing draining: %w", err)
				}
				continue
			}

			signal, err := w.drainModels(ctx, uniqueNamespaces)
			if err != nil {
				_ = w.drainingService.SetDrainingPhase(ctx, objectstore.PhaseError)
				return errors.Errorf("draining models: %w", err)
			}

			if err := w.waitForDraining(ctx, signal, uniqueNamespaces); err != nil {
				_ = w.drainingService.SetDrainingPhase(ctx, objectstore.PhaseError)
				return errors.Errorf("waiting for draining: %w", err)
			}

			if err := w.completeDraining(ctx); err != nil {
				_ = w.drainingService.SetDrainingPhase(ctx, objectstore.PhaseError)
				return errors.Errorf("completing draining: %w", err)
			}
		}
	}
}

// HandleConfigChange starts the whole draining process if the object store
// type has changed.
func (w *Worker) handleConfigChange(ctx context.Context) error {
	_, err := w.controllerConfigService.ControllerConfig(ctx)
	if err != nil {
		return errors.Capture(err)
	}

	phase, err := w.drainingService.GetDrainingPhase(ctx)
	if err != nil {
		return errors.Capture(err)
	}

	objectStoreType := objectstore.FileBackend
	objectStoreTypeChanged := objectStoreType != w.objectStoreType

	if !objectStoreTypeChanged {
		w.logger.Debugf(ctx, "object store type has not changed: %q", w.objectStoreType)
		return nil
	} else if phase.IsDraining() {
		w.logger.Infof(ctx, "object store is already draining, no action taken")
		return nil
	}

	w.logger.Debugf(ctx, "object store type changed: %q => %q", w.objectStoreType, objectStoreType)

	// Force the draining process to move into the draining phase.
	// Persist state before updating in-memory state to avoid divergence
	// if SetDrainingPhase fails.
	if err := w.drainingService.SetDrainingPhase(ctx, objectstore.PhaseDraining); err != nil {
		return errors.Capture(err)
	}

	w.objectStoreType = objectStoreType

	return nil
}

func (w *Worker) drainAgentBinaries(ctx context.Context) error {
	w.logger.Infof(ctx, "draining controller agent binaries")
	signal := make(chan drainResult, 1)

	namespace := "controller"
	err := w.runner.StartWorker(ctx, namespace, func(ctx context.Context) (worker.Worker, error) {
		fileSystem := w.newFileSystem(namespace, w.rootDir, w.logger)
		return w.newDrainWorker(
			signal,
			fileSystem,
			w.client,
			w.controllerObjectStoreService,
			w.rootBucketName,
			namespace,
			w.selectFileHash,
			w.clock,
			w.logger,
		), nil
	})
	if errors.Is(err, coreerrors.AlreadyExists) {
		// A controller drain worker already exists from a previous invocation.
		// This is terminal because the existing worker has a different signal
		// channel and will never notify us.
		return errors.Errorf("worker for controller agent binaries: %w", err).Add(ErrWorkerInUnknownState)
	} else if err != nil {
		return errors.Errorf("starting worker for controller agent binaries: %w", err)
	}

	select {
	case <-w.catacomb.Dying():
		return w.catacomb.ErrDying()
	case <-w.clock.After(defaultDrainTimeout):
		return errors.Errorf("timeout waiting for controller agent binaries to drain")
	case result := <-signal:
		if result.Err != nil {
			return errors.Errorf("drain worker for controller agent binaries failed: %w", result.Err)
		}
		w.logger.Infof(ctx, "drain worker for controller agent binaries completed")
		return nil
	}
}

// drainModels starts a worker for each model in the state and waits for them
// to complete. It signals the completion of each worker through a channel.
func (w *Worker) drainModels(ctx context.Context, namespaces []string) (<-chan drainResult, error) {
	signal := make(chan drainResult, len(namespaces))
	for _, namespace := range namespaces {
		w.logger.Infof(ctx, "draining model %q", namespace)

		err := w.runner.StartWorker(ctx, namespace, func(ctx context.Context) (worker.Worker, error) {
			metadataService := w.objectStoreServicesGetter.ServicesForModel(model.UUID(namespace))
			fileSystem := w.newFileSystem(namespace, w.rootDir, w.logger)
			return w.newDrainWorker(
				signal,
				fileSystem,
				w.client,
				metadataService.ObjectStore(),
				w.rootBucketName,
				namespace,
				w.selectFileHash,
				w.clock,
				w.logger,
			), nil
		})
		if errors.Is(err, coreerrors.AlreadyExists) {
			// This is terminal.
			return nil, errors.Errorf("worker for model %q: %w", namespace, err).Add(ErrWorkerInUnknownState)
		} else if err != nil {
			return nil, errors.Errorf("starting worker for model %q: %w", namespace, err)
		}
	}
	return signal, nil
}

// waitForDraining waits for all the draining workers to complete. It will
// block until all the workers have completed, the timeout expires, or the
// context is cancelled.
func (w *Worker) waitForDraining(ctx context.Context, signal <-chan drainResult, namespaces []string) error {
	remaining := map[string]struct{}{}
	for _, namespace := range namespaces {
		remaining[namespace] = struct{}{}
	}

	timeout := w.clock.After(defaultDrainTimeout)
	for {
		select {
		case <-w.catacomb.Dying():
			return w.catacomb.ErrDying()
		case <-timeout:
			return errors.Errorf("timeout waiting for %d drain workers to complete", len(remaining))
		case result := <-signal:
			// Ignore results for namespaces we're not tracking (defensive).
			if _, ok := remaining[result.Namespace]; !ok {
				continue
			}

			if result.Err != nil {
				return errors.Errorf("drain worker for model %q failed: %w", result.Namespace, result.Err)
			}

			w.logger.Infof(ctx, "drain worker for model %q completed", result.Namespace)

			delete(remaining, result.Namespace)

			if len(remaining) == 0 {
				return nil
			}

			w.logger.Infof(ctx, "waiting for %d more drain workers to complete", len(remaining))
		}
	}
}

// completeDraining updates the agent configuration to indicate that the
// object store type has changed and then flushes the object store workers.
// It sets the draining phase to completed, which will cause the main loop
// to unlock the guard and allow the object store to be used again.
//
// The ordering is important for crash recovery:
// 1. Set phase to completed (persistent, authoritative state)
// 2. Update agent config (idempotent, re-applied on restart if missed)
// 3. Flush workers (idempotent, re-applied on restart if missed)
//
// If a crash occurs after step 1, on restart the worker sees PhaseCompleted,
// unlocks the guard, and the manifold start logic reconciles the remaining
// steps.
//
// NOTE: The fortress guard ensures no new objectstore operations can begin
// during draining (all callers go through the objectStoreFacade which uses
// fortress.Guest.Visit). However, callers that obtained an io.ReadCloser
// from a Get() call before lockdown may still be streaming data when
// FlushWorkers kills the underlying objectstore child workers. This is a
// known limitation — such readers will receive an IO error. The practical
// risk is low because draining is a rare operational event and callers
// handle read errors. Fixing this would require reference-counting active
// readers, which is disproportionate to the risk.
func (w *Worker) completeDraining(ctx context.Context) error {
	w.logger.Infof(ctx, "completing object store draining")

	// Set the draining phase to completed first. This is the persistent
	// source of truth that determines recovery behavior on restart.
	if err := w.drainingService.SetDrainingPhase(ctx, objectstore.PhaseCompleted); err != nil {
		return errors.Capture(err)
	}

	// Update the agent configuration to reflect the new object store type.
	if err := w.agent.ChangeConfig(func(setter agent.ConfigSetter) error {
		w.logger.Debugf(ctx, "setting object store type: %q => %q", setter.ObjectStoreType(), w.objectStoreType)
		setter.SetObjectStoreType(w.objectStoreType)
		return nil
	}); err != nil {
		return errors.Capture(err)
	}

	// Flush the object store workers to ensure that they are all stopped and
	// removed. This is necessary to ensure that the object store is in a clean
	// state before we start using it again.
	if err := w.objectStoreFlusher.FlushWorkers(ctx); err != nil {
		return errors.Capture(err)
	}

	w.logger.Infof(ctx, "object store draining completed successfully")

	return nil
}

// unique returns a slice of unique namespaces from the given slice.
func unique(namespaces []string) []string {
	uniqueNamespaces := make(map[string]struct{}, len(namespaces))
	for _, namespace := range namespaces {
		uniqueNamespaces[namespace] = struct{}{}
	}

	result := make([]string, 0, len(uniqueNamespaces))
	for namespace := range uniqueNamespaces {
		result = append(result, namespace)
	}

	slices.Sort(result)
	return result
}
