// Copyright 2021 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package caasunitterminationworker

import (
	"os"
	"os/signal"
	"syscall"

	"github.com/juju/clock"
	"github.com/juju/errors"
	"github.com/juju/names/v5"
	"github.com/juju/worker/v4"
	"gopkg.in/tomb.v2"

	"github.com/juju/juju/agent"
	"github.com/juju/juju/api/agent/caasapplication"
)

// TerminationSignal is SIGTERM which is sent by most container runtimes when
// a container should terminate gracefully.
const TerminationSignal = syscall.SIGTERM

type terminationWorker struct {
	tomb tomb.Tomb

	agent          agent.Agent
	state          State
	unitTerminator UnitTerminator
	logger         Logger
	clock          clock.Clock
}

type Config struct {
	Agent          agent.Agent
	State          State
	UnitTerminator UnitTerminator
	Logger         Logger
	Clock          clock.Clock
}

type State interface {
	UnitTerminating(tag names.UnitTag) (caasapplication.UnitTermination, error)
}

type UnitTerminator interface {
	Terminate() error
}

// NewWorker returns a worker that waits for a
// TerminationSignal signal, and then exits
// with worker.ErrTerminateAgent.
func NewWorker(config Config) worker.Worker {
	w := terminationWorker{
		agent:          config.Agent,
		state:          config.State,
		unitTerminator: config.UnitTerminator,
		logger:         config.Logger,
		clock:          config.Clock,
	}
	c := make(chan os.Signal, 1)
	signal.Notify(c, TerminationSignal)
	w.tomb.Go(func() error {
		defer signal.Stop(c)
		return w.loop(c)
	})
	return &w
}

func (w *terminationWorker) Kill() {
	w.tomb.Kill(nil)
}

func (w *terminationWorker) Wait() error {
	return w.tomb.Wait()
}

func (w *terminationWorker) loop(c <-chan os.Signal) (err error) {
	select {
	case <-c:
		w.logger.Infof("terminating due to SIGTERM")
		term, err := w.state.UnitTerminating(w.agent.CurrentConfig().Tag().(names.UnitTag))
		if err != nil {
			w.logger.Errorf("error while terminating unit: %v", err)
			return err
		}
		if !term.WillRestart {
			// Lifecycle watcher will handle termination of the unit.
			return nil
		}
		err = w.unitTerminator.Terminate()
		if err != nil {
			w.logger.Errorf("error while terminating unit: %v", err)
			return errors.Annotatef(err, "failed to terminate unit agent worker")
		}
		return nil
	case <-w.tomb.Dying():
		return tomb.ErrDying
	}
}
