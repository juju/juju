// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package crossmodel

import (
	"github.com/juju/errors"

	"github.com/juju/juju/apiserver/common"
	"github.com/juju/juju/apiserver/params"
	"github.com/juju/juju/model/crossmodel"
	"github.com/juju/juju/state"
	"github.com/juju/juju/state/watcher"
)

func init() {
	common.RegisterStandardFacade("OfferedServices", 1, newOfferedServiceAPI)
}

type OfferedServiceLister interface {
	ListOffers(filter ...crossmodel.OfferedServiceFilter) ([]crossmodel.OfferedService, error)
}

type OfferedServiceAPI struct {
	st              stateAccessor
	offeredServices OfferedServiceLister
	resources       *common.Resources
	authorizer      common.Authorizer
}

// createServiceDirectoryAPI returns a new cross model API facade.
func createOfferedServiceAPI(
	st stateAccessor,
	offeredServiceLister OfferedServiceLister,
	resources *common.Resources,
	authorizer common.Authorizer,
) (*OfferedServiceAPI, error) {
	if !authorizer.AuthEnvironManager() {
		return nil, common.ErrPerm
	}

	return &OfferedServiceAPI{
		st:              st,
		offeredServices: offeredServiceLister,
		authorizer:      authorizer,
		resources:       resources,
	}, nil
}

func newOfferedServiceAPI(
	st *state.State,
	resources *common.Resources,
	authorizer common.Authorizer,
) (*OfferedServiceAPI, error) {
	return createOfferedServiceAPI(getState(st), state.NewOfferedServices(st), resources, authorizer)
}

// WatchOfferedServices creates a watcher to listen to changes to the offered services.
func (s *OfferedServiceAPI) WatchOfferedServices() (params.StringsWatchResult, error) {
	nothing := params.StringsWatchResult{}
	watch := s.st.WatchOfferedServices()
	if changes, ok := <-watch.Changes(); ok {
		return params.StringsWatchResult{
			StringsWatcherId: s.resources.Register(watch),
			Changes:          changes,
		}, nil
	}
	return nothing, watcher.EnsureErr(watch)
}

// OfferedServices returns the offered services matching the query parameters.
func (s *OfferedServiceAPI) OfferedServices(filter params.OfferedServiceQueryParams) (params.OfferedServiceResults, error) {
	offers := make([]params.OfferedServiceResult, len(filter.URLS))
	for i, url := range filter.URLS {
		offerResults, err := s.offeredServices.ListOffers(crossmodel.OfferedServiceFilter{
			ServiceURL: url,
		})
		if err != nil {
			offers[i].Error = common.ServerError(err)
			continue
		}
		if len(offerResults) == 0 {
			offers[i].Error = common.ServerError(errors.NotFoundf("offered service at %q", url))
			continue
		}
		if len(offerResults) != 1 {
			offers[i].Error = common.ServerError(errors.Errorf("expected 1 result, got %d", len(offerResults)))
			continue
		}
		offers[i].Result = params.OfferedService{
			ServiceURL:  offerResults[0].ServiceURL,
			ServiceName: offerResults[0].ServiceName,
			Registered:  offerResults[0].Registered,
			Endpoints:   offerResults[0].Endpoints,
		}
	}
	return params.OfferedServiceResults{offers}, nil
}
