// Copyright 2023 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package secretbackendrotate

import (
	"fmt"
	"time"

	"github.com/juju/clock"
	"github.com/juju/errors"
	"github.com/juju/worker/v3"
	"github.com/juju/worker/v3/catacomb"

	"github.com/juju/juju/core/secrets"
	"github.com/juju/juju/core/watcher"
	"github.com/juju/juju/internal/worker/secrettriggerschedule"
)

// logger is here to stop the desire of creating a package level logger.
// Don't do this, instead use the one passed as manifold config.
type logger interface{}

var _ logger = struct{}{}

// Logger represents the methods used by the worker to log information.
type Logger interface {
	Infof(string, ...any)
	Debugf(string, ...any)
	Warningf(string, ...any)
}

// SecretBackendManagerFacade instances provide a watcher for secret rotation changes.
type SecretBackendManagerFacade interface {
	WatchTokenRotationChanges() (watcher.SecretBackendRotateWatcher, error)
	RotateBackendTokens(info ...string) error
}

// Config defines the operation of the Worker.
type Config struct {
	SecretBackendManagerFacade SecretBackendManagerFacade
	Logger                     Logger
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
		Site: &w.catacomb,
		Work: w.loop,
	})
	return w, errors.Trace(err)
}

type tokenRotateInfo struct {
	backendName string
	schedule    secrettriggerschedule.TriggerSchedule
}

func (s tokenRotateInfo) GoString() string {
	return fmt.Sprintf(
		"%s token rotation: at %s",
		s.backendName, s.schedule.NextTime.Format(time.RFC3339),
	)
}

// Worker fires events when secrets should be rotated.
type Worker struct {
	catacomb catacomb.Catacomb
	config   Config

	watcher     watcher.SecretBackendRotateWatcher
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
	defer func() {
		secrettriggerschedule.StopTimer(w.timer)
	}()
	if err := w.startWatcher(); err != nil {
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
		case ch, ok := <-w.watcher.Changes():
			if !ok {
				return errors.New("secret rotation change channel closed")
			}
			w.handleTokenRotateChanges(ch)
		case now := <-timeout:
			if err := w.rotate(now); err != nil {
				return errors.Annotatef(err, "rotating secret backends")
			}
		}
	}
}

func (w *Worker) rotate(now time.Time) error {
	w.config.Logger.Debugf("processing secret backend token rotation at %s", now)
	// The active timer has fired and is no longer armed until
	// computeNextRotateTime schedules the next deadline.
	w.nextTrigger = time.Time{}

	var toRotate []string
	for _, info := range w.backendInfo {
		if !info.schedule.NextTime.After(now) && info.schedule.Fired {
			return w.resyncWatcher()
		}
	}
	for id, info := range w.backendInfo {
		w.config.Logger.Debugf(
			"checking %s: rotate at %s... time diff %s",
			id, info.schedule.NextTime, info.schedule.NextTime.Sub(now),
		)
		if info.schedule.NextTime.After(now) {
			continue
		}
		toRotate = append(toRotate, id)
		// Keep the entry awaiting confirmation of the new rotation time via the
		// watcher. If the watcher update is dropped we resync the watcher after
		// RotateRetryDelay. The prevents the watcher from becoming stale.
		info.schedule = info.schedule.AfterFire(w.config.Logger, "secret backend token rotation", info.backendName, secrets.RotateRetryDelay, now)
		w.backendInfo[id] = info
	}

	if len(toRotate) > 0 {
		if err := w.config.SecretBackendManagerFacade.RotateBackendTokens(toRotate...); err != nil {
			return errors.Annotatef(err, "cannot rotate secret backend tokens for backend ids %q", toRotate)
		}
	}
	w.computeNextRotateTime()
	return nil
}

func (w *Worker) resyncWatcher() error {
	w.config.Logger.Warningf("resyncing secret backend token rotation watcher after unconfirmed rotation")
	if w.watcher != nil {
		if err := worker.Stop(w.watcher); err != nil {
			w.config.Logger.Debugf("stopping token rotation watcher for resync: %v", err)
		}
		w.watcher = nil
	}
	clear(w.backendInfo)
	w.computeNextRotateTime()
	return w.startWatcher()
}

func (w *Worker) startWatcher() error {
	changes, err := w.config.SecretBackendManagerFacade.WatchTokenRotationChanges()
	if err != nil {
		return errors.Trace(err)
	}
	if err := w.catacomb.Add(changes); err != nil {
		return errors.Trace(err)
	}
	w.watcher = changes
	return nil
}

func (w *Worker) handleTokenRotateChanges(changes []watcher.SecretBackendRotateChange) {
	w.config.Logger.Debugf("got rotate secret changes: %#v", changes)
	if len(changes) == 0 {
		return
	}

	for _, ch := range changes {
		// Next rotate time of 0 means the rotation has been deleted.
		if ch.NextTriggerTime.IsZero() {
			w.config.Logger.Debugf("token for %q no longer rotated", ch.Name)
			delete(w.backendInfo, ch.ID)
			continue
		}
		if existing, ok := w.backendInfo[ch.ID]; ok && existing.schedule.ShouldIgnoreTime(ch.NextTriggerTime) {
			w.config.Logger.Debugf(
				"ignoring stale token rotation change for %q (next %s)",
				ch.Name, ch.NextTriggerTime,
			)
			continue
		}
		w.backendInfo[ch.ID] = tokenRotateInfo{
			backendName: ch.Name,
			schedule:    secrettriggerschedule.NewTriggerSchedule(ch.NextTriggerTime),
		}
	}
	w.computeNextRotateTime()
}

func (w *Worker) computeNextRotateTime() {
	w.config.Logger.Debugf("computing next rotated time for secret backends %#v", w.backendInfo)

	soonest := secrettriggerschedule.SoonestTime(w.backendInfo, func(info tokenRotateInfo) time.Time {
		return info.schedule.NextTime
	})
	var changed bool
	w.timer, w.nextTrigger, changed = secrettriggerschedule.ScheduleTimer(
		w.config.Clock, w.timer, w.nextTrigger, soonest,
	)
	if changed && w.timer != nil {
		w.config.Logger.Debugf(
			"next token will rotate in %v at %s",
			w.nextTrigger.Sub(w.config.Clock.Now()), w.nextTrigger,
		)
	}
}
