// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package logfwd

import (
	"io"

	"github.com/juju/errors"
	"github.com/juju/names/v4"

	"github.com/juju/juju/apiserver/common"
	"github.com/juju/juju/apiserver/facade"
	"github.com/juju/juju/apiserver/params"
	"github.com/juju/juju/state"
)

// NewFacade creates a new LogForwardingAPI. It is used for API registration.
func NewFacade(st *state.State, _ facade.Resources, auth facade.Authorizer) (*LogForwardingAPI, error) {
	return NewLogForwardingAPI(&stateAdapter{st}, auth)
}

// LastSentTracker exposes the functionality of state.LastSentTracker.
type LastSentTracker interface {
	io.Closer

	// Get retrieves the record ID and timestamp.
	Get() (recID int64, recTimestamp int64, err error)

	// Set records the record ID and timestamp.
	Set(recID int64, recTimestamp int64) error
}

// LogForwardingState supports interacting with state for the
// LogForwarding facade.
type LogForwardingState interface {
	// NewLastSentTracker creates a new tracker for the given model
	// and log sink.
	NewLastSentTracker(tag names.ModelTag, sink string) LastSentTracker
}

// LogForwardingAPI is the concrete implementation of the api end point.
type LogForwardingAPI struct {
	state LogForwardingState
}

// NewLogForwardingAPI creates a new server-side logger API end point.
func NewLogForwardingAPI(st LogForwardingState, auth facade.Authorizer) (*LogForwardingAPI, error) {
	if !auth.AuthController() {
		return nil, common.ErrPerm
	}
	api := &LogForwardingAPI{
		state: st,
	}
	return api, nil
}

// GetLastSent is a bulk call that gets the log forwarding "last sent"
// record ID for each requested target.
func (api *LogForwardingAPI) GetLastSent(args params.LogForwardingGetLastSentParams) params.LogForwardingGetLastSentResults {
	results := make([]params.LogForwardingGetLastSentResult, len(args.IDs))
	for i, id := range args.IDs {
		results[i] = api.get(id)
	}
	return params.LogForwardingGetLastSentResults{
		Results: results,
	}
}

func (api *LogForwardingAPI) get(id params.LogForwardingID) params.LogForwardingGetLastSentResult {
	var res params.LogForwardingGetLastSentResult
	lst, err := api.newLastSentTracker(id)
	if err != nil {
		res.Error = common.ServerError(err)
		return res
	}
	defer lst.Close()

	recID, recTimestamp, err := lst.Get()
	if err != nil {
		res.Error = common.ServerError(err)
		if errors.Cause(err) == state.ErrNeverForwarded {
			res.Error.Code = params.CodeNotFound
		}
		return res
	}
	res.RecordID = recID
	res.RecordTimestamp = recTimestamp
	return res
}

// SetLastSent is a bulk call that sets the log forwarding "last sent"
// record ID for each requested target.
func (api *LogForwardingAPI) SetLastSent(args params.LogForwardingSetLastSentParams) params.ErrorResults {
	results := make([]params.ErrorResult, len(args.Params), len(args.Params))
	for i, arg := range args.Params {
		results[i].Error = api.set(arg)
	}
	return params.ErrorResults{
		Results: results,
	}
}

func (api *LogForwardingAPI) set(arg params.LogForwardingSetLastSentParam) *params.Error {
	lst, err := api.newLastSentTracker(arg.LogForwardingID)
	if err != nil {
		return common.ServerError(err)
	}
	defer lst.Close()

	err = lst.Set(arg.RecordID, arg.RecordTimestamp)
	return common.ServerError(err)
}

func (api *LogForwardingAPI) newLastSentTracker(id params.LogForwardingID) (LastSentTracker, error) {
	tag, err := names.ParseModelTag(id.ModelTag)
	if err != nil {
		return nil, err
	}
	tracker := api.state.NewLastSentTracker(tag, id.Sink)
	return tracker, nil
}

type stateAdapter struct {
	*state.State
}

// NewLastSentTracker implements LogForwardingState.
func (st stateAdapter) NewLastSentTracker(tag names.ModelTag, sink string) LastSentTracker {
	return state.NewLastSentLogTracker(st, tag.Id(), sink)
}
