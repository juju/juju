package unitassigner

import (
	"github.com/juju/errors"
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
	WatchForUnitAssignment() state.StringsWatcher
	AssignStagedUnits(ids []string) ([]state.UnitAssignmentResult, error)
}

// API implements the functionality for assigning units to machines.
type API struct {
	st  st
	res *common.Resources
}

// New returns a new unitAssigner api instance.
func New(st *state.State, res *common.Resources, _ common.Authorizer) (*API, error) {
	return &API{st: st, res: res}, nil
}

//  AssignUnits assigns the units with the given ids to the correct machine.
func (a *API) AssignUnits(args params.AssignUnitsParams) (params.AssignUnitsResults, error) {
	var result params.AssignUnitsResults
	res, err := a.st.AssignStagedUnits(args.IDs)
	if err != nil {
		result.Error = common.ServerError(err)
	}
	result.Results = make([]params.AssignUnitsResult, len(res))
	for i, r := range res {
		result.Results[i].Unit = r.Unit
		result.Results[i].Error = common.ServerError(r.Error)
	}

	for _, id := range args.IDs {
		if !resContains(result.Results, id) {
			result.Results = append(result.Results,
				params.AssignUnitsResult{
					Unit:  id,
					Error: common.ServerError(errors.NotFoundf("unit %q", id)),
				})
		}
	}

	return result, nil
}

func resContains(res []params.AssignUnitsResult, id string) bool {
	for _, r := range res {
		if r.Unit == id {
			return true
		}
	}
	return false
}

// WatchUnitAssignments returns a strings watcher that is notified when new unit
// assignments are added to the db.
func (a *API) WatchUnitAssignments() (params.StringsWatchResult, error) {
	watch := a.st.WatchForUnitAssignment()
	if changes, ok := <-watch.Changes(); ok {
		return params.StringsWatchResult{
			StringsWatcherId: a.res.Register(watch),
			Changes:          changes,
		}, nil
	}
	return params.StringsWatchResult{}, watcher.EnsureErr(watch)
}
