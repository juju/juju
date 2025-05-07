// Copyright 2020 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package deployer_test

import (
	"context"
	"time"

	"github.com/juju/tc"
	"github.com/juju/worker/v4"
	"github.com/juju/worker/v4/dependency"

	"github.com/juju/juju/core/logger"
	"github.com/juju/juju/internal/testing"
	"github.com/juju/juju/internal/worker/deployer"
)

func (s *unitWorkersStub) Manifolds(config deployer.UnitManifoldsConfig) dependency.Manifolds {
	return dependency.Manifolds{
		"worker": s.Manifold(config.Agent.CurrentConfig().Tag().Id()),
	}
}

func (s *unitWorkersStub) Manifold(unitName string) dependency.Manifold {
	return dependency.Manifold{
		Inputs: []string{},
		Start: func(ctx context.Context, getter dependency.Getter) (worker.Worker, error) {
			if s.startError != nil {
				return nil, s.startError
			}
			s.logger.Infof(context.TODO(), "manifold start called for %q", unitName)
			w := &unitWorker{
				logger:  s.logger,
				stop:    make(chan struct{}),
				name:    unitName,
				started: s.started,
				stopped: s.stopped,
				waitErr: s.workerError,
			}
			w.start()
			return w, nil
		},
	}
}

type unitWorkersStub struct {
	started chan string
	stopped chan string
	logger  logger.Logger

	// If startError is non-nil, it is returned from the manifold Start func.
	startError error
	// this is the error that is returned from the worker Wait function.
	workerError error
}

func (s *unitWorkersStub) waitForStart(c *tc.C, unitName string) {
	for {
		select {
		case unit := <-s.started:
			if unit == unitName {
				return
			}
			c.Logf("unexpected start %q", unit)
		case <-time.After(testing.LongWait):
			c.Fatalf("unit %q didn't start", unitName)
		}
	}
}

type unitWorker struct {
	logger  logger.Logger
	stop    chan struct{}
	name    string
	started chan<- string
	stopped chan<- string
	waitErr error
}

func (w *unitWorker) start() {
	w.logger.Infof(context.TODO(), "%q start", w.name)
	w.started <- w.name
}

func (w *unitWorker) Kill() {
	w.logger.Infof(context.TODO(), "%q kill", w.name)
	select {
	case <-w.stop:
	default:
		close(w.stop)
	}
	w.stopped <- w.name
}

func (w *unitWorker) Wait() error {
	if w.waitErr != nil {
		return w.waitErr
	}
	<-w.stop
	return nil
}
