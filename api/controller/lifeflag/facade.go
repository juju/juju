// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package lifeflag

import (
	"github.com/juju/errors"
	"github.com/juju/names/v4"

	"github.com/juju/juju/api/base"
	"github.com/juju/juju/apiserver/params"
	"github.com/juju/juju/core/life"
	"github.com/juju/juju/core/watcher"
)

// NewWatcherFunc exists to let us test Watch properly.
type NewWatcherFunc func(base.APICaller, params.NotifyWatchResult) watcher.NotifyWatcher

// Facade makes calls to the LifeFlag facade.
type Facade struct {
	caller     base.FacadeCaller
	newWatcher NewWatcherFunc
}

// NewFacade returns a new Facade using the supplied caller.
func NewFacade(caller base.APICaller, newWatcher NewWatcherFunc) *Facade {
	return &Facade{
		caller:     base.NewFacadeCaller(caller, "LifeFlag"),
		newWatcher: newWatcher,
	}
}

// ErrNotFound indicates that the requested entity no longer exists.
//
// We avoid errors.NotFound, because errors.NotFound is non-specific, and
// it's our job to communicate *this specific condition*. There are many
// possible sources of errors.NotFound in the world, and it's not safe or
// sane for a client to treat a generic NotFound as specific to the entity
// in question.
//
// We're still vulnerable to apiservers returning unjustified CodeNotFound
// but at least we're safe from accidental errors.NotFound injection in
// the api client mechanism.
var ErrNotFound = errors.New("entity not found")

// Watch returns a NotifyWatcher that sends a value whenever the
// entity's life value may have changed; or ErrNotFound; or some
// other error.
func (facade *Facade) Watch(entity names.Tag) (watcher.NotifyWatcher, error) {
	args := params.Entities{
		Entities: []params.Entity{{Tag: entity.String()}},
	}
	var results params.NotifyWatchResults
	err := facade.caller.FacadeCall("Watch", args, &results)
	if err != nil {
		return nil, errors.Trace(err)
	}
	if count := len(results.Results); count != 1 {
		return nil, errors.Errorf("expected 1 Watch result, got %d", count)
	}
	result := results.Results[0]
	if err := result.Error; err != nil {
		if params.IsCodeNotFound(err) {
			return nil, ErrNotFound
		}
		return nil, errors.Trace(result.Error)
	}
	w := facade.newWatcher(facade.caller.RawAPICaller(), result)
	return w, nil
}

// Life returns the entity's life value; or ErrNotFound; or some
// other error.
func (facade *Facade) Life(entity names.Tag) (life.Value, error) {
	args := params.Entities{
		Entities: []params.Entity{{Tag: entity.String()}},
	}
	var results params.LifeResults
	err := facade.caller.FacadeCall("Life", args, &results)
	if err != nil {
		return "", errors.Trace(err)
	}
	if count := len(results.Results); count != 1 {
		return "", errors.Errorf("expected 1 Life result, got %d", count)
	}
	result := results.Results[0]
	if err := result.Error; err != nil {
		if params.IsCodeNotFound(err) {
			return "", ErrNotFound
		}
		return "", errors.Trace(result.Error)
	}
	life := result.Life
	if err := life.Validate(); err != nil {
		return "", errors.Trace(err)
	}
	return life, nil
}
