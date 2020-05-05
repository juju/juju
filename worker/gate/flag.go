// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package gate

import (
	"github.com/juju/errors"
	"github.com/juju/worker/v2"
	"github.com/juju/worker/v2/dependency"
	"gopkg.in/tomb.v2"

	"github.com/juju/juju/cmd/jujud/agent/engine"
)

// FlagManifoldConfig holds the dependencies required to run a Flag
// in a dependency.Engine.
type FlagManifoldConfig struct {
	GateName  string
	NewWorker func(gate Waiter) (worker.Worker, error)
}

// start is a dependency.StartFunc that uses config.
func (config FlagManifoldConfig) start(context dependency.Context) (worker.Worker, error) {
	var gate Waiter
	if err := context.Get(config.GateName, &gate); err != nil {
		return nil, errors.Trace(err)
	}
	worker, err := config.NewWorker(gate)
	if err != nil {
		return nil, errors.Trace(err)
	}
	return worker, nil

}

// FlagManifold runs a worker that implements engine.Flag such that
// it's only considered set when the referenced gate is unlocked.
func FlagManifold(config FlagManifoldConfig) dependency.Manifold {
	return dependency.Manifold{
		Inputs: []string{config.GateName},
		Start:  config.start,
		Output: engine.FlagOutput,
		Filter: bounceUnlocked,
	}
}

// NewFlag returns a worker that implements engine.Flag,
// backed by the supplied gate's unlockedness.
func NewFlag(gate Waiter) (*Flag, error) {
	w := &Flag{
		gate:     gate,
		unlocked: gate.IsUnlocked(),
	}
	w.tomb.Go(w.run)
	return w, nil
}

// Flag uses a gate to implement engine.Flag.
type Flag struct {
	tomb     tomb.Tomb
	gate     Waiter
	unlocked bool
}

// Kill is part of the worker.Worker interface.
func (w *Flag) Kill() {
	w.tomb.Kill(nil)
}

// Wait is part of the worker.Worker interface.
func (w *Flag) Wait() error {
	return w.tomb.Wait()
}

// Check is part of the engine.Flag interface.
func (w *Flag) Check() bool {
	return w.unlocked
}

func (w *Flag) run() error {
	var bounce <-chan struct{}
	if !w.unlocked {
		bounce = w.gate.Unlocked()
	}
	select {
	case <-w.tomb.Dying():
		return tomb.ErrDying
	case <-bounce:
		return ErrUnlocked
	}
}

// ErrUnlocked indicates that a Flag's gate has been unlocked and
// it should be restarted to reflect the new value.
var ErrUnlocked = errors.New("gate unlocked")

// bounceUnlocked returns dependency.ErrBounce if passed an error caused
// by ErrUnlocked; and otherwise returns the original error.
func bounceUnlocked(err error) error {
	if errors.Cause(err) == ErrUnlocked {
		return dependency.ErrBounce
	}
	return err
}
