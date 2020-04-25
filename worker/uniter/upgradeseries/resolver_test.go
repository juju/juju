// Copyright 2018 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package upgradeseries_test

import (
	"github.com/golang/mock/gomock"
	"github.com/juju/charm/v7/hooks"
	"github.com/juju/testing"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/core/model"
	"github.com/juju/juju/worker/uniter/hook"
	"github.com/juju/juju/worker/uniter/operation"
	"github.com/juju/juju/worker/uniter/operation/mocks"
	"github.com/juju/juju/worker/uniter/remotestate"
	"github.com/juju/juju/worker/uniter/resolver"
	"github.com/juju/juju/worker/uniter/upgradeseries"
)

type ResolverSuite struct {
	testing.IsolationSuite
}

var _ = gc.Suite(&ResolverSuite{})

func (ResolverSuite) TestNextOpWithRemoveStateCompleted(c *gc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	mockFactory := mocks.NewMockFactory(ctrl)

	res := upgradeseries.NewResolver()
	_, err := res.NextOp(resolver.LocalState{}, remotestate.Snapshot{
		UpgradeSeriesStatus: model.UpgradeSeriesPrepareCompleted,
	}, mockFactory)
	c.Assert(err, gc.Equals, resolver.ErrDoNotProceed)
}

func (ResolverSuite) TestNextOpWithPreSeriesUpgrade(c *gc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	mockOp := mocks.NewMockOperation(ctrl)

	mockFactory := mocks.NewMockFactory(ctrl)
	mockFactory.EXPECT().NewRunHook(hook.Info{Kind: hooks.PreSeriesUpgrade}).Return(mockOp, nil)

	res := upgradeseries.NewResolver()
	op, err := res.NextOp(resolver.LocalState{
		State: operation.State{
			Kind: operation.Continue,
		},
		UpgradeSeriesStatus: model.UpgradeSeriesNotStarted,
	}, remotestate.Snapshot{
		UpgradeSeriesStatus: model.UpgradeSeriesPrepareStarted,
	}, mockFactory)
	c.Assert(err, gc.IsNil)
	c.Assert(op, gc.NotNil)
}

func (ResolverSuite) TestNextOpWithPostSeriesUpgrade(c *gc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	mockOp := mocks.NewMockOperation(ctrl)

	mockFactory := mocks.NewMockFactory(ctrl)
	mockFactory.EXPECT().NewRunHook(hook.Info{Kind: hooks.PostSeriesUpgrade}).Return(mockOp, nil)

	res := upgradeseries.NewResolver()
	op, err := res.NextOp(resolver.LocalState{
		State: operation.State{
			Kind: operation.Continue,
		},
		UpgradeSeriesStatus: model.UpgradeSeriesNotStarted,
	}, remotestate.Snapshot{
		UpgradeSeriesStatus: model.UpgradeSeriesCompleteStarted,
	}, mockFactory)
	c.Assert(err, gc.IsNil)
	c.Assert(op, gc.NotNil)
}

func (ResolverSuite) TestNextOpWithFinishUpgradeSeries(c *gc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	mockOp := mocks.NewMockOperation(ctrl)

	mockFactory := mocks.NewMockFactory(ctrl)
	mockFactory.EXPECT().NewNoOpFinishUpgradeSeries().Return(mockOp, nil)

	res := upgradeseries.NewResolver()
	op, err := res.NextOp(resolver.LocalState{
		State: operation.State{
			Kind: operation.Continue,
		},
		UpgradeSeriesStatus: model.UpgradeSeriesCompleted,
	}, remotestate.Snapshot{
		UpgradeSeriesStatus: model.UpgradeSeriesNotStarted,
	}, mockFactory)
	c.Assert(err, gc.IsNil)
	c.Assert(op, gc.NotNil)
}

func (ResolverSuite) TestNextOpWithNoState(c *gc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	mockFactory := mocks.NewMockFactory(ctrl)

	res := upgradeseries.NewResolver()
	_, err := res.NextOp(resolver.LocalState{}, remotestate.Snapshot{}, mockFactory)
	c.Assert(err, gc.Equals, resolver.ErrNoOperation)
}
