// Copyright 2021 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package raft

import (
	"fmt"
	"time"

	gomock "github.com/golang/mock/gomock"
	"github.com/hashicorp/raft"
	"github.com/juju/clock/testclock"
	"github.com/juju/errors"
	"github.com/juju/loggo"
	"github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/core/raft/queue"
)

type applyOperationSuite struct {
	testing.IsolationSuite

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

	s.expectSuccessMetrics(c)

	cmds, done := commandsN(1)
	timeout := time.Second

	s.raft.EXPECT().State().Return(raft.Leader)
	s.raft.EXPECT().Apply(cmds[0].Command, timeout).Return(s.applyFuture)
	s.applyFuture.EXPECT().Error().Return(nil)
	s.applyFuture.EXPECT().Response().Return(s.response)
	s.response.EXPECT().Notify(s.target)
	s.response.EXPECT().Error().Return(nil)

	applier := NewApplier(s.raft, s.target, s.metrics, s.clock, fakeLogger{})
	applier.ApplyOperation(cmds, timeout)

	assertNilErrorsN(c, done, 1)
}

func (s *applyOperationSuite) TestApplyLeaseMultipleCommands(c *gc.C) {
	defer s.setupMocks(c).Finish()

	s.expectSuccessMetrics(c)

	cmds, done := commandsN(2)
	timeout := time.Second

	s.raft.EXPECT().State().Return(raft.Leader)
	s.raft.EXPECT().Apply(cmds[0].Command, timeout).Return(s.applyFuture)
	s.raft.EXPECT().Apply(cmds[1].Command, timeout).Return(s.applyFuture)
	s.applyFuture.EXPECT().Error().Return(nil).Times(2)
	s.applyFuture.EXPECT().Response().Return(s.response).Times(2)
	s.response.EXPECT().Notify(s.target).Times(2)
	s.response.EXPECT().Error().Return(nil).Times(2)

	applier := NewApplier(s.raft, s.target, s.metrics, s.clock, fakeLogger{})
	applier.ApplyOperation(cmds, timeout)

	assertNilErrorsN(c, done, 2)
}

func (s *applyOperationSuite) TestApplyLeaseWithProgrammingError(c *gc.C) {
	defer s.setupMocks(c).Finish()

	s.expectFailureMetrics(c)

	cmds, done := commandsN(1)
	timeout := time.Second

	s.raft.EXPECT().State().Return(raft.Leader)
	s.raft.EXPECT().Apply(cmds[0].Command, timeout).Return(s.applyFuture)
	s.applyFuture.EXPECT().Error().Return(nil)
	s.applyFuture.EXPECT().Response().Return(struct{}{})

	applier := NewApplier(s.raft, s.target, s.metrics, s.clock, fakeLogger{})
	applier.ApplyOperation(cmds, timeout)

	assertError(c, done, `invalid FSM response`)
}

func (s *applyOperationSuite) TestApplyLeaseWithError(c *gc.C) {
	defer s.setupMocks(c).Finish()

	s.expectFailureMetrics(c)

	cmds, done := commandsN(1)
	timeout := time.Second

	s.raft.EXPECT().State().Return(raft.Leader)
	s.raft.EXPECT().Apply(cmds[0].Command, timeout).Return(s.applyFuture)
	s.applyFuture.EXPECT().Error().Return(errors.New("boom"))

	applier := NewApplier(s.raft, s.target, s.metrics, s.clock, fakeLogger{})
	applier.ApplyOperation(cmds, timeout)

	assertError(c, done, `boom`)
}

func (s *applyOperationSuite) TestApplyLeaseNotLeaderWithNoLeaderAddress(c *gc.C) {
	defer s.setupMocks(c).Finish()

	s.expectLeaderErrorMetrics(c)

	cmds, done := commandsN(1)
	timeout := time.Second

	s.raft.EXPECT().State().Return(raft.Follower)
	s.raft.EXPECT().Leader().Return(raft.ServerAddress(""))

	applier := NewApplier(s.raft, s.target, s.metrics, s.clock, fakeLogger{})
	applier.ApplyOperation(cmds, timeout)

	assertError(c, done, `not currently the leader.*`)
}

func (s *applyOperationSuite) TestApplyLeaseNotLeader(c *gc.C) {
	defer s.setupMocks(c).Finish()

	s.expectLeaderErrorMetrics(c)

	cmds, done := commandsN(1)
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

	applier := NewApplier(s.raft, s.target, s.metrics, s.clock, fakeLogger{})
	applier.ApplyOperation(cmds, timeout)

	assertError(c, done, `not currently the leader, try "1"`)
}

func (s *applyOperationSuite) TestApplyLeaseNotLeaderWithNoLeaderID(c *gc.C) {
	defer s.setupMocks(c).Finish()

	s.expectLeaderErrorMetrics(c)

	cmds, done := commandsN(1)
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

	applier := NewApplier(s.raft, s.target, s.metrics, s.clock, fakeLogger{})
	applier.ApplyOperation(cmds, timeout)

	assertError(c, done, `not currently the leader, try ""`)
}

func (s *applyOperationSuite) TestApplyLeaseNotLeaderWithConfigurationError(c *gc.C) {
	defer s.setupMocks(c).Finish()

	s.expectLeaderErrorMetrics(c)

	cmds, done := commandsN(1)
	timeout := time.Second

	s.raft.EXPECT().State().Return(raft.Follower)
	s.raft.EXPECT().Leader().Return(raft.ServerAddress("1.1.1.1"))
	s.raft.EXPECT().GetConfiguration().Return(s.configFuture)
	s.configFuture.EXPECT().Error().Return(errors.New("boom"))

	applier := NewApplier(s.raft, s.target, s.metrics, s.clock, fakeLogger{})
	applier.ApplyOperation(cmds, timeout)

	assertError(c, done, `boom`)
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

	return ctrl
}

func (s *applyOperationSuite) expectSuccessMetrics(c *gc.C) {
	s.metrics.EXPECT().Record(gomock.Any(), "success").AnyTimes()
}

func (s *applyOperationSuite) expectFailureMetrics(c *gc.C) {
	s.metrics.EXPECT().Record(gomock.Any(), "failure").AnyTimes()
}

func (s *applyOperationSuite) expectLeaderErrorMetrics(c *gc.C) {
	s.metrics.EXPECT().RecordLeaderError(gomock.Any()).AnyTimes()
}

func opName(i int) []byte {
	return []byte(fmt.Sprintf("abc-%d", i))
}

func commandsN(n int) ([]queue.Operation, chan error) {
	done := make(chan error)
	res := make([]queue.Operation, n)
	for i := 0; i < n; i++ {
		res[i] = queue.Operation{
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

func (fakeLogger) Criticalf(message string, args ...interface{})               {}
func (fakeLogger) Warningf(message string, args ...interface{})                {}
func (fakeLogger) Errorf(message string, args ...interface{})                  {}
func (fakeLogger) Infof(message string, args ...interface{})                   {}
func (fakeLogger) Tracef(message string, args ...interface{})                  {}
func (fakeLogger) Logf(level loggo.Level, message string, args ...interface{}) {}
func (fakeLogger) IsTraceEnabled() bool                                        { return true }
