// Copyright 2026 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package backuplock

import (
	"context"
	"time"

	"github.com/juju/clock"
	"github.com/juju/errors"
	"gopkg.in/tomb.v2"

	"github.com/juju/juju/core/lease"
	"github.com/juju/juju/core/logger"
	"github.com/juju/juju/internal/uuid"
)

const leaseDuration = time.Minute
const renewInterval = 20 * time.Second

// Config supplies the lease manager shared by all controller API servers.
type Config struct {
	Manager             lease.Manager
	ControllerUUID      string
	ControllerModelUUID string
	Clock               clock.Clock
	Logger              logger.Logger
}

// Worker renews leases outside HTTP handlers and cancels creation on lease loss.
type Worker struct {
	tomb    tomb.Tomb
	cfg     Config
	acquire chan request
	release chan release
}

type request struct {
	ctx    context.Context
	result chan result
}

type result struct {
	ctx    context.Context
	holder string
	err    error
}

type release struct {
	holder string
	done   chan struct{}
}

type heldLease struct {
	ctx     context.Context
	cancel  context.CancelCauseFunc
	holder  string
	claimer lease.Claimer
	revoker lease.Revoker
	checker lease.Checker
	expiry  clock.Timer
	expired chan struct{}
}

// NewWorker starts a lifecycle-managed guard. It does not touch the database
// until a backup request arrives.
func NewWorker(cfg Config) (*Worker, error) {
	if cfg.Manager == nil || cfg.Clock == nil || cfg.Logger == nil {
		return nil, errors.NotValidf("backup lock dependencies")
	}
	if _, err := uuid.UUIDFromString(cfg.ControllerUUID); err != nil {
		return nil, errors.Annotate(err, "backup controller UUID")
	}
	if _, err := uuid.UUIDFromString(cfg.ControllerModelUUID); err != nil {
		return nil, errors.Annotate(err, "backup controller model UUID")
	}
	w := &Worker{cfg: cfg, acquire: make(chan request), release: make(chan release)}
	w.tomb.Go(w.loop)
	return w, nil
}

// Acquire fails immediately if another backup owns the lease. The returned
// context is cancelled on lease loss. Release must be called after creation
// stops, before downloading the completed archive, even if context is cancelled.
func (w *Worker) Acquire(ctx context.Context) (context.Context, func(), error) {
	if err := ctx.Err(); err != nil {
		return nil, nil, err
	}
	req := request{ctx: ctx, result: make(chan result, 1)}
	select {
	case <-ctx.Done():
		return nil, nil, ctx.Err()
	case <-w.tomb.Dying():
		return nil, nil, tomb.ErrDying
	case w.acquire <- req:
	}
	// Once submitted, observe the response even if the caller is cancelled:
	// abandoning a successful acquisition would strand its owner.
	select {
	case <-w.tomb.Dying():
		return nil, nil, tomb.ErrDying
	case res := <-req.result:
		if res.err != nil {
			return nil, nil, res.err
		}
		return res.ctx, func() { w.releaseLease(res.holder) }, nil
	}
}

func (w *Worker) releaseLease(holder string) {
	req := release{holder: holder, done: make(chan struct{})}
	select {
	case <-w.tomb.Dying():
		return
	case w.release <- req:
	}
	select {
	case <-w.tomb.Dying():
	case <-req.done:
	}
}

// Kill implements worker.Worker.
func (w *Worker) Kill() { w.tomb.Kill(nil) }

// Wait implements worker.Worker.
func (w *Worker) Wait() error { return w.tomb.Wait() }

func (w *Worker) loop() error {
	timer := w.cfg.Clock.NewTimer(renewInterval)
	defer timer.Stop()
	var held *heldLease
	defer func() {
		if held != nil {
			held.cancel(context.Canceled)
			held.stopExpiry()
			// Creation may still be unwinding. Do not revoke until its caller
			// releases; shutdown leaves the lease to expire instead.
		}
	}()
	for {
		select {
		case <-w.tomb.Dying():
			return tomb.ErrDying
		case req := <-w.acquire:
			if held != nil {
				req.result <- result{err: errors.NotYetAvailablef("a backup is already being created")}
				continue
			}
			var err error
			held, err = w.claim(req.ctx)
			if err != nil {
				req.result <- result{err: err}
				continue
			}
			timer.Reset(renewInterval)
			req.result <- result{ctx: held.ctx, holder: held.holder}
		case req := <-w.release:
			if held != nil && held.holder == req.holder {
				held.cancel(context.Canceled)
				held.stopExpiry()
				if err := held.revoker.Revoke(w.cfg.ControllerUUID, held.holder); err != nil {
					w.cfg.Logger.Warningf(context.Background(), "releasing backup creation lease: %v", err)
				}
				held = nil
			}
			close(req.done)
		case <-timer.Chan():
			if held == nil || held.ctx.Err() != nil {
				continue
			}
			started := w.cfg.Clock.Now()
			err := held.checker.Token(w.cfg.ControllerUUID, held.holder).Check()
			if err == nil {
				err = held.claimer.Claim(w.cfg.ControllerUUID, held.holder, leaseDuration)
			}
			if err != nil {
				held.cancel(errors.Annotate(err, "renewing backup creation lease"))
				continue
			}
			if held.ctx.Err() != nil {
				continue
			}
			held.stopExpiry()
			held.watchExpiry(w.cfg.Clock, started.Add(leaseDuration))
			timer.Reset(renewInterval)
		}
	}
}

func (w *Worker) claim(ctx context.Context) (*heldLease, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	claimer, err := w.cfg.Manager.Claimer(lease.BackupCreationNamespace, w.cfg.ControllerModelUUID)
	if err != nil {
		return nil, err
	}
	revoker, err := w.cfg.Manager.Revoker(lease.BackupCreationNamespace, w.cfg.ControllerModelUUID)
	if err != nil {
		return nil, err
	}
	checker, err := w.cfg.Manager.Checker(lease.BackupCreationNamespace, w.cfg.ControllerModelUUID)
	if err != nil {
		return nil, err
	}
	holder, err := uuid.NewUUID()
	if err != nil {
		return nil, err
	}
	started := w.cfg.Clock.Now()
	if err := claimer.Claim(w.cfg.ControllerUUID, holder.String(), leaseDuration); err != nil {
		if errors.Is(err, lease.ErrClaimDenied) {
			return nil, errors.NotYetAvailablef("a backup is already being created")
		}
		return nil, errors.Annotate(err, "claiming backup creation lease")
	}
	leaseCtx, cancel := context.WithCancelCause(ctx)
	held := &heldLease{ctx: leaseCtx, cancel: cancel, holder: holder.String(), claimer: claimer, revoker: revoker, checker: checker}
	held.watchExpiry(w.cfg.Clock, started.Add(leaseDuration))
	return held, nil
}

// A separate deadline cancels reads even if a renewal call itself stalls.
func (h *heldLease) watchExpiry(clk clock.Clock, expiry time.Time) {
	h.expired = make(chan struct{})
	done := h.expired
	h.expiry = clk.AfterFunc(expiry.Sub(clk.Now()), func() {
		defer close(done)
		h.cancel(errors.New("backup creation lease expired"))
	})
}

func (h *heldLease) stopExpiry() {
	if !h.expiry.Stop() {
		<-h.expired
	}
}
