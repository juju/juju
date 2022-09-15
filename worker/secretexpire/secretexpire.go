// Copyright 2022 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package secretexpire

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
	Warningf(string, ...interface{})
}

// SecretManagerFacade instances provide a watcher for secret revision expiry changes.
type SecretManagerFacade interface {
	WatchSecretRevisionsExpiryChanges(ownerTag string) (watcher.SecretTriggerWatcher, error)
}

// Config defines the operation of the Worker.
type Config struct {
	SecretManagerFacade SecretManagerFacade
	Logger              Logger
	Clock               clock.Clock

	SecretOwner     names.Tag
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
	if config.SecretOwner == nil {
		return errors.NotValidf("nil SecretOwner")
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
		config:  config,
		secrets: make(map[string]secretRevisionExpiryInfo),
	}
	err := catacomb.Invoke(catacomb.Plan{
		Site: &w.catacomb,
		Work: w.loop,
	})
	return w, errors.Trace(err)
}

type secretRevisionExpiryInfo struct {
	uri        *secrets.URI
	revision   int
	expireTime time.Time
	retryCount int
}

func (s secretRevisionExpiryInfo) GoString() string {
	return fmt.Sprintf("%s expiry: in %v at %s", expiryKey(s.uri, s.revision), s.expireTime.Sub(time.Now()), s.expireTime.Format(time.RFC3339))
}

// Worker fires events when secret revisions should be expired.
type Worker struct {
	catacomb catacomb.Catacomb
	config   Config

	secrets map[string]secretRevisionExpiryInfo

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
	changes, err := w.config.SecretManagerFacade.WatchSecretRevisionsExpiryChanges(w.config.SecretOwner.String())
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
				return errors.New("secret revision expiry change channel closed")
			}
			w.handleSecretRevisionExpiryChanges(ch)
		case now := <-timeout:
			w.expire(now)
		}
	}
}

func (w *Worker) expire(now time.Time) {
	w.config.Logger.Debugf("processing secret expiry for %q at %s", w.config.SecretOwner, now)

	var toExpire []string
	for id, info := range w.secrets {
		w.config.Logger.Debugf("expire %s at %s... time diff %s", id, info.expireTime, info.expireTime.Sub(now))
		// A one minute granularity is acceptable for secret expiry.
		if info.expireTime.Truncate(time.Minute).Before(now) {
			if info.retryCount > 0 {
				w.config.Logger.Warningf("retry attempt %d to expire secret %q revision %d", info.retryCount, info.uri, info.revision)
			}
			toExpire = append(toExpire, expiryKey(info.uri, info.revision))
			// Once secret revision has been queued for expiry, requeue it
			// a short time later. The charm is expected to delete the revision
			// on expiry; if not, the expire hook will run until it does.
			newInfo := info
			newInfo.expireTime = info.expireTime.Add(secrets.ExpireRetryDelay)
			newInfo.retryCount++
			w.secrets[id] = newInfo
		}
	}

	if len(toExpire) > 0 {
		select {
		case <-w.catacomb.Dying():
			return
		case w.config.ExpireRevisions <- toExpire:
		}
	}
	w.computeNextExpireTime()
}

func expiryKey(uri *secrets.URI, revision int) string {
	return fmt.Sprintf("%s/%d", uri.String(), revision)
}

func (w *Worker) handleSecretRevisionExpiryChanges(changes []watcher.SecretTriggerChange) {
	w.config.Logger.Debugf("got revision expiry secret changes: %#v", changes)
	if len(changes) == 0 {
		return
	}

	for _, ch := range changes {
		// Next trigger time of 0 means the expiry has been deleted.
		if ch.NextTriggerTime.IsZero() {
			w.config.Logger.Debugf("secret revision %d no longer expires: %v", ch.URI.String(), ch.Revision)
			delete(w.secrets, expiryKey(ch.URI, ch.Revision))
			continue
		}
		w.secrets[expiryKey(ch.URI, ch.Revision)] = secretRevisionExpiryInfo{
			uri:        ch.URI,
			revision:   ch.Revision,
			expireTime: ch.NextTriggerTime,
		}
	}
	w.computeNextExpireTime()
}

func (w *Worker) computeNextExpireTime() {
	w.config.Logger.Debugf("computing next expire time for secret revisions %#v", w.secrets)

	if len(w.secrets) == 0 {
		w.timer = nil
		return
	}

	// Find the minimum (next) expireTime from all the secrets.
	var soonestExpireTime time.Time
	for _, info := range w.secrets {
		if !soonestExpireTime.IsZero() && info.expireTime.After(soonestExpireTime) {
			continue
		}
		soonestExpireTime = info.expireTime
	}
	// There's no need to start or reset the timer if there's no changes to make.
	if soonestExpireTime.IsZero() || w.nextTrigger == soonestExpireTime {
		return
	}

	// Account for the worker not running when a secret
	// revision should have been expired.
	now := w.config.Clock.Now()
	if soonestExpireTime.Before(now) {
		soonestExpireTime = now
	}

	nextDuration := soonestExpireTime.Sub(now)
	w.config.Logger.Debugf("next secret revision for %q will expire in %v at %s", w.config.SecretOwner, nextDuration, soonestExpireTime)

	w.nextTrigger = soonestExpireTime
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
