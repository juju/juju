// Copyright 2021 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package globalclockupdater

import (
	"time"

	"github.com/golang/mock/gomock"
	"github.com/hashicorp/raft"
	"github.com/juju/errors"
	"github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/v3/core/globalclock"
	"github.com/juju/juju/v3/core/raftlease"
)

type updaterSuite struct {
	testing.IsolationSuite

	raftApplier  *MockRaftApplier
	notifyTarget *MockNotifyTarget
	clock        *MockReadOnlyClock
	sleeper      *MockSleeper
	timer        *MockTimer
	logger       *MockLogger
	raftFuture   *MockApplyFuture
	fsmResponse  *MockFSMResponse
}

var _ = gc.Suite(&updaterSuite{})

func (s *updaterSuite) TestAdvance(c *gc.C) {
	defer s.setupMocks(c).Finish()

	now := time.Now()

	s.clock.EXPECT().GlobalTime().Return(now)
	s.expectTimeout()
	s.expectRaftApply(c, now, nil)

	done := make(chan struct{}, 1)

	updater := newUpdater(s.raftApplier, s.notifyTarget, s.clock, s.sleeper, s.timer, s.logger)
	err := updater.Advance(time.Second, done)
	c.Assert(err, jc.ErrorIsNil)
}

func (s *updaterSuite) TestAdvanceErrEnqueueTimeout(c *gc.C) {
	defer s.setupMocks(c).Finish()

	now := time.Now()

	enqueueErr := raft.ErrEnqueueTimeout

	s.clock.EXPECT().GlobalTime().Return(now)
	s.expectTimeout()
	s.expectRaftApply(c, now, enqueueErr)

	done := make(chan struct{}, 1)

	updater := newUpdater(s.raftApplier, s.notifyTarget, s.clock, s.sleeper, s.timer, s.logger)
	err := updater.Advance(time.Second, done)
	c.Assert(err, gc.ErrorMatches, globalclock.ErrTimeout.Error())
}

func (s *updaterSuite) TestAdvanceWithUnknownError(c *gc.C) {
	defer s.setupMocks(c).Finish()

	now := time.Now()

	s.clock.EXPECT().GlobalTime().Return(now)
	s.expectTimeout()
	s.expectRaftApply(c, now, errors.New("boom"))

	done := make(chan struct{}, 1)

	updater := newUpdater(s.raftApplier, s.notifyTarget, s.clock, s.sleeper, s.timer, s.logger)
	err := updater.Advance(time.Second, done)
	c.Assert(err, gc.ErrorMatches, "boom")
}

func (s *updaterSuite) TestAdvanceLeadershipAborted(c *gc.C) {
	defer s.setupMocks(c).Finish()

	now := time.Now()

	s.clock.EXPECT().GlobalTime().Return(now)
	s.expectTimeout()
	s.expectRaftApply(c, now, raft.ErrLeadershipLost)
	s.expectRaftState()

	done := make(chan struct{}, 1)
	close(done)

	updater := newUpdater(s.raftApplier, s.notifyTarget, s.clock, s.sleeper, s.timer, s.logger)
	err := updater.Advance(time.Second, done)
	c.Assert(err, gc.ErrorMatches, "setTime: lease operation aborted")
}

func (s *updaterSuite) TestAdvanceLeadershipTimeout(c *gc.C) {
	defer s.setupMocks(c).Finish()

	now := time.Now()

	s.clock.EXPECT().GlobalTime().Return(now)
	s.expectRaftApply(c, now, raft.ErrLeadershipLost)
	s.expectRaftState()

	ch := make(chan time.Time)
	close(ch)

	s.timer.EXPECT().After(leaderTimeout).Return(ch)

	done := make(chan struct{}, 1)

	updater := newUpdater(s.raftApplier, s.notifyTarget, s.clock, s.sleeper, s.timer, s.logger)
	err := updater.Advance(time.Second, done)
	c.Assert(err, gc.ErrorMatches, `timed out waiting for local Raft state to be "Leader"`)
}

func (s *updaterSuite) TestAdvanceWithLeadershipLostErrorAndRetries(c *gc.C) {
	defer s.setupMocks(c).Finish()

	now := time.Now()

	s.clock.EXPECT().GlobalTime().Return(now).Times(2)
	s.expectTimeout()
	s.expectRaftApply(c, now, raft.ErrLeadershipLost)
	s.expectRaftState()

	// After the sleep, we should retry the raft apply again. Notice that we
	// should update the prevTime.
	s.sleeper.EXPECT().Sleep(time.Second)
	s.expectRaftApply(c, now, nil)

	done := make(chan struct{}, 1)

	updater := newUpdater(s.raftApplier, s.notifyTarget, s.clock, s.sleeper, s.timer, s.logger)
	err := updater.Advance(time.Second, done)
	c.Assert(err, jc.ErrorIsNil)
}

func (s *updaterSuite) setupMocks(c *gc.C) *gomock.Controller {
	ctrl := gomock.NewController(c)

	s.clock = NewMockReadOnlyClock(ctrl)
	s.raftApplier = NewMockRaftApplier(ctrl)
	s.notifyTarget = NewMockNotifyTarget(ctrl)
	s.logger = NewMockLogger(ctrl)
	s.sleeper = NewMockSleeper(ctrl)
	s.timer = NewMockTimer(ctrl)
	s.raftFuture = NewMockApplyFuture(ctrl)
	s.fsmResponse = NewMockFSMResponse(ctrl)

	return ctrl
}

func (s *updaterSuite) expectTimeout() *gomock.Call {
	ch := make(chan time.Time)
	return s.timer.EXPECT().After(leaderTimeout).Return(ch)
}

func (s *updaterSuite) expectRaftApply(c *gc.C, now time.Time, err error) *gomock.Call {
	cmd, cmdErr := (&raftlease.Command{
		Version:   raftlease.CommandVersion,
		Operation: raftlease.OperationSetTime,
		OldTime:   now,
		NewTime:   now.Add(time.Second),
	}).Marshal()
	c.Assert(cmdErr, jc.ErrorIsNil)

	call := inOrder(
		s.raftApplier.EXPECT().Apply(cmd, applyTimeout).Return(s.raftFuture),
		s.raftFuture.EXPECT().Error().Return(err),
	)

	if err != nil {
		call = inOrder(
			call,
			s.logger.EXPECT().Warningf(err.Error()),
		)
	} else {
		call = inOrder(
			call,
			s.raftFuture.EXPECT().Response().Return(s.fsmResponse),
			s.fsmResponse.EXPECT().Error().Return(nil),
			s.fsmResponse.EXPECT().Notify(s.notifyTarget),
		)
	}
	return call
}

func (s *updaterSuite) expectRaftState() {
	s.raftApplier.EXPECT().State().Return(raft.Follower)
	s.logger.EXPECT().Warningf("clock update still pending; local Raft state is %q", raft.Follower)
}

// This is the PR to gomock, that should hopefully land in the future. For now
// we can copy it here.
// See: https://github.com/golang/mock/pull/199
func inOrder(calls ...*gomock.Call) (last *gomock.Call) {
	if len(calls) == 1 {
		return calls[0]
	}

	for i := 1; i < len(calls); i++ {
		calls[i].After(calls[i-1])
		last = calls[i]
	}
	return
}
