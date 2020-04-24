// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package apicaller

import (
	"github.com/juju/errors"
	"github.com/juju/worker/v2"
	"gopkg.in/tomb.v2"

	"github.com/juju/juju/api"
)

// logger is here to stop the desire of creating a package level logger.
// Don't do this, instead use the one passed as manifold config.
var logger interface{}

// newAPIConnWorker returns a worker that exists for as long as the associated
// connection, and provides access to a base.APICaller via its manifold's Output
// func. If the worker is killed, the connection will be closed; and if the
// connection is broken, the worker will be killed.
//
// The lack of error return is considered and intentional; it signals the
// transfer of responsibility for the connection from the caller to the
// worker.
func newAPIConnWorker(conn api.Connection) worker.Worker {
	w := &apiConnWorker{conn: conn}
	w.tomb.Go(w.loop)
	return w
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

// loop is somewhat out of the ordinary, because an api.Connection
// *does* maintain an internal workeresque heartbeat goroutine, but it
// doesn't implement Worker.
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
