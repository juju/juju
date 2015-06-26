// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package apicaller

import (
	"github.com/juju/errors"
	"github.com/juju/loggo"
	"launchpad.net/tomb"

	"github.com/juju/juju/api/base"
	jujudagent "github.com/juju/juju/cmd/jujud/agent"
	"github.com/juju/juju/worker"
	"github.com/juju/juju/worker/agent"
)

var logger = loggo.GetLogger("juju.worker.apicaller")

// Connection includes the relevant features of a *api.State, and exists primarily
// so that we can patch out the openConnection function in tests (and not have to
// return a real *State).
type Connection interface {
	base.APICaller
	Broken() <-chan struct{}
	Close() error
}

// openConnection exists to be patched out in export_test.go (and let us test
// this component without using a real API connection).
var openConnection = func(agent agent.Agent) (Connection, error) {
	currentConfig := agent.CurrentConfig()
	st, _, err := jujudagent.OpenAPIState(currentConfig, agent)
	if err != nil {
		return nil, errors.Trace(err)
	}
	return st, nil
}

// newApiConnWorker returns a worker that exists for as long as the associated
// connection, and provides access to a base.APICaller via its manifold's Output
// func. If the worker is killed, the connection will be closed; and if the
// connection is broken, the worker will be killed.
func newApiConnWorker(conn Connection) (worker.Worker, error) {
	w := &apiConnWorker{conn: conn}
	go func() {
		defer w.tomb.Done()
		w.tomb.Kill(w.loop())
	}()
	return w, nil
}

type apiConnWorker struct {
	tomb tomb.Tomb
	conn Connection
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
