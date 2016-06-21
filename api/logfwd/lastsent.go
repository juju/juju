// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package logfwd

import (
	"time"

	"github.com/juju/errors"
	"gopkg.in/juju/names.v2"

	"github.com/juju/juju/apiserver/common"
	"github.com/juju/juju/apiserver/params"
)

// LastSentID is the data that identifies a log forwarding
// "last sent" value. The controller has a mapping from a set of IDs
// to a timestamp (for each ID). The timestamp corresponds to the last
// log record that the specific log forwarding machinery sent to the
// identified "sink" (for a given model).
type LastSentID struct {
	// ModelTag identifies the model associated with the log record.
	Model names.ModelTag

	// Sink is the name of the log forwarding target to which a log
	// record was last sent.
	Sink string
}

// LastSentInfo holds the info about a "last sent" value.
type LastSentInfo struct {
	LastSentID

	// Timestamp identifies the last log record that was forwarded
	// for a given model and sink.
	//
	// Note that if more than one log record has the same timestamp
	// down to the nanosecond then the timestamp will not identify any
	// of them uniquely. However, the likelihood of such a collision
	// is remote (though it grows with more agents and more activity).
	Timestamp time.Time
}

// LastSentResult holds a single result from a bulk API call.
type LastSentResult struct {
	LastSentInfo

	// Error holds the error, if any, that resulted while handling the
	// request for the ID.
	Error error
}

// FacadeCaller provides the functionality to call methods on a facade.
type FacadeCaller interface {
	// FacadeCall is the same method as on base.FacadeCaller.
	FacadeCall(request string, params, response interface{}) error
}

// LastSentClient exposes the "last sent" methods of the LogForwarding
// API facade.
type LastSentClient struct {
	caller FacadeCaller
}

// NewLastSentClient creates a new API client for the facade.
func NewLastSentClient(newFacadeCaller func(string) FacadeCaller) *LastSentClient {
	return &LastSentClient{
		caller: newFacadeCaller("LogForwarding"),
	}
}

// GetList makes a "GetLastSent" call on the facade and returns the
// results in the same order.
func (c LastSentClient) GetList(ids []LastSentID) ([]LastSentResult, error) {
	var args params.LogForwardingGetLastSentParams
	args.IDs = make([]params.LogForwardingID, len(ids))
	for i, id := range ids {
		args.IDs[i] = params.LogForwardingID{
			ModelTag: id.Model.String(),
			Sink:     id.Sink,
		}
	}

	var apiResults params.LogForwardingGetLastSentResults
	err := c.caller.FacadeCall("GetLastSent", args, &apiResults)
	if err != nil {
		return nil, errors.Trace(err)
	}

	results := make([]LastSentResult, len(ids))
	for i, apiRes := range apiResults.Results {
		results[i] = LastSentResult{
			LastSentInfo: LastSentInfo{
				LastSentID: ids[i],
				Timestamp:  apiRes.Timestamp,
			},
			Error: common.RestoreError(apiRes.Error),
		}
	}
	return results, nil
}

// SetList makes a "SetLastSent" call on the facade and returns the
// results in the same order.
func (c LastSentClient) SetList(reqs []LastSentInfo) ([]LastSentResult, error) {
	var args params.LogForwardingSetLastSentParams
	args.Params = make([]params.LogForwardingSetLastSentParam, len(reqs))
	for i, req := range reqs {
		args.Params[i] = params.LogForwardingSetLastSentParam{
			LogForwardingID: params.LogForwardingID{
				ModelTag: req.Model.String(),
				Sink:     req.Sink,
			},
			Timestamp: req.Timestamp,
		}
	}

	var apiResults params.ErrorResults
	err := c.caller.FacadeCall("SetLastSent", args, &apiResults)
	if err != nil {
		return nil, errors.Trace(err)
	}

	results := make([]LastSentResult, len(reqs))
	for i, apiRes := range apiResults.Results {
		results[i] = LastSentResult{
			LastSentInfo: reqs[i],
			Error:        common.RestoreError(apiRes.Error),
		}
	}
	return results, nil
}
