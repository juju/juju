// Copyright 2021 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package secretrotate

import (
	"fmt"
	"time"

	"github.com/juju/clock"
	"github.com/juju/errors"
	"github.com/juju/names/v4"
	"github.com/juju/worker/v3"
	"github.com/juju/worker/v3/catacomb"

	"github.com/juju/juju/core/secrets"
	"github.com/juju/juju/core/watcher"
)

// logger is here to stop the desire of creating a package level logger.
// Don't do this, instead use the one passed as manifold config.
type logger interface{}

var _ logger = struct{}{}

// Logger represents the methods used by the worker to log information.
type Logger interface {
	Debugf(string, ...interface{})
}

// SecretManagerFacade instances provide a watcher for secret rotation changes.
type SecretManagerFacade interface {
	WatchSecretsRotationChanges(ownerTag string) (watcher.SecretTriggerWatcher, error)
}

// Config defines the operation of the Worker.
type Config struct {
	SecretManagerFacade SecretManagerFacade
	Logger              Logger
	Clock               clock.Clock

	SecretOwner   names.Tag
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
	if config.SecretOwner == nil {
		return errors.NotValidf("nil SecretOwner")
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
	URI        *secrets.URI
	rotateTime time.Time
}

func (s secretRotateInfo) GoString() string {
	return fmt.Sprintf("%s rotation: in %v at %s", s.URI.ID, s.rotateTime.Sub(time.Now()), s.rotateTime.Format(time.RFC3339))
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
	changes, err := w.config.SecretManagerFacade.WatchSecretsRotationChanges(w.config.SecretOwner.String())
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
			w.handleSecretRotateChanges(ch)
		case now := <-timeout:
			w.rotate(now)
		}
	}
}

func (w *Worker) rotate(now time.Time) {
	w.config.Logger.Debugf("processing secret rotation for %q at %s", w.config.SecretOwner, now)

	var toRotate []string
	for id, info := range w.secrets {
		w.config.Logger.Debugf("rotate %s at %s... time diff %s", id, info.rotateTime, info.rotateTime.Sub(now))
		// A one minute granularity is acceptable for secret rotation.
		if info.rotateTime.Truncate(time.Minute).Before(now) {
			toRotate = append(toRotate, info.URI.ShortString())
			// Once secret has been queued for rotation, delete it here since
			// it will re-appear via the watcher after the rotation is actually
			// performed and the last rotated time is updated.
			delete(w.secrets, id)
		}
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
			w.config.Logger.Debugf("secret no longer rotated: %v", ch.URI.ShortString())
			delete(w.secrets, ch.URI.ID)
			continue
		}
		w.secrets[ch.URI.ID] = secretRotateInfo{
			URI:        ch.URI,
			rotateTime: ch.NextTriggerTime,
		}
	}
	w.computeNextRotateTime()
}

func (w *Worker) computeNextRotateTime() {
	w.config.Logger.Debugf("computing next rotated time for secrets %#v", w.secrets)

	if len(w.secrets) == 0 {
		w.timer = nil
		return
	}

	// Find the minimum (next) rotateTime from all the secrets.
	var soonestRotateTime time.Time
	for _, info := range w.secrets {
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
	w.config.Logger.Debugf("next secret for %q will rotate in %v at %s", w.config.SecretOwner, nextDuration, soonestRotateTime)

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
