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
		config: config,
		logger: loggo.GetLogger(name),
	}
	ready := make(chan struct{})
	err := catacomb.Invoke(catacomb.Plan{
		Site: &w.catacomb,
		Work: func() error {
			// Run once to prime presence before diving into the loop.
			pinger := w.startPinger()
			if ready != nil {
				close(ready)
				ready = nil
			}
			if pinger != nil {
				w.waitOnPinger(pinger)
			}
			return w.loop()
		},
	})
	if err != nil {
		return nil, errors.Trace(err)
	}
	<-ready
	return w, nil
}

// Worker creates a Pinger as configured, and recreates it as it fails
// until the Worker is stopped; at which point it shuts down any extant
// Pinger before returning.
type Worker struct {
	catacomb catacomb.Catacomb
	config   Config
	logger   loggo.Logger
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
	clock := w.config.Clock
	for {
		select {
		case <-w.catacomb.Dying():
			return w.catacomb.ErrDying()
		case <-clock.After(delay):
			delay = 0
			pinger := w.startPinger()
			if pinger == nil {
				// Failed to start.
				delay = w.config.RetryDelay
				continue
			}
			w.waitOnPinger(pinger)
		}
	}
}

// startPinger starts a single Pinger. It returns nil if the pinger
// could not be started.
func (w *Worker) startPinger() Pinger {
	w.logger.Debugf("starting pinger...")
	pinger, err := w.config.Start()
	if err != nil {
		w.logger.Errorf("pinger failed to start: %v", err)
		return nil
	}
	w.logger.Debugf("pinger started")
	return pinger
}

// waitOnPinger waits indefinitely for the given Pinger to complete,
// stopping it only when the Worker is Kill()ed.
func (w *Worker) waitOnPinger(pinger Pinger) {
	// Start a goroutine that waits for the Worker to be stopped,
	// and then stops the Pinger.  Note also that we ignore errors
	// out of Stop(): they will be caught by the Pinger anyway, and
	// we'll see them come out of Wait() below.
	go func() {
		<-w.catacomb.Dying()
		pinger.Stop()
	}()

	// Now, just wait for the Pinger to stop. It might be caused by
	// the Worker's death, or it might have failed on its own; in
	// any case, errors are worth recording, but we don't need to
	// respond in any way because that's loop()'s responsibility.
	if err := pinger.Wait(); err != nil {
		w.logger.Errorf("pinger failed: %v", err)
	}
}
