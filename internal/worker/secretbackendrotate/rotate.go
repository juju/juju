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

// errResetRequired is returned when a rotation's confirmation window
// elapses without an updated trigger time from the controller. The
// worker exits with it so that the runner created by NewWithReset can
// replace it with a fresh instance that re-reads the rotation state
// from the controller.
var errResetRequired = errors.ConstError(
	"secret backend token rotation watcher reset required")

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

// NewWithReset returns a worker that runs the secret backend token
// rotation worker described by config, restarting it after a short
// delay whenever it exits with errResetRequired. Any other error is
// fatal and is returned from Wait.
func NewWithReset(config Config) (worker.Worker, error) {
	if err := config.Validate(); err != nil {
		return nil, errors.Trace(err)
	}
	r := worker.NewRunner(worker.RunnerParams{
		Clock: config.Clock,
		IsFatal: func(err error) bool {
			return !errors.Is(err, errResetRequired)
		},
		RestartDelay: worker.DefaultRestartDelay,
		Logger:       runnerLogger{config.Logger},
	})
	if err := r.StartWorker("secret-backend-token-rotation", func() (worker.Worker, error) {
		return NewWorker(config)
	}); err != nil {
		_ = worker.Stop(r)
		return nil, errors.Trace(err)
	}
	return r, nil
}

// runnerLogger adapts the package Logger to the runner logger
// interface; error-level runner logs are emitted as warnings.
type runnerLogger struct {
	Logger
}

// Errorf is part of the logger interface accepted by worker.NewRunner.
func (l runnerLogger) Errorf(format string, args ...any) {
	l.Logger.Warningf(format, args...)
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
		secrettriggerschedule.Stop(w.timer)
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
	var resetRequired bool
	for id, info := range w.backendInfo {
		w.config.Logger.Debugf(
			"checking %s: rotate at %s... time diff %s",
			id, info.schedule.NextTime, info.schedule.NextTime.Sub(now),
		)
		if info.schedule.NextTime.After(now) {
			continue
		}
		if info.schedule.Fired {
			// Record the reset so the worker exits with errResetRequired
			// below, but keep processing any other due entries so
			// their rotations are not discarded.
			resetRequired = true
			w.config.Logger.Warningf(
				"restarting secret backend token rotation watcher for %q",
				info.backendName,
			)
			continue
		}
		toRotate = append(toRotate, id)
		// Keep the entry awaiting confirmation of the new rotation time via
		// the watcher. If the confirmation does not arrive within
		// RotateRetryDelay, exit with errResetRequired so the runner
		// created by NewWithReset restarts the worker: a fresh watcher
		// re-reads the rotation state from the controller, preventing
		// the watcher from becoming stale.
		info.schedule = info.schedule.AfterFire(w.config.Logger, "secret backend token rotation", info.backendName, secrets.RotateRetryDelay, now)
		w.backendInfo[id] = info
	}

	if len(toRotate) > 0 {
		if err := w.config.SecretBackendManagerFacade.RotateBackendTokens(toRotate...); err != nil {
			return errors.Annotatef(err, "cannot rotate secret backend tokens for backend ids %q", toRotate)
		}
	}
	if resetRequired {
		return errResetRequired
	}
	w.computeNextRotateTime()
	return nil
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
