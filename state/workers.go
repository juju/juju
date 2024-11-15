// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state

import (
	"context"
	"time"

	"github.com/juju/errors"
	"github.com/juju/pubsub/v2"
	"github.com/juju/worker/v4"

	corelogger "github.com/juju/juju/core/logger"
	internallogger "github.com/juju/juju/internal/logger"
	jworker "github.com/juju/juju/internal/worker"
	"github.com/juju/juju/state/watcher"
)

const (
	txnLogWorker = "txnlog"
)

// workers runs the workers that a State instance requires.
// It wraps a Runner instance which restarts any of the
// workers when they fail.
type workers struct {
	state *State
	*worker.Runner

	hub *pubsub.SimpleHub
}

func newWorkers(st *State, hub *pubsub.SimpleHub) (*workers, error) {
	if hub == nil {
		return nil, errors.NotValidf("missing hub")
	}
	ws := &workers{
		state: st,
		hub:   hub,
		Runner: worker.NewRunner(worker.RunnerParams{
			Logger:       workerLogger{logger: internallogger.GetLogger("juju.state.watcher")},
			IsFatal:      func(err error) bool { return err == jworker.ErrRestartAgent },
			RestartDelay: time.Second,
			Clock:        st.clock(),
		}),
	}
	_ = ws.StartWorker(txnLogWorker, func() (worker.Worker, error) {
		return watcher.NewHubWatcher(watcher.HubWatcherConfig{
			Hub:       hub,
			Clock:     st.clock(),
			ModelUUID: st.ModelUUID(),
			Logger:    internallogger.GetLogger("juju.state.watcher"),
		})
	})
	return ws, nil
}

func (ws *workers) txnLogWatcher() watcher.BaseWatcher {
	w, err := ws.Worker(txnLogWorker, nil)
	if err != nil {
		return watcher.NewDead(errors.Trace(err))
	}
	return w.(watcher.BaseWatcher)
}

type workerLogger struct {
	logger corelogger.Logger
}

func (w workerLogger) Debugf(msg string, args ...interface{}) {
	w.logger.Debugf(context.TODO(), msg, args...)
}

func (w workerLogger) Infof(msg string, args ...interface{}) {
	w.logger.Infof(context.TODO(), msg, args...)
}

func (w workerLogger) Errorf(msg string, args ...interface{}) {
	w.logger.Errorf(context.TODO(), msg, args...)
}
