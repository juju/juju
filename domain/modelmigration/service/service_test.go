// Copyright 2024 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package service

import (
	"context"

	"github.com/juju/collections/set"
	"github.com/juju/tc"
	"go.uber.org/mock/gomock"

	coreerrors "github.com/juju/juju/core/errors"
	"github.com/juju/juju/core/instance"
	"github.com/juju/juju/core/network"
	"github.com/juju/juju/core/providertracker"
	"github.com/juju/juju/core/semversion"
	"github.com/juju/juju/core/status"
	"github.com/juju/juju/environs/instances"
)

type serviceSuite struct {
	instanceProvider *MockInstanceProvider
	resourceProvider *MockResourceProvider
	state            *MockState
}

var _ = tc.Suite(&serviceSuite{})

func (s *serviceSuite) setupMocks(c *tc.C) *gomock.Controller {
	ctrl := gomock.NewController(c)
	s.instanceProvider = NewMockInstanceProvider(ctrl)
	s.resourceProvider = NewMockResourceProvider(ctrl)
	s.state = NewMockState(ctrl)
	return ctrl
}

func (s *serviceSuite) instanceProviderGetter(_ *tc.C) providertracker.ProviderGetter[InstanceProvider] {
	return func(_ context.Context) (InstanceProvider, error) {
		return s.instanceProvider, nil
	}
}

func (s *serviceSuite) resourceProviderGetter(_ *tc.C) providertracker.ProviderGetter[ResourceProvider] {
	return func(_ context.Context) (ResourceProvider, error) {
		return s.resourceProvider, nil
	}
}

// TestAdoptResources is testing the happy path of adopting a models cloud
// resources.
func (s *serviceSuite) TestAdoptResources(c *tc.C) {
	defer s.setupMocks(c).Finish()

	sourceControllerVersion, err := semversion.Parse("4.1.1")
	c.Assert(err, tc.ErrorIsNil)

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
	c.Check(err, tc.ErrorIsNil)
}

// TestAdoptResourcesProviderNotSupported is asserting that if the provider does
// not support the Resources interface we don't attempt to migrate any cloud
// resources and no error is produced.
func (s *serviceSuite) TestAdoptResourcesProviderNotSupported(c *tc.C) {
	defer s.setupMocks(c).Finish()

	resourceGetter := func(_ context.Context) (ResourceProvider, error) {
		return nil, coreerrors.NotSupported
	}

	sourceControllerVersion, err := semversion.Parse("4.1.1")
	c.Assert(err, tc.ErrorIsNil)

	s.state.EXPECT().GetControllerUUID(gomock.Any()).Return(
		"deadbeef-1bad-500d-9000-4b1d0d06f00d",
		nil,
	).AnyTimes()

	err = NewService(
		s.instanceProviderGetter(c),
		resourceGetter,
		s.state,
	).AdoptResources(context.Background(), sourceControllerVersion)
	c.Check(err, tc.ErrorIsNil)
}

// TestAdoptResourcesProviderNotImplemented is asserting that if the resource
// provider returns a not implemented error while trying to adopt a models
// resources no error is produced from the service and no resources are adopted.
func (s *serviceSuite) TestAdoptResourcesProviderNotImplemented(c *tc.C) {
	defer s.setupMocks(c).Finish()

	sourceControllerVersion, err := semversion.Parse("4.1.1")
	c.Assert(err, tc.ErrorIsNil)

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
	c.Check(err, tc.ErrorIsNil)
}

// TestMachinesFromProviderDiscrepancy is testing the return value from
// [Service.CheckMachines] and that it reports discrepancies from the cloud.
func (s *serviceSuite) TestMachinesFromProviderNotInModel(c *tc.C) {
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
	c.Check(err, tc.ErrorMatches, "provider instance IDs.*instance1.*")
}

// TestMachineInstanceIDsNotInProvider is testing the return value from
// [Service.CheckMachines] and that it reports discrepancies from the model
// on the DB.
func (s *serviceSuite) TestMachineInstanceIDsNotInProvider(c *tc.C) {
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
	c.Check(err, tc.ErrorMatches, "instance IDs.*instance1.*")
}

type instanceStub struct {
	instances.Instance
	id string
}

func (i *instanceStub) Id() instance.Id {
	return instance.Id(i.id)
}

func (i *instanceStub) Status(context.Context) instance.Status {
	return instance.Status{
		Status:  status.Maintenance,
		Message: "some message",
	}
}

func (i *instanceStub) Addresses(context.Context) (network.ProviderAddresses, error) {
	return network.ProviderAddresses{}, nil
}
