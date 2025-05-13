// Copyright 2023 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package secretbackendrotate

import (
	"context"
	"fmt"
	"time"

	"github.com/juju/clock"
	"github.com/juju/errors"
	"github.com/juju/worker/v4"
	"github.com/juju/worker/v4/catacomb"

	"github.com/juju/juju/core/logger"
	"github.com/juju/juju/core/watcher"
)

// SecretBackendManagerFacade instances provide a watcher for secret rotation changes.
type SecretBackendManagerFacade interface {
	WatchTokenRotationChanges(context.Context) (watcher.SecretBackendRotateWatcher, error)
	RotateBackendTokens(ctx context.Context, info ...string) error
}

// Config defines the operation of the Worker.
type Config struct {
	SecretBackendManagerFacade SecretBackendManagerFacade
	Logger                     logger.Logger
	Clock                      clock.Clock
}

// Validate returns an error if config cannot drive the Worker.
func (config Config) Validate() error {
	if config.SecretBackendManagerFacade == nil {
		return errors.NotValidf("nil Facade")
	}
	if config.Clock == nil {
		return errors.NotValidf("nil Clock")
	}
	if config.Logger == nil {
		return errors.NotValidf("nil Logger")
	}
	return nil
}

// NewWorker returns a Secret Backend token rotation Worker backed by config, or an error.
func NewWorker(config Config) (worker.Worker, error) {
	if err := config.Validate(); err != nil {
		return nil, errors.Trace(err)
	}

	w := &Worker{
		config:      config,
		backendInfo: make(map[string]tokenRotateInfo),
	}
	err := catacomb.Invoke(catacomb.Plan{
		Name: "secret-backend-rotate",
		Site: &w.catacomb,
		Work: w.loop,
	})
	return w, errors.Trace(err)
}

type tokenRotateInfo struct {
	ID          string
	backendName string
	rotateTime  time.Time
	whenFunc    func(rotateTime time.Time) time.Duration
}

func (s tokenRotateInfo) GoString() string {
	return fmt.Sprintf("%s token rotation: in %v at %s", s.backendName, s.whenFunc(s.rotateTime), s.rotateTime.Format(time.RFC3339))
}

// Worker fires events when secrets should be rotated.
type Worker struct {
	catacomb catacomb.Catacomb
	config   Config

	backendInfo map[string]tokenRotateInfo

	timer       clock.Timer
	nextTrigger time.Time
}

// Kill is defined on worker.Worker.
func (w *Worker) Kill() {
	w.catacomb.Kill(nil)
}

// Wait is part of the worker.Worker interface.
func (w *Worker) Wait() error {
	return w.catacomb.Wait()
}

func (w *Worker) loop() (err error) {
	ctx, cancel := w.scopeContext()
	defer cancel()

	changes, err := w.config.SecretBackendManagerFacade.WatchTokenRotationChanges(ctx)
	if err != nil {
		return errors.Trace(err)
	}
	if err := w.catacomb.Add(changes); err != nil {
		return errors.Trace(err)
	}
	for {
		var timeout <-chan time.Time
		if w.timer != nil {
			timeout = w.timer.Chan()
		}
		select {
		case <-w.catacomb.Dying():
			return w.catacomb.ErrDying()
		case ch, ok := <-changes.Changes():
			if !ok {
				return errors.New("secret rotation change channel closed")
			}
			w.handleTokenRotateChanges(ctx, ch)
		case now := <-timeout:
			if err := w.rotate(ctx, now); err != nil {
				return errors.Annotatef(err, "rotating secret backends")
			}
		}
	}
}

func (w *Worker) rotate(ctx context.Context, now time.Time) error {
	w.config.Logger.Debugf(ctx, "processing secret backend token rotation at %s", now)

	var toRotate []string
	for id, info := range w.backendInfo {
		w.config.Logger.Debugf(ctx, "checking %s: rotate at %s... time diff %s", id, info.rotateTime, info.rotateTime.Sub(now))
		// A one minute granularity is acceptable for secret rotation.
		if info.rotateTime.Truncate(time.Minute).Before(now) {
			w.config.Logger.Debugf(ctx, "rotating token for %s", info.backendName)
			toRotate = append(toRotate, id)
			// Once backend has been queued for rotation, delete it here since
			// it will re-appear via the watcher after the rotation is actually
			// performed and the last rotated time is updated.
			delete(w.backendInfo, id)
		}
	}

	if err := w.config.SecretBackendManagerFacade.RotateBackendTokens(ctx, toRotate...); err != nil {
		return errors.Annotatef(err, "cannot rotate secret backend tokens for backend ids %q", toRotate)
	}
	w.computeNextRotateTime(ctx)
	return nil
}

func (w *Worker) handleTokenRotateChanges(ctx context.Context, changes []watcher.SecretBackendRotateChange) {
	w.config.Logger.Debugf(ctx, "got rotate secret changes: %#v", changes)
	if len(changes) == 0 {
		return
	}

	for _, ch := range changes {
		// Next rotate time of 0 means the rotation has been deleted.
		if ch.NextTriggerTime.IsZero() {
			w.config.Logger.Debugf(ctx, "token for %q no longer rotated", ch.Name)
			delete(w.backendInfo, ch.ID)
			continue
		}
		w.backendInfo[ch.ID] = tokenRotateInfo{
			ID:          ch.ID,
			backendName: ch.Name,
			rotateTime:  ch.NextTriggerTime,
			whenFunc:    func(rotateTime time.Time) time.Duration { return rotateTime.Sub(w.config.Clock.Now()) },
		}
	}
	w.computeNextRotateTime(ctx)
}

func (w *Worker) computeNextRotateTime(ctx context.Context) {
	w.config.Logger.Debugf(ctx, "computing next rotated time for secret backends %#v", w.backendInfo)

	if len(w.backendInfo) == 0 {
		w.timer = nil
		return
	}

	// Find the minimum (next) rotateTime from all the tokens.
	var soonestRotateTime time.Time
	for _, info := range w.backendInfo {
		if !soonestRotateTime.IsZero() && info.rotateTime.After(soonestRotateTime) {
			continue
		}
		soonestRotateTime = info.rotateTime
	}
	// There's no need to start or reset the timer if there's no changes to make.
	if soonestRotateTime.IsZero() || w.nextTrigger == soonestRotateTime {
		return
	}

	// Account for the worker not running when a secret
	// should have been rotated.
	now := w.config.Clock.Now()
	if soonestRotateTime.Before(now) {
		soonestRotateTime = now
	}

	nextDuration := soonestRotateTime.Sub(now)
	w.config.Logger.Debugf(ctx, "next token will rotate in %v at %s", nextDuration, soonestRotateTime)

	w.nextTrigger = soonestRotateTime
	if w.timer == nil {
		w.timer = w.config.Clock.NewTimer(nextDuration)
	} else {
		// See the docs on Timer.Reset() that says it isn't safe to call
		// on a non-stopped channel, and if it is stopped, you need to check
		// if the channel needs to be drained anyway. It isn't safe to drain
		// unconditionally in case another goroutine has already noticed,
		// but make an attempt.
		if !w.timer.Stop() {
			select {
			case <-w.timer.Chan():
			default:
			}
		}
		w.timer.Reset(nextDuration)
	}
}

func (w *Worker) scopeContext() (context.Context, context.CancelFunc) {
	return context.WithCancel(w.catacomb.Context(context.Background()))
}
