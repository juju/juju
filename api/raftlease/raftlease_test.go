// Copyright 2021 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

// Package raftlease implements the API for sending raft lease messages between
// api servers.
package raftlease

import (
	"time"

	"github.com/golang/mock/gomock"
	"github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/api/base/mocks"
	"github.com/juju/juju/apiserver/params"
)

type RaftLeaseSuite struct {
	testing.IsolationSuite

	facade *mocks.MockFacadeCaller
	caller *mocks.MockAPICaller
}

var _ = gc.Suite(&RaftLeaseSuite{})

func (s *RaftLeaseSuite) TestApplyLease(c *gc.C) {
	defer s.setupMocks(c).Finish()

	arg := params.LeaseOperations{
		Operations: []params.LeaseOperation{{
			Command: "do it",
			Timeout: time.Second,
		}},
	}
	result := params.ErrorResults{
		Results: []params.ErrorResult{{}},
	}
	s.facade.EXPECT().FacadeCall("ApplyLease", arg, gomock.Any()).SetArg(2, result)

	client := s.newAPI(c)
	err := client.ApplyLease("do it", time.Second)
	c.Assert(err, jc.ErrorIsNil)
}

func (s *RaftLeaseSuite) TestApplyLeaseNotTheLeader(c *gc.C) {
	defer s.setupMocks(c).Finish()

	info := map[string]interface{}{
		"server-address": "10.0.0.8",
		"server-id":      "1",
	}
	arg := params.LeaseOperations{
		Operations: []params.LeaseOperation{{
			Command: "do it",
			Timeout: time.Second,
		}},
	}
	result := params.ErrorResults{
		Results: []params.ErrorResult{{
			Error: &params.Error{
				Code:    params.CodeNotLeader,
				Message: "not the leader",
				Info:    info,
			},
		}},
	}
	s.facade.EXPECT().FacadeCall("ApplyLease", arg, gomock.Any()).SetArg(2, result)

	client := s.newAPI(c)
	err := client.ApplyLease("do it", time.Second)
	c.Assert(err, gc.ErrorMatches, "not the leader")
	c.Assert(params.IsCodeNotLeader(err), jc.IsTrue)
	c.Assert(err.(*params.Error).Info, gc.DeepEquals, info)
}

func (s *RaftLeaseSuite) TestApplyLeaseNotNotTheLeaderError(c *gc.C) {
	defer s.setupMocks(c).Finish()

	arg := params.LeaseOperations{
		Operations: []params.LeaseOperation{{
			Command: "do it",
			Timeout: time.Second,
		}},
	}
	result := params.ErrorResults{
		Results: []params.ErrorResult{{
			Error: &params.Error{
				Code:    params.CodeBadRequest,
				Message: "bad request",
			},
		}},
	}
	s.facade.EXPECT().FacadeCall("ApplyLease", arg, gomock.Any()).SetArg(2, result)

	client := s.newAPI(c)
	err := client.ApplyLease("do it", time.Second)
	c.Assert(err, gc.ErrorMatches, "bad request")
	c.Assert(params.IsCodeNotLeader(err), jc.IsFalse)
}

func (s *RaftLeaseSuite) TestApplyLeaseToManyErrors(c *gc.C) {
	defer s.setupMocks(c).Finish()

	arg := params.LeaseOperations{
		Operations: []params.LeaseOperation{{
			Command: "do it",
			Timeout: time.Second,
		}},
	}
	result := params.ErrorResults{
		Results: []params.ErrorResult{{
			Error: &params.Error{
				Code:    params.CodeBadRequest,
				Message: "bad request",
			},
		}, {
			Error: &params.Error{
				Code:    params.CodeBadRequest,
				Message: "bad request",
			},
		}},
	}
	s.facade.EXPECT().FacadeCall("ApplyLease", arg, gomock.Any()).SetArg(2, result)

	client := s.newAPI(c)
	err := client.ApplyLease("do it", time.Second)
	c.Assert(err, gc.ErrorMatches, "expected 1 result, got 2")
}

func (s *RaftLeaseSuite) setupMocks(c *gc.C) *gomock.Controller {
	ctrl := gomock.NewController(c)

	s.facade = mocks.NewMockFacadeCaller(ctrl)
	s.caller = mocks.NewMockAPICaller(ctrl)

	return ctrl
}

func (s *RaftLeaseSuite) newAPI(c *gc.C) *API {
	return &API{
		facade: s.facade,
		caller: s.caller,
	}
}
