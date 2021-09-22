// Copyright 2021 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package raftlease

import (
	"errors"

	"github.com/golang/mock/gomock"
	"github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	apiservererrors "github.com/juju/juju/apiserver/errors"
	"github.com/juju/juju/apiserver/params"
)

type RaftLeaseSuite struct {
	testing.IsolationSuite

	auth *MockAuthorizer
	raft *MockRaftContext
}

var _ = gc.Suite(&RaftLeaseSuite{})

func (s *RaftLeaseSuite) TestApplyLease(c *gc.C) {
	defer s.setupMocks(c).Finish()

	s.auth.EXPECT().AuthController().Return(true)
	s.raft.EXPECT().ApplyLease([]byte("do it")).Return(nil)

	facade, err := NewFacade(s.auth, s.raft)
	c.Assert(err, jc.ErrorIsNil)

	results, err := facade.ApplyLease(params.LeaseOperations{
		Operations: []params.LeaseOperation{{
			Command: "do it",
		}},
	})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(results, gc.DeepEquals, params.ErrorResults{
		Results: make([]params.ErrorResult, 1),
	})
}

func (s *RaftLeaseSuite) TestApplyLeaseNotLeaderError(c *gc.C) {
	defer s.setupMocks(c).Finish()

	s.auth.EXPECT().AuthController().Return(true)
	s.raft.EXPECT().ApplyLease([]byte("do it 0")).Return(nil)
	s.raft.EXPECT().ApplyLease([]byte("do it 1")).Return(apiservererrors.NewNotLeaderError("10.0.0.8", "1"))

	facade, err := NewFacade(s.auth, s.raft)
	c.Assert(err, jc.ErrorIsNil)

	results, err := facade.ApplyLease(params.LeaseOperations{
		Operations: []params.LeaseOperation{{
			Command: "do it 0",
		}, {
			Command: "do it 1",
		}, {
			Command: "do it 2",
		}},
	})
	c.Assert(err, jc.ErrorIsNil)

	paramErr := &params.Error{
		Message: `not currently the leader, try "1"`,
		Code:    "not leader",
		Info: map[string]interface{}{
			"server-address": "10.0.0.8",
			"server-id":      "1",
		},
	}

	c.Assert(results, gc.DeepEquals, params.ErrorResults{
		Results: []params.ErrorResult{
			{},
			{Error: paramErr},
			{Error: paramErr},
		},
	})
}

func (s *RaftLeaseSuite) TestApplyLeaseError(c *gc.C) {
	defer s.setupMocks(c).Finish()

	s.auth.EXPECT().AuthController().Return(true)
	s.raft.EXPECT().ApplyLease([]byte("do it 0")).Return(nil)
	s.raft.EXPECT().ApplyLease([]byte("do it 1")).Return(errors.New("boom"))
	s.raft.EXPECT().ApplyLease([]byte("do it 2")).Return(nil)

	facade, err := NewFacade(s.auth, s.raft)
	c.Assert(err, jc.ErrorIsNil)

	results, err := facade.ApplyLease(params.LeaseOperations{
		Operations: []params.LeaseOperation{{
			Command: "do it 0",
		}, {
			Command: "do it 1",
		}, {
			Command: "do it 2",
		}},
	})
	c.Assert(err, jc.ErrorIsNil)

	c.Assert(results, gc.DeepEquals, params.ErrorResults{
		Results: []params.ErrorResult{
			{},
			{Error: &params.Error{
				Message: `boom`,
			}},
			{},
		},
	})
}

func (s *RaftLeaseSuite) TestApplyLeaseAuthFailure(c *gc.C) {
	defer s.setupMocks(c).Finish()

	s.auth.EXPECT().AuthController().Return(false)

	_, err := NewFacade(s.auth, s.raft)
	c.Assert(err, gc.ErrorMatches, `permission denied`)
}

func (s *RaftLeaseSuite) setupMocks(c *gc.C) *gomock.Controller {
	ctrl := gomock.NewController(c)

	s.auth = NewMockAuthorizer(ctrl)
	s.raft = NewMockRaftContext(ctrl)

	return ctrl
}
