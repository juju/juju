// Copyright 2024 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package service

import (
	"context"
	"testing"

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
	"github.com/juju/juju/internal/errors"
)

type serviceSuite struct {
	controllerState  *MockControllerState
	modelState       *MockModelState
	instanceProvider *MockInstanceProvider
	resourceProvider *MockResourceProvider
}

func TestServiceSuite(t *testing.T) {
	tc.Run(t, &serviceSuite{})
}

// TestAdoptResources is testing the happy path of adopting a models cloud
// resources.
func (s *serviceSuite) TestAdoptResources(c *tc.C) {
	defer s.setupMocks(c).Finish()

	sourceControllerVersion, err := semversion.Parse("4.1.1")
	c.Assert(err, tc.ErrorIsNil)

	s.modelState.EXPECT().GetControllerUUID(gomock.Any()).Return(
		"deadbeef-1bad-500d-9000-4b1d0d06f00d",
		nil,
	)
	s.resourceProvider.EXPECT().AdoptResources(
		gomock.Any(),
		"deadbeef-1bad-500d-9000-4b1d0d06f00d",
		sourceControllerVersion,
	).Return(nil)

	err = NewService(
		s.controllerState,
		s.modelState,
		"test-model-uuid",
		s.instanceProviderGetter(c),
		s.resourceProviderGetter(c),
	).AdoptResources(c.Context(), sourceControllerVersion)
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

	s.modelState.EXPECT().GetControllerUUID(gomock.Any()).Return(
		"deadbeef-1bad-500d-9000-4b1d0d06f00d",
		nil,
	).AnyTimes()

	err = NewService(
		s.controllerState,
		s.modelState,
		"test-model-uuid",
		s.instanceProviderGetter(c),
		resourceGetter,
	).AdoptResources(c.Context(), sourceControllerVersion)
	c.Check(err, tc.ErrorIsNil)
}

// TestAdoptResourcesProviderNotImplemented is asserting that if the resource
// provider returns a not implemented error while trying to adopt a models
// resources no error is produced from the service and no resources are adopted.
func (s *serviceSuite) TestAdoptResourcesProviderNotImplemented(c *tc.C) {
	defer s.setupMocks(c).Finish()

	sourceControllerVersion, err := semversion.Parse("4.1.1")
	c.Assert(err, tc.ErrorIsNil)

	s.modelState.EXPECT().GetControllerUUID(gomock.Any()).Return(
		"deadbeef-1bad-500d-9000-4b1d0d06f00d",
		nil,
	)
	s.resourceProvider.EXPECT().AdoptResources(
		gomock.Any(),
		"deadbeef-1bad-500d-9000-4b1d0d06f00d",
		sourceControllerVersion,
	).Return(coreerrors.NotImplemented)

	err = NewService(
		s.controllerState,
		s.modelState,
		"test-model-uuid",
		s.instanceProviderGetter(c),
		s.resourceProviderGetter(c),
	).AdoptResources(c.Context(), sourceControllerVersion)
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
	s.modelState.EXPECT().GetAllInstanceIDs(gomock.Any()).
		Return(set.NewStrings("instance0"), nil)

	_, err := NewService(
		s.controllerState,
		s.modelState,
		"test-model-uuid",
		s.instanceProviderGetter(c),
		s.resourceProviderGetter(c),
	).CheckMachines(c.Context())
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
	s.modelState.EXPECT().GetAllInstanceIDs(gomock.Any()).
		Return(set.NewStrings("instance0", "instance1"), nil)

	_, err := NewService(
		s.controllerState,
		s.modelState,
		"test-model-uuid",
		s.instanceProviderGetter(c),
		s.resourceProviderGetter(c),
	).CheckMachines(c.Context())
	c.Check(err, tc.ErrorMatches, "instance IDs.*instance1.*")
}

func (s *serviceSuite) TestActivateImport(c *tc.C) {
	defer s.setupMocks(c).Finish()

	currentVersion := semversion.MustParse("4.0.0")
	desiredVersion := semversion.MustParse("4.0.1")

	mExp := s.modelState.EXPECT()
	cExp := s.controllerState.EXPECT()

	// These are expected to be called in order. The agent version must be
	// updated before the model importing status is deleted. And we want the
	// controller state to have the model importing status deleted last.
	gomock.InOrder(
		cExp.GetControllerTargetVersion(gomock.Any()).Return(desiredVersion, nil),
		mExp.GetModelTargetAgentVersion(gomock.Any()).Return(currentVersion, nil),
		mExp.SetModelTargetAgentVersion(gomock.Any(), currentVersion, desiredVersion).Return(nil),
		mExp.DeleteModelImportingStatus(gomock.Any()).Return(nil),
		cExp.DeleteModelImportingStatus(gomock.Any(), "test-model-uuid").Return(nil),
	)

	err := NewService(
		s.controllerState,
		s.modelState,
		"test-model-uuid",
		s.instanceProviderGetter(c),
		s.resourceProviderGetter(c),
	).ActivateImport(c.Context())
	c.Check(err, tc.ErrorIsNil)
}

func (s *serviceSuite) TestActivateImportSameVersion(c *tc.C) {
	defer s.setupMocks(c).Finish()

	currentVersion := semversion.MustParse("4.0.0")
	desiredVersion := semversion.MustParse("4.0.0")

	mExp := s.modelState.EXPECT()
	cExp := s.controllerState.EXPECT()

	// These are expected to be called in order. The agent version must be
	// updated before the model importing status is deleted. And we want the
	// controller state to have the model importing status deleted last.
	gomock.InOrder(
		cExp.GetControllerTargetVersion(gomock.Any()).Return(desiredVersion, nil),
		mExp.GetModelTargetAgentVersion(gomock.Any()).Return(currentVersion, nil),
		mExp.DeleteModelImportingStatus(gomock.Any()).Return(nil),
		cExp.DeleteModelImportingStatus(gomock.Any(), "test-model-uuid").Return(nil),
	)

	err := NewService(
		s.controllerState,
		s.modelState,
		"test-model-uuid",
		s.instanceProviderGetter(c),
		s.resourceProviderGetter(c),
	).ActivateImport(c.Context())
	c.Check(err, tc.ErrorIsNil)
}

func (s *serviceSuite) TestActivateImportControllerFails(c *tc.C) {
	defer s.setupMocks(c).Finish()

	cExp := s.controllerState.EXPECT()

	cExp.GetControllerTargetVersion(gomock.Any()).Return(semversion.Zero, errors.Errorf("front fell off"))

	err := NewService(
		s.controllerState,
		s.modelState,
		"test-model-uuid",
		s.instanceProviderGetter(c),
		s.resourceProviderGetter(c),
	).ActivateImport(c.Context())
	c.Check(err, tc.ErrorMatches, ".*front fell off")
}

func (s *serviceSuite) TestActivateImportModelFails(c *tc.C) {
	defer s.setupMocks(c).Finish()

	desiredVersion := semversion.MustParse("4.0.1")

	mExp := s.modelState.EXPECT()
	cExp := s.controllerState.EXPECT()

	cExp.GetControllerTargetVersion(gomock.Any()).Return(desiredVersion, nil)
	mExp.GetModelTargetAgentVersion(gomock.Any()).Return(semversion.Zero, errors.Errorf("front fell off"))

	err := NewService(
		s.controllerState,
		s.modelState,
		"test-model-uuid",
		s.instanceProviderGetter(c),
		s.resourceProviderGetter(c),
	).ActivateImport(c.Context())
	c.Check(err, tc.ErrorMatches, ".*front fell off")
}

func (s *serviceSuite) setupMocks(c *tc.C) *gomock.Controller {
	ctrl := gomock.NewController(c)

	s.controllerState = NewMockControllerState(ctrl)
	s.modelState = NewMockModelState(ctrl)

	s.instanceProvider = NewMockInstanceProvider(ctrl)
	s.resourceProvider = NewMockResourceProvider(ctrl)

	c.Cleanup(func() {
		s.controllerState = nil
		s.modelState = nil
		s.instanceProvider = nil
		s.resourceProvider = nil
	})

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
