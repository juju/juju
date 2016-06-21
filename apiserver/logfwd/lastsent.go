// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package logfwd

import (
	"io"

	"github.com/juju/errors"
	"gopkg.in/juju/names.v2"

	"github.com/juju/juju/apiserver/common"
	"github.com/juju/juju/apiserver/params"
	"github.com/juju/juju/state"
)

func init() {
	common.RegisterStandardFacade("LogForwarding", 1, NewLogForwardingAPI)
}

// LogForwardingAPI is the concrete implementation of the api end point.
type LogForwardingAPI struct {
	state      *state.State
	resources  *common.Resources
	authorizer common.Authorizer
}

// NewLogForwardingAPI creates a new server-side logger API end point.
func NewLogForwardingAPI(st *state.State, res *common.Resources, auth common.Authorizer) (*LogForwardingAPI, error) {
	if !auth.AuthMachineAgent() {
		return nil, common.ErrPerm
	}
	api := &LogForwardingAPI{
		state:      st,
		resources:  res,
		authorizer: auth,
	}
	return api, nil
}

// GetLastSent is a bulk call that gets the log forwarding "last sent"
// timestamp for each requested target.
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
	lsl, err := api.newLastSentLogger(id)
	if err != nil {
		res.Error = common.ServerError(err)
		return res
	}
	defer lsl.Close()

	ts, err := lsl.Get()
	if err != nil {
		res.Error = common.ServerError(err)
		if errors.Cause(err) == state.ErrNeverForwarded {
			res.Error.Code = params.CodeNotFound
		}
		return res
	}
	res.Timestamp = ts
	return res
}

// SetLastSent is a bulk call that sets the log forwarding "last sent"
// timestamp for each requested target.
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
	lsl, err := api.newLastSentLogger(arg.LogForwardingID)
	if err != nil {
		return common.ServerError(err)
	}
	defer lsl.Close()

	err = lsl.Set(arg.Timestamp)
	return common.ServerError(err)
}

func (api *LogForwardingAPI) newLastSentLogger(id params.LogForwardingID) (*lastSentCloser, error) {
	tag, err := names.ParseModelTag(id.ModelTag)
	if err != nil {
		return nil, err
	}
	if _, err := api.state.GetModel(tag); err != nil {
		return nil, err
	}
	st, err := api.state.ForModel(tag)
	if err != nil {
		return nil, err
	}
	lastSent := state.NewLastSentLogger(st, id.Sink)
	return &lastSentCloser{lastSent, st}, nil
}

type lastSentCloser struct {
	*state.DbLoggerLastSent
	io.Closer
}
