// Copyright 2024 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package service

import (
	"context"

	jc "github.com/juju/testing/checkers"
	"github.com/juju/version/v2"
	"go.uber.org/mock/gomock"
	gc "gopkg.in/check.v1"

	coreerrors "github.com/juju/juju/core/errors"
	"github.com/juju/juju/core/providertracker"
)

type serviceSuite struct {
	instanceProvider *MockInstanceProvider
	resourceProvider *MockResourceProvider
	state            *MockState
}

var _ = gc.Suite(&serviceSuite{})

func (s *serviceSuite) setupMocks(c *gc.C) *gomock.Controller {
	ctrl := gomock.NewController(c)
	s.instanceProvider = NewMockInstanceProvider(ctrl)
	s.resourceProvider = NewMockResourceProvider(ctrl)
	s.state = NewMockState(ctrl)
	return ctrl
}

func (s *serviceSuite) instanceProviderGetter(_ *gc.C) providertracker.ProviderGetter[InstanceProvider] {
	return func(_ context.Context) (InstanceProvider, error) {
		return s.instanceProvider, nil
	}
}

func (s *serviceSuite) resourceProviderGetter(_ *gc.C) providertracker.ProviderGetter[ResourceProvider] {
	return func(_ context.Context) (ResourceProvider, error) {
		return s.resourceProvider, nil
	}
}

// TestAdoptResources is testing the happy path of adopting a models cloud
// resources.
func (s *serviceSuite) TestAdoptResources(c *gc.C) {
	defer s.setupMocks(c).Finish()

	sourceControllerVersion, err := version.Parse("4.1.1")
	c.Assert(err, jc.ErrorIsNil)

	s.state.EXPECT().GetControllerUUID(gomock.Any()).Return(
		"deadbeef-1bad-500d-9000-4b1d0d06f00d",
		nil,
	)
	s.resourceProvider.EXPECT().AdoptResources(
		gomock.Any(),
		"deadbeef-1bad-500d-9000-4b1d0d06f00d",
		sourceControllerVersion,
	).Return(nil)

	err = NewService(
		s.instanceProviderGetter(c),
		s.resourceProviderGetter(c),
		s.state,
	).AdoptResources(context.Background(), sourceControllerVersion)
	c.Check(err, jc.ErrorIsNil)
}

// TestAdoptResourcesProviderNotSupported is asserting that if the provider does
// not support the Resources interface we don't attempt to migrate any cloud
// resources and no error is produced.
func (s *serviceSuite) TestAdoptResourcesProviderNotSupported(c *gc.C) {
	defer s.setupMocks(c).Finish()

	resourceGetter := func(_ context.Context) (ResourceProvider, error) {
		return nil, coreerrors.NotSupported
	}

	sourceControllerVersion, err := version.Parse("4.1.1")
	c.Assert(err, jc.ErrorIsNil)

	s.state.EXPECT().GetControllerUUID(gomock.Any()).Return(
		"deadbeef-1bad-500d-9000-4b1d0d06f00d",
		nil,
	).AnyTimes()

	err = NewService(
		s.instanceProviderGetter(c),
		resourceGetter,
		s.state,
	).AdoptResources(context.Background(), sourceControllerVersion)
	c.Check(err, jc.ErrorIsNil)
}

// TestAdoptResourcesProviderNotImplemented is asserting that if the resource
// provider returns a not implemented error while trying to adopt a models
// resources no error is produced from the service and no resources are adopted.
func (s *serviceSuite) TestAdoptResourcesProviderNotImplemented(c *gc.C) {
	defer s.setupMocks(c).Finish()

	sourceControllerVersion, err := version.Parse("4.1.1")
	c.Assert(err, jc.ErrorIsNil)

	s.state.EXPECT().GetControllerUUID(gomock.Any()).Return(
		"deadbeef-1bad-500d-9000-4b1d0d06f00d",
		nil,
	)
	s.resourceProvider.EXPECT().AdoptResources(
		gomock.Any(),
		"deadbeef-1bad-500d-9000-4b1d0d06f00d",
		sourceControllerVersion,
	).Return(coreerrors.NotImplemented)

	err = NewService(
		s.instanceProviderGetter(c),
		s.resourceProviderGetter(c),
		s.state,
	).AdoptResources(context.Background(), sourceControllerVersion)
	c.Check(err, jc.ErrorIsNil)
}

// TestMachinesFromProviderDiscrepancy is testing the return value from
// [Service.CheckMachines] and that it reports discrepancies from the cloud.
// TODO (tlm): This test is not fully implemented and will be done when instance
// data is moved over to DQlite.
func (s *serviceSuite) TestMachinesFromProviderDiscrepancy(c *gc.C) {
	defer s.setupMocks(c).Finish()

	s.instanceProvider.EXPECT().AllInstances(gomock.Any()).Return(nil, nil)

	_, err := NewService(
		s.instanceProviderGetter(c),
		s.resourceProviderGetter(c),
		s.state,
	).CheckMachines(context.Background())
	c.Check(err, jc.ErrorIsNil)
}
