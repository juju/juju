package unitassigner

import (
	"github.com/juju/juju/apiserver/common"
	"github.com/juju/juju/apiserver/params"
	"github.com/juju/juju/state"
	"github.com/juju/juju/state/watcher"
)

func init() {
	common.RegisterStandardFacade("UnitAssigner", 1, New)
}

// st defines the state methods this facade needs, so they can be mocked
// for testing.
type st interface {
	WatchForUnitAssignment() state.NotifyWatcher
	AssignStagedUnits() error
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

func (a API) AssignUnits() (params.AssignUnitsResult, error) {
	var result params.AssignUnitsResult
	if err := a.st.AssignStagedUnits(); err != nil {
		result.Error = common.ServerError(err)
	}
	return result, nil
}

func (a API) WatchUnitAssignments() (params.NotifyWatchResult, error) {
	watch := a.st.WatchForUnitAssignment()
	if _, ok := <-watch.Changes(); ok {
		return params.NotifyWatchResult{
			NotifyWatcherId: a.res.Register(watch),
		}, nil
	}
	return params.NotifyWatchResult{}, watcher.EnsureErr(watch)
}
