package unitassigner

import (
	"github.com/juju/juju/apiserver/common"
	"github.com/juju/juju/apiserver/params"
	"github.com/juju/juju/state"
	"github.com/juju/juju/state/watcher"
)

func init() {
	common.RegisterStandardFacade("UnitAssigner", 0, New)
}

// st defines the state methods this facade needs, so they can be mocked
// for testing.
type st interface {
	WatchForUnitAssignment() state.NotifyWatcher
	AssignStagedUnits() ([]state.UnitAssignmentResult, error)
}

// API implements the functionality for assigning units to machines.
type API struct {
	st   st
	res  *common.Resources
	auth common.Authorizer
}

func New(st *state.State, res *common.Resources, auth common.Authorizer) (*API, error) {
	return &API{st: st, res: res, auth: auth}, nil
}

func (a *API) AssignUnits() (params.AssignUnitsResults, error) {
	var result params.AssignUnitsResults
	res, err := a.st.AssignStagedUnits()
	if err != nil {
		result.Error = common.ServerError(err)
	}
	result.Results = make([]params.AssignUnitsResult, len(res))
	for i, r := range res {
		result.Results[i].Unit = r.Unit
		result.Results[i].Error = common.ServerError(r.Error)
	}
	return result, nil
}

func (a *API) WatchUnitAssignments() (params.NotifyWatchResult, error) {
	watch := a.st.WatchForUnitAssignment()
	if _, ok := <-watch.Changes(); ok {
		return params.NotifyWatchResult{
			NotifyWatcherId: a.res.Register(watch),
		}, nil
	}
	return params.NotifyWatchResult{}, watcher.EnsureErr(watch)
}
