// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package secretsrevoker

import (
	"time"

	"github.com/juju/clock"
	"github.com/juju/errors"
	"github.com/juju/worker/v3"
	"github.com/juju/worker/v3/catacomb"

	"github.com/juju/juju/core/watcher"
)

const (
	// quantTerm is the default quantisation term used in the default time
	// quantisation function.
	quantTerm = time.Minute
)

// Logger is a logger interface.
type Logger interface {
	Debugf(string, ...any)
	Warningf(string, ...any)
	Infof(string, ...any)
}

// SecretsRevokerFacade is used by the secrets revoker to watch and act on the
// expiry of secret backend issued tokens.
type SecretsRevokerFacade interface {
	WatchIssuedTokenExpiry() (watcher.StringsWatcher, error)
	RevokeIssuedTokens(until time.Time) (time.Time, error)
}

// QuantiseTimeFunc is used to pass the secrets revoker worker a quantisation
// function for time.
type QuantiseTimeFunc func(time.Time) time.Time

// Config is the configuration for the secrets revoker worker.
type Config struct {
	Facade       SecretsRevokerFacade
	Logger       Logger
	Clock        clock.Clock
	QuantiseTime QuantiseTimeFunc
}

// Validate returns an error when the config is invalid.
func (config Config) Validate() error {
	if config.Facade == nil {
		return errors.NotValidf("nil Facade")
	}
	if config.Logger == nil {
		return errors.NotValidf("nil Logger")
	}
	if config.Clock == nil {
		return errors.NotValidf("nil Clock")
	}
	if config.QuantiseTime == nil {
		return errors.NotValidf("nil QuantiseTime")
	}
	return nil
}

// DefaultQuantiseTime is the default time quantisation function for the secret
// revoker worker's scheduler.
func DefaultQuantiseTime(t time.Time) time.Time {
	return t.Truncate(quantTerm).Add(quantTerm)
}

// NewWorker returns a new secrets revoker worker that is responsible for
// revoking secret backend issued tokens when they expire.
func NewWorker(config Config) (worker.Worker, error) {
	if err := config.Validate(); err != nil {
		return nil, errors.Trace(err)
	}

	w := &revoker{config: config}
	err := catacomb.Invoke(catacomb.Plan{
		Site: &w.catacomb,
		Work: w.loop,
	})
	return w, errors.Trace(err)
}

// revoker is the secrets revoker worker.
type revoker struct {
	catacomb catacomb.Catacomb
	config   Config
}

// Kill fulfills worker.Worker.
func (w *revoker) Kill() {
	w.catacomb.Kill(nil)
}

// Wait fulfills worker.Worker.
func (w *revoker) Wait() error {
	return w.catacomb.Wait()
}

// loop handles watching for the expiry of secret backend issued tokens and
// scheduling in the future a time when to attempt to revoke those secret
// backend issued tokens.
func (w *revoker) loop() (err error) {
	logger := w.config.Logger
	clk := w.config.Clock
	quantiseTime := w.config.QuantiseTime

	watcher, err := w.config.Facade.WatchIssuedTokenExpiry()
	if err != nil {
		return errors.Trace(err)
	}
	if err := w.catacomb.Add(watcher); err != nil {
		return errors.Trace(err)
	}

	var (
		alarm clock.Alarm
		next  time.Time
		fire  <-chan time.Time
	)
	for {
		select {
		case <-w.catacomb.Dying():
			if !next.IsZero() {
				logger.Warningf("revoker dying with scheduled token revocations")
			}
			return errors.Trace(w.catacomb.ErrDying())
		case changes, ok := <-watcher.Changes():
			if !ok {
				return errors.New("secret issued token expiry watcher closed")
			}
			if len(changes) == 0 {
				continue
			}
			earliest := next
			for _, v := range changes {
				ts, err := time.Parse(time.RFC3339, v)
				if err != nil {
					logger.Warningf("invalid issued token expiry time: %v", err)
					continue
				}
				if earliest.IsZero() || earliest.After(ts) {
					earliest = ts
				}
			}
			if earliest.IsZero() {
				continue
			}
			earliestQuantised := quantiseTime(earliest)
			if !next.Equal(earliestQuantised) {
				next = earliestQuantised
				logger.Debugf("scheduling revoke at %v", next)
				if alarm == nil {
					alarm = clk.NewAlarm(next)
					fire = alarm.Chan()
				} else {
					alarm.Reset(next)
				}
			}
		case <-fire:
			logger.Debugf("revoking issued tokens until %v", next)
			nextRevoke, err := w.config.Facade.RevokeIssuedTokens(next)
			if err != nil {
				return errors.Annotate(err, "failed to revoke tokens")
			}
			if nextRevoke.IsZero() {
				logger.Debugf("sleeping until token expiry trigger")
				next = time.Time{}
				continue
			}
			next = quantiseTime(nextRevoke)
			logger.Debugf("scheduling revoke at %v", next)
			alarm.Reset(next)
		}
	}
}
