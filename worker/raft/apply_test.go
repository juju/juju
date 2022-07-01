// Copyright 2021 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package raft

import (
	"fmt"
	"time"

	"github.com/golang/mock/gomock"
	"github.com/hashicorp/raft"
	"github.com/juju/clock/testclock"
	"github.com/juju/errors"
	"github.com/juju/loggo"
	"github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/v3/core/raft/queue"
)

type applyOperationSuite struct {
	testing.IsolationSuite

	applier LeaseApplier

	raft         *MockRaft
	target       *MockNotifyTarget
	applyFuture  *MockApplyFuture
	configFuture *MockConfigurationFuture
	response     *MockFSMResponse
	metrics      *MockApplierMetrics
	clock        *testclock.Clock
}

var _ = gc.Suite(&applyOperationSuite{})

func (s *applyOperationSuite) TestApplyLease(c *gc.C) {
	defer s.setupMocks(c).Finish()

	s.expectSuccessMetrics()

	cmds, done := commandsN(1)
	timeout := time.Second

	s.raft.EXPECT().Apply(cmds[0].Command, timeout).Return(s.applyFuture)
	s.applyFuture.EXPECT().Error().Return(nil)
	s.applyFuture.EXPECT().Response().Return(s.response)
	s.response.EXPECT().Notify(s.target)
	s.response.EXPECT().Error().Return(nil)

	s.applier.ApplyOperation(cmds, timeout)

	assertNilErrorsN(c, done, 1)
}

func (s *applyOperationSuite) TestApplyLeaseMultipleCommands(c *gc.C) {
	defer s.setupMocks(c).Finish()

	s.expectSuccessMetrics()

	cmds, done := commandsN(2)
	timeout := time.Second

	s.raft.EXPECT().Apply(cmds[0].Command, timeout).Return(s.applyFuture)
	s.raft.EXPECT().Apply(cmds[1].Command, timeout).Return(s.applyFuture)
	s.applyFuture.EXPECT().Error().Return(nil).Times(2)
	s.applyFuture.EXPECT().Response().Return(s.response).Times(2)
	s.response.EXPECT().Notify(s.target).Times(2)
	s.response.EXPECT().Error().Return(nil).Times(2)

	s.applier.ApplyOperation(cmds, timeout)

	assertNilErrorsN(c, done, 2)
}

func (s *applyOperationSuite) TestApplyLeaseWithProgrammingError(c *gc.C) {
	defer s.setupMocks(c).Finish()

	s.expectFailureMetrics()

	cmds, done := commandsN(1)
	timeout := time.Second

	s.raft.EXPECT().Apply(cmds[0].Command, timeout).Return(s.applyFuture)
	s.applyFuture.EXPECT().Error().Return(nil)
	s.applyFuture.EXPECT().Response().Return(struct{}{})

	s.applier.ApplyOperation(cmds, timeout)

	assertError(c, done, `invalid FSM response`)
}

func (s *applyOperationSuite) TestApplyLeaseWithError(c *gc.C) {
	defer s.setupMocks(c).Finish()

	s.expectFailureMetrics()

	cmds, done := commandsN(1)
	timeout := time.Second

	s.raft.EXPECT().Apply(cmds[0].Command, timeout).Return(s.applyFuture)
	s.applyFuture.EXPECT().Error().Return(errors.New("boom"))

	s.applier.ApplyOperation(cmds, timeout)

	assertError(c, done, `boom`)
}

func (s *applyOperationSuite) TestApplyLeaseNotLeaderWithNoLeaderAddress(c *gc.C) {
	defer s.setupMocks(c).Finish()

	s.expectFailureMetrics()

	cmds, done := commandsN(1)
	timeout := time.Second

	s.applyFuture.EXPECT().Error().Return(raft.ErrNotLeader)

	exp := s.raft.EXPECT()
	exp.Apply(cmds[0].Command, timeout).Return(s.applyFuture)
	exp.State().Return(raft.Follower)
	exp.Leader().Return(raft.ServerAddress(""))

	s.applier.ApplyOperation(cmds, timeout)

	assertError(c, done, `not currently the leader, try ""`)
}

func (s *applyOperationSuite) TestApplyLeaseNotLeader(c *gc.C) {
	defer s.setupMocks(c).Finish()

	s.expectFailureMetrics()

	cmds, done := commandsN(1)
	timeout := time.Second

	s.applyFuture.EXPECT().Error().Return(raft.ErrNotLeader)

	exp := s.raft.EXPECT()
	exp.Apply(cmds[0].Command, timeout).Return(s.applyFuture)
	exp.State().Return(raft.Follower)
	exp.Leader().Return(raft.ServerAddress("1.1.1.1"))
	exp.GetConfiguration().Return(s.configFuture)
	s.configFuture.EXPECT().Error().Return(nil)
	s.configFuture.EXPECT().Configuration().Return(raft.Configuration{
		Servers: []raft.Server{{
			Address: "1.1.1.1",
			ID:      "1",
		}},
	})

	s.applier.ApplyOperation(cmds, timeout)

	assertError(c, done, `not currently the leader, try "1"`)
}

func (s *applyOperationSuite) TestApplyLeaseLeaderLost(c *gc.C) {
	defer s.setupMocks(c).Finish()

	s.expectFailureMetrics()

	cmds, done := commandsN(1)
	timeout := time.Second

	s.applyFuture.EXPECT().Error().Return(raft.ErrLeadershipLost)

	exp := s.raft.EXPECT()
	exp.Apply(cmds[0].Command, timeout).Return(s.applyFuture)
	exp.State().Return(raft.Follower)
	exp.Leader().Return(raft.ServerAddress("1.1.1.1"))
	exp.GetConfiguration().Return(s.configFuture)
	s.configFuture.EXPECT().Error().Return(nil)
	s.configFuture.EXPECT().Configuration().Return(raft.Configuration{
		Servers: []raft.Server{{
			Address: "1.1.1.1",
			ID:      "1",
		}},
	})

	s.applier.ApplyOperation(cmds, timeout)

	assertError(c, done, `not currently the leader, try "1"`)
}

func (s *applyOperationSuite) TestApplyLeaseLeaderTransferring(c *gc.C) {
	defer s.setupMocks(c).Finish()

	s.expectFailureMetrics()

	cmds, done := commandsN(1)
	timeout := time.Second

	s.applyFuture.EXPECT().Error().Return(raft.ErrLeadershipTransferInProgress)

	exp := s.raft.EXPECT()
	exp.Apply(cmds[0].Command, timeout).Return(s.applyFuture)
	exp.State().Return(raft.Follower)
	exp.Leader().Return(raft.ServerAddress("1.1.1.1"))
	exp.GetConfiguration().Return(s.configFuture)
	s.configFuture.EXPECT().Error().Return(nil)
	s.configFuture.EXPECT().Configuration().Return(raft.Configuration{
		Servers: []raft.Server{{
			Address: "1.1.1.1",
			ID:      "1",
		}},
	})

	s.applier.ApplyOperation(cmds, timeout)

	assertError(c, done, `not currently the leader, try "1"`)
}

func (s *applyOperationSuite) TestApplyLeaseNotLeaderWithNoLeaderID(c *gc.C) {
	defer s.setupMocks(c).Finish()

	s.expectFailureMetrics()

	cmds, done := commandsN(1)
	timeout := time.Second

	s.applyFuture.EXPECT().Error().Return(raft.ErrNotLeader)

	exp := s.raft.EXPECT()
	exp.Apply(cmds[0].Command, timeout).Return(s.applyFuture)
	exp.State().Return(raft.Follower)
	exp.Leader().Return(raft.ServerAddress("1.1.1.1"))
	exp.GetConfiguration().Return(s.configFuture)
	s.configFuture.EXPECT().Error().Return(nil)
	s.configFuture.EXPECT().Configuration().Return(raft.Configuration{
		Servers: []raft.Server{{
			Address: "2.2.2.2",
			ID:      "2",
		}},
	})

	s.applier.ApplyOperation(cmds, timeout)

	assertError(c, done, `not currently the leader, try ""`)
}

func (s *applyOperationSuite) TestApplyLeaseNotLeaderWithConfigurationError(c *gc.C) {
	defer s.setupMocks(c).Finish()

	s.expectFailureMetrics()

	cmds, done := commandsN(1)
	timeout := time.Second

	s.applyFuture.EXPECT().Error().Return(raft.ErrNotLeader)

	exp := s.raft.EXPECT()
	exp.Apply(cmds[0].Command, timeout).Return(s.applyFuture)
	exp.State().Return(raft.Follower)
	exp.Leader().Return(raft.ServerAddress("1.1.1.1"))
	exp.GetConfiguration().Return(s.configFuture)

	s.configFuture.EXPECT().Error().Return(raft.ErrNotLeader)

	s.applier.ApplyOperation(cmds, timeout)

	assertError(c, done, raft.ErrNotLeader.Error())
}

func (s *applyOperationSuite) setupMocks(c *gc.C) *gomock.Controller {
	ctrl := gomock.NewController(c)

	s.raft = NewMockRaft(ctrl)
	s.target = NewMockNotifyTarget(ctrl)
	s.applyFuture = NewMockApplyFuture(ctrl)
	s.configFuture = NewMockConfigurationFuture(ctrl)
	s.response = NewMockFSMResponse(ctrl)
	s.metrics = NewMockApplierMetrics(ctrl)

	s.clock = testclock.NewClock(time.Now())
	s.applier = NewApplier(s.raft, s.target, s.metrics, s.clock, fakeLogger{})

	return ctrl
}

func (s *applyOperationSuite) expectSuccessMetrics() {
	s.metrics.EXPECT().Record(gomock.Any(), "success").AnyTimes()
}

func (s *applyOperationSuite) expectFailureMetrics() {
	s.metrics.EXPECT().Record(gomock.Any(), "failure").AnyTimes()
}

func opName(i int) []byte {
	return []byte(fmt.Sprintf("abc-%d", i))
}

func commandsN(n int) ([]queue.OutOperation, chan error) {
	done := make(chan error)
	res := make([]queue.OutOperation, n)
	for i := 0; i < n; i++ {
		res[i] = queue.OutOperation{
			Command: opName(i),
			Done: func(err error) {
				go func() {
					done <- err
				}()
			},
		}
	}
	return res, done
}

func assertNilErrorsN(c *gc.C, done chan error, n int) {
	results := make([]error, 0)

	for {
		select {
		case err := <-done:
			results = append(results, err)
		case <-time.After(testing.LongWait):
			c.Fatal("timed out waiting for done")
		}

		if len(results) == n {
			break
		}
	}
	c.Assert(len(results), gc.Equals, n)
	for _, k := range results {
		c.Assert(k, jc.ErrorIsNil)
	}
}

func assertError(c *gc.C, done chan error, err string) {
	results := make([]error, 0)

	for {
		select {
		case err := <-done:
			results = append(results, err)
		case <-time.After(testing.LongWait):
			c.Fatal("timed out waiting for done")
		}

		if len(results) == 1 {
			break
		}
	}
	c.Assert(len(results), gc.Equals, 1)
	for _, k := range results {
		c.Assert(k, gc.ErrorMatches, err)
	}
}

type fakeLogger struct{}

func (fakeLogger) Criticalf(_ string, _ ...interface{})           {}
func (fakeLogger) Warningf(_ string, _ ...interface{})            {}
func (fakeLogger) Errorf(_ string, _ ...interface{})              {}
func (fakeLogger) Infof(_ string, _ ...interface{})               {}
func (fakeLogger) Debugf(_ string, _ ...interface{})              {}
func (fakeLogger) Tracef(_ string, _ ...interface{})              {}
func (fakeLogger) Logf(_ loggo.Level, _ string, _ ...interface{}) {}
func (fakeLogger) IsTraceEnabled() bool                           { return true }
