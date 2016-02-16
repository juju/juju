// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package voyeur

import (
	"github.com/juju/errors"
	"github.com/juju/loggo"
	"github.com/juju/utils/voyeur"
	"launchpad.net/tomb"

	"github.com/juju/juju/worker"
	"github.com/juju/juju/worker/dependency"
)

var logger = loggo.GetLogger("juju.worker.voyeur")

type ManifoldConfig struct {
	Value *voyeur.Value
}

// Manifold returns a dependency.Manifold which wraps a voyeur.Value
// from github.com/juju/utils for use within the dependency engine
// framework. It will watch the voyeur.Value and restart itself
// whenever Set is called the voyeur.Value.
//
// The manifold assumes that the the voyeur.Value has been created
// with an initial value (using voyeur.NewValue) or already has a
// value set (using the Set method). The initial event from the
// Value's Next() is consumed to avoid unnecessary worker restarts.
//
// NOTE: the manifold currently provides no way to access the voyeur's
// value as this was not required for the initial use case. An output
// function could be added to faciliate this if necessary.
func Manifold(config ManifoldConfig) dependency.Manifold {
	return dependency.Manifold{
		Start: func(dependency.GetResourceFunc) (worker.Worker, error) {
			if config.Value == nil {
				return nil, errors.NotValidf("nil Value")
			}

			w := &valueWatcher{
				value: config.Value,
			}
			go func() {
				defer w.tomb.Done()
				w.tomb.Kill(w.watch())
			}()
			return w, nil
		},
	}
}

type valueWatcher struct {
	tomb  tomb.Tomb
	value *voyeur.Value
}

func (w *valueWatcher) watch() error {
	watch := w.value.Watch()
	defer watch.Close()

	watchCh := make(chan bool)
	go func() {
		// Consume the initial event to avoid unnecessary worker
		// restart churn.
		if !watch.Next() {
			return
		}

		if watch.Next() {
			select {
			case watchCh <- true:
			case <-w.tomb.Dying():
			}
		}
	}()

	select {
	case <-watchCh:
		// The voyeur changed, restart so that dependents get
		// restarted too. ErrBounce ensures that the manifold is
		// restarted quickly.
		return dependency.ErrBounce
	case <-w.tomb.Dying():
		return tomb.ErrDying
	}
}

// Kill is part of the worker.Worker interface.
func (w *valueWatcher) Kill() {
	w.tomb.Kill(nil)
}

// Wait is part of the worker.Worker interface.
func (w *valueWatcher) Wait() error {
	return w.tomb.Wait()
}
