// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package logfwd_test

import (
	"time"

	"github.com/juju/errors"
	"github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"
	"gopkg.in/juju/names.v2"

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
	tsSpam := time.Unix(12345, 0)
	tsEggs := time.Unix(12345, 54321)
	caller.ReturnFacadeCallGet = params.LogForwardingGetLastSentResults{
		Results: []params.LogForwardingGetLastSentResult{{
			Timestamp: tsSpam.UTC(),
		}, {
			Timestamp: tsEggs.UTC(),
		}, {
			Error: common.ServerError(errors.NewNotFound(state.ErrNeverForwarded, "")),
		}},
	}
	client := logfwd.NewLastSentClient(caller.newFacadeCaller)
	model := "deadbeef-2f18-4fd2-967d-db9663db7bea"
	modelTag := names.NewModelTag(model)

	results, err := client.GetList([]logfwd.LastSentID{{
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
			Timestamp: tsSpam.UTC(),
		},
	}, {
		LastSentInfo: logfwd.LastSentInfo{
			LastSentID: logfwd.LastSentID{
				Model: modelTag,
				Sink:  "eggs",
			},
			Timestamp: tsEggs.UTC(),
		},
	}, {
		LastSentInfo: logfwd.LastSentInfo{
			LastSentID: logfwd.LastSentID{
				Model: modelTag,
				Sink:  "ham",
			},
		},
		Error: common.RestoreError(&params.Error{
			Message: `cannot find timestamp of the last forwarded record`,
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
	tsSpam := time.Unix(12345, 0)
	tsEggs := time.Unix(12345, 54321)
	tsHam := time.Unix(98789, 12321)
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

	results, err := client.SetList([]logfwd.LastSentInfo{{
		LastSentID: logfwd.LastSentID{
			Model: modelTag,
			Sink:  "spam",
		},
		Timestamp: tsSpam,
	}, {
		LastSentID: logfwd.LastSentID{
			Model: modelTag,
			Sink:  "eggs",
		},
		Timestamp: tsEggs,
	}, {
		LastSentID: logfwd.LastSentID{
			Model: modelTag,
			Sink:  "ham",
		},
		Timestamp: tsHam,
	}})
	c.Assert(err, jc.ErrorIsNil)

	c.Check(results, jc.DeepEquals, []logfwd.LastSentResult{{
		LastSentInfo: logfwd.LastSentInfo{
			LastSentID: logfwd.LastSentID{
				Model: modelTag,
				Sink:  "spam",
			},
			Timestamp: tsSpam,
		},
	}, {
		LastSentInfo: logfwd.LastSentInfo{
			LastSentID: logfwd.LastSentID{
				Model: modelTag,
				Sink:  "eggs",
			},
			Timestamp: tsEggs,
		},
	}, {
		LastSentInfo: logfwd.LastSentInfo{
			LastSentID: logfwd.LastSentID{
				Model: modelTag,
				Sink:  "ham",
			},
			Timestamp: tsHam,
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
			Timestamp: tsSpam,
		}, {
			LogForwardingID: params.LogForwardingID{
				ModelTag: modelTag.String(),
				Sink:     "eggs",
			},
			Timestamp: tsEggs,
		}, {
			LogForwardingID: params.LogForwardingID{
				ModelTag: modelTag.String(),
				Sink:     "ham",
			},
			Timestamp: tsHam,
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
