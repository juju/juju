// Copyright 2018 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package common

import (
	"github.com/juju/errors"
	"gopkg.in/juju/names.v2"

	"github.com/juju/juju/api/base"
	"github.com/juju/juju/apiserver/params"
)

const leadershipFacade = "LeadershipPinning"

// LeadershipPinningAPI provides common client-side API functions
// for manipulating and querying application leadership pinning.
type LeadershipPinningAPI struct {
	facade base.FacadeCaller
}

// NewLeadershipPinningAPI creates and returns a new leadership API client.
func NewLeadershipPinningAPI(caller base.APICaller) *LeadershipPinningAPI {
	facadeCaller := base.NewFacadeCaller(
		caller,
		leadershipFacade,
	)
	return NewLeadershipPinningAPIFromFacade(facadeCaller)
}

// NewLeadershipPinningAPIFromFacade creates and returns a new leadership API
// client based on the input client facade and facade caller.
func NewLeadershipPinningAPIFromFacade(facade base.FacadeCaller) *LeadershipPinningAPI {
	return &LeadershipPinningAPI{
		facade: facade,
	}
}

// PinLeadership (leadership.Pinner) sends a request to the API server to pin
// leadership for the input application on behalf of the input entity.
func (l *LeadershipPinningAPI) PinLeadership(appName string) error {
	return errors.Trace(l.pinOp("PinLeadership", appName))
}

// UnpinLeadership (leadership.Pinner) sends a request to the API server to
// unpin leadership for the input application on behalf of the input entity.
func (l *LeadershipPinningAPI) UnpinLeadership(appName string) error {
	return errors.Trace(l.pinOp("UnpinLeadership", appName))
}

// pinOp makes the appropriate facade call for leadership pinning manipulations
// based on the input application and method name.
func (l *LeadershipPinningAPI) pinOp(callName, appName string) error {
	arg := params.PinLeadershipBulkParams{
		Params: []params.PinLeadershipParams{{
			ApplicationTag: names.NewApplicationTag(appName).String(),
		}},
	}
	var results params.ErrorResults
	err := l.facade.FacadeCall(callName, arg, &results)
	if err != nil {
		return errors.Trace(err)
	}

	if len(results.Results) != 1 {
		return errors.Errorf("expected 1 result, got %d", len(results.Results))
	}
	result := results.Results[0]
	if result.Error != nil {
		return result.Error
	}
	return nil
}
