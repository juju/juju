// Copyright 2024 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package service

import (
	stdtesting "testing"

	"github.com/juju/tc"
	"go.uber.org/mock/gomock"

	"github.com/juju/juju/internal/errors"
	logtesting "github.com/juju/juju/internal/logger/testing"
	"github.com/juju/juju/internal/testhelpers"
)

type serviceSuite struct {
	testhelpers.IsolationSuite

	service *Service

	state                 *MockState
	storageRegistryGetter *MockModelStorageRegistryGetter
	storageRegistry       *MockProviderRegistry
}

func TestServiceSuite(t *stdtesting.T) {
	tc.Run(t, &serviceSuite{})
}

func (s *serviceSuite) TestGetStorageRegistry(c *tc.C) {
	defer s.setupMocks(c).Finish()

	s.storageRegistryGetter.EXPECT().GetStorageRegistry(gomock.Any()).Return(s.storageRegistry, nil)

	reg, err := s.service.GetStorageRegistry(c.Context())
	c.Assert(err, tc.ErrorIsNil)
	c.Check(reg, tc.Equals, s.storageRegistry)
}

func (s *serviceSuite) TestStorageRegistryError(c *tc.C) {
	defer s.setupMocks(c).Finish()

	s.storageRegistryGetter.EXPECT().GetStorageRegistry(gomock.Any()).Return(nil, errors.Errorf("boom"))

	_, err := s.service.GetStorageRegistry(c.Context())
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
