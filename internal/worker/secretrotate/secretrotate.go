// Copyright 2021 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package secretrotate

import (
	"fmt"
	"time"

	"github.com/juju/clock"
	"github.com/juju/errors"
	"github.com/juju/names/v5"
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
	Infof(string, ...interface{})
	Debugf(string, ...interface{})
	Warningf(string, ...interface{})
}

// SecretManagerFacade instances provide a watcher for secret rotation changes.
type SecretManagerFacade interface {
	WatchSecretsRotationChanges(ownerTags ...names.Tag) (watcher.SecretTriggerWatcher, error)
}

// Config defines the operation of the Worker.
type Config struct {
	SecretManagerFacade SecretManagerFacade
	Logger              Logger
	Clock               clock.Clock

	SecretOwners  []names.Tag
	RotateSecrets chan<- []string
}

// Validate returns an error if config cannot drive the Worker.
func (config Config) Validate() error {
	if config.SecretManagerFacade == nil {
		return errors.NotValidf("nil Facade")
	}
	if config.Clock == nil {
		return errors.NotValidf("nil Clock")
	}
	if config.Logger == nil {
		return errors.NotValidf("nil Logger")
	}
	if len(config.SecretOwners) == 0 {
		return errors.NotValidf("empty SecretOwners")
	}
	if config.RotateSecrets == nil {
		return errors.NotValidf("nil RotateSecretsChannel")
	}
	return nil
}

// New returns a Secret Rotation Worker backed by config, or an error.
func New(config Config) (worker.Worker, error) {
	if err := config.Validate(); err != nil {
		return nil, errors.Trace(err)
	}

	w := &Worker{
		config:  config,
		secrets: make(map[string]secretRotateInfo),
	}
	err := catacomb.Invoke(catacomb.Plan{
		Site: &w.catacomb,
		Work: w.loop,
	})
	return w, errors.Trace(err)
}

type secretRotateInfo struct {
	URI      *secrets.URI
	schedule secrettriggerschedule.TriggerSchedule
}

func (s secretRotateInfo) GoString() string {
	return fmt.Sprintf(
		"%s rotation: at %s",
		s.URI.ID, s.schedule.NextTime.Format(time.RFC3339),
	)
}

// Worker fires events when secrets should be rotated.
type Worker struct {
	catacomb catacomb.Catacomb
	config   Config

	secrets map[string]secretRotateInfo

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
	changes, err := w.config.SecretManagerFacade.WatchSecretsRotationChanges(w.config.SecretOwners...)
	if err != nil {
		return errors.Trace(err)
	}
	if err := w.catacomb.Add(changes); err != nil {
		return errors.Trace(err)
	}
	for {
		var timeToRotate <-chan time.Time
		if w.timer != nil {
			timeToRotate = w.timer.Chan()
		}
		select {
		case <-w.catacomb.Dying():
			return w.catacomb.ErrDying()
		case ch, ok := <-changes.Changes():
			if !ok {
				return errors.New("secret rotation change channel closed")
			}
			w.handleSecretRotateChanges(ch)
		case now := <-timeToRotate:
			w.rotate(now)
		}
	}
}

func (w *Worker) rotate(now time.Time) {
	w.config.Logger.Debugf("processing secret rotation for %q at %s", w.config.SecretOwners, now)
	// The active timer has fired and is no longer armed until
	// computeNextRotateTime schedules the next deadline.
	w.nextTrigger = time.Time{}

	var toRotate []string
	for id, info := range w.secrets {
		w.config.Logger.Debugf(
			"checking %s: rotate at %s... time diff %s",
			id, info.schedule.NextTime, info.schedule.NextTime.Sub(now),
		)
		if info.schedule.NextTime.After(now) {
			continue
		}
		toRotate = append(toRotate, info.URI.String())
		// Keep the entry until the controller confirms the new
		// rotation time via the watcher. If that update is dropped,
		// retry with exponential backoff up to MaxTriggerRetryDelay
		// rather than losing the schedule until the worker is restarted.
		info.schedule = secrettriggerschedule.AfterFire(info.schedule, w.config.Logger, "secret rotation", info.URI.String(), secrets.RotateRetryDelay, now)
		w.secrets[id] = info
	}

	if len(toRotate) > 0 {
		select {
		case <-w.catacomb.Dying():
			return
		case w.config.RotateSecrets <- toRotate:
		}
	}
	w.computeNextRotateTime()
}

func (w *Worker) handleSecretRotateChanges(changes []watcher.SecretTriggerChange) {
	w.config.Logger.Debugf("got rotate secret changes: %#v", changes)
	if len(changes) == 0 {
		return
	}

	for _, ch := range changes {
		// Next rotate time of 0 means the rotation has been deleted.
		if ch.NextTriggerTime.IsZero() {
			w.config.Logger.Debugf("secret no longer rotated: %v", ch.URI.String())
			delete(w.secrets, ch.URI.ID)
			continue
		}
		if existing, ok := w.secrets[ch.URI.ID]; ok && existing.schedule.ShouldIgnoreTime(ch.NextTriggerTime) {
			w.config.Logger.Debugf(
				"ignoring stale secret rotation change for %s (next %s)",
				ch.URI.ID, ch.NextTriggerTime,
			)
			continue
		}
		w.secrets[ch.URI.ID] = secretRotateInfo{
			URI:      ch.URI,
			schedule: secrettriggerschedule.NewTriggerSchedule(ch.NextTriggerTime),
		}
	}
	w.computeNextRotateTime()
}

func (w *Worker) computeNextRotateTime() {
	w.config.Logger.Debugf("computing next rotated time for secrets %#v", w.secrets)

	soonest := secrettriggerschedule.SoonestTime(w.secrets, func(info secretRotateInfo) time.Time {
		return info.schedule.NextTime
	})
	var nextDuration time.Duration
	var changed bool
	w.timer, w.nextTrigger, nextDuration, changed = secrettriggerschedule.ScheduleTimer(
		w.config.Clock, w.timer, w.nextTrigger, soonest,
	)
	if changed && w.timer != nil {
		w.config.Logger.Debugf(
			"next secret for %q will rotate in %v at %s",
			w.config.SecretOwners, nextDuration, w.nextTrigger,
		)
	}
}
