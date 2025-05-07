// Copyright 2024 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package service

import (
	"context"

	"github.com/juju/tc"
	"github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	"go.uber.org/mock/gomock"

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

var _ = tc.Suite(&serviceSuite{})

func (s *serviceSuite) TestGetStorageRegistry(c *tc.C) {
	defer s.setupMocks(c).Finish()

	s.storageRegistryGetter.EXPECT().GetStorageRegistry(context.Background()).Return(s.storageRegistry, nil)

	reg, err := s.service.GetStorageRegistry(context.Background())
	c.Assert(err, jc.ErrorIsNil)
	c.Check(reg, tc.Equals, s.storageRegistry)
}

func (s *serviceSuite) TestStorageRegistryError(c *tc.C) {
	defer s.setupMocks(c).Finish()

	s.storageRegistryGetter.EXPECT().GetStorageRegistry(context.Background()).Return(nil, errors.Errorf("boom"))

	_, err := s.service.GetStorageRegistry(context.Background())
	c.Assert(err, tc.ErrorMatches, "getting storage registry: boom")
}

func (s *serviceSuite) setupMocks(c *tc.C) *gomock.Controller {
	ctrl := gomock.NewController(c)

	s.state = NewMockState(ctrl)
	s.storageRegistryGetter = NewMockModelStorageRegistryGetter(ctrl)
	s.storageRegistry = NewMockProviderRegistry(ctrl)

	s.service = NewService(s.state, logtesting.WrapCheckLog(c), s.storageRegistryGetter)

	return ctrl
}
