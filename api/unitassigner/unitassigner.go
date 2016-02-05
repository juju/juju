// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package unitassigner

import (
	"github.com/juju/errors"
	"github.com/juju/names"

	"github.com/juju/juju/api/base"
	apiwatcher "github.com/juju/juju/api/watcher"
	"github.com/juju/juju/apiserver/params"
	"github.com/juju/juju/watcher"
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

// AssignUnits tells the controller to run whatever unit assignments it has.
// Unit assignments for units that no longer exist will return an error that
// satisfies errors.IsNotFound.
func (a API) AssignUnits(tags []names.UnitTag) ([]error, error) {
	entities := make([]params.Entity, len(tags))
	for i, tag := range tags {
		entities[i] = params.Entity{Tag: tag.String()}
	}
	args := params.Entities{Entities: entities}
	var result params.ErrorResults
	if err := a.facade.FacadeCall("AssignUnits", args, &result); err != nil {
		return nil, err
	}

	errs := make([]error, len(result.Results))
	for i, e := range result.Results {
		if e.Error != nil {
			errs[i] = convertNotFound(e.Error)
		}
	}
	return errs, nil
}

// convertNotFound converts param notfound errors into errors.notfound values.
func convertNotFound(err error) error {
	if params.IsCodeNotFound(err) {
		return errors.NewNotFound(err, "")
	}
	return err
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
	w := apiwatcher.NewStringsWatcher(a.facade.RawAPICaller(), result)
	return w, nil
}

// SetAgentStatus sets the status of the unit agents.
func (a API) SetAgentStatus(args params.SetStatus) error {
	var result params.ErrorResults
	err := a.facade.FacadeCall("SetAgentStatus", args, &result)
	if err != nil {
		return err
	}
	return result.Combine()
}
