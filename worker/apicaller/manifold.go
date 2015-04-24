// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package apicaller

import (
	"github.com/juju/errors"
	"launchpad.net/tomb"

	coreagent "github.com/juju/juju/agent"
	"github.com/juju/juju/api/base"
	jujudagent "github.com/juju/juju/cmd/jujud/agent"
	"github.com/juju/juju/worker"
	"github.com/juju/juju/worker/agent"
	"github.com/juju/juju/worker/dependency"
	"github.com/juju/juju/worker/util"
)

// ManifoldConfig defines the names of the manifolds on which a Manifold will depend.
type ManifoldConfig util.AgentManifoldConfig

// Manifold returns a manifold whose worker wraps an API connection made on behalf of
// the dependency identified by AgentName.
func Manifold(config ManifoldConfig) dependency.Manifold {
	return dependency.Manifold{
		Inputs: []string{
			config.AgentName,
		},
		Output: outputFunc,
		Start:  startFunc(config),
	}
}

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

// startFunc returns a StartFunc that creates a worker based on the manifolds
// named in the supplied config.
func startFunc(config ManifoldConfig) dependency.StartFunc {
	return func(getResource dependency.GetResourceFunc) (worker.Worker, error) {

		// Get dependencies and open a connection.
		var agent agent.Agent
		if err := getResource(config.AgentName, &agent); err != nil {
			return nil, err
		}
		conn, err := openConnection(agent)
		if err != nil {
			return nil, errors.Annotate(err, "cannot open api")
		}

		// Add the environment uuid to agent config if not present.
		currentConfig := agent.CurrentConfig()
		if currentConfig.Environment().Id() == "" {
			err := agent.ChangeConfig(func(setter coreagent.ConfigSetter) error {
				environTag, err := conn.EnvironTag()
				if err != nil {
					return errors.Annotate(err, "no environment uuid set on api")
				}
				return setter.Migrate(coreagent.MigrateParams{
					Environment: environTag,
				})
			})
			if err != nil {
				logger.Warningf("unable to save environment uuid: %v", err)
				// Not really fatal, just annoying.
			}
		}

		// Return the worker.
		w := &apiConnWorker{conn: conn}
		go func() {
			defer w.tomb.Done()
			w.tomb.Kill(w.loop())
		}()
		return w, nil
	}
}

// outputFunc extracts a base.APICaller from a *apiConnWorker.
func outputFunc(in worker.Worker, out interface{}) error {
	inWorker, _ := in.(*apiConnWorker)
	outPointer, _ := out.(*base.APICaller)
	if inWorker == nil || outPointer == nil {
		return errors.Errorf("expected %T->%T; got %T->%T", inWorker, outPointer, in, out)
	}
	*outPointer = inWorker.conn
	return nil
}

// apiConnWorker is a basic worker that exists to hold a reference to the
// *api.State it manages for other workers, and to fail when it detects an
// error in the connection.
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
