// Copyright 2022 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package secretexpire

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

// errResetRequired is returned when the retries for an expired secret
// revision are exhausted without the controller confirming the expiry.
// The worker exits with it so that it can be replaced by a fresh
// instance that re-reads the expiry state from the controller.
var errResetRequired = errors.ConstError(
	"secret revision expiry watcher reset required")

// Logger represents the methods used by the worker to log information.
type Logger interface {
	Infof(string, ...any)
	Debugf(string, ...any)
	Warningf(string, ...any)
}

// SecretManagerFacade instances provide a watcher for secret revision expiry changes.
type SecretManagerFacade interface {
	WatchSecretRevisionsExpiryChanges(ownerTags ...names.Tag) (watcher.SecretTriggerWatcher, error)
}

// Config defines the operation of the Worker.
type Config struct {
	SecretManagerFacade SecretManagerFacade
	Logger              Logger
	Clock               clock.Clock

	SecretOwners    []names.Tag
	ExpireRevisions chan<- []string
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
	if config.ExpireRevisions == nil {
		return errors.NotValidf("nil ExpireRevisionsChannel")
	}
	return nil
}

// New returns a Secret Expiry Worker backed by config, or an error.
func New(config Config) (worker.Worker, error) {
	if err := config.Validate(); err != nil {
		return nil, errors.Trace(err)
	}

	w := &Worker{
		config:          config,
		secretRevisions: make(map[string]secretRevisionExpiryInfo),
	}
	err := catacomb.Invoke(catacomb.Plan{
		Site: &w.catacomb,
		Work: w.loop,
	})
	return w, errors.Trace(err)
}

// NewWithReset returns a worker that runs the secret expiry worker
// described by config, restarting it after a short delay whenever it
// exits with errResetRequired. Any other error is fatal and is
// returned from Wait.
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
	if err := r.StartWorker("secret-expiry", func() (worker.Worker, error) {
		return New(config)
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

type secretRevisionExpiryInfo struct {
	uri      *secrets.URI
	revision int
	schedule secrettriggerschedule.TriggerSchedule
}

func (s secretRevisionExpiryInfo) GoString() string {
	return fmt.Sprintf(
		"%s expiry: at %s",
		expiryKey(s.uri, s.revision), s.schedule.NextTime.Format(time.RFC3339),
	)
}

// Worker fires events when secret revisions should be expired.
type Worker struct {
	catacomb catacomb.Catacomb
	config   Config

	watcher         watcher.SecretTriggerWatcher
	secretRevisions map[string]secretRevisionExpiryInfo

	alarm       clock.Alarm
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
		secrettriggerschedule.Stop(w.alarm)
	}()
	if err := w.startWatcher(); err != nil {
		return errors.Trace(err)
	}
	for {
		var timeout <-chan time.Time
		if w.alarm != nil {
			timeout = w.alarm.Chan()
		}
		select {
		case <-w.catacomb.Dying():
			return w.catacomb.ErrDying()
		case ch, ok := <-w.watcher.Changes():
			if !ok {
				return errors.New("secret revision expiry change channel closed")
			}
			w.handleSecretRevisionExpiryChanges(ch)
		case now := <-timeout:
			if err := w.expire(now); err != nil {
				return errors.Trace(err)
			}
		}
	}
}

func (w *Worker) expire(now time.Time) error {
	w.config.Logger.Debugf("processing secret expiry for %q at %s", w.config.SecretOwners, now)
	// The active alarm has fired and is no longer armed until
	// computeNextExpireTime schedules the next deadline.
	w.nextTrigger = time.Time{}

	var toExpire []string
	var resetRequired bool
	for id, info := range w.secretRevisions {
		w.config.Logger.Debugf(
			"expire %s at %s... time diff %s",
			id, info.schedule.NextTime, info.schedule.NextTime.Sub(now),
		)
		if info.schedule.NextTime.After(now) {
			continue
		}
		if info.schedule.RetriesExhausted() {
			// Record the reset so the worker exits with errResetRequired
			// below, but keep processing any other due entries so
			// their expiries are not discarded.
			resetRequired = true
			w.config.Logger.Warningf(
				"restarting secret expiry watcher for %q after %d retries",
				w.config.SecretOwners, secrettriggerschedule.MaxTriggerRetries,
			)
			continue
		}
		key := expiryKey(info.uri, info.revision)
		toExpire = append(toExpire, key)
		// Once secret revision has been queued for expiry, requeue it
		// every ExpireRetryDelay up to MaxTriggerRetries attempts. The
		// charm is expected to delete the revision on expiry. If not,
		// exit with errResetRequired once MaxTriggerRetries is
		// exhausted. Restarting with a fresh watcher guards against
		// the watcher becoming stale and re-reads the expiry state
		// from the controller.
		info.schedule = info.schedule.AfterFire(w.config.Logger, "secret revision expiry", key, secrets.ExpireRetryDelay, now)
		w.secretRevisions[id] = info
	}
	if len(toExpire) > 0 {
		select {
		case <-w.catacomb.Dying():
			return nil
		case w.config.ExpireRevisions <- toExpire:
		}
	}
	if resetRequired {
		return errResetRequired
	}
	w.computeNextExpireTime()
	return nil
}

func (w *Worker) startWatcher() error {
	changes, err := w.config.SecretManagerFacade.WatchSecretRevisionsExpiryChanges(w.config.SecretOwners...)
	if err != nil {
		return errors.Trace(err)
	}
	if err := w.catacomb.Add(changes); err != nil {
		return errors.Trace(err)
	}
	w.watcher = changes
	return nil
}

func (w *Worker) handleSecretRevisionExpiryChanges(changes []watcher.SecretTriggerChange) {
	w.config.Logger.Debugf("got revision expiry secret changes: %#v", changes)
	if len(changes) == 0 {
		return
	}

	for _, ch := range changes {
		key := expiryKey(ch.URI, ch.Revision)
		// Next trigger time of 0 means the expiry has been deleted.
		if ch.NextTriggerTime.IsZero() {
			w.config.Logger.Debugf("secret revision no longer expires: %s", key)
			delete(w.secretRevisions, key)
			continue
		}
		if existing, ok := w.secretRevisions[key]; ok && existing.schedule.ShouldIgnoreTime(ch.NextTriggerTime) {
			w.config.Logger.Debugf(
				"ignoring stale expiry change for %s (next %s)",
				key, ch.NextTriggerTime,
			)
			continue
		}
		w.secretRevisions[key] = secretRevisionExpiryInfo{
			uri:      ch.URI,
			revision: ch.Revision,
			schedule: secrettriggerschedule.NewTriggerSchedule(ch.NextTriggerTime),
		}
	}
	w.computeNextExpireTime()
}

func expiryKey(uri *secrets.URI, revision int) string {
	return fmt.Sprintf("%s/%d", uri.ID, revision)
}

func (w *Worker) computeNextExpireTime() {
	w.config.Logger.Debugf("computing next expire time for secret revisions %#v", w.secretRevisions)

	soonest := secrettriggerschedule.SoonestTime(w.secretRevisions, func(info secretRevisionExpiryInfo) time.Time {
		return info.schedule.NextTime
	})
	var changed bool
	w.alarm, w.nextTrigger, changed = secrettriggerschedule.ScheduleAlarm(
		w.config.Clock, w.alarm, w.nextTrigger, soonest,
	)
	if changed && w.alarm != nil {
		w.config.Logger.Debugf("next secret revision for %q will expire at %s", w.config.SecretOwners, w.nextTrigger)
	}
}
