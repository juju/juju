// Copyright 2026 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package commands

import (
	"context"
	stderrors "errors"
	"testing"

	"github.com/juju/tc"
	"github.com/canonical/gomock/gomock"

	"github.com/juju/juju/api"
	modelcmdmocks "github.com/juju/juju/cmd/modelcmd/mocks"
)

type modelUpgraderCompatSuite struct{}

func TestModelUpgraderCompatSuite(t *testing.T) {
	tc.Run(t, &modelUpgraderCompatSuite{})
}

func (s *modelUpgraderCompatSuite) TestModelUpgraderAPIRootUsesModelRootWhenFacadeV1Advertised(c *tc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	modelRoot := modelcmdmocks.NewMockConnection(ctrl)
	modelRoot.EXPECT().BestFacadeVersion("ModelUpgrader").Return(1)

	root, err := modelUpgraderAPIRoot(
		c.Context(),
		func(context.Context) (api.Connection, error) {
			return modelRoot, nil
		},
		func(context.Context) (api.Connection, error) {
			c.Fatalf("controller root should not be opened")
			return nil, nil
		},
	)

	c.Assert(err, tc.ErrorIsNil)
	c.Check(root, tc.Equals, modelRoot)
}

func (s *modelUpgraderCompatSuite) TestModelUpgraderAPIRootFallsBackWhenModelRootAdvertisesV2(c *tc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	modelRoot := modelcmdmocks.NewMockConnection(ctrl)
	modelRoot.EXPECT().BestFacadeVersion("ModelUpgrader").Return(2)
	modelRoot.EXPECT().Close().Return(nil)
	controllerRoot := modelcmdmocks.NewMockConnection(ctrl)

	controllerCalled := false
	root, err := modelUpgraderAPIRoot(
		c.Context(),
		func(context.Context) (api.Connection, error) {
			return modelRoot, nil
		},
		func(context.Context) (api.Connection, error) {
			controllerCalled = true
			return controllerRoot, nil
		},
	)

	c.Assert(err, tc.ErrorIsNil)
	c.Check(root, tc.Equals, controllerRoot)
	c.Check(controllerCalled, tc.IsTrue)
}

func (s *modelUpgraderCompatSuite) TestModelUpgraderAPIRootFallsBackWhenFacadeMissing(c *tc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	modelRoot := modelcmdmocks.NewMockConnection(ctrl)
	modelRoot.EXPECT().BestFacadeVersion("ModelUpgrader").Return(0)
	modelRoot.EXPECT().Close().Return(nil)
	controllerRoot := modelcmdmocks.NewMockConnection(ctrl)

	controllerCalled := false
	root, err := modelUpgraderAPIRoot(
		c.Context(),
		func(context.Context) (api.Connection, error) {
			return modelRoot, nil
		},
		func(context.Context) (api.Connection, error) {
			controllerCalled = true
			return controllerRoot, nil
		},
	)

	c.Assert(err, tc.ErrorIsNil)
	c.Check(root, tc.Equals, controllerRoot)
	c.Check(controllerCalled, tc.IsTrue)
}

func (s *modelUpgraderCompatSuite) TestModelUpgraderAPIRootReturnsCloseError(c *tc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	modelRoot := modelcmdmocks.NewMockConnection(ctrl)
	modelRoot.EXPECT().BestFacadeVersion("ModelUpgrader").Return(0)
	modelRoot.EXPECT().Close().Return(stderrors.New("boom"))

	controllerCalled := false
	root, err := modelUpgraderAPIRoot(
		c.Context(),
		func(context.Context) (api.Connection, error) {
			return modelRoot, nil
		},
		func(context.Context) (api.Connection, error) {
			controllerCalled = true
			return nil, nil
		},
	)

	c.Assert(root, tc.IsNil)
	c.Check(err, tc.ErrorMatches, "boom")
	c.Check(controllerCalled, tc.IsFalse)
}
