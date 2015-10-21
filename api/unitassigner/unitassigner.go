// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package unitassigner

import (
	"github.com/juju/errors"
	"github.com/juju/juju/api/base"
	"github.com/juju/juju/api/watcher"
	"github.com/juju/juju/apiserver/params"

	"github.com/juju/names"
)

const uaFacade = "UnitAssigner"

// API provides access to the UnitAssigner API facade.
type API struct {
	facade base.FacadeCaller
}

// New creates a new client-side UnitAssigner facade.
func New(caller base.APICaller) API {
	fc := base.NewFacadeCaller(caller, uaFacade)
	return API{facade: fc}
}

// AssignUnits tells the state server to run whatever unit assignments it has.
func (a API) AssignUnits(ids []string) ([]error, error) {
	entities := make([]params.Entity, len(ids))
	for i, id := range ids {
		if !names.IsValidUnit(id) {
			return nil, errors.Errorf("%q is not a valid unit id", id)
		}
		entities[i] = params.Entity{Tag: names.NewUnitTag(id).String()}
	}
	args := params.Entities{Entities: entities}
	var result params.ErrorResults
	if err := a.facade.FacadeCall("AssignUnits", args, &result); err != nil {
		return nil, err
	}
	errs := make([]error, len(result.Results))
	for i, e := range result.Results {
		if e.Error != nil {
			errs[i] = errors.Errorf(e.Error.Error())
		}
	}
	return errs, nil
}

// WatchUnitAssignments watches the server for new unit assignments to be
// created.
func (a API) WatchUnitAssignments() (watcher.StringsWatcher, error) {
	var result params.StringsWatchResult
	err := a.facade.FacadeCall("WatchUnitAssignments", nil, &result)
	if err != nil {
		return nil, err
	}
	if result.Error != nil {
		return nil, result.Error
	}
	w := watcher.NewStringsWatcher(a.facade.RawAPICaller(), result)
	return w, nil
}
