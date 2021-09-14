// Copyright 2021 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package raft

import (
	"time"

	gomock "github.com/golang/mock/gomock"
	"github.com/hashicorp/raft"
	"github.com/juju/errors"
	"github.com/juju/loggo"
	"github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/worker/raft/queue"
)

type applyOperationSuite struct {
	testing.IsolationSuite

	raft         *MockRaft
	target       *MockNotifyTarget
	applyFuture  *MockApplyFuture
	configFuture *MockConfigurationFuture
	response     *MockFSMResponse
}

var _ = gc.Suite(&applyOperationSuite{})

func (s *applyOperationSuite) TestApplyLease(c *gc.C) {
	defer s.setupMocks(c).Finish()

	cmd := []byte("do it")
	timeout := time.Second

	s.raft.EXPECT().State().Return(raft.Leader)
	s.raft.EXPECT().Apply(cmd, timeout).Return(s.applyFuture)
	s.applyFuture.EXPECT().Error().Return(nil)
	s.applyFuture.EXPECT().Response().Return(s.response)
	s.response.EXPECT().Notify(s.target)
	s.response.EXPECT().Error().Return(nil)

	err := ApplyOperation(s.raft, queue.Operation{Command: cmd, Timeout: timeout}, s.target, fakeLogger{})
	c.Assert(err, jc.ErrorIsNil)
}

func (s *applyOperationSuite) TestApplyLeaseWithError(c *gc.C) {
	defer s.setupMocks(c).Finish()

	cmd := []byte("do it")
	timeout := time.Second

	s.raft.EXPECT().State().Return(raft.Leader)
	s.raft.EXPECT().Apply(cmd, timeout).Return(s.applyFuture)
	s.applyFuture.EXPECT().Error().Return(errors.New("boom"))

	err := ApplyOperation(s.raft, queue.Operation{Command: cmd, Timeout: timeout}, s.target, fakeLogger{})
	c.Assert(err, gc.ErrorMatches, `boom`)
}

func (s *applyOperationSuite) TestApplyLeaseNotLeaderWithNoLeaderAddress(c *gc.C) {
	defer s.setupMocks(c).Finish()

	cmd := []byte("do it")
	timeout := time.Second

	s.raft.EXPECT().State().Return(raft.Follower)
	s.raft.EXPECT().Leader().Return(raft.ServerAddress(""))

	err := ApplyOperation(s.raft, queue.Operation{Command: cmd, Timeout: timeout}, s.target, fakeLogger{})
	c.Assert(err, gc.ErrorMatches, `not currently the leader.*`)
}

func (s *applyOperationSuite) TestApplyLeaseNotLeader(c *gc.C) {
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

	err := ApplyOperation(s.raft, queue.Operation{Command: cmd, Timeout: timeout}, s.target, fakeLogger{})
	c.Assert(err, gc.ErrorMatches, `not currently the leader, try "1"`)
}

func (s *applyOperationSuite) TestApplyLeaseNotLeaderWithNoLeaderID(c *gc.C) {
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

	err := ApplyOperation(s.raft, queue.Operation{Command: cmd, Timeout: timeout}, s.target, fakeLogger{})
	c.Assert(err, gc.ErrorMatches, `not currently the leader, try ""`)
}

func (s *applyOperationSuite) TestApplyLeaseNotLeaderWithConfigurationError(c *gc.C) {
	defer s.setupMocks(c).Finish()

	cmd := []byte("do it")
	timeout := time.Second

	s.raft.EXPECT().State().Return(raft.Follower)
	s.raft.EXPECT().Leader().Return(raft.ServerAddress("1.1.1.1"))
	s.raft.EXPECT().GetConfiguration().Return(s.configFuture)
	s.configFuture.EXPECT().Error().Return(errors.New("boom"))

	err := ApplyOperation(s.raft, queue.Operation{Command: cmd, Timeout: timeout}, s.target, fakeLogger{})
	c.Assert(err, gc.ErrorMatches, `boom`)
}

func (s *applyOperationSuite) setupMocks(c *gc.C) *gomock.Controller {
	ctrl := gomock.NewController(c)

	s.raft = NewMockRaft(ctrl)
	s.target = NewMockNotifyTarget(ctrl)
	s.applyFuture = NewMockApplyFuture(ctrl)
	s.configFuture = NewMockConfigurationFuture(ctrl)
	s.response = NewMockFSMResponse(ctrl)

	return ctrl
}

type fakeLogger struct{}

func (fakeLogger) Warningf(message string, args ...interface{})                {}
func (fakeLogger) Errorf(message string, args ...interface{})                  {}
func (fakeLogger) Infof(message string, args ...interface{})                   {}
func (fakeLogger) Tracef(message string, args ...interface{})                  {}
func (fakeLogger) Logf(level loggo.Level, message string, args ...interface{}) {}
