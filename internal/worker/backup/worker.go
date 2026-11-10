// Copyright 2026 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package backup

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
	tomb tomb.Tomb
	cfg  Config

	claimer lease.Claimer
	checker lease.Checker
	revoker lease.Revoker

	requests chan request
}

type request struct {
	ctx    context.Context
	notes  string
	result chan result
	done   chan struct{}
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
	// The unbuffered result transfers cleanup ownership only when received.
	req := request{
		ctx:    w.tomb.Context(ctx),
		notes:  notes,
		result: make(chan result),
		done:   make(chan struct{}),
	}

	select {
	case <-ctx.Done():
		return nil, "", nil, context.Cause(ctx)
	case <-w.tomb.Dying():
		return nil, "", nil, context.Canceled
	case w.requests <- req:
	}

	// Accepted requests always deliver a result after their work stops.
	// Acknowledge it after handling cleanup, so Wait covers the handoff too.
	defer close(req.done)
	res := <-req.result
	if err := context.Cause(ctx); err != nil {
		if res.cleanup != nil {
			res.cleanup()
		}
		return nil, "", nil, err
	}
	return res.meta, res.archive, res.cleanup, res.err
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
			w.tomb.Go(func() error {
				w.request(req)
				return nil
			})
		}
	}
}

func (w *Worker) request(req request) {
	// Sending the result only confirms receipt. Create may still need to
	// discard the archive after caller cancellation, so keep this request
	// tracked by the tomb until Create acknowledges completion of that cleanup.
	// On success, Create acknowledges the handoff before returning; the caller
	// owns archive cleanup after download.
	defer func() { <-req.done }()

	if err := req.ctx.Err(); err != nil {
		req.result <- result{err: context.Cause(req.ctx)}
		return
	}

	res := w.withLease(req.ctx, func(ctx context.Context, holder string) result {
		metadata, archive, cleanup, err := w.create(ctx, holder, req.notes)
		if err != nil {
			return result{err: err}
		}

		return result{
			meta:    metadata,
			archive: archive,
			cleanup: cleanup,
		}
	})
	req.result <- res
}

func (w *Worker) withLease(ctx context.Context, fn func(context.Context, string) result) result {
	ctx, cancel := context.WithCancelCause(ctx)
	defer cancel(nil)

	// Claim extends a lease already held by the same holder. Each creation
	// therefore needs its own holder, retained for renewal and release.
	attemptUUID, err := uuid.NewUUID()
	if err != nil {
		return result{err: errors.Annotate(err, "generating backup creation lease holder")}
	}
	holder := w.cfg.ControllerNodeID + ":" + attemptUUID.String()
	if err := w.claimer.Claim(w.cfg.ControllerUUID, holder, leaseDuration); err != nil {
		switch {
		case errors.Is(err, lease.ErrClaimDenied):
			return result{err: errors.NotYetAvailablef("a backup is already being created")}

		case errors.Is(err, lease.ErrHeld):
			return result{err: errors.NotYetAvailablef("a backup is already being created")}

		default:
			return result{err: errors.Annotate(err, "claiming backup creation lease")}
		}
	}
	defer w.revoke(ctx, holder)
	if err := ctx.Err(); err != nil {
		return result{err: context.Cause(ctx)}
	}

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
	waitDone := make(chan struct{})
	defer func() {
		// Stop the expiry monitor before the deferred lease revocation.
		waitCancel()
		<-waitDone
	}()

	waitErr := make(chan error)
	go func() {
		defer close(waitDone)
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
	if err := w.checker.Token(w.cfg.ControllerUUID, holder).Check(); err != nil {
		return result{err: errors.Annotate(err, "checking backup creation lease token")}
	}
	if err := ctx.Err(); err != nil {
		return result{err: context.Cause(ctx)}
	}

	return fn(ctx, holder)
}

func (w *Worker) create(ctx context.Context, holder, notes string) (*corebackups.Metadata, string, func(), error) {
	renewCtx, renewCancel := context.WithCancel(ctx)
	renewDone := make(chan struct{})
	defer func() {
		// An in-flight renewal must finish before we release the lease.
		renewCancel()
		<-renewDone
	}()

	go func() {
		defer close(renewDone)
		w.renew(renewCtx, renewCancel, w.cfg.ControllerUUID, holder)
	}()

	meta, archive, cleanup, err := w.cfg.CreateArchive.Create(renewCtx, notes)
	if err == nil {
		err = context.Cause(renewCtx)
	}
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
			if ctx.Err() != nil {
				return
			}
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
