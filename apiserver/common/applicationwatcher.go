// Copyright 2020 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package common

import (
	"github.com/juju/charm/v9"
	"github.com/juju/errors"
	corecharm "github.com/juju/juju/core/charm"
	"github.com/juju/worker/v2"
	"github.com/juju/worker/v2/catacomb"

	"github.com/juju/juju/apiserver/facade"
	"github.com/juju/juju/apiserver/params"
	"github.com/juju/juju/state"
	"github.com/juju/juju/state/watcher"
)

// ApplicationFilter to apply to watched model.
type ApplicationFilter int

const (
	// ApplicationFilterNone has no filtering.
	ApplicationFilterNone ApplicationFilter = iota
	// ApplicationFilterCAASLegacy filters to include only legacy applications.
	ApplicationFilterCAASLegacy ApplicationFilter = iota
	// ApplicationFilterCAASEmbedded filters to include only embedded applications.
	ApplicationFilterCAASEmbedded ApplicationFilter = iota
)

// ApplicationWatcherFacade implements a common WatchApplications method for use by
// various facades.
type ApplicationWatcherFacade struct {
	state     AppWatcherState
	resources facade.Resources
	filter    ApplicationFilter
}

// NewApplicationWatcherFacadeFromState returns the optionally filtering WatchApplications facde call.
func NewApplicationWatcherFacadeFromState(st *state.State, resources facade.Resources, filter ApplicationFilter) *ApplicationWatcherFacade {
	return NewApplicationWatcherFacade(&appWatcherStateShim{st}, resources, filter)
}

// NewApplicationWatcherFacade returns the optionally filtering WatchApplications facde call.
func NewApplicationWatcherFacade(st AppWatcherState, resources facade.Resources, filter ApplicationFilter) *ApplicationWatcherFacade {
	return &ApplicationWatcherFacade{
		state:     st,
		resources: resources,
		filter:    filter,
	}
}

// WatchApplications starts a StringsWatcher to watch applications deployed to this model.
func (a *ApplicationWatcherFacade) WatchApplications() (_ params.StringsWatchResult, err error) {
	watch := a.state.WatchApplications()
	if a.filter == ApplicationFilterNone {
		// Consume the initial event and forward it to the result.
		if changes, ok := <-watch.Changes(); ok {
			return params.StringsWatchResult{
				StringsWatcherId: a.resources.Register(watch),
				Changes:          changes,
			}, nil
		}
		return params.StringsWatchResult{}, watcher.EnsureErr(watch)
	}
	filterWatcher, err := newApplicationWatcher(a.state, watch, a.filter)
	if err != nil {
		_ = watch.Stop()
		return params.StringsWatchResult{}, errors.Trace(err)
	}
	// Consume the initial event and forward it to the result.
	if changes, ok := <-filterWatcher.Changes(); ok {
		return params.StringsWatchResult{
			StringsWatcherId: a.resources.Register(filterWatcher),
			Changes:          changes,
		}, nil
	}
	return params.StringsWatchResult{}, watcher.EnsureErr(filterWatcher)
}

type applicationWatcher struct {
	source state.StringsWatcher
	out    chan []string
	state  AppWatcherState
	filter ApplicationFilter

	catacomb catacomb.Catacomb
}

func newApplicationWatcher(st AppWatcherState, source state.StringsWatcher, filter ApplicationFilter) (*applicationWatcher, error) {
	w := &applicationWatcher{
		state:  st,
		source: source,
		out:    make(chan []string),
		filter: filter,
	}
	err := catacomb.Invoke(catacomb.Plan{
		Site: &w.catacomb,
		Work: w.loop,
		Init: []worker.Worker{source},
	})
	return w, errors.Trace(err)
}

func (w *applicationWatcher) loop() error {
	defer close(w.out)
	for {
		select {
		case <-w.catacomb.Dying():
			return w.catacomb.ErrDying()
		case changes, ok := <-w.source.Changes():
			if !ok {
				return w.catacomb.ErrDying()
			}
			filteredChanges, err := w.handle(changes)
			if err != nil {
				return errors.Trace(err)
			}
			select {
			case <-w.catacomb.Dying():
				return w.catacomb.ErrDying()
			case w.out <- filteredChanges:
			}
		}
	}
}

func (w *applicationWatcher) handle(changes []string) ([]string, error) {
	filteredChanges := []string(nil)
	for _, name := range changes {
		app, err := w.state.Application(name)
		if err != nil {
			return nil, errors.Trace(err)
		}
		ch, _, err := app.Charm()
		if err != nil {
			return nil, errors.Trace(err)
		}
		// TODO(CAAS): Improve application filtering logic.
		switch w.filter {
		case ApplicationFilterCAASLegacy:
			if corecharm.Format(ch) >= corecharm.FormatV2 {
				// Filter out embedded applications.
				continue
			}
		case ApplicationFilterCAASEmbedded:
			if corecharm.Format(ch) == corecharm.FormatV1 {
				// Filter out non-embedded applications.
				continue
			}
		default:
			return nil, errors.Errorf("unknown application filter %v", w.filter)
		}
		filteredChanges = append(filteredChanges, name)
	}
	return filteredChanges, nil
}

// Changes is part of corewatcher.StringsWatcher.
func (w *applicationWatcher) Changes() <-chan []string {
	return w.out
}

// Kill is part of worker.Worker.
func (w *applicationWatcher) Kill() {
	w.catacomb.Kill(nil)
}

// Wait is part of worker.Worker.
func (w *applicationWatcher) Wait() error {
	return w.catacomb.Wait()
}

// Stop is part of facade.Resource.
func (w *applicationWatcher) Stop() error {
	w.Kill()
	return w.Wait()
}

// Err is part of state/watcher.Errer.
func (w *applicationWatcher) Err() error {
	return w.catacomb.Err()
}

// AppWatcherState is State for AppWatcher.
type AppWatcherState interface {
	WatchApplications() state.StringsWatcher
	Application(name string) (AppWatcherApplication, error)
}

// AppWatcherApplication is Application for AppWatcher.
type AppWatcherApplication interface {
	Charm() (charm.CharmMeta, bool, error)
}

type appWatcherStateShim struct {
	*state.State
}

func (s *appWatcherStateShim) Application(name string) (AppWatcherApplication, error) {
	app, err := s.State.Application(name)
	if err != nil {
		return nil, err
	}
	return &appWatcherApplicationShim{app}, nil
}

type appWatcherApplicationShim struct {
	*state.Application
}

func (s *appWatcherApplicationShim) Charm() (charm.CharmMeta, bool, error) {
	ch, force, err := s.Application.Charm()
	if err != nil {
		return nil, false, err
	}
	return &appWatcherCharmShim{ch}, force, nil
}

type appWatcherCharmShim struct {
	*state.Charm
}
