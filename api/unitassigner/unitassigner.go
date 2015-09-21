// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package unitassigner

import (
	"fmt"

	"github.com/juju/juju/api/base"
	"github.com/juju/juju/api/watcher"
	"github.com/juju/juju/apiserver/params"
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
func (a API) AssignUnits() error {
	var result params.AssignUnitsResult
	if err := a.facade.FacadeCall("AssignUnits", nil, &result); err != nil {
		return err
	}
	if result.Error != nil {
		return result.Error
	}
	return nil
}

// WatchUnitAssignments watches the server for new unit assignments to be
// created.
func (a API) WatchUnitAssignments() (watcher.NotifyWatcher, error) {
	var results params.NotifyWatchResults
	err := a.facade.FacadeCall("WatchUnitAssignments", nil, &results)
	if err != nil {
		return nil, err
	}
	if len(results.Results) != 1 {
		return nil, fmt.Errorf("expected 1 result, got %d", len(results.Results))
	}
	result := results.Results[0]
	if result.Error != nil {
		return nil, result.Error
	}
	w := watcher.NewNotifyWatcher(a.facade.RawAPICaller(), result)
	return w, nil
}
