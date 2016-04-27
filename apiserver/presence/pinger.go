// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package presence

import (
	"fmt"
	"time"

	"github.com/juju/errors"
	"github.com/juju/juju/worker"
	"github.com/juju/juju/worker/catacomb"
	"github.com/juju/loggo"
	"github.com/juju/names"
	"github.com/juju/utils/clock"
)

// Pinger exposes some methods implemented by state/presence.Pinger.
type Pinger interface {
	// Stop kills the pinger, then waits for it to exit.
	Stop() error
	// Wait waits for the pinger to stop.
	Wait() error
}

// Config contains the information necessary to drive a Worker.
type Config struct {

	// Identity records the entity whose connectedness is being
	// affirmed by this worker. It's used to create a logger that
	// can let us see which agent's pinger is actually failing.
	Identity names.Tag

	// Start starts a new, running Pinger or returns an error.
	Start func() (Pinger, error)

	// Clock is used to throttle failed Start attempts.
	Clock clock.Clock

	// RetryDelay controls by how much we throttle failed Start
	// attempts. Note that we only apply the delay when a Start
	// fails; if a Pinger ran, however briefly, we'll try to restart
	// it immediately, so as to minimise the changes of erroneously
	// causing agent-lost to be reported.
	RetryDelay time.Duration
}

// Validate returns an error if Config cannot be expected to drive a
// Worker.
func (config Config) Validate() error {
	if config.Identity == nil {
		return errors.NotValidf("nil Identity")
	}
	if config.Start == nil {
		return errors.NotValidf("nil Start")
	}
	if config.Clock == nil {
		return errors.NotValidf("nil Clock")
	}
	if config.RetryDelay <= 0 {
		return errors.NotValidf("non-positive RetryDelay")
	}
	return nil
}

// New returns a Worker backed by Config. The caller is responsible for
// Kill()ing the Worker and handling any errors returned from Wait();
// but as it happens it's designed to be an apiserver/common.Resource,
// and never to exit unless Kill()ed, so in practice Stop(), which will
// call Kill() and Wait() internally, is Good Enough.
func New(config Config) (*Worker, error) {
	if err := config.Validate(); err != nil {
		return nil, errors.Trace(err)
	}
	name := fmt.Sprintf("juju.apiserver.presence.%s", config.Identity)
	w := &Worker{
		config:  config,
		logger:  loggo.GetLogger(name),
		running: make(chan struct{}),
	}
	err := catacomb.Invoke(catacomb.Plan{
		Site: &w.catacomb,
		Work: w.loop,
	})
	if err != nil {
		return nil, errors.Trace(err)
	}

	// To support unhappy assumptions in apiserver/server_test.go,
	// we block New until at least one attempt to start a Pinger
	// has been made. This preserves the apparent behaviour of an
	// unwrapped Pinger under normal conditions.
	select {
	case <-w.catacomb.Dying():
		if err := w.Wait(); err != nil {
			return nil, errors.Trace(err)
		}
		return nil, errors.New("worker stopped abnormally without reporting an error")
	case <-w.running:
		return w, nil
	}
}

// Worker creates a Pinger as configured, and recreates it as it fails
// until the Worker is stopped; at which point it shuts down any extant
// Pinger before returning.
type Worker struct {
	catacomb catacomb.Catacomb
	config   Config
	logger   loggo.Logger
	running  chan struct{}
}

// Kill is part of the worker.Worker interface.
func (w *Worker) Kill() {
	w.catacomb.Kill(nil)
}

// Wait is part of the worker.Worker interface.
func (w *Worker) Wait() error {
	return w.catacomb.Wait()
}

// Stop is part of the apiserver/common.Resource interface.
//
// It's not a very good idea -- see comments on lp:1572237 -- but we're
// only addressing the proximate cause of the issue here.
func (w *Worker) Stop() error {
	return worker.Stop(w)
}

// loop runs Pingers until w is stopped.
func (w *Worker) loop() error {
	var delay time.Duration
	for {
		select {
		case <-w.catacomb.Dying():
			return w.catacomb.ErrDying()
		case <-w.config.Clock.After(delay):
			maybePinger := w.maybeStartPinger()
			w.reportRunning()
			w.waitPinger(maybePinger)
		}
		delay = w.config.RetryDelay
	}
}

// maybeStartPinger starts and returns a new Pinger; or, if it
// encounters an error, logs it and returns nil.
func (w *Worker) maybeStartPinger() Pinger {
	w.logger.Tracef("starting pinger...")
	pinger, err := w.config.Start()
	if err != nil {
		w.logger.Errorf("cannot start pinger: %v", err)
		return nil
	}
	w.logger.Tracef("pinger started")
	return pinger
}

// reportRunning is a foul hack designed to delay apparent worker start
// until at least one ping has been delivered (or attempted). It only
// exists to make various distant tests, which should ideally not be
// depending on these implementation details, reliable.
func (w *Worker) reportRunning() {
	select {
	case <-w.running:
	default:
		close(w.running)
	}
}

// waitPinger waits for the death of either the pinger or the worker;
// stops the pinger if necessary; and returns once the pinger is
// finished. If pinger is nil, it returns immediately.
func (w *Worker) waitPinger(pinger Pinger) {
	if pinger == nil {
		return
	}

	// Set up a channel that will last as long as this method call.
	done := make(chan struct{})
	defer close(done)

	// Start a goroutine to stop the Pinger if the worker is killed.
	// If the enclosing method completes, we know that the Pinger
	// has already stopped, and we can return immediately.
	//
	// Note that we ignore errors out of Stop(), depending on the
	// Pinger to manage errors properly and report them via Wait()
	// below.
	go func() {
		select {
		case <-done:
		case <-w.catacomb.Dying():
			w.logger.Tracef("stopping pinger")
			pinger.Stop()
		}
	}()

	// Now, just wait for the Pinger to stop. It might be caused by
	// the Worker's death, or it might have failed on its own; in
	// any case, errors are worth recording, but we don't need to
	// respond in any way because that's loop()'s responsibility.
	w.logger.Tracef("waiting for pinger...")
	if err := pinger.Wait(); err != nil {
		w.logger.Errorf("pinger failed: %v", err)
	}
}
