// Copyright 2018 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package service

import (
	"testing"

	"github.com/juju/tc"
	"go.uber.org/mock/gomock"

	"github.com/juju/juju/core/instance"
	"github.com/juju/juju/core/machine"
	"github.com/juju/juju/environs/instances"
	"github.com/juju/juju/internal/errors"
	"github.com/juju/juju/internal/testhelpers"
	jujutesting "github.com/juju/juju/internal/testing"
)

func TestCheckMachinesSuite(t *testing.T) {
	tc.Run(t, &CheckMachinesSuite{})
}

type CheckMachinesSuite struct {
	testhelpers.IsolationSuite

	context CredentialValidationContext

	machineService  *MockMachineService
	providerService *MockCloudProvider
	instance        *MockInstance
}

func (s *CheckMachinesSuite) SetUpTest(c *tc.C) {
	s.IsolationSuite.SetUpTest(c)

	s.context = CredentialValidationContext{
		ControllerUUID: jujutesting.ControllerTag.Id(),
		ModelType:      "iaas",
	}
}

func (s *CheckMachinesSuite) setupMocks(c *tc.C) *gomock.Controller {
	ctrl := gomock.NewController(c)
	s.machineService = NewMockMachineService(ctrl)
	s.providerService = NewMockCloudProvider(ctrl)
	s.instance = NewMockInstance(ctrl)
	return ctrl
}

func (s *CheckMachinesSuite) TestCheckMachinesSuccess(c *tc.C) {
	defer s.setupMocks(c).Finish()

	s.machineService.EXPECT().GetAllProvisionedMachineInstanceID(gomock.Any()).Return(
		map[machine.Name]instance.Id{}, nil)
	s.providerService.EXPECT().AllInstances(gomock.Any()).Return(
		[]instances.Instance{}, nil)

	results, err := checkMachineInstances(c.Context(), s.machineService, s.providerService, false)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(results, tc.HasLen, 0)
}

func (s *CheckMachinesSuite) TestCheckMachinesInstancesMissing(c *tc.C) {
	defer s.setupMocks(c).Finish()

	s.machineService.EXPECT().GetAllProvisionedMachineInstanceID(gomock.Any()).Return(
		map[machine.Name]instance.Id{
			"2": instance.Id("birds"),
		}, nil)
	s.providerService.EXPECT().AllInstances(gomock.Any()).Return(
		[]instances.Instance{}, nil)

	results, err := checkMachineInstances(c.Context(), s.machineService, s.providerService, false)
	c.Assert(err, tc.ErrorIsNil)

	c.Assert(results, tc.HasLen, 1)
	c.Check(results[0], tc.ErrorMatches, `couldn't find instance "birds" for machine "2"`)
}

func (s *CheckMachinesSuite) TestCheckMachinesProviderInstancesMissing(c *tc.C) {
	defer s.setupMocks(c).Finish()

	s.machineService.EXPECT().GetAllProvisionedMachineInstanceID(gomock.Any()).Return(
		map[machine.Name]instance.Id{
			"2": instance.Id("birds"),
		}, nil)
	s.providerService.EXPECT().AllInstances(gomock.Any()).Return(
		[]instances.Instance{
			s.instance,
			s.instance,
		}, nil)

	gomock.InOrder(
		s.instance.EXPECT().Id().Return("birds"),
		s.instance.EXPECT().Id().Return("wind-up"),
	)

	results, err := checkMachineInstances(c.Context(), s.machineService, s.providerService, true)
	c.Assert(err, tc.ErrorIsNil)

	c.Assert(results, tc.HasLen, 1)
	c.Check(results[0], tc.ErrorMatches, `no machine with instance "wind-up"`)
}

func (s *CheckMachinesSuite) TestCheckMachinesExtraInstances(c *tc.C) {
	defer s.setupMocks(c).Finish()

	s.machineService.EXPECT().GetAllProvisionedMachineInstanceID(gomock.Any()).Return(
		map[machine.Name]instance.Id{}, nil)
	s.providerService.EXPECT().AllInstances(gomock.Any()).Return(
		[]instances.Instance{
			s.instance,
		}, nil)

	s.instance.EXPECT().Id().Return("wind-up")

	results, err := checkMachineInstances(c.Context(), s.machineService, s.providerService, false)
	c.Assert(err, tc.ErrorIsNil)
	c.Check(results, tc.IsNil)
}

func (s *CheckMachinesSuite) TestCheckMachinesErrorGettingMachineInstances(c *tc.C) {
	defer s.setupMocks(c).Finish()

	s.machineService.EXPECT().GetAllProvisionedMachineInstanceID(gomock.Any()).Return(
		map[machine.Name]instance.Id{}, errors.Errorf("boom"))

	_, err := checkMachineInstances(c.Context(), s.machineService, s.providerService, false)
	c.Assert(err, tc.ErrorMatches, ".*boom")
}

func (s *CheckMachinesSuite) TestCheckMachinesErrorGettingProviderInstances(c *tc.C) {
	defer s.setupMocks(c).Finish()

	s.machineService.EXPECT().GetAllProvisionedMachineInstanceID(gomock.Any()).Return(
		map[machine.Name]instance.Id{}, nil)
	s.providerService.EXPECT().AllInstances(gomock.Any()).Return(
		[]instances.Instance{}, errors.Errorf("boom"))

	_, err := checkMachineInstances(c.Context(), s.machineService, s.providerService, false)
	c.Assert(err, tc.ErrorMatches, ".*boom")
}
