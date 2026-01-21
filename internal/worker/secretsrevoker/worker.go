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
	quantTerm = time.Minute
)

type Logger interface {
	Debugf(string, ...any)
	Warningf(string, ...any)
	Infof(string, ...any)
}

type SecretsRevokerFacade interface {
	WatchIssuedTokenExpiry() (watcher.StringsWatcher, error)
	RevokeIssuedTokens(until time.Time) (time.Time, error)
}

type Config struct {
	Facade SecretsRevokerFacade
	Logger Logger
	Clock  clock.Clock
}

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
	return nil
}

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

func (w *revoker) loop() (err error) {
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
			return errors.Trace(w.catacomb.ErrDying())
		case changes, ok := <-watcher.Changes():
			if !ok {
				return errors.New("secret issued token expiry watcher closed")
			}
			nextChanged := false
			for _, v := range changes {
				ts, err := time.Parse(time.RFC3339, v)
				if err != nil {
					w.config.Logger.Warningf(
						"invalid issued token expiry time: %v", err)
					continue
				}
				tq := ts.Truncate(quantTerm).Add(quantTerm)
				if next.IsZero() || next.After(tq) {
					next = tq
					nextChanged = true
				}
			}
			if nextChanged {
				if alarm == nil {
					alarm = w.config.Clock.NewAlarm(next)
					fire = alarm.Chan()
				} else {
					alarm.Reset(next)
				}
			}
		case <-fire:
			nq := w.config.Clock.Now().Truncate(quantTerm).Add(quantTerm)
			nextRevoke, err := w.config.Facade.RevokeIssuedTokens(nq)
			if err != nil {
				return errors.Annotate(err, "failed to revoke tokens")
			}
			if nextRevoke.IsZero() {
				continue
			}
			next = nextRevoke.Truncate(quantTerm).Add(quantTerm)
			alarm.Reset(next)
		}
	}
}
