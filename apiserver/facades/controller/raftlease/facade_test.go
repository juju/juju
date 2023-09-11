// Copyright 2021 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package raftlease

import (
	"context"
	"encoding/json"
	"errors"

	"github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	"go.uber.org/mock/gomock"
	gc "gopkg.in/check.v1"

	apiservererrors "github.com/juju/juju/apiserver/errors"
	"github.com/juju/juju/core/raftlease"
	"github.com/juju/juju/rpc/params"
)

type RaftLeaseSuite struct {
	testing.IsolationSuite

	context *MockContext
	auth    *MockAuthorizer
	raft    *MockRaftContext
}

var _ = gc.Suite(&RaftLeaseSuite{})

func (s *RaftLeaseSuite) TestApplyLeaseV2(c *gc.C) {
	defer s.setupMocks(c).Finish()

	s.auth.EXPECT().AuthController().Return(true)
	s.raft.EXPECT().ApplyLease(gomock.Any(), raftlease.Command{
		Operation: "claim",
		Lease:     "singular-worker",
	}).Return(nil)

	facade, err := newFacadeV2(s.context)
	c.Assert(err, jc.ErrorIsNil)

	results, err := facade.ApplyLease(context.Background(), params.LeaseOperationsV2{
		Operations: []params.LeaseOperationCommand{{
			Operation: "claim",
			Lease:     "singular-worker",
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
	s.raft.EXPECT().ApplyLease(gomock.Any(), raftlease.Command{
		Operation: "claim",
		Lease:     "singular-worker-0",
	}).Return(nil)
	s.raft.EXPECT().ApplyLease(gomock.Any(), raftlease.Command{
		Operation: "claim",
		Lease:     "singular-worker-1",
	}).Return(apiservererrors.NewNotLeaderError("10.0.0.8", "1"))

	facade, err := newFacadeV2(s.context)
	c.Assert(err, jc.ErrorIsNil)

	results, err := facade.ApplyLease(context.Background(), params.LeaseOperationsV2{
		Operations: []params.LeaseOperationCommand{{
			Operation: "claim",
			Lease:     "singular-worker-0",
		}, {
			Operation: "claim",
			Lease:     "singular-worker-1",
		}, {
			Operation: "claim",
			Lease:     "singular-worker-2",
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
	s.raft.EXPECT().ApplyLease(gomock.Any(), raftlease.Command{
		Operation: "claim",
		Lease:     "singular-worker-0",
	}).Return(nil)
	s.raft.EXPECT().ApplyLease(gomock.Any(), raftlease.Command{
		Operation: "claim",
		Lease:     "singular-worker-1",
	}).Return(errors.New("boom"))
	s.raft.EXPECT().ApplyLease(gomock.Any(), raftlease.Command{
		Operation: "claim",
		Lease:     "singular-worker-2",
	}).Return(nil)

	facade, err := NewFacade(s.context)
	c.Assert(err, jc.ErrorIsNil)

	results, err := facade.ApplyLease(context.Background(), params.LeaseOperationsV2{
		Operations: []params.LeaseOperationCommand{{
			Operation: "claim",
			Lease:     "singular-worker-0",
		}, {
			Operation: "claim",
			Lease:     "singular-worker-1",
		}, {
			Operation: "claim",
			Lease:     "singular-worker-2",
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

func (s *RaftLeaseSuite) TestApplyLeaseAuthFailureV2(c *gc.C) {
	defer s.setupMocks(c).Finish()

	s.auth.EXPECT().AuthController().Return(false)

	_, err := newFacadeV2(s.context)
	c.Assert(err, gc.ErrorMatches, `permission denied`)
}

func (s *RaftLeaseSuite) setupMocks(c *gc.C) *gomock.Controller {
	ctrl := gomock.NewController(c)

	s.auth = NewMockAuthorizer(ctrl)
	s.raft = NewMockRaftContext(ctrl)

	s.context = NewMockContext(ctrl)
	s.context.EXPECT().Auth().Return(s.auth).AnyTimes()
	s.context.EXPECT().Raft().Return(s.raft).AnyTimes()

	return ctrl
}

func MustCreateStringCommand(c *gc.C, lease string) string {
	cmd := params.LeaseOperationCommand{
		Operation: "claim",
		Lease:     lease,
	}
	bytes, err := json.Marshal(cmd)
	c.Assert(err, jc.ErrorIsNil)

	return string(bytes)
}
