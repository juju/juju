// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package centralhub

import (
	"github.com/juju/errors"
	"github.com/juju/pubsub"
	"github.com/juju/worker/v2"
	"github.com/juju/worker/v2/dependency"
	"gopkg.in/tomb.v2"
)

// ManifoldConfig provides the dependencies for Manifold.
type ManifoldConfig struct {
	StateConfigWatcherName string
	// TODO: remove Hub config when apiserver and peergrouper can depend on
	// this hub.
	Hub *pubsub.StructuredHub
}

// Manifold returns a manifold whose worker simply provides the central hub.
// This hub is a dependency for any other workers that need the hub.
func Manifold(config ManifoldConfig) dependency.Manifold {
	return dependency.Manifold{
		Inputs: []string{
			config.StateConfigWatcherName,
		},
		Start: func(context dependency.Context) (worker.Worker, error) {
			// Confirm we're running in a state server by asking the
			// stateconfigwatcher manifold.
			var haveStateConfig bool
			if err := context.Get(config.StateConfigWatcherName, &haveStateConfig); err != nil {
				return nil, err
			}
			if !haveStateConfig {
				return nil, dependency.ErrMissing
			}

			if config.Hub == nil {
				return nil, errors.NotValidf("missing hub")
			}

			w := &centralHub{
				hub: config.Hub,
			}
			w.tomb.Go(func() error {
				<-w.tomb.Dying()
				return nil
			})
			return w, nil
		},
		Output: outputFunc,
	}
}

// outputFunc extracts a pubsub.Hub from a *centralHub.
func outputFunc(in worker.Worker, out interface{}) error {
	inWorker, _ := in.(*centralHub)
	if inWorker == nil {
		return errors.Errorf("in should be a %T; got %T", inWorker, in)
	}

	switch outPointer := out.(type) {
	case **pubsub.StructuredHub:
		*outPointer = inWorker.hub
	default:
		return errors.Errorf("out should be *pubsub.StructuredHub; got %T", out)
	}
	return nil
}

type centralHub struct {
	tomb tomb.Tomb
	hub  *pubsub.StructuredHub
}

// Kill is part of the worker.Worker interface.
func (w *centralHub) Kill() {
	w.tomb.Kill(nil)
}

// Wait is part of the worker.Worker interface.
func (w *centralHub) Wait() error {
	return w.tomb.Wait()
}
