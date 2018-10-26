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

// PinMachineApplications pins leadership for applications represented by units
// running on the local machine.
// If the caller is not a machine agent, an error will be returned.
// The return is a collection of applications determined to be running on the
// machine, with the result of each individual pin operation.
func (a *LeadershipPinningAPI) PinMachineApplications() (map[names.ApplicationTag]error, error) {
	res, err := a.pinMachineAppsOps("PinMachineApplications")
	return res, errors.Trace(err)
}

// UnpinMachineApplications pins leadership for applications represented by
// units running on the local machine.
// If the caller is not a machine agent, an error will be returned.
// The return is a collection of applications determined to be running on the
// machine, with the result of each individual unpin operation.
func (a *LeadershipPinningAPI) UnpinMachineApplications() (map[names.ApplicationTag]error, error) {
	res, err := a.pinMachineAppsOps("UnpinMachineApplications")
	return res, errors.Trace(err)
}

// pinMachineAppsOps makes a facade call to the input method name and
// transforms the response into map.
func (a *LeadershipPinningAPI) pinMachineAppsOps(callName string) (map[names.ApplicationTag]error, error) {
	var callResult params.PinApplicationsResults
	err := a.facade.FacadeCall(callName, nil, &callResult)
	if err != nil {
		return nil, errors.Trace(err)
	}

	callResults := callResult.Results
	result := make(map[names.ApplicationTag]error, len(callResults))
	for _, res := range callResults {
		var appErr error
		if res.Error != nil {
			appErr = res.Error
		}
		tag, err := names.ParseApplicationTag(res.ApplicationTag)
		if err != nil {
			return nil, errors.Trace(err)
		}
		result[tag] = appErr
	}
	return result, nil
}
