// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package remoterelations

import (
	"github.com/juju/errors"
	"gopkg.in/juju/names.v2"

	"github.com/juju/juju/api/base"
	apiwatcher "github.com/juju/juju/api/watcher"
	"github.com/juju/juju/apiserver/params"
	"github.com/juju/juju/watcher"
)

const remoteRelationsFacade = "RemoteRelations"

// Client provides access to the remoterelations api facade.
type Client struct {
	facade base.FacadeCaller
}

// NewClient creates a new client-side RemoteRelations facade.
func NewClient(caller base.APICaller) *Client {
	facadeCaller := base.NewFacadeCaller(caller, remoteRelationsFacade)
	return &Client{facadeCaller}
}

// WatchRemoteApplications returns a strings watcher that notifies of the addition,
// removal, and lifecycle changes of remote applications in the model.
func (st *Client) WatchRemoteApplications() (watcher.StringsWatcher, error) {
	var result params.StringsWatchResult
	err := st.facade.FacadeCall("WatchRemoteApplications", nil, &result)
	if err != nil {
		return nil, err
	}
	if result.Error != nil {
		return nil, result.Error
	}
	w := apiwatcher.NewStringsWatcher(st.facade.RawAPICaller(), result)
	return w, nil
}

// WatchRemoteApplicationRelations returns remote relations watchers that delivers
// changes according to the addition, removal, and lifecycle changes of
// relations that the specified remote application is involved in; and also
// according to the entering, departing, and change of unit settings in
// those relations.
func (st *Client) WatchRemoteApplicationRelations(application string) (watcher.StringsWatcher, error) {
	if !names.IsValidApplication(application) {
		return nil, errors.NotValidf("application name %q", application)
	}
	applicationTag := names.NewApplicationTag(application)
	args := params.Entities{
		Entities: []params.Entity{{Tag: applicationTag.String()}},
	}

	var results params.StringsWatchResults
	err := st.facade.FacadeCall("WatchRemoteApplicationRelations", args, &results)
	if err != nil {
		return nil, err
	}
	if len(results.Results) != 1 {
		return nil, errors.Errorf("expected 1 result, got %d", len(results.Results))
	}
	result := results.Results[0]
	if result.Error != nil {
		return nil, result.Error
	}
	w := apiwatcher.NewStringsWatcher(st.facade.RawAPICaller(), result)
	return w, nil
}

// WatchLocalRelationUnits returns a watcher that notifies of changes to the
// local units in the relation with the given key.
func (st *Client) WatchLocalRelationUnits(relationKey string) (watcher.RelationUnitsWatcher, error) {
	if !names.IsValidRelation(relationKey) {
		return nil, errors.NotValidf("relation key %q", relationKey)
	}
	relationTag := names.NewRelationTag(relationKey)
	args := params.Entities{
		Entities: []params.Entity{{Tag: relationTag.String()}},
	}
	var results params.RelationUnitsWatchResults
	err := st.facade.FacadeCall("WatchLocalRelationUnits", args, &results)
	if err != nil {
		return nil, errors.Trace(err)
	}
	if len(results.Results) != 1 {
		return nil, errors.Errorf("expected 1 result, got %d", len(results.Results))
	}
	result := results.Results[0]
	if result.Error != nil {
		return nil, result.Error
	}
	w := apiwatcher.NewRelationUnitsWatcher(st.facade.RawAPICaller(), result)
	return w, nil
}
