// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package apicaller

import (
	"github.com/juju/errors"
	"github.com/juju/loggo"
	"launchpad.net/tomb"

	"github.com/juju/juju/agent"
	"github.com/juju/juju/api"
	"github.com/juju/juju/worker"
)

var logger = loggo.GetLogger("juju.worker.apicaller")

// openConnection exists to be patched out in export_test.go (and let us test
// this component without using a real API connection).
var openConnection = func(a agent.Agent) (api.Connection, error) {
	st, _, err := OpenAPIState(a)
	if err != nil {
		return nil, errors.Trace(err)
	}
	return st, nil
}

// newApiConnWorker returns a worker that exists for as long as the associated
// connection, and provides access to a base.APICaller via its manifold's Output
// func. If the worker is killed, the connection will be closed; and if the
// connection is broken, the worker will be killed.
func newApiConnWorker(conn api.Connection) (worker.Worker, error) {
	w := &apiConnWorker{conn: conn}
	go func() {
		defer w.tomb.Done()
		w.tomb.Kill(w.loop())
	}()
	return w, nil
}

type apiConnWorker struct {
	tomb tomb.Tomb
	conn api.Connection
}

// Kill is part of the worker.Worker interface.
func (w *apiConnWorker) Kill() {
	w.tomb.Kill(nil)
}

// Wait is part of the worker.Worker interface.
func (w *apiConnWorker) Wait() error {
	return w.tomb.Wait()
}

// loop is somewhat out of the ordinary, because an api.State *does* maintain an
// internal workeresque heartbeat goroutine, but it doesn't implement Worker.
func (w *apiConnWorker) loop() (err error) {
	// TODO(fwereade): we should make this rational at some point.

	defer func() {
		// Since we can't tell for sure what error killed the connection, any
		// error out of Close is more important and relevant than any error we
		// might return in the loop.
		if closeErr := w.conn.Close(); closeErr != nil {
			err = closeErr
		}
	}()

	// Note that we should never return a nil error from this loop. If we're
	// shut down deliberately we should return ErrDying, to be overwritten by
	// any non-nil Close error and otherwise not perturb the tomb; and if the
	// connection closes on its own there's surely *something* wrong even if
	// there's no error reported from Close. (sample problem: *someone else*
	// closed the conn that we're meant to be in control of).
	for {
		select {
		case <-w.tomb.Dying():
			return tomb.ErrDying
		case <-w.conn.Broken():
			return errors.New("api connection broken unexpectedly")
		}
	}
}
