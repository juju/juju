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
	// Database lease timestamps have second precision. Stop conservatively.
	leaseSafetyMargin = time.Second
	cleanupTimeout    = 5 * time.Second
)

// LeaseService provides cancellable, holder-specific lease operations.
// Operations on this service are serialised by the worker.
type LeaseService interface {
	ClaimLease(context.Context, lease.Key, lease.Request) error
	ExtendLease(context.Context, lease.Key, lease.Request) error
	RevokeLease(context.Context, lease.Key, string) error
}

// Config supplies archive generation and controller-wide lease maintenance.
type Config struct {
	LeaseService        LeaseService
	ControllerUUID      string
	ControllerModelUUID string
	CreateArchive       corebackups.CreateFunc
	Clock               clock.Clock
	Logger              logger.Logger
}

// Worker creates archives on demand and maintains their creation lease.
type Worker struct {
	tomb     tomb.Tomb
	cfg      Config
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
	w := &Worker{cfg: cfg, requests: make(chan request)}
	w.tomb.Go(w.loop)
	return w, nil
}

// Validate checks the dependencies before starting the worker.
func (c Config) Validate() error {
	if c.LeaseService == nil {
		return errors.NotValidf("nil LeaseService")
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
	return nil
}

// Create creates an archive, failing immediately if creation is already busy.
// Once accepted, a request waits for generation and maintenance to stop, even
// when cancelled. Successful callers own cleanup of the returned archive.
func (w *Worker) Create(ctx context.Context, notes string) (*corebackups.Metadata, string, func(), error) {
	if err := ctx.Err(); err != nil {
		return nil, "", nil, context.Cause(ctx)
	}
	req := request{ctx: ctx, notes: notes, result: make(chan result, 1)}
	select {
	case <-ctx.Done():
		return nil, "", nil, context.Cause(ctx)
	case <-w.tomb.Dying():
		return nil, "", nil, tomb.ErrDying
	case w.requests <- req:
	}
	// Never abandon an accepted result: it may own a completed archive.
	res := <-req.result
	if res.err == nil {
		if err := ctx.Err(); err != nil {
			res.err = context.Cause(ctx)
		} else {
			select {
			case <-w.tomb.Dying():
				res.err = tomb.ErrDying
			default:
			}
		}
		if res.err != nil {
			res.cleanup()
			return nil, "", nil, res.err
		}
	}
	return res.meta, res.archive, res.cleanup, res.err
}

// Kill implements worker.Worker.
func (w *Worker) Kill() { w.tomb.Kill(nil) }

// Wait implements worker.Worker, including all accepted creation requests.
func (w *Worker) Wait() error { return w.tomb.Wait() }

func (w *Worker) loop() error {
	completed := make(chan result, 1)
	var active *request
	for {
		select {
		case <-w.tomb.Dying():
			if active != nil {
				active.result <- <-completed
			}
			return tomb.ErrDying
		case res := <-completed:
			active.result <- res
			active = nil
		case req := <-w.requests:
			if active != nil {
				req.result <- result{err: errors.NotYetAvailablef("a backup is already being created")}
				continue
			}
			active = &req
			w.tomb.Go(func() error {
				completed <- w.create(req)
				return nil
			})
		}
	}
}

func (w *Worker) create(req request) result {
	parent, stop := context.WithCancel(req.ctx)
	defer stop()
	ctx, cancel := context.WithCancelCause(w.tomb.Context(parent))
	defer cancel(nil)
	if ctx.Err() != nil {
		return result{err: context.Cause(ctx)}
	}
	holder, err := uuid.NewUUID()
	if err != nil {
		return result{err: err}
	}
	key := lease.Key{Namespace: lease.BackupCreationNamespace, ModelUUID: w.cfg.ControllerModelUUID, Lease: w.cfg.ControllerUUID}
	claim := lease.Request{Holder: holder.String(), Duration: leaseDuration}
	started := w.cfg.Clock.Now()
	err = w.cfg.LeaseService.ClaimLease(ctx, key, claim)
	if errors.Is(err, lease.ErrHeld) {
		return result{err: errors.NotYetAvailablef("a backup is already being created")}
	}
	// A failed claim can have committed. Release only this invocation's holder.
	defer w.revoke(ctx, key, claim.Holder)
	if err != nil {
		if ctx.Err() != nil {
			err = context.Cause(ctx)
		}
		return result{err: errors.Annotate(err, "claiming backup creation lease")}
	}
	expiry := &leaseExpiry{clock: w.cfg.Clock, cancel: cancel}
	expiry.reset(started)
	renewalCtx, stopRenewal := context.WithCancel(ctx)
	var renewal tomb.Tomb
	renewal.Go(func() error {
		defer expiry.stop()
		w.renew(renewalCtx, cancel, key, claim, expiry)
		return nil
	})
	var res result
	if ctx.Err() == nil {
		res.meta, res.archive, res.cleanup, res.err = w.cfg.CreateArchive(ctx, req.notes)
	}
	stopRenewal()
	_ = renewal.Wait()
	if ctx.Err() != nil {
		res.err = context.Cause(ctx)
	}
	if res.err != nil {
		if res.cleanup != nil {
			res.cleanup()
		}
		return result{err: res.err}
	}
	return res
}

func (w *Worker) renew(ctx context.Context, cancel context.CancelCauseFunc, key lease.Key, claim lease.Request, expiry *leaseExpiry) {
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
			started := w.cfg.Clock.Now()
			if err := w.cfg.LeaseService.ExtendLease(ctx, key, claim); err != nil {
				// Only suppress cancellation caused by finishing generation. A
				// concurrent lease failure must still invalidate the archive.
				if ctx.Err() == nil || !errors.Is(err, ctx.Err()) {
					if errors.Is(err, lease.ErrInvalid) {
						err = lease.ErrNotHeld
					}
					cancel(errors.Annotate(err, "renewing backup creation lease"))
				}
				return
			}
			if ctx.Err() != nil {
				return
			}
			expiry.reset(started)
			timer.Reset(renewInterval)
		}
	}
}

func (w *Worker) revoke(ctx context.Context, key lease.Key, holder string) {
	cleanupCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), cleanupTimeout)
	defer cancel()
	if err := w.cfg.LeaseService.RevokeLease(cleanupCtx, key, holder); err != nil && !errors.Is(err, lease.ErrInvalid) {
		w.cfg.Logger.Warningf(cleanupCtx, "releasing backup creation lease: %v", err)
	}
}

// leaseExpiry cancels generation independently of a stalled renewal call.
// Only the renewal goroutine manipulates it once generation has started.
type leaseExpiry struct {
	clock  clock.Clock
	cancel context.CancelCauseFunc
	timer  clock.Timer
	done   chan struct{}
}

func (e *leaseExpiry) reset(started time.Time) {
	e.stop()
	delay := started.Add(leaseDuration - leaseSafetyMargin).Sub(e.clock.Now())
	if delay <= 0 {
		e.cancel(errors.New("backup creation lease expired"))
		return
	}
	e.done = make(chan struct{})
	done := e.done
	e.timer = e.clock.AfterFunc(delay, func() {
		defer close(done)
		e.cancel(errors.New("backup creation lease expired"))
	})
}

func (e *leaseExpiry) stop() {
	if e.timer != nil {
		if !e.timer.Stop() {
			<-e.done
		}
		e.timer = nil
	}
}
