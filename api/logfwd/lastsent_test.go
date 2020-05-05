// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package logfwd_test

import (
	"time"

	"github.com/juju/errors"
	"github.com/juju/names/v4"
	"github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/api/logfwd"
	"github.com/juju/juju/apiserver/common"
	"github.com/juju/juju/apiserver/params"
	"github.com/juju/juju/state"
)

type LastSentSuite struct {
	testing.IsolationSuite
}

var _ = gc.Suite(&LastSentSuite{})

func (s *LastSentSuite) TestGetLastSent(c *gc.C) {
	stub := &testing.Stub{}
	caller := &stubFacadeCaller{stub: stub}
	caller.ReturnFacadeCallGet = params.LogForwardingGetLastSentResults{
		Results: []params.LogForwardingGetLastSentResult{{
			RecordID:        10,
			RecordTimestamp: 100,
		}, {
			RecordID:        20,
			RecordTimestamp: 200,
		}, {
			Error: common.ServerError(errors.NewNotFound(state.ErrNeverForwarded, "")),
		}},
	}
	client := logfwd.NewLastSentClient(caller.newFacadeCaller)
	model := "deadbeef-2f18-4fd2-967d-db9663db7bea"
	modelTag := names.NewModelTag(model)

	results, err := client.GetLastSent([]logfwd.LastSentID{{
		Model: modelTag,
		Sink:  "spam",
	}, {
		Model: modelTag,
		Sink:  "eggs",
	}, {
		Model: modelTag,
		Sink:  "ham",
	}})
	c.Assert(err, jc.ErrorIsNil)

	c.Check(results, jc.DeepEquals, []logfwd.LastSentResult{{
		LastSentInfo: logfwd.LastSentInfo{
			LastSentID: logfwd.LastSentID{
				Model: modelTag,
				Sink:  "spam",
			},
			RecordID:        10,
			RecordTimestamp: time.Unix(0, 100),
		},
	}, {
		LastSentInfo: logfwd.LastSentInfo{
			LastSentID: logfwd.LastSentID{
				Model: modelTag,
				Sink:  "eggs",
			},
			RecordID:        20,
			RecordTimestamp: time.Unix(0, 200),
		},
	}, {
		LastSentInfo: logfwd.LastSentInfo{
			LastSentID: logfwd.LastSentID{
				Model: modelTag,
				Sink:  "ham",
			},
		},
		Error: common.RestoreError(&params.Error{
			Message: `cannot find ID of the last forwarded record`,
			Code:    params.CodeNotFound,
		}),
	}})
	stub.CheckCallNames(c, "newFacadeCaller", "FacadeCall")
	stub.CheckCall(c, 0, "newFacadeCaller", "LogForwarding")
	stub.CheckCall(c, 1, "FacadeCall", "GetLastSent", params.LogForwardingGetLastSentParams{
		IDs: []params.LogForwardingID{{
			ModelTag: modelTag.String(),
			Sink:     "spam",
		}, {
			ModelTag: modelTag.String(),
			Sink:     "eggs",
		}, {
			ModelTag: modelTag.String(),
			Sink:     "ham",
		}},
	})
}

func (s *LastSentSuite) TestSetLastSent(c *gc.C) {
	stub := &testing.Stub{}
	caller := &stubFacadeCaller{stub: stub}
	apiError := common.ServerError(errors.New("<failed>"))
	caller.ReturnFacadeCallSet = params.ErrorResults{
		Results: []params.ErrorResult{{
			Error: nil,
		}, {
			Error: nil,
		}, {
			Error: apiError,
		}},
	}
	client := logfwd.NewLastSentClient(caller.newFacadeCaller)
	model := "deadbeef-2f18-4fd2-967d-db9663db7bea"
	modelTag := names.NewModelTag(model)

	results, err := client.SetLastSent([]logfwd.LastSentInfo{{
		LastSentID: logfwd.LastSentID{
			Model: modelTag,
			Sink:  "spam",
		},
		RecordID:        10,
		RecordTimestamp: time.Unix(0, 100),
	}, {
		LastSentID: logfwd.LastSentID{
			Model: modelTag,
			Sink:  "eggs",
		},
		RecordID:        20,
		RecordTimestamp: time.Unix(0, 200),
	}, {
		LastSentID: logfwd.LastSentID{
			Model: modelTag,
			Sink:  "ham",
		},
		RecordID:        15,
		RecordTimestamp: time.Unix(0, 150),
	}})
	c.Assert(err, jc.ErrorIsNil)

	c.Check(results, jc.DeepEquals, []logfwd.LastSentResult{{
		LastSentInfo: logfwd.LastSentInfo{
			LastSentID: logfwd.LastSentID{
				Model: modelTag,
				Sink:  "spam",
			},
			RecordID:        10,
			RecordTimestamp: time.Unix(0, 100),
		},
	}, {
		LastSentInfo: logfwd.LastSentInfo{
			LastSentID: logfwd.LastSentID{
				Model: modelTag,
				Sink:  "eggs",
			},
			RecordID:        20,
			RecordTimestamp: time.Unix(0, 200),
		},
	}, {
		LastSentInfo: logfwd.LastSentInfo{
			LastSentID: logfwd.LastSentID{
				Model: modelTag,
				Sink:  "ham",
			},
			RecordID:        15,
			RecordTimestamp: time.Unix(0, 150),
		},
		Error: common.RestoreError(apiError),
	}})
	stub.CheckCallNames(c, "newFacadeCaller", "FacadeCall")
	stub.CheckCall(c, 0, "newFacadeCaller", "LogForwarding")
	stub.CheckCall(c, 1, "FacadeCall", "SetLastSent", params.LogForwardingSetLastSentParams{
		Params: []params.LogForwardingSetLastSentParam{{
			LogForwardingID: params.LogForwardingID{
				ModelTag: modelTag.String(),
				Sink:     "spam",
			},
			RecordID:        10,
			RecordTimestamp: 100,
		}, {
			LogForwardingID: params.LogForwardingID{
				ModelTag: modelTag.String(),
				Sink:     "eggs",
			},
			RecordID:        20,
			RecordTimestamp: 200,
		}, {
			LogForwardingID: params.LogForwardingID{
				ModelTag: modelTag.String(),
				Sink:     "ham",
			},
			RecordID:        15,
			RecordTimestamp: 150,
		}},
	})
}

type stubFacadeCaller struct {
	stub *testing.Stub

	ReturnFacadeCallGet params.LogForwardingGetLastSentResults
	ReturnFacadeCallSet params.ErrorResults
}

func (s *stubFacadeCaller) newFacadeCaller(facade string) logfwd.FacadeCaller {
	s.stub.AddCall("newFacadeCaller", facade)
	s.stub.NextErr()

	return s
}

func (s *stubFacadeCaller) FacadeCall(request string, args, response interface{}) error {
	s.stub.AddCall("FacadeCall", request, args)
	if err := s.stub.NextErr(); err != nil {
		return err
	}

	switch request {
	case "GetLastSent":
		actual := response.(*params.LogForwardingGetLastSentResults)
		*actual = s.ReturnFacadeCallGet
	case "SetLastSent":
		actual := response.(*params.ErrorResults)
		*actual = s.ReturnFacadeCallSet
	}
	return nil
}
