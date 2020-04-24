// Copyright 2018 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package common

import (
	"github.com/juju/errors"
	"github.com/juju/names/v4"

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

// PinnedLeadership returns a collection of application names for which
// leadership is currently pinned, with the entities requiring each
// application's pinned behaviour.
func (a *LeadershipPinningAPI) PinnedLeadership() (map[string][]names.Tag, error) {
	var callResult params.PinnedLeadershipResult
	err := a.facade.FacadeCall("PinnedLeadership", nil, &callResult)
	if err != nil {
		return nil, errors.Trace(err)
	}

	pinned := make(map[string][]names.Tag, len(callResult.Result))
	for app, entities := range callResult.Result {
		entityTags := make([]names.Tag, len(entities))
		for i, e := range entities {
			tag, err := names.ParseTag(e)
			if err != nil {
				return nil, errors.Trace(err)
			}
			entityTags[i] = tag
		}

		pinned[app] = entityTags
	}
	return pinned, nil
}

// PinMachineApplications pins leadership for applications represented by units
// running on the local machine.
// If the caller is not a machine agent, an error will be returned.
// The return is a collection of applications determined to be running on the
// machine, with the result of each individual pin operation.
func (a *LeadershipPinningAPI) PinMachineApplications() (map[string]error, error) {
	res, err := a.pinMachineAppsOps("PinMachineApplications")
	return res, errors.Trace(err)
}

// UnpinMachineApplications pins leadership for applications represented by
// units running on the local machine.
// If the caller is not a machine agent, an error will be returned.
// The return is a collection of applications determined to be running on the
// machine, with the result of each individual unpin operation.
func (a *LeadershipPinningAPI) UnpinMachineApplications() (map[string]error, error) {
	res, err := a.pinMachineAppsOps("UnpinMachineApplications")
	return res, errors.Trace(err)
}

// pinMachineAppsOps makes a facade call to the input method name and
// transforms the response into map.
func (a *LeadershipPinningAPI) pinMachineAppsOps(callName string) (map[string]error, error) {
	var callResult params.PinApplicationsResults
	err := a.facade.FacadeCall(callName, nil, &callResult)
	if err != nil {
		return nil, errors.Trace(err)
	}

	callResults := callResult.Results
	result := make(map[string]error, len(callResults))
	for _, res := range callResults {
		var appErr error
		if res.Error != nil {
			appErr = res.Error
		}
		result[res.ApplicationName] = appErr
	}
	return result, nil
}
