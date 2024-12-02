// Copyright 2024 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package service

import (
	"context"

	"github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	"go.uber.org/mock/gomock"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/internal/errors"
	logtesting "github.com/juju/juju/internal/logger/testing"
)

type serviceSuite struct {
	testing.IsolationSuite

	service *Service

	state                 *MockState
	storageRegistryGetter *MockModelStorageRegistryGetter
	storageRegistry       *MockProviderRegistry
}

var _ = gc.Suite(&serviceSuite{})

func (s *serviceSuite) TestGetStorageRegistry(c *gc.C) {
	defer s.setupMocks(c).Finish()

	s.storageRegistryGetter.EXPECT().GetStorageRegistry(context.Background()).Return(s.storageRegistry, nil)

	reg, err := s.service.GetStorageRegistry(context.Background())
	c.Assert(err, jc.ErrorIsNil)
	c.Check(reg, gc.Equals, s.storageRegistry)
}

func (s *serviceSuite) TestStorageRegistryError(c *gc.C) {
	defer s.setupMocks(c).Finish()

	s.storageRegistryGetter.EXPECT().GetStorageRegistry(context.Background()).Return(nil, errors.Errorf("boom"))

	_, err := s.service.GetStorageRegistry(context.Background())
	c.Assert(err, gc.ErrorMatches, "getting storage registry: boom")
}

func (s *serviceSuite) setupMocks(c *gc.C) *gomock.Controller {
	ctrl := gomock.NewController(c)

	s.state = NewMockState(ctrl)
	s.storageRegistryGetter = NewMockModelStorageRegistryGetter(ctrl)
	s.storageRegistry = NewMockProviderRegistry(ctrl)

	s.service = NewService(s.state, logtesting.WrapCheckLog(c), s.storageRegistryGetter)

	return ctrl
}
