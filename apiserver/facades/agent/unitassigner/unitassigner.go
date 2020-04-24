// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package unitassigner

import (
	"github.com/juju/errors"
	"github.com/juju/names/v4"

	"github.com/juju/juju/apiserver/common"
	"github.com/juju/juju/apiserver/facade"
	"github.com/juju/juju/apiserver/params"
	"github.com/juju/juju/state"
	"github.com/juju/juju/state/watcher"
)

// assignerState defines the state methods this facade needs, so they can be mocked
// for testing.
type assignerState interface {
	WatchForUnitAssignment() state.StringsWatcher
	AssignStagedUnits(ids []string) ([]state.UnitAssignmentResult, error)
}

type statusSetter interface {
	SetStatus(args params.SetStatus) (params.ErrorResults, error)
}

// API implements the functionality for assigning units to machines.
type API struct {
	st           assignerState
	res          facade.Resources
	statusSetter statusSetter
}

// New returns a new unitAssigner api instance.
func New(st *state.State, res facade.Resources, _ facade.Authorizer) (*API, error) {
	setter := common.NewStatusSetter(&common.UnitAgentFinder{st}, common.AuthAlways())
	return &API{
		st:           st,
		res:          res,
		statusSetter: setter,
	}, nil
}

//  AssignUnits assigns the units with the given ids to the correct machine. The
//  error results are returned in the same order as the given entities.
func (a *API) AssignUnits(args params.Entities) (params.ErrorResults, error) {
	result := params.ErrorResults{}

	// state uses ids, but the API uses Tags, so we have to convert back and
	// forth (whee!).  The list of ids is (crucially) in the same order as the
	// list of tags.  This is the same order as the list of errors we return.
	ids := make([]string, len(args.Entities))
	for i, e := range args.Entities {
		tag, err := names.ParseUnitTag(e.Tag)
		if err != nil {
			return result, err
		}
		ids[i] = tag.Id()
	}

	res, err := a.st.AssignStagedUnits(ids)
	if err != nil {
		return result, common.ServerError(err)
	}

	// The results come back from state in an undetermined order and do not
	// include results for units that were not found, so we have to make up for
	// that here.
	resultMap := make(map[string]error, len(ids))
	for _, r := range res {
		resultMap[r.Unit] = r.Error
	}

	result.Results = make([]params.ErrorResult, len(args.Entities))
	for i, id := range ids {
		if err, ok := resultMap[id]; ok {
			result.Results[i].Error = common.ServerError(err)
		} else {
			result.Results[i].Error =
				common.ServerError(errors.NotFoundf("unit %q", args.Entities[i].Tag))
		}
	}

	return result, nil
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

// SetAgentStatus will set status for agents of Units passed in args, if one
// of the args is not an Unit it will fail.
func (a *API) SetAgentStatus(args params.SetStatus) (params.ErrorResults, error) {
	return a.statusSetter.SetStatus(args)
}
