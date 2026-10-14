// Copyright 2026 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package backupcreation

import (
	"context"
	"time"

	"github.com/juju/clock"
	"github.com/juju/errors"
	"github.com/juju/worker/v5"
	"gopkg.in/tomb.v2"

	corebackups "github.com/juju/juju/core/backups"
	"github.com/juju/juju/core/lease"
	"github.com/juju/juju/core/logger"
	"github.com/juju/juju/internal/uuid"
)

const (
	leaseDuration = time.Minute
	renewInterval = 20 * time.Second
)

// Config supplies archive generation and controller-wide lease maintenance.
type Config struct {
	LeaseManager        lease.Manager
	ControllerUUID      string
	ControllerModelUUID string
	ControllerNodeID    string
	CreateArchive       corebackups.BackupCreator
	Clock               clock.Clock
	Logger              logger.Logger
}

// Worker creates archives on demand and maintains their creation lease.
type Worker struct {
	tomb   tomb.Tomb
	cfg    Config
	holder string

	claimer lease.Claimer
	checker lease.Checker
	revoker lease.Revoker

	requests chan request
}

type request struct {
	ctx    context.Context
	notes  string
	result chan result
}

type result struct {
	meta    *corebackups.Metadata
	archive string
	cleanup func()
	err     error
}

// NewWorker starts a worker without performing any database operations.
func NewWorker(cfg Config) (worker.Worker, error) {
	if err := cfg.Validate(); err != nil {
		return nil, err
	}
	claimer, err := cfg.LeaseManager.Claimer(lease.BackupCreationNamespace, cfg.ControllerModelUUID)
	if err != nil {
		return nil, errors.Annotate(err, "getting backup creation lease claimer")
	}
	checker, err := cfg.LeaseManager.Checker(lease.BackupCreationNamespace, cfg.ControllerModelUUID)
	if err != nil {
		return nil, errors.Annotate(err, "getting backup creation lease checker")
	}
	revoker, err := cfg.LeaseManager.Revoker(lease.BackupCreationNamespace, cfg.ControllerModelUUID)
	if err != nil {
		return nil, errors.Annotate(err, "getting backup creation lease revoker")
	}
	w := &Worker{
		cfg: cfg,

		holder: cfg.ControllerNodeID,

		claimer:  claimer,
		checker:  checker,
		revoker:  revoker,
		requests: make(chan request),
	}
	w.tomb.Go(w.loop)
	return w, nil
}

// Validate checks the dependencies before starting the worker.
func (c Config) Validate() error {
	if c.LeaseManager == nil {
		return errors.NotValidf("nil LeaseManager")
	}
	if c.CreateArchive == nil {
		return errors.NotValidf("nil CreateArchive")
	}
	if c.Clock == nil {
		return errors.NotValidf("nil Clock")
	}
	if c.Logger == nil {
		return errors.NotValidf("nil Logger")
	}
	if _, err := uuid.UUIDFromString(c.ControllerUUID); err != nil {
		return errors.Annotate(err, "backup controller UUID")
	}
	if _, err := uuid.UUIDFromString(c.ControllerModelUUID); err != nil {
		return errors.Annotate(err, "backup controller model UUID")
	}
	if c.ControllerNodeID == "" {
		return errors.NotValidf("empty ControllerNodeID")
	}
	return nil
}

// Create creates an archive, failing immediately if creation is already busy.
// Once accepted, a request waits for generation and maintenance to stop, even
// when cancelled. Successful callers own cleanup of the returned archive.
func (w *Worker) Create(ctx context.Context, notes string) (*corebackups.Metadata, string, func(), error) {
	req := request{
		ctx:    w.tomb.Context(ctx),
		notes:  notes,
		result: make(chan result, 1),
	}

	select {
	case <-ctx.Done():
		return nil, "", nil, context.Cause(ctx)
	case <-w.tomb.Dying():
		return nil, "", nil, corebackups.ErrBackupCreationWorkerDying
	case w.requests <- req:
	}

	select {
	case <-ctx.Done():
		return nil, "", nil, context.Cause(ctx)
	case <-w.tomb.Dying():
		return nil, "", nil, corebackups.ErrBackupCreationWorkerDying
	case res := <-req.result:
		return res.meta, res.archive, res.cleanup, res.err
	}
}

// Kill implements worker.Worker.
func (w *Worker) Kill() {
	w.tomb.Kill(nil)
}

// Wait implements worker.Worker, including all accepted creation requests.
func (w *Worker) Wait() error {
	return w.tomb.Wait()
}

func (w *Worker) loop() error {
	for {
		select {
		case <-w.tomb.Dying():
			return tomb.ErrDying

		case req := <-w.requests:
			// Allow concurrent requests to be queued. Though they'll be gated
			// on attempting to claim the backup during lease creation. As
			// you can only have one backup creation lease at a time, this will
			// ensure that only one backup is created at a time.
			go w.request(req)
		}
	}
}

func (w *Worker) request(req request) {
	if err := req.ctx.Err(); err != nil {
		select {
		case <-w.tomb.Dying():
		case req.result <- result{err: context.Cause(req.ctx)}:
		}
		return
	}

	res := w.withLease(req, func(ctx context.Context) result {
		metadata, archive, cleanup, err := w.create(ctx, req.notes)
		if err != nil {
			return result{err: err}
		}

		return result{
			meta:    metadata,
			archive: archive,
			cleanup: cleanup,
		}
	})
	select {
	case <-w.tomb.Dying():
		if res.cleanup != nil {
			res.cleanup()
		}
	case req.result <- res:
	}
}

func (w *Worker) withLease(req request, fn func(context.Context) result) result {
	ctx, cancel := context.WithCancelCause(req.ctx)
	defer cancel(nil)

	if err := w.claimer.Claim(w.cfg.ControllerUUID, w.holder, leaseDuration); err != nil {
		switch {
		case errors.Is(err, lease.ErrClaimDenied):
			return result{err: errors.NotYetAvailablef("a backup is already being created")}

		case errors.Is(err, lease.ErrHeld):
			// The lease is already held by the holder, if this is the case a
			// concurrent request is being processed.
			//
			// TODO (stickupkid): Add metrics to this to track how many
			// concurrent requests are being made to the backup creation worker.
			return result{err: errors.NotYetAvailablef("a backup is already being created")}

		default:
			return result{err: errors.Annotate(err, "claiming backup creation lease")}
		}
	}
	defer w.revoke(ctx, w.holder)

	// The leaseCtx will be cancelled when the lease is no longer held by the
	// leaseholder. This may or may not be the same as the holderName for the
	// lease. That check is done by the Token checker.
	start := make(chan struct{})

	// Start will be closed when we start waiting for the lease to expire.
	// If the lease is not held, the function will return immediately and
	// the context will be cancelled.

	// WaitUntilExpired will be run against the backup creation lease. To
	// ensure that after we've waited that we still hold the lease, we need to
	// check that the lease is still held by the holder. Then we can guarantee
	// that the lease is held by the holder for the duration of archive
	// generation. Although convoluted this is necessary to ensure that the
	// lease is held by the holder for the duration of the function. The
	// context will be cancelled when the lease is no longer held by the
	// leaseholder for the lease name.

	waitCtx, waitCancel := context.WithCancel(ctx)
	defer waitCancel()

	waitErr := make(chan error)
	go func() {
		err := w.claimer.WaitUntilExpired(waitCtx, w.cfg.ControllerUUID, start)

		// Ensure that the lease context is cancelled when the wait has
		// completed. We do this as quick as possible to ensure that archive
		// generation is cancelled as soon as possible.
		if err == nil {
			cancel(errors.New("backup creation lease expired"))
		} else {
			cancel(errors.Annotate(err, "waiting for backup creation lease expiry"))
		}

		// The waitErr might not be read, so we need to provide another way
		// to collapse the goroutine. Using the waitCtx this goroutine will
		// be cancelled when archive generation is complete.
		select {
		case <-waitCtx.Done():
		case waitErr <- err:
		}
	}()

	select {
	case <-ctx.Done():
		// If the leaseCtx is cancelled, then the waiting for the lease to
		// expire finished unexpectedly. Return the context error.
		return result{err: context.Cause(ctx)}
	case err := <-waitErr:
		if err == nil {
			// This shouldn't happen, but if it does, we need to return an
			// error. If we're attempting to wait whilst holding the lease,
			// before running archive generation and then wait returns nil, we
			// don't know if the lease is held by the holder or what state we're
			// in.
			return result{err: errors.New("backup creation lease expired before generation")}
		}
		if ctxErr := ctx.Err(); ctxErr != nil {
			return result{err: context.Cause(ctx)}
		}

		return result{err: errors.Annotate(err, "waiting for backup creation lease expiry")}
	case <-start:
	}

	// Ensure that the lease is held by the holder before proceeding. We're
	// guaranteed that the lease is held by the holder, otherwise the context
	// will have been cancelled.
	if err := w.checker.Token(w.cfg.ControllerUUID, w.holder).Check(); err != nil {
		return result{err: errors.Annotate(err, "checking backup creation lease token")}
	}

	return fn(ctx)
}

func (w *Worker) create(ctx context.Context, notes string) (*corebackups.Metadata, string, func(), error) {
	renewCtx, renewCancel := context.WithCancel(ctx)
	defer renewCancel()

	go func() {
		w.renew(renewCtx, renewCancel, w.cfg.ControllerUUID, w.holder)
	}()

	meta, archive, cleanup, err := w.cfg.CreateArchive.Create(renewCtx, notes)
	if err != nil {
		if cleanup != nil {
			cleanup()
		}
		return nil, "", nil, errors.Annotate(err, "creating backup archive")
	}

	return meta, archive, cleanup, nil
}

func (w *Worker) renew(ctx context.Context, cancel context.CancelFunc, leaseName, holder string) {
	timer := w.cfg.Clock.NewTimer(renewInterval)
	defer timer.Stop()

	for {
		select {
		case <-ctx.Done():
			return

		case <-timer.Chan():
			if err := w.claimer.Claim(leaseName, holder, leaseDuration); err != nil {
				cancel()
				return
			}
			timer.Reset(renewInterval)
		}
	}
}

func (w *Worker) revoke(ctx context.Context, holder string) {
	if err := w.revoker.Revoke(w.cfg.ControllerUUID, holder); err != nil && !errors.Is(err, lease.ErrNotHeld) {
		w.cfg.Logger.Warningf(context.WithoutCancel(ctx), "releasing backup creation lease: %v", err)
	}
}
