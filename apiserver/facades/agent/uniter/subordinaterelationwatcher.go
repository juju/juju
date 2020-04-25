// Copyright 2017 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package uniter

import (
	"github.com/juju/charm/v7"
	"github.com/juju/collections/set"
	"github.com/juju/errors"
	"github.com/juju/worker/v2/catacomb"

	"github.com/juju/juju/state"
)

type subRelationsWatcher struct {
	catacomb      catacomb.Catacomb
	backend       *state.State
	app           *state.Application
	principalName string

	// Maps relation keys to whether that relation should be
	// included. Needed particularly for when the relation goes away.
	relations map[string]bool
	out       chan []string
}

// newSubordinateRelationsWatcher creates a watcher that will notify
// about relation lifecycle events for subordinateApp, but filtered to
// be relevant to a unit deployed to a container with the
// principalName app. Global relations will be included, but only
// container-scoped relations for the principal application will be
// emitted - other container-scoped relations will be filtered out.
func newSubordinateRelationsWatcher(backend *state.State, subordinateApp *state.Application, principalName string) (
	state.StringsWatcher, error,
) {
	w := &subRelationsWatcher{
		backend:       backend,
		app:           subordinateApp,
		principalName: principalName,
		relations:     make(map[string]bool),
		out:           make(chan []string),
	}
	err := catacomb.Invoke(catacomb.Plan{
		Site: &w.catacomb,
		Work: w.loop,
	})
	return w, errors.Trace(err)
}

func (w *subRelationsWatcher) loop() error {
	defer close(w.out)
	relationsw := w.app.WatchRelations()
	if err := w.catacomb.Add(relationsw); err != nil {
		return errors.Trace(err)
	}
	var (
		sentInitial bool
		out         chan []string

		currentRelations = set.NewStrings()
	)
	for {
		select {
		case <-w.catacomb.Dying():
			return w.catacomb.ErrDying()
		case out <- currentRelations.Values():
			sentInitial = true
			currentRelations = set.NewStrings()
			out = nil
		case newRelations, ok := <-relationsw.Changes():
			if !ok {
				return w.catacomb.ErrDying()
			}
			for _, relation := range newRelations {
				if currentRelations.Contains(relation) {
					continue
				}
				shouldSend, err := w.shouldSend(relation)
				if err != nil {
					return errors.Trace(err)
				}
				if shouldSend {
					currentRelations.Add(relation)
				}
			}
			if !sentInitial || currentRelations.Size() > 0 {
				out = w.out
			}
		}
	}
}

func (w *subRelationsWatcher) shouldSend(key string) (bool, error) {
	if shouldSend, found := w.relations[key]; found {
		return shouldSend, nil
	}
	result, err := w.shouldSendCheck(key)
	if err == nil {
		w.relations[key] = result
	}
	return result, errors.Trace(err)
}

func (w *subRelationsWatcher) shouldSendCheck(key string) (bool, error) {
	rel, err := w.backend.KeyRelation(key)
	if errors.IsNotFound(err) {
		// We never saw it, and it's already gone away, so we can drop it.
		logger.Debugf("couldn't find unknown relation %q", key)
		return false, nil
	} else if err != nil {
		return false, errors.Trace(err)
	}

	thisEnd, err := rel.Endpoint(w.app.Name())
	if err != nil {
		return false, errors.Trace(err)
	}
	if thisEnd.Scope == charm.ScopeGlobal {
		return true, nil
	}

	// Only allow container relations if the other end is our
	// principal or the other end is a subordinate.
	otherEnds, err := rel.RelatedEndpoints(w.app.Name())
	if err != nil {
		return false, errors.Trace(err)
	}
	for _, otherEnd := range otherEnds {
		if otherEnd.ApplicationName == w.principalName {
			return true, nil
		}
		otherApp, err := w.backend.Application(otherEnd.ApplicationName)
		if err != nil {
			return false, errors.Trace(err)
		}
		if !otherApp.IsPrincipal() {
			return true, nil
		}
	}
	return false, nil
}

// Changes implements watcher.StringsWatcher.
func (w *subRelationsWatcher) Changes() <-chan []string {
	return w.out
}

// Err implements watcher.StringsWatcher.
func (w *subRelationsWatcher) Err() error {
	return w.catacomb.Err()
}

// Kill implements watcher.StringsWatcher.
func (w *subRelationsWatcher) Kill() {
	w.catacomb.Kill(nil)
}

// Stop implements watcher.StringsWatcher.
func (w *subRelationsWatcher) Stop() error {
	w.Kill()
	return w.Wait()
}

// Wait implements watcher.StringsWatcher.
func (w *subRelationsWatcher) Wait() error {
	return w.catacomb.Wait()
}
