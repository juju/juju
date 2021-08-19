// Copyright 2021 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package apiserver

import (
	"time"

	"github.com/golang/mock/gomock"
	"github.com/hashicorp/raft"
	"github.com/juju/errors"
	"github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"
)

type raftMediatorSuite struct {
	testing.IsolationSuite

	raft         *MockRaft
	target       *MockNotifyTarget
	applyFuture  *MockApplyFuture
	configFuture *MockConfigurationFuture
	response     *MockFSMResponse
}

var _ = gc.Suite(&raftMediatorSuite{})

func (s *raftMediatorSuite) TestApplyLease(c *gc.C) {
	defer s.setupMocks(c).Finish()

	cmd := []byte("do it")
	timeout := time.Second

	s.raft.EXPECT().State().Return(raft.Leader)
	s.raft.EXPECT().Apply(cmd, timeout).Return(s.applyFuture)
	s.applyFuture.EXPECT().Error().Return(nil)
	s.applyFuture.EXPECT().Response().Return(s.response)
	s.response.EXPECT().Notify(s.target)

	mediator := raftMediator{
		raft:         s.raft,
		notifyTarget: s.target,
	}
	err := mediator.ApplyLease(cmd, timeout)
	c.Assert(err, jc.ErrorIsNil)
}

func (s *raftMediatorSuite) TestApplyLeaseWithError(c *gc.C) {
	defer s.setupMocks(c).Finish()

	cmd := []byte("do it")
	timeout := time.Second

	s.raft.EXPECT().State().Return(raft.Leader)
	s.raft.EXPECT().Apply(cmd, timeout).Return(s.applyFuture)
	s.applyFuture.EXPECT().Error().Return(errors.New("boom"))

	mediator := raftMediator{
		raft:         s.raft,
		notifyTarget: s.target,
	}
	err := mediator.ApplyLease(cmd, timeout)
	c.Assert(err, gc.ErrorMatches, `boom`)
}

func (s *raftMediatorSuite) TestApplyLeaseNotLeaderWithNoLeaderAddress(c *gc.C) {
	defer s.setupMocks(c).Finish()

	cmd := []byte("do it")
	timeout := time.Second

	s.raft.EXPECT().State().Return(raft.Follower)
	s.raft.EXPECT().Leader().Return(raft.ServerAddress(""))

	mediator := raftMediator{
		raft:         s.raft,
		notifyTarget: s.target,
	}
	err := mediator.ApplyLease(cmd, timeout)
	c.Assert(err, gc.ErrorMatches, `not currently the leader.*`)
}

func (s *raftMediatorSuite) TestApplyLeaseNotLeader(c *gc.C) {
	defer s.setupMocks(c).Finish()

	cmd := []byte("do it")
	timeout := time.Second

	s.raft.EXPECT().State().Return(raft.Follower)
	s.raft.EXPECT().Leader().Return(raft.ServerAddress("1.1.1.1"))
	s.raft.EXPECT().GetConfiguration().Return(s.configFuture)
	s.configFuture.EXPECT().Error().Return(nil)
	s.configFuture.EXPECT().Configuration().Return(raft.Configuration{
		Servers: []raft.Server{{
			Address: "1.1.1.1",
			ID:      "1",
		}},
	})

	mediator := raftMediator{
		raft:         s.raft,
		notifyTarget: s.target,
	}
	err := mediator.ApplyLease(cmd, timeout)
	c.Assert(err, gc.ErrorMatches, `not currently the leader, try "1"`)
}

func (s *raftMediatorSuite) TestApplyLeaseNotLeaderWithNoLeaderID(c *gc.C) {
	defer s.setupMocks(c).Finish()

	cmd := []byte("do it")
	timeout := time.Second

	s.raft.EXPECT().State().Return(raft.Follower)
	s.raft.EXPECT().Leader().Return(raft.ServerAddress("1.1.1.1"))
	s.raft.EXPECT().GetConfiguration().Return(s.configFuture)
	s.configFuture.EXPECT().Error().Return(nil)
	s.configFuture.EXPECT().Configuration().Return(raft.Configuration{
		Servers: []raft.Server{{
			Address: "2.2.2.2",
			ID:      "1",
		}},
	})

	mediator := raftMediator{
		raft:         s.raft,
		notifyTarget: s.target,
	}
	err := mediator.ApplyLease(cmd, timeout)
	c.Assert(err, gc.ErrorMatches, `not currently the leader, try ""`)
}

func (s *raftMediatorSuite) TestApplyLeaseNotLeaderWithConfigurationError(c *gc.C) {
	defer s.setupMocks(c).Finish()

	cmd := []byte("do it")
	timeout := time.Second

	s.raft.EXPECT().State().Return(raft.Follower)
	s.raft.EXPECT().Leader().Return(raft.ServerAddress("1.1.1.1"))
	s.raft.EXPECT().GetConfiguration().Return(s.configFuture)
	s.configFuture.EXPECT().Error().Return(errors.New("boom"))

	mediator := raftMediator{
		raft:         s.raft,
		notifyTarget: s.target,
	}
	err := mediator.ApplyLease(cmd, timeout)
	c.Assert(err, gc.ErrorMatches, `boom`)
}

func (s *raftMediatorSuite) setupMocks(c *gc.C) *gomock.Controller {
	ctrl := gomock.NewController(c)

	s.raft = NewMockRaft(ctrl)
	s.target = NewMockNotifyTarget(ctrl)
	s.applyFuture = NewMockApplyFuture(ctrl)
	s.configFuture = NewMockConfigurationFuture(ctrl)
	s.response = NewMockFSMResponse(ctrl)

	return ctrl
}
