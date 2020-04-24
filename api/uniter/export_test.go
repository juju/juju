// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package uniter

import (
	"fmt"

	"github.com/juju/juju/api/base"
	"github.com/juju/juju/api/base/testing"
	"github.com/juju/juju/api/common"
	apiwatcher "github.com/juju/juju/api/watcher"
	"github.com/juju/juju/apiserver/params"
	"github.com/juju/juju/core/life"
	"github.com/juju/juju/core/watcher"
	"github.com/juju/names/v4"
)

var (
	NewSettings = newSettings
)

// PatchUnitResponse changes the internal FacadeCaller to one that lets you return
// canned results. The responseFunc will get the 'response' interface object,
// and can set attributes of it to fix the response to the caller.
// It can also return an error to have the FacadeCall return an error. The expected
// request is specified using the expectedRequest parameter. If the request name does
// not match, the function panics.
// The function returned by PatchResponses is a cleanup function that returns
// the client to its original state.
func PatchUnitResponse(p testing.Patcher, u *Unit, expectedRequest string, responseFunc func(interface{}) error) {
	testing.PatchFacadeCall(p, &u.st.facade, func(request string, params, response interface{}) error {
		if request != expectedRequest {
			panic(fmt.Errorf("unexpected request %q received - expecting %q", request, expectedRequest))
		}
		return responseFunc(response)
	})
}

func PatchUnitUpgradeSeriesFacade(u *Unit, facadeCaller base.FacadeCaller) {
	u.st.UpgradeSeriesAPI = common.NewUpgradeSeriesAPI(facadeCaller, u.Tag())
}

// CreateUnit creates uniter.Unit for tests.
func CreateUnit(st *State, tag names.UnitTag) *Unit {
	return &Unit{
		st:           st,
		tag:          tag,
		life:         life.Alive,
		resolvedMode: params.ResolvedNone,
	}
}

func NewStateV2(
	caller base.APICaller,
	authTag names.UnitTag,
) *State {
	return newStateForVersion(caller, authTag, 2)
}

func NewStateV4(
	caller base.APICaller,
	authTag names.UnitTag,
) *State {
	return newStateForVersion(caller, authTag, 4)
}

func newStateForVersion(
	caller base.APICaller,
	authTag names.UnitTag,
	version int,
) *State {
	facadeCaller := base.NewFacadeCallerForVersion(
		caller,
		uniterFacade,
		version,
	)
	state := &State{
		ModelWatcher:     common.NewModelWatcher(facadeCaller),
		APIAddresser:     common.NewAPIAddresser(facadeCaller),
		UpgradeSeriesAPI: common.NewUpgradeSeriesAPI(facadeCaller, authTag),
		StorageAccessor:  NewStorageAccessor(facadeCaller),
		facade:           facadeCaller,
		unitTag:          authTag,
	}

	newWatcher := func(result params.NotifyWatchResult) watcher.NotifyWatcher {
		return apiwatcher.NewNotifyWatcher(caller, result)
	}
	state.LeadershipSettings = NewLeadershipSettingsAccessor(
		facadeCaller.FacadeCall,
		newWatcher,
		ErrIfNotVersionFn(2, state.BestAPIVersion()),
	)
	return state
}
