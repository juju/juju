// Copyright 2019 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package caasoperator

import (
	"github.com/juju/collections/set"
	"github.com/juju/errors"
	"github.com/juju/worker/v2"
	"github.com/juju/worker/v2/catacomb"

	corewatcher "github.com/juju/juju/core/watcher"
	"github.com/juju/juju/state"
)

type unitIDWatcher struct {
	catacomb catacomb.Catacomb
	out      chan []string
	src      corewatcher.StringsWatcher
	model    watcherModelInterface
}

type watcherModelInterface interface {
	Containers(providerIds ...string) ([]state.CloudContainer, error)
}

// newUnitIDWatcher watches a StringsWatcher and converts ProviderIDs into UnitIDs and re-emits them.
// If the ProviderID does not match any Units, then the ProviderID is ignored.
func newUnitIDWatcher(model watcherModelInterface, src corewatcher.StringsWatcher) (*unitIDWatcher, error) {
	w := &unitIDWatcher{
		out:   make(chan []string),
		src:   src,
		model: model,
	}
	err := catacomb.Invoke(catacomb.Plan{
		Site: &w.catacomb,
		Work: w.loop,
		Init: []worker.Worker{src},
	})
	return w, err
}

func (w *unitIDWatcher) Kill() {
	w.catacomb.Kill(nil)
}

func (w *unitIDWatcher) Wait() error {
	return w.catacomb.Wait()
}

func (w *unitIDWatcher) Stop() error {
	w.Kill()
	return w.Wait()
}

func (w *unitIDWatcher) Changes() <-chan []string {
	return w.out
}

func (w *unitIDWatcher) Err() error {
	return w.catacomb.Err()
}

func (w *unitIDWatcher) loop() error {
	defer close(w.out)

	var out chan []string
	var result []string

	// initial event is sent regardless of if it's
	// empty.
	sendInitial := true

	for {
		select {
		case <-w.catacomb.Dying():
			return w.catacomb.ErrDying()
		case providerIDs, ok := <-w.src.Changes():
			if !ok {
				return errors.Errorf("event watcher closed")
			}
			uniqueIDs := set.NewStrings(providerIDs...)
			containers, err := w.model.Containers(uniqueIDs.Values()...)
			if err != nil {
				return errors.Trace(err)
			}
			providerIDToUnitName := map[string]string{}
			for _, containerInfo := range containers {
				providerIDToUnitName[containerInfo.ProviderId()] = containerInfo.Unit()
			}
			for _, providerID := range providerIDs {
				if unitName, ok := providerIDToUnitName[providerID]; ok {
					result = append(result, unitName)
				}
			}
			if len(result) == 0 && !sendInitial {
				continue
			}
			out = w.out
		case out <- result:
			out = nil
			result = nil
			sendInitial = false
		}
	}
}
