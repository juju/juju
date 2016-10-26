// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package crossmodel

import (
	"github.com/juju/errors"

	"github.com/juju/juju/apiserver/common"
	"github.com/juju/juju/apiserver/facade"
	"github.com/juju/juju/apiserver/params"
	"github.com/juju/juju/core/crossmodel"
	"github.com/juju/juju/feature"
	"github.com/juju/juju/state"
	"github.com/juju/juju/state/watcher"
)

func init() {
	common.RegisterStandardFacadeForFeature("OfferedApplications", 1, newOfferedApplicationAPI, feature.CrossModelRelations)
}

// OfferedApplicationLister instances allow offered applications to be queried.
type OfferedApplicationLister interface {
	// ListOffers returns the offered applications matching the filter.
	ListOffers(filter ...crossmodel.OfferedApplicationFilter) ([]crossmodel.OfferedApplication, error)
}

// OfferedApplicationAPI is a facade used to access offered applications.
type OfferedApplicationAPI struct {
	st                  Backend
	offeredApplications OfferedApplicationLister
	resources           facade.Resources
	authorizer          facade.Authorizer
}

// createApplicationDirectoryAPI returns a new cross model API facade.
func createOfferedApplicationAPI(
	st Backend,
	offeredApplicationLister OfferedApplicationLister,
	resources facade.Resources,
	authorizer facade.Authorizer,
) (*OfferedApplicationAPI, error) {
	if !authorizer.AuthModelManager() {
		return nil, common.ErrPerm
	}

	return &OfferedApplicationAPI{
		st:                  st,
		offeredApplications: offeredApplicationLister,
		authorizer:          authorizer,
		resources:           resources,
	}, nil
}

func newOfferedApplicationAPI(
	st *state.State,
	resources facade.Resources,
	authorizer facade.Authorizer,
) (*OfferedApplicationAPI, error) {
	return createOfferedApplicationAPI(getStateAccess(st), state.NewOfferedApplications(st), resources, authorizer)
}

// WatchOfferedApplications creates a watcher to listen to changes to the offered applications.
func (s *OfferedApplicationAPI) WatchOfferedApplications() (params.StringsWatchResult, error) {
	nothing := params.StringsWatchResult{}
	watch := s.st.WatchOfferedApplications()
	if changes, ok := <-watch.Changes(); ok {
		return params.StringsWatchResult{
			StringsWatcherId: s.resources.Register(watch),
			Changes:          changes,
		}, nil
	}
	return nothing, watcher.EnsureErr(watch)
}

// OfferedApplications returns the offered applications matching the query parameters.
func (s *OfferedApplicationAPI) OfferedApplications(filter params.ApplicationURLs) (params.OfferedApplicationResults, error) {
	offers := make([]params.OfferedApplicationResult, len(filter.ApplicationURLs))
	for i, url := range filter.ApplicationURLs {
		offerResults, err := s.offeredApplications.ListOffers(crossmodel.OfferedApplicationFilter{
			ApplicationURL: url,
		})
		if err != nil {
			offers[i].Error = common.ServerError(err)
			continue
		}
		if len(offerResults) == 0 {
			offers[i].Error = common.ServerError(errors.NotFoundf("offered application at %q", url))
			continue
		}
		if len(offerResults) != 1 {
			offers[i].Error = common.ServerError(errors.Errorf("expected 1 result, got %d", len(offerResults)))
			continue
		}
		offers[i].Result = params.OfferedApplication{
			ApplicationURL:  offerResults[0].ApplicationURL,
			ApplicationName: offerResults[0].ApplicationName,
			CharmName:       offerResults[0].CharmName,
			Description:     offerResults[0].Description,
			Registered:      offerResults[0].Registered,
			Endpoints:       offerResults[0].Endpoints,
		}
	}
	return params.OfferedApplicationResults{offers}, nil
}
