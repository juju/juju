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
	common.RegisterStandardFacade("LogFwdLastSent", 1, NewLastSentAPI)
}

// LastSentAPI is the concrete implementation of the api end point.
type LastSentAPI struct {
	state      *state.State
	resources  *common.Resources
	authorizer common.Authorizer
}

// NewLastSentAPI creates a new server-side logger API end point.
func NewLastSentAPI(st *state.State, res *common.Resources, auth common.Authorizer) (*LastSentAPI, error) {
	if !auth.AuthMachineAgent() {
		return nil, common.ErrPerm
	}
	api := &LastSentAPI{
		state:      st,
		resources:  res,
		authorizer: auth,
	}
	return api, nil
}

// Get is a bulk call that gets the log forwarding "last sent" timestamp
// for each requested target.
func (api *LastSentAPI) Get(args params.LogFwdLastSentGetParams) params.LogFwdLastSentGetResults {
	results := make([]params.LogFwdLastSentGetResult, len(args.IDs))
	for i, id := range args.IDs {
		results[i] = api.get(id)
	}
	return params.LogFwdLastSentGetResults{
		Results: results,
	}
}

func (api *LastSentAPI) get(id params.LogFwdLastSentID) params.LogFwdLastSentGetResult {
	var res params.LogFwdLastSentGetResult
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

// Set is a bulk call that sets the log forwarding "last sent" timestamp
// for each requested target.
func (api *LastSentAPI) Set(args params.LogFwdLastSentSetParams) params.ErrorResults {
	results := make([]params.ErrorResult, len(args.Params), len(args.Params))
	for i, arg := range args.Params {
		results[i].Error = api.set(arg)
	}
	return params.ErrorResults{
		Results: results,
	}
}

func (api *LastSentAPI) set(arg params.LogFwdLastSentSetParam) *params.Error {
	lsl, err := api.newLastSentLogger(arg.LogFwdLastSentID)
	if err != nil {
		return common.ServerError(err)
	}
	defer lsl.Close()

	err = lsl.Set(arg.Timestamp)
	return common.ServerError(err)
}

func (api *LastSentAPI) newLastSentLogger(id params.LogFwdLastSentID) (*lastSentCloser, error) {
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
