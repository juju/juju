// Copyright 2022 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package leadership_test

import (
	"context"

	jc "github.com/juju/testing/checkers"
	"go.uber.org/mock/gomock"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/core/life"
	"github.com/juju/juju/internal/charm/hooks"
	loggertesting "github.com/juju/juju/internal/logger/testing"
	"github.com/juju/juju/internal/worker/uniter/hook"
	"github.com/juju/juju/internal/worker/uniter/leadership"
	"github.com/juju/juju/internal/worker/uniter/operation"
	"github.com/juju/juju/internal/worker/uniter/operation/mocks"
	"github.com/juju/juju/internal/worker/uniter/remotestate"
	"github.com/juju/juju/internal/worker/uniter/resolver"
	coretesting "github.com/juju/juju/testing"
)

var _ = gc.Suite(&resolverSuite{})

type resolverSuite struct {
	coretesting.BaseSuite
}

func (s *resolverSuite) TestNextOpNotInstalled(c *gc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	f := mocks.NewMockFactory(ctrl)
	logger := loggertesting.WrapCheckLog(c)

	r := leadership.NewResolver(logger)
	_, err := r.NextOp(context.Background(), resolver.LocalState{}, remotestate.Snapshot{}, f)
	c.Assert(err, gc.Equals, resolver.ErrNoOperation)
}

func (s *resolverSuite) TestNextOpAcceptLeader(c *gc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	f := mocks.NewMockFactory(ctrl)
	op := mocks.NewMockOperation(ctrl)
	logger := loggertesting.WrapCheckLog(c)

	f.EXPECT().NewAcceptLeadership().Return(op, nil)

	r := leadership.NewResolver(logger)
	result, err := r.NextOp(context.Background(), resolver.LocalState{
		State: operation.State{Installed: true, Kind: operation.Continue},
	}, remotestate.Snapshot{
		Leader: true,
	}, f)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(result, gc.Equals, op)
}

func (s *resolverSuite) TestNextOpResignLeader(c *gc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	f := mocks.NewMockFactory(ctrl)
	op := mocks.NewMockOperation(ctrl)
	logger := loggertesting.WrapCheckLog(c)

	f.EXPECT().NewResignLeadership().Return(op, nil)

	r := leadership.NewResolver(logger)
	result, err := r.NextOp(context.Background(), resolver.LocalState{
		State: operation.State{Installed: true, Leader: true, Kind: operation.Continue},
	}, remotestate.Snapshot{}, f)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(result, gc.Equals, op)
}

func (s *resolverSuite) TestNextOpResignLeaderDying(c *gc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	f := mocks.NewMockFactory(ctrl)
	op := mocks.NewMockOperation(ctrl)
	logger := loggertesting.WrapCheckLog(c)

	f.EXPECT().NewResignLeadership().Return(op, nil)

	r := leadership.NewResolver(logger)
	result, err := r.NextOp(context.Background(), resolver.LocalState{
		State: operation.State{Installed: true, Leader: true, Kind: operation.Continue},
	}, remotestate.Snapshot{
		Leader: true, Life: life.Dying,
	}, f)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(result, gc.Equals, op)
}

func (s *resolverSuite) TestNextOpLeaderSettings(c *gc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	f := mocks.NewMockFactory(ctrl)
	op := mocks.NewMockOperation(ctrl)
	logger := loggertesting.WrapCheckLog(c)

	f.EXPECT().NewRunHook(hook.Info{Kind: hooks.LeaderSettingsChanged}).Return(op, nil)

	r := leadership.NewResolver(logger)
	result, err := r.NextOp(context.Background(), resolver.LocalState{
		State:                 operation.State{Installed: true, Kind: operation.Continue},
		LeaderSettingsVersion: 1,
	}, remotestate.Snapshot{LeaderSettingsVersion: 2}, f)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(result, gc.Equals, op)
}

func (s *resolverSuite) TestNextOpNoLeaderSettingsWhenDying(c *gc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	f := mocks.NewMockFactory(ctrl)
	logger := loggertesting.WrapCheckLog(c)

	r := leadership.NewResolver(logger)
	_, err := r.NextOp(context.Background(), resolver.LocalState{
		State:                 operation.State{Installed: true, Kind: operation.Continue},
		LeaderSettingsVersion: 1,
	}, remotestate.Snapshot{Life: life.Dying, LeaderSettingsVersion: 2}, f)
	c.Assert(err, gc.Equals, resolver.ErrNoOperation)
}
