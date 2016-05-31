// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package applicationscaler

import (
	"github.com/juju/errors"
	"github.com/juju/juju/apiserver/common"
	"github.com/juju/juju/apiserver/params"
	"github.com/juju/juju/state"
	"github.com/juju/juju/state/watcher"
	"gopkg.in/juju/names.v2"
)

// Backend exposes functionality required by Facade.
type Backend interface {

	// WatchScaledServices returns a watcher that sends service ids
	// that might not have enough units.
	WatchScaledServices() state.StringsWatcher

	// RescaleService ensures that the named service has at least its
	// configured minimum unit count.
	RescaleService(name string) error
}

// Facade allows model-manager clients to watch and rescale services.
type Facade struct {
	backend   Backend
	resources *common.Resources
}

// NewFacade creates a new authorized Facade.
func NewFacade(backend Backend, res *common.Resources, auth common.Authorizer) (*Facade, error) {
	if !auth.AuthModelManager() {
		return nil, common.ErrPerm
	}
	return &Facade{
		backend:   backend,
		resources: res,
	}, nil
}

// Watch returns a watcher that sends the names of services whose
// unit count may be below their configured minimum.
func (facade *Facade) Watch() (params.StringsWatchResult, error) {
	watch := facade.backend.WatchScaledServices()
	if changes, ok := <-watch.Changes(); ok {
		id := facade.resources.Register(watch)
		return params.StringsWatchResult{
			StringsWatcherId: id,
			Changes:          changes,
		}, nil
	}
	return params.StringsWatchResult{}, watcher.EnsureErr(watch)
}

// Rescale causes any supplied services to be scaled up to their
// minimum size.
func (facade *Facade) Rescale(args params.Entities) params.ErrorResults {
	result := params.ErrorResults{
		Results: make([]params.ErrorResult, len(args.Entities)),
	}
	for i, entity := range args.Entities {
		err := facade.rescaleOne(entity.Tag)
		result.Results[i].Error = common.ServerError(err)
	}
	return result
}

// rescaleOne scales up the supplied service, if necessary; or returns a
// suitable error.
func (facade *Facade) rescaleOne(tagString string) error {
	tag, err := names.ParseTag(tagString)
	if err != nil {
		return errors.Trace(err)
	}
	ApplicationTag, ok := tag.(names.ApplicationTag)
	if !ok {
		return common.ErrPerm
	}
	return facade.backend.RescaleService(ApplicationTag.Id())
}
