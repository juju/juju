// Copyright 2024 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package service

import (
	"context"

	"github.com/juju/collections/set"
	jc "github.com/juju/testing/checkers"
	"go.uber.org/mock/gomock"
	gc "gopkg.in/check.v1"

	coreerrors "github.com/juju/juju/core/errors"
	"github.com/juju/juju/core/instance"
	"github.com/juju/juju/core/network"
	"github.com/juju/juju/core/providertracker"
	"github.com/juju/juju/core/semversion"
	"github.com/juju/juju/core/status"
	"github.com/juju/juju/environs/envcontext"
	"github.com/juju/juju/environs/instances"
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

	sourceControllerVersion, err := semversion.Parse("4.1.1")
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

	sourceControllerVersion, err := semversion.Parse("4.1.1")
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

	sourceControllerVersion, err := semversion.Parse("4.1.1")
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
func (s *serviceSuite) TestMachinesFromProviderNotInModel(c *gc.C) {
	defer s.setupMocks(c).Finish()

	s.instanceProvider.EXPECT().AllInstances(gomock.Any()).
		Return([]instances.Instance{
			&instanceStub{
				id: "instance0",
			},
			&instanceStub{
				id: "instance1",
			},
		},
			nil)
	s.state.EXPECT().GetAllInstanceIDs(context.Background()).
		Return(set.NewStrings("instance0"), nil)

	_, err := NewService(
		s.instanceProviderGetter(c),
		s.resourceProviderGetter(c),
		s.state,
	).CheckMachines(context.Background())
	c.Check(err, gc.ErrorMatches, "provider instance IDs.*instance1.*")
}

// TestMachineInstanceIDsNotInProvider is testing the return value from
// [Service.CheckMachines] and that it reports discrepancies from the model
// on the DB.
func (s *serviceSuite) TestMachineInstanceIDsNotInProvider(c *gc.C) {
	defer s.setupMocks(c).Finish()

	s.instanceProvider.EXPECT().AllInstances(gomock.Any()).
		Return([]instances.Instance{
			&instanceStub{
				id: "instance0",
			},
		},
			nil)
	s.state.EXPECT().GetAllInstanceIDs(context.Background()).
		Return(set.NewStrings("instance0", "instance1"), nil)

	_, err := NewService(
		s.instanceProviderGetter(c),
		s.resourceProviderGetter(c),
		s.state,
	).CheckMachines(context.Background())
	c.Check(err, gc.ErrorMatches, "instance IDs.*instance1.*")
}

type instanceStub struct {
	instances.Instance
	id string
}

func (i *instanceStub) Id() instance.Id {
	return instance.Id(i.id)
}

func (i *instanceStub) Status(envcontext.ProviderCallContext) instance.Status {
	return instance.Status{
		Status:  status.Maintenance,
		Message: "some message",
	}
}

func (i *instanceStub) Addresses(envcontext.ProviderCallContext) (network.ProviderAddresses, error) {
	return network.ProviderAddresses{}, nil
}
