// Copyright 2020 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package spaces

import (
	"github.com/juju/errors"
	"github.com/juju/names/v5"
	"github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	"go.uber.org/mock/gomock"
	gc "gopkg.in/check.v1"

	apiservererrors "github.com/juju/juju/apiserver/errors"
	facademocks "github.com/juju/juju/apiserver/facade/mocks"
	"github.com/juju/juju/controller"
	"github.com/juju/juju/environs/context"
	environmocks "github.com/juju/juju/environs/mocks"
	coretesting "github.com/juju/juju/testing"
)

// ReloadSpacesAPISuite is used to test API calls using mocked model operations.
type ReloadSpacesAPISuite struct {
	testing.IsolationSuite
}

var _ = gc.Suite(&ReloadSpacesAPISuite{})

func (s *ReloadSpacesAPISuite) TestReloadSpaces(c *gc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	context := context.NewEmptyCloudCallContext()
	authorizer := func() error { return nil }

	mockNetworkEnviron := environmocks.NewMockNetworkingEnviron(ctrl)

	mockEnvirons := NewMockReloadSpacesEnviron(ctrl)
	mockEnvirons.EXPECT().GetEnviron(
		environConfGetter{
			ReloadSpacesEnviron: mockEnvirons,
			controllerUUID:      coretesting.ControllerTag.Id(),
		}, gomock.Any()).Return(mockNetworkEnviron, nil)

	mockState := NewMockReloadSpacesState(ctrl)
	mockState.EXPECT().ControllerConfig().Return(controller.Config{
		"controller-uuid": coretesting.ControllerTag.Id(),
	}, nil)

	mockEnvironSpaces := NewMockEnvironSpaces(ctrl)
	mockEnvironSpaces.EXPECT().ReloadSpaces(context, mockState, mockNetworkEnviron).Return(nil)

	spacesAPI := NewReloadSpacesAPI(mockState, mockEnvirons, mockEnvironSpaces, context, authorizer)
	err := spacesAPI.ReloadSpaces()
	c.Check(err, jc.ErrorIsNil)
}

func (s *ReloadSpacesAPISuite) TestReloadSpacesWithNoEnviron(c *gc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	context := context.NewEmptyCloudCallContext()
	authorizer := func() error { return nil }

	mockNetworkEnviron := environmocks.NewMockNetworkingEnviron(ctrl)

	mockEnvirons := NewMockReloadSpacesEnviron(ctrl)
	mockEnvirons.EXPECT().GetEnviron(
		environConfGetter{
			ReloadSpacesEnviron: mockEnvirons,
			controllerUUID:      coretesting.ControllerTag.Id(),
		}, gomock.Any()).Return(mockNetworkEnviron, errors.New("boom"))

	mockState := NewMockReloadSpacesState(ctrl)
	mockState.EXPECT().ControllerConfig().Return(controller.Config{
		"controller-uuid": coretesting.ControllerTag.Id(),
	}, nil)

	mockEnvironSpaces := NewMockEnvironSpaces(ctrl)

	spacesAPI := NewReloadSpacesAPI(mockState, mockEnvirons, mockEnvironSpaces, context, authorizer)
	err := spacesAPI.ReloadSpaces()
	c.Check(err, gc.ErrorMatches, "boom")
}

func (s *ReloadSpacesAPISuite) TestReloadSpacesWithReloadSpaceError(c *gc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	context := context.NewEmptyCloudCallContext()
	authorizer := func() error { return nil }

	mockNetworkEnviron := environmocks.NewMockNetworkingEnviron(ctrl)

	mockEnvirons := NewMockReloadSpacesEnviron(ctrl)
	mockEnvirons.EXPECT().GetEnviron(
		environConfGetter{
			ReloadSpacesEnviron: mockEnvirons,
			controllerUUID:      coretesting.ControllerTag.Id(),
		}, gomock.Any()).Return(mockNetworkEnviron, nil)
	mockEnvirons.EXPECT()

	mockState := NewMockReloadSpacesState(ctrl)
	mockState.EXPECT().ControllerConfig().Return(controller.Config{
		"controller-uuid": coretesting.ControllerTag.Id(),
	}, nil)

	mockEnvironSpaces := NewMockEnvironSpaces(ctrl)
	mockEnvironSpaces.EXPECT().ReloadSpaces(context, mockState, mockNetworkEnviron).Return(errors.New("boom"))

	spacesAPI := NewReloadSpacesAPI(mockState, mockEnvirons, mockEnvironSpaces, context, authorizer)
	err := spacesAPI.ReloadSpaces()
	c.Check(err, gc.ErrorMatches, "boom")
}

type ReloadSpacesAuthorizerSuite struct {
	testing.IsolationSuite
}

var _ = gc.Suite(&ReloadSpacesAuthorizerSuite{})

func (s *ReloadSpacesAuthorizerSuite) TestDefaultAuthorizer(c *gc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	tag := names.NewModelTag("123")

	authorizer := facademocks.NewMockAuthorizer(ctrl)
	authorizer.EXPECT().HasPermission(gomock.Any(), names.NewModelTag("123")).Return(nil)

	blockChecker := NewMockBlockChecker(ctrl)
	blockChecker.EXPECT().ChangeAllowed().Return(nil)

	state := NewMockAuthorizerState(ctrl)
	state.EXPECT().ModelTag().Return(tag)

	authorizerFn := DefaultReloadSpacesAuthorizer(authorizer, blockChecker, state)
	err := authorizerFn()
	c.Check(err, jc.ErrorIsNil)
}

func (s *ReloadSpacesAuthorizerSuite) TestDefaultAuthorizerCannotWrite(c *gc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	tag := names.NewModelTag("123")

	authorizer := facademocks.NewMockAuthorizer(ctrl)
	authorizer.EXPECT().HasPermission(gomock.Any(), names.NewModelTag("123")).Return(apiservererrors.ErrPerm)

	blockChecker := NewMockBlockChecker(ctrl)

	state := NewMockAuthorizerState(ctrl)
	state.EXPECT().ModelTag().Return(tag)

	authorizerFn := DefaultReloadSpacesAuthorizer(authorizer, blockChecker, state)
	err := authorizerFn()
	c.Check(err, gc.ErrorMatches, "permission denied")
}

// Note: If HasPermission returns an error, but returns true then they can go
// through to the block checker.
func (s *ReloadSpacesAuthorizerSuite) TestDefaultAuthorizerNotFound(c *gc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	tag := names.NewModelTag("123")

	authorizer := facademocks.NewMockAuthorizer(ctrl)
	authorizer.EXPECT().HasPermission(gomock.Any(), names.NewModelTag("123")).Return(nil)

	blockChecker := NewMockBlockChecker(ctrl)
	blockChecker.EXPECT().ChangeAllowed().Return(nil)

	state := NewMockAuthorizerState(ctrl)
	state.EXPECT().ModelTag().Return(tag)

	authorizerFn := DefaultReloadSpacesAuthorizer(authorizer, blockChecker, state)
	err := authorizerFn()
	c.Check(err, jc.ErrorIsNil)
}
