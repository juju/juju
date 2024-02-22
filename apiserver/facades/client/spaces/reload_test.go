// Copyright 2020 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package spaces

import (
	"context"

	"github.com/juju/errors"
	"github.com/juju/names/v5"
	"github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	"go.uber.org/mock/gomock"
	gc "gopkg.in/check.v1"

	apiservererrors "github.com/juju/juju/apiserver/errors"
	facademocks "github.com/juju/juju/apiserver/facade/mocks"
	apiservertesting "github.com/juju/juju/apiserver/testing"
	"github.com/juju/juju/environs/config"
	"github.com/juju/juju/environs/envcontext"
	environmocks "github.com/juju/juju/environs/mocks"
)

// ReloadSpacesAPISuite is used to test API calls using mocked model operations.
type ReloadSpacesAPISuite struct {
	testing.IsolationSuite
}

var _ = gc.Suite(&ReloadSpacesAPISuite{})

func (s *ReloadSpacesAPISuite) TestReloadSpaces(c *gc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	authorizer := func(context.Context) error { return nil }

	mockNetworkEnviron := environmocks.NewMockNetworkingEnviron(ctrl)

	mockEnvirons := NewMockReloadSpacesEnviron(ctrl)
	mockEnvirons.EXPECT().GetEnviron(gomock.Any(), mockEnvirons, gomock.Any()).Return(mockNetworkEnviron, nil)

	mockState := NewMockReloadSpacesState(ctrl)
	mockSpaceService := NewMockSpaceService(ctrl)

	mockModel := NewMockModel(ctrl)
	mockState.EXPECT().Model().Return(mockModel, nil)
	mockModel.EXPECT().Config().Return(&config.Config{}, nil)

	mockEnvironSpaces := NewMockEnvironSpaces(ctrl)
	mockEnvironSpaces.EXPECT().ReloadSpaces(gomock.Any(), mockState, mockSpaceService, mockNetworkEnviron, nil).Return(nil)

	spacesAPI := NewReloadSpacesAPI(mockState, mockEnvirons, mockEnvironSpaces, apiservertesting.NoopModelCredentialInvalidatorGetter, authorizer, mockSpaceService)
	err := spacesAPI.ReloadSpaces(envcontext.WithoutCredentialInvalidator(context.Background()))
	c.Check(err, jc.ErrorIsNil)
}

func (s *ReloadSpacesAPISuite) TestReloadSpacesWithNoEnviron(c *gc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	authorizer := func(context.Context) error { return nil }

	mockNetworkEnviron := environmocks.NewMockNetworkingEnviron(ctrl)

	mockEnvirons := NewMockReloadSpacesEnviron(ctrl)
	mockEnvirons.EXPECT().GetEnviron(gomock.Any(), mockEnvirons, gomock.Any()).Return(mockNetworkEnviron, errors.New("boom"))

	mockState := NewMockReloadSpacesState(ctrl)
	mockSpaceService := NewMockSpaceService(ctrl)
	mockEnvironSpaces := NewMockEnvironSpaces(ctrl)

	spacesAPI := NewReloadSpacesAPI(mockState, mockEnvirons, mockEnvironSpaces, apiservertesting.NoopModelCredentialInvalidatorGetter, authorizer, mockSpaceService)
	err := spacesAPI.ReloadSpaces(context.Background())
	c.Check(err, gc.ErrorMatches, "boom")
}

func (s *ReloadSpacesAPISuite) TestReloadSpacesWithReloadSpaceError(c *gc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	authorizer := func(context.Context) error { return nil }

	mockNetworkEnviron := environmocks.NewMockNetworkingEnviron(ctrl)

	mockEnvirons := NewMockReloadSpacesEnviron(ctrl)
	mockEnvirons.EXPECT().GetEnviron(gomock.Any(), mockEnvirons, gomock.Any()).Return(mockNetworkEnviron, nil)

	mockState := NewMockReloadSpacesState(ctrl)
	mockSpaceService := NewMockSpaceService(ctrl)
	mockEnvironSpaces := NewMockEnvironSpaces(ctrl)
	mockEnvironSpaces.EXPECT().ReloadSpaces(gomock.Any(), mockState, mockSpaceService, mockNetworkEnviron, nil).Return(errors.New("boom"))

	mockModel := NewMockModel(ctrl)
	mockState.EXPECT().Model().Return(mockModel, nil)
	mockModel.EXPECT().Config().Return(&config.Config{}, nil)

	spacesAPI := NewReloadSpacesAPI(mockState, mockEnvirons, mockEnvironSpaces, apiservertesting.NoopModelCredentialInvalidatorGetter, authorizer, mockSpaceService)
	err := spacesAPI.ReloadSpaces(context.Background())
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
	blockChecker.EXPECT().ChangeAllowed(gomock.Any()).Return(nil)

	state := NewMockAuthorizerState(ctrl)
	state.EXPECT().ModelTag().Return(tag)

	authorizerFn := DefaultReloadSpacesAuthorizer(authorizer, blockChecker, state)
	err := authorizerFn(context.Background())
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
	err := authorizerFn(context.Background())
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
	blockChecker.EXPECT().ChangeAllowed(gomock.Any()).Return(nil)

	state := NewMockAuthorizerState(ctrl)
	state.EXPECT().ModelTag().Return(tag)

	authorizerFn := DefaultReloadSpacesAuthorizer(authorizer, blockChecker, state)
	err := authorizerFn(context.Background())
	c.Check(err, jc.ErrorIsNil)
}
