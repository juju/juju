// Copyright 2024 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package service

import (
	"testing"

	"github.com/juju/tc"
	"go.uber.org/mock/gomock"

	coreagentbinary "github.com/juju/juju/core/agentbinary"
	corearch "github.com/juju/juju/core/arch"
	coreerrors "github.com/juju/juju/core/errors"
	coremachine "github.com/juju/juju/core/machine"
	"github.com/juju/juju/core/semversion"
	coreunit "github.com/juju/juju/core/unit"
	unittesting "github.com/juju/juju/core/unit/testing"
	jujuversion "github.com/juju/juju/core/version"
	"github.com/juju/juju/domain/agentbinary"
	applicationerrors "github.com/juju/juju/domain/application/errors"
	machineerrors "github.com/juju/juju/domain/machine/errors"
	"github.com/juju/juju/domain/modelagent"
	modelagenterrors "github.com/juju/juju/domain/modelagent/errors"
	"github.com/juju/juju/internal/errors"
	"github.com/juju/juju/internal/uuid"
)

// modelUpgradeSuite is a set of tests for confirming the behaviour of upgrading
// a model.
type modelUpgradeSuite struct {
	agentBinaryFinder *MockAgentBinaryFinder
	modelState        *MockModelState
	controllerState   *MockControllerState
}

type serviceSuite struct {
	agentBinaryFinder *MockAgentBinaryFinder
	modelState        *MockModelState
	controllerState   *MockControllerState
}

// TestModelUpgradeSuite runs the tests that comprise the model upgrade suite.
func TestModelUpgradeSuite(t *testing.T) {
	tc.Run(t, &modelUpgradeSuite{})
}

// TestServiceSuite runs the tests that comprise the service suite.
func TestServiceSuite(t *testing.T) {
	tc.Run(t, &serviceSuite{})
}

func (s *modelUpgradeSuite) setupMocks(c *tc.C) *gomock.Controller {
	ctrl := gomock.NewController(c)
	s.agentBinaryFinder = NewMockAgentBinaryFinder(ctrl)
	s.modelState = NewMockModelState(ctrl)
	s.controllerState = NewMockControllerState(ctrl)
	return ctrl
}

func (s *serviceSuite) setupMocks(c *tc.C) *gomock.Controller {
	ctrl := gomock.NewController(c)
	s.agentBinaryFinder = NewMockAgentBinaryFinder(ctrl)
	s.modelState = NewMockModelState(ctrl)
	s.controllerState = NewMockControllerState(ctrl)

	c.Cleanup(func() {
		s.agentBinaryFinder = nil
		s.modelState = nil
		s.controllerState = nil
	})
	return ctrl
}

// TearDownTest is called after each test to nil out the mocks. This helps
// ensure correct setup of mocks for each test.
func (s *serviceSuite) TearDownTest(c *tc.C) {
	s.agentBinaryFinder = nil
	s.modelState = nil
	s.controllerState = nil
}

// TestGetModelAgentVersionSuccess tests the happy path for
// Service.GetModelAgentVersion.
func (s *serviceSuite) TestGetModelAgentVersionSuccess(c *tc.C) {
	defer s.setupMocks(c).Finish()

	expectedVersion, err := semversion.Parse("4.21.65")
	c.Assert(err, tc.ErrorIsNil)
	s.modelState.EXPECT().GetModelTargetAgentVersion(gomock.Any()).Return(expectedVersion, nil)

	svc := NewService(s.agentBinaryFinder, s.modelState, s.controllerState)
	ver, err := svc.GetModelTargetAgentVersion(c.Context())
	c.Check(err, tc.ErrorIsNil)
	c.Check(ver, tc.DeepEquals, expectedVersion)
}

// TestGetModelAgentVersionNotFound tests that Service.GetModelAgentVersion
// returns an appropriate error when the agent version cannot be found.
func (s *serviceSuite) TestGetModelAgentVersionModelNotFound(c *tc.C) {
	defer s.setupMocks(c).Finish()

	s.modelState.EXPECT().GetModelTargetAgentVersion(gomock.Any()).Return(semversion.Zero, modelagenterrors.AgentVersionNotFound)

	svc := NewService(s.agentBinaryFinder, s.modelState, s.controllerState)
	_, err := svc.GetModelTargetAgentVersion(c.Context())
	c.Check(err, tc.ErrorIs, modelagenterrors.AgentVersionNotFound)
}

// TestGetMachineTargetAgentVersion is asserting the happy path for getting
// a machine's target agent version.
func (s *serviceSuite) TestGetMachineTargetAgentVersion(c *tc.C) {
	defer s.setupMocks(c).Finish()

	machineName := coremachine.Name("0")
	uuid := uuid.MustNewUUID().String()
	ver := coreagentbinary.Version{
		Number: semversion.MustParse("4.0.0"),
		Arch:   "amd64",
	}

	s.modelState.EXPECT().GetMachineUUIDByName(gomock.Any(), machineName).Return(uuid, nil)
	s.modelState.EXPECT().GetMachineTargetAgentVersion(gomock.Any(), uuid).Return(ver, nil)

	svc := NewService(s.agentBinaryFinder, s.modelState, s.controllerState)
	rval, err := svc.GetMachineTargetAgentVersion(c.Context(), machineName)
	c.Check(err, tc.ErrorIsNil)
	c.Check(rval, tc.Equals, ver)
}

// TestGetMachineTargetAgentVersionNotFound is testing that the service
// returns a [machineerrors.MachineNotFound] error when no machine exists for
// a given name.
func (s *serviceSuite) TestGetMachineTargetAgentVersionNotFound(c *tc.C) {
	defer s.setupMocks(c).Finish()

	s.modelState.EXPECT().GetMachineUUIDByName(gomock.Any(), coremachine.Name("0")).Return(
		"", machineerrors.MachineNotFound,
	)

	svc := NewService(s.agentBinaryFinder, s.modelState, s.controllerState)
	_, err := svc.GetMachineTargetAgentVersion(
		c.Context(),
		coremachine.Name("0"),
	)
	c.Check(err, tc.ErrorIs, machineerrors.MachineNotFound)
}

// TestGetUnitTargetAgentVersion is asserting the happy path for getting
// a unit's target agent version.
func (s *serviceSuite) TestGetUnitTargetAgentVersion(c *tc.C) {
	defer s.setupMocks(c).Finish()

	ver := coreagentbinary.Version{
		Number: semversion.MustParse("4.0.0"),
		Arch:   "amd64",
	}

	uuid := unittesting.GenUnitUUID(c)
	s.modelState.EXPECT().GetUnitUUIDByName(gomock.Any(), coreunit.Name("foo/0")).Return(uuid, nil)
	s.modelState.EXPECT().GetUnitTargetAgentVersion(gomock.Any(), uuid).Return(ver, nil)

	svc := NewService(s.agentBinaryFinder, s.modelState, s.controllerState)
	rval, err := svc.GetUnitTargetAgentVersion(c.Context(), "foo/0")
	c.Check(err, tc.ErrorIsNil)
	c.Check(rval, tc.Equals, ver)
}

// TestGetUnitTargetAgentVersionNotFound is testing that the service
// returns a [applicationerrors.UnitNotFound] error when no unit exists for
// a given name.
func (s *serviceSuite) TestGetUnitTargetAgentVersionNotFound(c *tc.C) {
	defer s.setupMocks(c).Finish()

	s.modelState.EXPECT().GetUnitUUIDByName(gomock.Any(), coreunit.Name("foo/0")).Return(
		"", applicationerrors.UnitNotFound,
	)

	svc := NewService(s.agentBinaryFinder, s.modelState, s.controllerState)
	_, err := svc.GetUnitTargetAgentVersion(
		c.Context(),
		"foo/0",
	)
	c.Check(err, tc.ErrorIs, applicationerrors.UnitNotFound)
}

// TestWatchUnitTargetAgentVersionNotFound is testing that the service
// returns a [applicationerrors.UnitNotFound] error when no unit exists for
// a given name.
func (s *serviceSuite) TestWatchUnitTargetAgentVersionNotFound(c *tc.C) {
	defer s.setupMocks(c).Finish()

	s.modelState.EXPECT().GetUnitUUIDByName(gomock.Any(), coreunit.Name("foo/0")).Return(
		"", applicationerrors.UnitNotFound,
	)

	svc := NewWatchableService(s.agentBinaryFinder, s.modelState, nil)
	_, err := svc.WatchUnitTargetAgentVersion(
		c.Context(),
		"foo/0",
	)
	c.Check(err, tc.ErrorIs, applicationerrors.UnitNotFound)
}

// TestWatchMachineTargetAgentVersionNotFound is testing that the service
// returns a [machineerrors.MachineNotFound] error when no machine exists for
// a given name.
func (s *serviceSuite) TestWatchMachineTargetAgentVersionNotFound(c *tc.C) {
	defer s.setupMocks(c).Finish()

	s.modelState.EXPECT().GetMachineUUIDByName(gomock.Any(), coremachine.Name("0")).Return(
		"", machineerrors.MachineNotFound,
	)

	svc := NewWatchableService(s.agentBinaryFinder, s.modelState, nil)
	_, err := svc.WatchMachineTargetAgentVersion(c.Context(), "0")
	c.Check(err, tc.ErrorIs, machineerrors.MachineNotFound)
}

// TestSetMachineReportedAgentVersionInvalid is here to assert that if pass a
// junk agent binary version to [Service.SetMachineReportedAgentVersion] we get
// back an error that satisfies [coreerrors.NotValid].
func (s *serviceSuite) TestSetMachineReportedAgentVersionInvalid(c *tc.C) {
	defer s.setupMocks(c).Finish()

	svc := NewService(s.agentBinaryFinder, s.modelState, s.controllerState)
	err := svc.SetMachineReportedAgentVersion(
		c.Context(),
		coremachine.Name("0"),
		coreagentbinary.Version{
			Number: semversion.Zero,
		},
	)
	c.Check(err, tc.ErrorIs, coreerrors.NotValid)
}

// TestSetMachineReportedAgentVersionSuccess asserts that if we try to set the
// reported agent version for a machine that doesn't exist we get an error
// satisfying [machineerrors.MachineNotFound]. Because the service relied on
// modelState for producing this error we need to simulate this in two different
// locations to assert the full functionality.
func (s *serviceSuite) TestSetMachineReportedAgentVersionNotFound(c *tc.C) {
	defer s.setupMocks(c).Finish()

	// MachineNotFound error location 1.
	s.modelState.EXPECT().GetMachineUUIDByName(gomock.Any(), coremachine.Name("0")).Return(
		"", machineerrors.MachineNotFound,
	)

	svc := NewService(s.agentBinaryFinder, s.modelState, s.controllerState)
	err := svc.SetMachineReportedAgentVersion(
		c.Context(),
		coremachine.Name("0"),
		coreagentbinary.Version{
			Number: semversion.MustParse("1.2.3"),
			Arch:   corearch.ARM64,
		},
	)
	c.Check(err, tc.ErrorIs, machineerrors.MachineNotFound)

	// MachineNotFound error location 2.
	machineUUID, err := uuid.NewUUID()
	c.Assert(err, tc.ErrorIsNil)

	s.modelState.EXPECT().GetMachineUUIDByName(gomock.Any(), coremachine.Name("0")).Return(
		machineUUID.String(), nil,
	)

	s.modelState.EXPECT().SetMachineRunningAgentBinaryVersion(
		gomock.Any(),
		machineUUID.String(),
		coreagentbinary.Version{
			Number: semversion.MustParse("1.2.3"),
			Arch:   corearch.ARM64,
		},
	).Return(machineerrors.MachineNotFound)

	err = svc.SetMachineReportedAgentVersion(
		c.Context(),
		coremachine.Name("0"),
		coreagentbinary.Version{
			Number: semversion.MustParse("1.2.3"),
			Arch:   corearch.ARM64,
		},
	)
	c.Check(err, tc.ErrorIs, machineerrors.MachineNotFound)
}

func (s *serviceSuite) TestSetMachineReportedAgentVersionDead(c *tc.C) {
	defer s.setupMocks(c).Finish()

	machineUUID, err := uuid.NewUUID()
	c.Assert(err, tc.ErrorIsNil)

	s.modelState.EXPECT().GetMachineUUIDByName(gomock.Any(), coremachine.Name("0")).Return(
		machineUUID.String(), nil,
	)

	s.modelState.EXPECT().SetMachineRunningAgentBinaryVersion(
		gomock.Any(),
		machineUUID.String(),
		coreagentbinary.Version{
			Number: semversion.MustParse("1.2.3"),
			Arch:   corearch.ARM64,
		},
	).Return(machineerrors.MachineIsDead)

	svc := NewService(s.agentBinaryFinder, s.modelState, s.controllerState)
	err = svc.SetMachineReportedAgentVersion(
		c.Context(),
		coremachine.Name("0"),
		coreagentbinary.Version{
			Number: semversion.MustParse("1.2.3"),
			Arch:   corearch.ARM64,
		},
	)
	c.Check(err, tc.ErrorIs, machineerrors.MachineIsDead)
}

// TestSetMachineReportedAgentVersion asserts the happy path of
// [Service.SetMachineReportedAgentVersion].
func (s *serviceSuite) TestSetMachineReportedAgentVersion(c *tc.C) {
	defer s.setupMocks(c).Finish()

	machineUUID, err := uuid.NewUUID()
	c.Assert(err, tc.ErrorIsNil)

	s.modelState.EXPECT().GetMachineUUIDByName(gomock.Any(), coremachine.Name("0")).Return(
		machineUUID.String(), nil,
	)
	s.modelState.EXPECT().SetMachineRunningAgentBinaryVersion(
		gomock.Any(),
		machineUUID.String(),
		coreagentbinary.Version{
			Number: semversion.MustParse("1.2.3"),
			Arch:   corearch.ARM64,
		},
	).Return(nil)

	svc := NewService(s.agentBinaryFinder, s.modelState, s.controllerState)
	err = svc.SetMachineReportedAgentVersion(
		c.Context(),
		coremachine.Name("0"),
		coreagentbinary.Version{
			Number: semversion.MustParse("1.2.3"),
			Arch:   corearch.ARM64,
		},
	)
	c.Check(err, tc.ErrorIsNil)
}

// TestSetReportedUnitAgentVersionInvalid is here to assert that if pass a
// junk agent binary version to [Service.SetReportedUnitAgentVersion] we get
// back an error that satisfies [coreerrors.NotValid].
func (s *serviceSuite) TestSetReportedUnitAgentVersionInvalid(c *tc.C) {
	defer s.setupMocks(c).Finish()

	svc := NewService(s.agentBinaryFinder, s.modelState, s.controllerState)
	err := svc.SetUnitReportedAgentVersion(
		c.Context(),
		coreunit.Name("foo/666"),
		coreagentbinary.Version{
			Number: semversion.Zero,
		},
	)
	c.Check(err, tc.ErrorIs, coreerrors.NotValid)
}

// TestSetReportedUnitAgentVersionNotFound asserts that if we try to set the
// reported agent version for a unit that doesn't exist we get an error
// satisfying [applicationerrors.UnitNotFound]. Because the service relied on
// modelState for producing this error we need to simulate this in two different
// locations to assert the full functionality.
func (s *serviceSuite) TestSetReportedUnitAgentVersionNotFound(c *tc.C) {
	defer s.setupMocks(c).Finish()

	// UnitNotFound error location 1.
	s.modelState.EXPECT().GetUnitUUIDByName(gomock.Any(), coreunit.Name("foo/666")).Return(
		"", applicationerrors.UnitNotFound,
	)

	svc := NewService(s.agentBinaryFinder, s.modelState, s.controllerState)
	err := svc.SetUnitReportedAgentVersion(
		c.Context(),
		coreunit.Name("foo/666"),
		coreagentbinary.Version{
			Number: semversion.MustParse("1.2.3"),
			Arch:   corearch.ARM64,
		},
	)
	c.Check(err, tc.ErrorIs, applicationerrors.UnitNotFound)

	// UnitNotFound error location 2.
	unitUUID := unittesting.GenUnitUUID(c)

	s.modelState.EXPECT().GetUnitUUIDByName(gomock.Any(), coreunit.Name("foo/666")).Return(
		unitUUID, nil,
	)

	s.modelState.EXPECT().SetUnitRunningAgentBinaryVersion(
		gomock.Any(),
		unitUUID,
		coreagentbinary.Version{
			Number: semversion.MustParse("1.2.3"),
			Arch:   corearch.ARM64,
		},
	).Return(applicationerrors.UnitNotFound)

	err = svc.SetUnitReportedAgentVersion(
		c.Context(),
		coreunit.Name("foo/666"),
		coreagentbinary.Version{
			Number: semversion.MustParse("1.2.3"),
			Arch:   corearch.ARM64,
		},
	)
	c.Check(err, tc.ErrorIs, applicationerrors.UnitNotFound)
}

// TestSetReportedUnitAgentVersionDead asserts that if we try to set the
// reported agent version for a dead unit we get an error satisfying
// [applicationerrors.UnitIsDead].
func (s *serviceSuite) TestSetReportedUnitAgentVersionDead(c *tc.C) {
	defer s.setupMocks(c).Finish()

	unitUUID := unittesting.GenUnitUUID(c)

	s.modelState.EXPECT().GetUnitUUIDByName(gomock.Any(), coreunit.Name("foo/666")).Return(
		coreunit.UUID(unitUUID.String()), nil,
	)

	s.modelState.EXPECT().SetUnitRunningAgentBinaryVersion(
		gomock.Any(),
		coreunit.UUID(unitUUID.String()),
		coreagentbinary.Version{
			Number: semversion.MustParse("1.2.3"),
			Arch:   corearch.ARM64,
		},
	).Return(applicationerrors.UnitIsDead)

	svc := NewService(s.agentBinaryFinder, s.modelState, s.controllerState)
	err := svc.SetUnitReportedAgentVersion(
		c.Context(),
		coreunit.Name("foo/666"),
		coreagentbinary.Version{
			Number: semversion.MustParse("1.2.3"),
			Arch:   corearch.ARM64,
		},
	)
	c.Check(err, tc.ErrorIs, applicationerrors.UnitIsDead)
}

// TestSetReportedUnitAgentVersion asserts the happy path of
// [Service.SetReportedUnitAgentVersion].
func (s *serviceSuite) TestSetReportedUnitAgentVersion(c *tc.C) {
	defer s.setupMocks(c).Finish()

	unitUUID := unittesting.GenUnitUUID(c)

	s.modelState.EXPECT().GetUnitUUIDByName(gomock.Any(), coreunit.Name("foo/666")).Return(
		coreunit.UUID(unitUUID.String()), nil,
	)

	s.modelState.EXPECT().SetUnitRunningAgentBinaryVersion(
		gomock.Any(),
		coreunit.UUID(unitUUID.String()),
		coreagentbinary.Version{
			Number: semversion.MustParse("1.2.3"),
			Arch:   corearch.ARM64,
		},
	).Return(nil)

	svc := NewService(s.agentBinaryFinder, s.modelState, s.controllerState)
	err := svc.SetUnitReportedAgentVersion(
		c.Context(),
		coreunit.Name("foo/666"),
		coreagentbinary.Version{
			Number: semversion.MustParse("1.2.3"),
			Arch:   corearch.ARM64,
		},
	)
	c.Check(err, tc.ErrorIsNil)
}

// TestGetMachineReportedAgentVersionMachineNotFound asserts that if we ask for
// the reported agent version of a machine and the machine does not exist we get
// back an error that satisfies [machineerrors.MachineNotFound].
func (s *serviceSuite) TestGetMachineReportedAgentVersionMachineNotFound(c *tc.C) {
	defer s.setupMocks(c).Finish()

	machineName := coremachine.Name("0")

	// First test of MachineNotFound when translating from name to uuid.
	s.modelState.EXPECT().GetMachineUUIDByName(gomock.Any(), machineName).Return(
		"", machineerrors.MachineNotFound)

	svc := NewService(s.agentBinaryFinder, s.modelState, s.controllerState)
	_, err := svc.GetMachineReportedAgentVersion(c.Context(), machineName)
	c.Check(err, tc.ErrorIs, machineerrors.MachineNotFound)

	// Section test of MachineNotFound when using the uuid to fetch the running
	// version.
	uuid := uuid.MustNewUUID().String()
	s.modelState.EXPECT().GetMachineUUIDByName(gomock.Any(), machineName).Return(uuid, nil)
	s.modelState.EXPECT().GetMachineRunningAgentBinaryVersion(gomock.Any(), uuid).Return(
		coreagentbinary.Version{}, machineerrors.MachineNotFound,
	)

	_, err = svc.GetMachineReportedAgentVersion(c.Context(), machineName)
	c.Check(err, tc.ErrorIs, machineerrors.MachineNotFound)
}

// TestGetMachineReportedAgentVersionAgentVersionNotFound asserts that if we ask
// for the reported agent version of a machine and one has not been set that an
// error statisfying [modelagenterrors.AgentVersionNotFound].
func (s *serviceSuite) TestGetMachineReportedAgentVersionAgentVersionNotFound(c *tc.C) {
	defer s.setupMocks(c).Finish()

	machineName := coremachine.Name("0")

	uuid := uuid.MustNewUUID().String()
	s.modelState.EXPECT().GetMachineUUIDByName(gomock.Any(), machineName).Return(uuid, nil)
	s.modelState.EXPECT().GetMachineRunningAgentBinaryVersion(gomock.Any(), uuid).Return(
		coreagentbinary.Version{}, modelagenterrors.AgentVersionNotFound,
	)

	svc := NewService(s.agentBinaryFinder, s.modelState, s.controllerState)
	_, err := svc.GetMachineReportedAgentVersion(c.Context(), machineName)
	c.Check(err, tc.ErrorIs, modelagenterrors.AgentVersionNotFound)
}

// TestGetMachineReportedAgentVersion is a happy path test of
// [Service.GetMachineReportedAgentVersion].
func (s *serviceSuite) TestGetMachineReportedAgentVersion(c *tc.C) {
	defer s.setupMocks(c).Finish()

	machineName := coremachine.Name("0")

	uuid := uuid.MustNewUUID().String()
	s.modelState.EXPECT().GetMachineUUIDByName(gomock.Any(), machineName).Return(uuid, nil)
	s.modelState.EXPECT().GetMachineRunningAgentBinaryVersion(gomock.Any(), uuid).Return(
		coreagentbinary.Version{
			Number: semversion.MustParse("4.1.1"),
			Arch:   corearch.ARM64,
		}, nil,
	)

	svc := NewService(s.agentBinaryFinder, s.modelState, s.controllerState)
	ver, err := svc.GetMachineReportedAgentVersion(c.Context(), machineName)
	c.Check(err, tc.ErrorIsNil)
	c.Check(ver, tc.DeepEquals, coreagentbinary.Version{
		Number: semversion.MustParse("4.1.1"),
		Arch:   corearch.ARM64,
	})
}

// TestGetUnitReportedAgentVersionUnitNotFound asserts that if we ask for
// the reported agent version of a unit and the unit does not exist we get
// back an error that satisfies [applicationerrors.UnitNotFound].
func (s *serviceSuite) TestGetUnitReportedAgentVersionUnitNotFound(c *tc.C) {
	defer s.setupMocks(c).Finish()

	unitName := unittesting.GenNewName(c, "foo/0")

	// First test of UnitNotFound when translating from name to uuid.
	s.modelState.EXPECT().GetUnitUUIDByName(gomock.Any(), unitName).Return(
		"", applicationerrors.UnitNotFound)

	svc := NewService(s.agentBinaryFinder, s.modelState, s.controllerState)
	_, err := svc.GetUnitReportedAgentVersion(c.Context(), unitName)
	c.Check(err, tc.ErrorIs, applicationerrors.UnitNotFound)

	// Section test of UnitNotFound when using the uuid to fetch the running
	// version.
	uuid := unittesting.GenUnitUUID(c)
	s.modelState.EXPECT().GetUnitUUIDByName(gomock.Any(), unitName).Return(uuid, nil)
	s.modelState.EXPECT().GetUnitRunningAgentBinaryVersion(gomock.Any(), uuid).Return(
		coreagentbinary.Version{}, applicationerrors.UnitNotFound,
	)

	_, err = svc.GetUnitReportedAgentVersion(c.Context(), unitName)
	c.Check(err, tc.ErrorIs, applicationerrors.UnitNotFound)
}

// TestGetUnitReportedAgentVersionAgentVersionNotFound asserts that if we ask
// for the reported agent version of a unit and one has not been set that an
// error statisfying [modelagenterrors.AgentVersionNotFound].
func (s *serviceSuite) TestGetUnitReportedAgentVersionAgentVersionNotFound(c *tc.C) {
	defer s.setupMocks(c).Finish()

	unitName := unittesting.GenNewName(c, "foo/0")
	unitUUID := unittesting.GenUnitUUID(c)

	s.modelState.EXPECT().GetUnitUUIDByName(gomock.Any(), unitName).Return(unitUUID, nil)
	s.modelState.EXPECT().GetUnitRunningAgentBinaryVersion(gomock.Any(), unitUUID).Return(
		coreagentbinary.Version{}, modelagenterrors.AgentVersionNotFound,
	)

	svc := NewService(s.agentBinaryFinder, s.modelState, s.controllerState)
	_, err := svc.GetUnitReportedAgentVersion(c.Context(), unitName)
	c.Check(err, tc.ErrorIs, modelagenterrors.AgentVersionNotFound)
}

// TestGetUnitReportedAgentVersion is a happy path test of
// [Service.GetMachineReportedAgentVersion].
func (s *serviceSuite) TestGetUnitReportedAgentVersion(c *tc.C) {
	defer s.setupMocks(c).Finish()

	unitName := unittesting.GenNewName(c, "foo/0")
	unitUUID := unittesting.GenUnitUUID(c)

	s.modelState.EXPECT().GetUnitUUIDByName(gomock.Any(), unitName).Return(unitUUID, nil)
	s.modelState.EXPECT().GetUnitRunningAgentBinaryVersion(gomock.Any(), unitUUID).Return(
		coreagentbinary.Version{
			Number: semversion.MustParse("4.1.1"),
			Arch:   corearch.ARM64,
		}, nil,
	)

	svc := NewService(s.agentBinaryFinder, s.modelState, s.controllerState)
	ver, err := svc.GetUnitReportedAgentVersion(c.Context(), unitName)
	c.Check(err, tc.ErrorIsNil)
	c.Check(ver, tc.DeepEquals, coreagentbinary.Version{
		Number: semversion.MustParse("4.1.1"),
		Arch:   corearch.ARM64,
	})
}

// TestGetMachinesReportedAgentVersionAgentVersionNotSet asserts error
// pass through on modelState of modelagenterrors.AgentVersionNotSet to
// satisfy contract.
func (s *serviceSuite) TestGetMachinesReportedAgentVersionAgentVersionNotSet(c *tc.C) {
	defer s.setupMocks(c).Finish()
	s.modelState.EXPECT().GetMachinesAgentBinaryMetadata(gomock.Any()).Return(
		nil, modelagenterrors.AgentVersionNotSet,
	)
	svc := NewService(s.agentBinaryFinder, s.modelState, s.controllerState)
	_, err := svc.GetMachinesAgentBinaryMetadata(c.Context())
	c.Check(err, tc.ErrorIs, modelagenterrors.AgentVersionNotSet)
}

// TestGetMachinesReportedAgentVersionMissingAgentBinaries asserts error pass
// through on modelState of modelagenterrors.MissingAgentBinaries to satisfy
// contract.
func (s *serviceSuite) TestGetMachinesReportedAgentVersionMissingAgentBinaries(c *tc.C) {
	defer s.setupMocks(c).Finish()
	s.modelState.EXPECT().GetMachinesAgentBinaryMetadata(gomock.Any()).Return(
		nil, modelagenterrors.MissingAgentBinaries,
	)
	svc := NewService(s.agentBinaryFinder, s.modelState, s.controllerState)
	_, err := svc.GetMachinesAgentBinaryMetadata(c.Context())
	c.Check(err, tc.ErrorIs, modelagenterrors.MissingAgentBinaries)
}

func (s *serviceSuite) TestGetMachineAgentBinaryMetadata(c *tc.C) {
	defer s.setupMocks(c).Finish()

	machineName := coremachine.Name("0")

	s.modelState.EXPECT().GetMachineAgentBinaryMetadata(gomock.Any(), machineName.String()).Return(
		coreagentbinary.Metadata{
			SHA256:  "h@sh256",
			SHA384:  "h@sh384",
			Size:    1234,
			Version: coreagentbinary.Version{Number: semversion.MustParse("4.1.1"), Arch: corearch.ARM64},
		}, nil,
	)

	svc := NewService(s.agentBinaryFinder, s.modelState, s.controllerState)
	ver, err := svc.GetMachineAgentBinaryMetadata(c.Context(), machineName)
	c.Check(err, tc.ErrorIsNil)
	c.Check(ver, tc.DeepEquals, coreagentbinary.Metadata{
		SHA256:  "h@sh256",
		SHA384:  "h@sh384",
		Size:    1234,
		Version: coreagentbinary.Version{Number: semversion.MustParse("4.1.1"), Arch: corearch.ARM64},
	})
}

func (s *serviceSuite) TestGetMachineAgentBinaryMetadataMachineNotFound(c *tc.C) {
	defer s.setupMocks(c).Finish()

	machineName := coremachine.Name("0")

	s.modelState.EXPECT().GetMachineAgentBinaryMetadata(gomock.Any(), machineName.String()).Return(
		coreagentbinary.Metadata{}, machineerrors.MachineNotFound,
	)

	svc := NewService(s.agentBinaryFinder, s.modelState, s.controllerState)
	_, err := svc.GetMachineAgentBinaryMetadata(c.Context(), machineName)
	c.Check(err, tc.ErrorIs, machineerrors.MachineNotFound)
}

// TestGetUnitReportedAgentVersionAgentVersionNotSet asserts error pass
// through on modelState of modelagenterrors.AgentVersionNotSet to satisfy
// contract.
func (s *serviceSuite) TestGetUnitReportedAgentVersionAgentVersionNotSet(c *tc.C) {
	defer s.setupMocks(c).Finish()
	s.modelState.EXPECT().GetUnitsAgentBinaryMetadata(gomock.Any()).Return(
		nil, modelagenterrors.AgentVersionNotSet,
	)
	svc := NewService(s.agentBinaryFinder, s.modelState, s.controllerState)
	_, err := svc.GetUnitsAgentBinaryMetadata(c.Context())
	c.Check(err, tc.ErrorIs, modelagenterrors.AgentVersionNotSet)
}

// TestGetUnitReportedAgentVersionMissingAgentBinaries asserts error pass
// through on modelState of modelagenterrors.MissingAgentBinaries to satisfy
// contract.
func (s *serviceSuite) TestGetUnitReportedAgentVersionMissingAgentBinaries(c *tc.C) {
	defer s.setupMocks(c).Finish()
	s.modelState.EXPECT().GetUnitsAgentBinaryMetadata(gomock.Any()).Return(
		nil, modelagenterrors.MissingAgentBinaries,
	)
	svc := NewService(s.agentBinaryFinder, s.modelState, s.controllerState)
	_, err := svc.GetUnitsAgentBinaryMetadata(c.Context())
	c.Check(err, tc.ErrorIs, modelagenterrors.MissingAgentBinaries)
}

// TestSetAgentStreamNotValidAgentStream is testing that if we supply an
// unknown agent stream to [Service.SetModelAgentStream] we get back an error
// satisfying [coreerrors.NotValid].
func (s *serviceSuite) TestSetAgentStreamNotValidAgentStream(c *tc.C) {
	defer s.setupMocks(c).Finish()

	// This is a fake stream that doesn't exist.
	agentStream := coreagentbinary.AgentStream("bad value")

	svc := NewService(s.agentBinaryFinder, s.modelState, s.controllerState)
	err := svc.SetModelAgentStream(
		c.Context(),
		agentStream,
	)
	c.Check(err, tc.ErrorIs, coreerrors.NotValid)
}

// TestSetAgentStream is testing the happy path of setting the model's agent
// stream.
func (s *serviceSuite) TestSetAgentStream(c *tc.C) {
	defer s.setupMocks(c).Finish()

	s.modelState.EXPECT().SetModelAgentStream(
		gomock.Any(),
		modelagent.AgentStreamTesting,
	).Return(nil)

	svc := NewService(s.agentBinaryFinder, s.modelState, s.controllerState)
	err := svc.SetModelAgentStream(
		c.Context(),
		coreagentbinary.AgentStreamTesting,
	)
	c.Check(err, tc.ErrorIsNil)
}

// getVersionMinorLess is a helper function for getting back a version that is
// one minor version less then the current version of Juju. This exists to
// support upgrade tests by contriving a version that needs to be upgraded
// relative to the current version of Juju.
func (s *modelUpgradeSuite) getVersionMinorLess() semversion.Number {
	rval := semversion.MustParse("4.0.1")
	// We don't want to drag the Minor version into negative numbers
	if rval.Minor > 0 {
		rval.Minor--
	} else {
		rval.Major--
	}
	return rval
}

// TestUpgradeModelTargetAgentVersionControllerModel tests that if a caller asks
// for the current model's target agent version to be upgrade, but the model
// hosts the current Juju controller. No upgrade is performed and the caller
// gets back an error satisfying
// [modelagenterrors.CannotUpgradeControllerModel].
func (s *modelUpgradeSuite) TestUpgradeModelTargetAgentVersionControllerModel(c *tc.C) {
	defer s.setupMocks(c).Finish()

	currentTargetVersion := s.getVersionMinorLess()
	desiredVersion := semversion.MustParse("4.0.1")
	s.controllerState.EXPECT().GetControllerAgentVersionsByArchitecture(
		gomock.Any(),
		[]agentbinary.Architecture{agentbinary.AMD64},
	).Return([]semversion.Number{desiredVersion}, nil).Times(2)
	s.agentBinaryFinder.EXPECT().HasBinariesForVersion(gomock.Any()).Return(true, nil)
	s.modelState.EXPECT().GetModelTargetAgentVersion(gomock.Any()).Return(currentTargetVersion, nil)
	s.modelState.EXPECT().IsControllerModel(gomock.Any()).Return(true, nil)

	svc := NewService(s.agentBinaryFinder, s.modelState, s.controllerState)
	_, err := svc.UpgradeModelTargetAgentVersion(c.Context())
	c.Check(err, tc.ErrorIs, modelagenterrors.CannotUpgradeControllerModel)
}

// TestUpgradeModelTargetAgentVersionMachineBaseValidation tests that if a
// caller asks for the current model's target agent version to be
// upgraded, but there are machines in the model that are not running a
// supported base. The upgrade must fail with an error satisfying
// [modelagenterrors.ModelUpgradeBlocker].
func (s *modelUpgradeSuite) TestUpgradeModelTargetAgentVersionMachineBaseValidation(c *tc.C) {
	defer s.setupMocks(c).Finish()

	currentTargetVersion := s.getVersionMinorLess()
	desiredVersion := semversion.MustParse("4.0.1")
	s.controllerState.EXPECT().GetControllerAgentVersionsByArchitecture(
		gomock.Any(),
		[]agentbinary.Architecture{agentbinary.AMD64},
	).Return([]semversion.Number{desiredVersion}, nil).Times(2)
	s.modelState.EXPECT().GetModelTargetAgentVersion(gomock.Any()).Return(currentTargetVersion, nil)
	s.modelState.EXPECT().IsControllerModel(gomock.Any()).Return(false, nil)
	s.agentBinaryFinder.EXPECT().HasBinariesForVersion(gomock.Any()).Return(true, nil)
	s.modelState.EXPECT().GetMachineCountNotUsingBase(gomock.Any(), gomock.Any()).Return(1, nil)

	svc := NewService(s.agentBinaryFinder, s.modelState, s.controllerState)
	_, err := svc.UpgradeModelTargetAgentVersion(c.Context())
	_, isBlockedErr := errors.AsType[modelagenterrors.ModelUpgradeBlocker](err)
	c.Check(isBlockedErr, tc.IsTrue)
}

// TestUpgradeModelTargetAgentVersion is a happy path test of
// [Service.UpgradeMoelTargetAgentVersion]. In this test we want to see that the
// model is upgraded to that highest available version available.
func (s *modelUpgradeSuite) TestUpgradeModelTargetAgentVersion(c *tc.C) {
	defer s.setupMocks(c).Finish()

	currentTargetVersion := s.getVersionMinorLess()
	version1 := semversion.MustParse("4.0.1")
	version2 := semversion.MustParse("4.0.2")
	version3 := semversion.MustParse("4.0.3")
	version4 := semversion.MustParse("4.0.4")
	desiredVersion := semversion.MustParse("4.0.5")
	// Our service has logic to narrow down to pick the highest version
	// which is `desiredVersion`.
	s.controllerState.EXPECT().GetControllerAgentVersionsByArchitecture(
		gomock.Any(),
		[]agentbinary.Architecture{agentbinary.AMD64},
	).Return([]semversion.Number{
		version2,
		version3,
		desiredVersion,
		version4,
		version1,
	}, nil).Times(2)
	s.agentBinaryFinder.EXPECT().HasBinariesForVersion(desiredVersion).Return(true, nil)
	s.modelState.EXPECT().GetMachineCountNotUsingBase(gomock.Any(), gomock.Any()).Return(0, nil)
	s.modelState.EXPECT().GetModelTargetAgentVersion(gomock.Any()).Return(currentTargetVersion, nil)
	s.modelState.EXPECT().IsControllerModel(gomock.Any()).Return(false, nil)
	s.modelState.EXPECT().SetModelTargetAgentVersion(
		gomock.Any(),
		currentTargetVersion,
		desiredVersion,
	).Return(nil)

	svc := NewService(s.agentBinaryFinder, s.modelState, s.controllerState)
	newVer, err := svc.UpgradeModelTargetAgentVersion(c.Context())
	c.Check(err, tc.ErrorIsNil)
	c.Check(newVer, tc.Equals, desiredVersion)
}

// TestUpgradeModelTargetAgentVersionStreamControllerModel tests that if a
// caller asks for the current model's target agent version to be upgrade, but
// the model hosts the current Juju controller. No upgrade is performed and the
// caller gets back an error satisfying
// [modelagenterrors.CannotUpgradeControllerModel].
func (s *modelUpgradeSuite) TestUpgradeModelTargetAgentVersionStreamControllerModel(c *tc.C) {
	defer s.setupMocks(c).Finish()

	currentTargetVersion := s.getVersionMinorLess()
	desiredVersion := semversion.MustParse("4.0.1")
	s.controllerState.EXPECT().GetControllerAgentVersionsByArchitecture(
		gomock.Any(),
		[]agentbinary.Architecture{agentbinary.AMD64},
	).Return([]semversion.Number{desiredVersion}, nil).Times(2)
	s.agentBinaryFinder.EXPECT().HasBinariesForVersion(gomock.Any()).Return(true, nil)
	s.modelState.EXPECT().GetModelTargetAgentVersion(gomock.Any()).Return(currentTargetVersion, nil)
	s.modelState.EXPECT().IsControllerModel(gomock.Any()).Return(true, nil)

	svc := NewService(s.agentBinaryFinder, s.modelState, s.controllerState)
	_, err := svc.UpgradeModelTargetAgentVersionStream(
		c.Context(), modelagent.AgentStreamDevel,
	)
	c.Check(err, tc.ErrorIs, modelagenterrors.CannotUpgradeControllerModel)
}

// TestUpgradeModelTargetAgentVersionStreamNotValid is a test that asserts if a
// caller asks for the current model's target agent version to be upgraded with
// an invalid agent stream, the caller gets back an error satisfying
// [coreerrors.NotValid].
func (s *modelUpgradeSuite) TestUpgradeModelTargetAgentVersionStreamNotValid(c *tc.C) {
	defer s.setupMocks(c).Finish()

	agentStream := modelagent.AgentStream(-1)
	desiredVersion := semversion.MustParse("4.0.1")
	s.controllerState.EXPECT().GetControllerAgentVersionsByArchitecture(
		gomock.Any(),
		[]agentbinary.Architecture{agentbinary.AMD64},
	).Return([]semversion.Number{desiredVersion}, nil)

	svc := NewService(s.agentBinaryFinder, s.modelState, s.controllerState)
	_, err := svc.UpgradeModelTargetAgentVersionStream(c.Context(), agentStream)
	c.Check(err, tc.ErrorIs, coreerrors.NotValid)
}

// TestUpgradeModelTargetAgentVersionStreamMachineBaseValidation tests that if a
// caller asks for the for the current model's target agent version to be
// upgraded, but there are machines in the model that are not running a
// supported base. The upgrade must fail with an error satisfying
// [modelagenterrors.ModelUpgradeBlocker].
func (s *modelUpgradeSuite) TestUpgradeModelTargetAgentVersionStreamMachineBaseValidation(c *tc.C) {
	defer s.setupMocks(c).Finish()

	currentTargetVersion := s.getVersionMinorLess()
	desiredVersion := semversion.MustParse("4.0.1")
	s.controllerState.EXPECT().GetControllerAgentVersionsByArchitecture(
		gomock.Any(),
		[]agentbinary.Architecture{agentbinary.AMD64},
	).Return([]semversion.Number{desiredVersion}, nil).Times(2)
	s.modelState.EXPECT().GetModelTargetAgentVersion(gomock.Any()).Return(currentTargetVersion, nil)
	s.modelState.EXPECT().IsControllerModel(gomock.Any()).Return(false, nil)
	s.agentBinaryFinder.EXPECT().HasBinariesForVersion(gomock.Any()).Return(true, nil)
	s.modelState.EXPECT().GetMachineCountNotUsingBase(gomock.Any(), gomock.Any()).Return(1, nil)

	svc := NewService(s.agentBinaryFinder, s.modelState, s.controllerState)
	_, err := svc.UpgradeModelTargetAgentVersionStream(
		c.Context(), modelagent.AgentStreamDevel,
	)
	_, isBlockedErr := errors.AsType[modelagenterrors.ModelUpgradeBlocker](err)
	c.Check(isBlockedErr, tc.IsTrue)
}

// TestUpgradeModelTargetAgentVersionStream is a happy path test of
// [Service.UpgradeMoelTargetAgentVersionStream]. In this test we want to see
// that the model is upgraded to that highest available version available.
func (s *modelUpgradeSuite) TestUpgradeModelTargetAgentVersionStream(c *tc.C) {
	defer s.setupMocks(c).Finish()

	currentTargetVersion := s.getVersionMinorLess()
	desiredVersion := semversion.MustParse("4.0.1")
	s.controllerState.EXPECT().GetControllerAgentVersionsByArchitecture(
		gomock.Any(),
		[]agentbinary.Architecture{agentbinary.AMD64},
	).Return([]semversion.Number{desiredVersion}, nil).Times(2)
	s.agentBinaryFinder.EXPECT().HasBinariesForVersion(desiredVersion).Return(true, nil)
	s.modelState.EXPECT().GetMachineCountNotUsingBase(gomock.Any(), gomock.Any()).Return(0, nil)
	s.modelState.EXPECT().GetModelTargetAgentVersion(gomock.Any()).Return(currentTargetVersion, nil)
	s.modelState.EXPECT().IsControllerModel(gomock.Any()).Return(false, nil)
	s.modelState.EXPECT().SetModelTargetAgentVersionAndStream(
		gomock.Any(),
		currentTargetVersion,
		desiredVersion,
		modelagent.AgentStreamDevel,
	).Return(nil)

	svc := NewService(s.agentBinaryFinder, s.modelState, s.controllerState)
	newVer, err := svc.UpgradeModelTargetAgentVersionStream(
		c.Context(), modelagent.AgentStreamDevel,
	)
	c.Check(err, tc.ErrorIsNil)
	c.Check(newVer, tc.Equals, desiredVersion)
}

// TestUpgradeModelTargetAgentVersionToDowngrade is a test that asserts if a
// model upgrade is requested to a specific version and it would be considered a
// downgrade, the caller gets back an error satisfying
// [modelagenterrors.DowngradeNotSupport].
func (s *modelUpgradeSuite) TestUpgradeModelTargetAgentVersionToDowngrade(c *tc.C) {
	defer s.setupMocks(c).Finish()

	s.modelState.EXPECT().GetModelTargetAgentVersion(gomock.Any()).Return(jujuversion.Current, nil)

	upgradeTo := semversion.MustParse("3.6.1")
	svc := NewService(s.agentBinaryFinder, s.modelState, s.controllerState)
	err := svc.UpgradeModelTargetAgentVersionTo(c.Context(), upgradeTo)
	c.Check(err, tc.ErrorIs, modelagenterrors.DowngradeNotSupported)
}

// TestUpgradeModelTargetAgentVersionToOverMax is a test that asserts if a model
// upgrade is requested to a version that is greater than the max supported
// version of the controller. The caller gets back an error satisfying
// [modelagenterrors.AgentVersionNotSupported].
func (s *modelUpgradeSuite) TestUpgradeModelTargetAgentVersionToOverMax(c *tc.C) {
	defer s.setupMocks(c).Finish()
	version := semversion.MustParse("4.0.0")
	s.modelState.EXPECT().GetModelTargetAgentVersion(gomock.Any()).Return(version, nil)
	s.controllerState.EXPECT().GetControllerAgentVersionsByArchitecture(
		gomock.Any(),
		[]agentbinary.Architecture{agentbinary.AMD64},
	).Return([]semversion.Number{version}, nil)
	// This is a version that is greater than the max supported version of the
	// controller.
	upgradeTo := version
	upgradeTo.Minor++
	svc := NewService(s.agentBinaryFinder, s.modelState, s.controllerState)
	err := svc.UpgradeModelTargetAgentVersionTo(c.Context(), upgradeTo)
	c.Check(err, tc.ErrorIs, modelagenterrors.AgentVersionNotSupported)
}

// TestUpgradeModelTargetAgentVersionToMissingAgentBinaries is a test that
// asserts if a model upgrade is requested to a version that does not have
// agent binaries available, the caller gets back an error satisfying
// [modelagenterrors.MissingAgentBinaries].
func (s *modelUpgradeSuite) TestUpgradeModelTargetAgentVersionToMissingAgentBinaries(c *tc.C) {
	defer s.setupMocks(c).Finish()

	currentTargetVersion := s.getVersionMinorLess()
	desiredVersion := semversion.MustParse("4.0.1")
	s.controllerState.EXPECT().GetControllerAgentVersionsByArchitecture(
		gomock.Any(),
		[]agentbinary.Architecture{agentbinary.AMD64},
	).Return([]semversion.Number{desiredVersion}, nil)
	s.modelState.EXPECT().GetModelTargetAgentVersion(gomock.Any()).Return(currentTargetVersion, nil)
	s.agentBinaryFinder.EXPECT().HasBinariesForVersion(desiredVersion).Return(false, nil)

	svc := NewService(s.agentBinaryFinder, s.modelState, s.controllerState)
	err := svc.UpgradeModelTargetAgentVersionTo(c.Context(), desiredVersion)
	c.Check(err, tc.ErrorIs, modelagenterrors.MissingAgentBinaries)
}

// TestUpgradeModelTargetAgentVersionToControllerModel is a test that asserts
// if the controller model is attempted to be upgraded the caller gets back an
// error satisfying [modelagenterrors.CannotUpgradeControllerModel].
func (s *modelUpgradeSuite) TestUpgradeModelTargetAgentVersionToControllerModel(c *tc.C) {
	defer s.setupMocks(c).Finish()

	currentTargetVersion := s.getVersionMinorLess()
	desiredVersion := semversion.MustParse("4.0.1")
	s.controllerState.EXPECT().GetControllerAgentVersionsByArchitecture(
		gomock.Any(),
		[]agentbinary.Architecture{agentbinary.AMD64},
	).Return([]semversion.Number{desiredVersion}, nil)
	s.modelState.EXPECT().GetModelTargetAgentVersion(gomock.Any()).Return(currentTargetVersion, nil)
	s.agentBinaryFinder.EXPECT().HasBinariesForVersion(desiredVersion).Return(true, nil)
	s.modelState.EXPECT().IsControllerModel(gomock.Any()).Return(true, nil)

	svc := NewService(s.agentBinaryFinder, s.modelState, s.controllerState)
	err := svc.UpgradeModelTargetAgentVersionTo(c.Context(), desiredVersion)
	c.Check(err, tc.ErrorIs, modelagenterrors.CannotUpgradeControllerModel)
}

// TestUpgradeModelTargetAgentVersionToMachineBaseValidation is a test that
// asserts a model cannot be upgraded to a new version when there exists
// machines in the model that are running unsupported bases. This test expects
// that the caller gets back an error satisfying
// [modelagenterrors.ModelUpgradeBlocker].
func (s *modelUpgradeSuite) TestUpgradeModelTargetAgentVersionToMachineBaseValidation(c *tc.C) {
	defer s.setupMocks(c).Finish()

	currentTargetVersion := s.getVersionMinorLess()
	desiredVersion := semversion.MustParse("4.0.1")
	s.controllerState.EXPECT().GetControllerAgentVersionsByArchitecture(
		gomock.Any(),
		[]agentbinary.Architecture{agentbinary.AMD64},
	).Return([]semversion.Number{desiredVersion}, nil)
	s.modelState.EXPECT().GetModelTargetAgentVersion(gomock.Any()).Return(currentTargetVersion, nil)
	s.agentBinaryFinder.EXPECT().HasBinariesForVersion(desiredVersion).Return(true, nil)
	s.modelState.EXPECT().IsControllerModel(gomock.Any()).Return(false, nil)
	s.modelState.EXPECT().GetMachineCountNotUsingBase(gomock.Any(), gomock.Any()).Return(1, nil)

	svc := NewService(s.agentBinaryFinder, s.modelState, s.controllerState)
	err := svc.UpgradeModelTargetAgentVersionTo(c.Context(), desiredVersion)
	_, isBlockedErr := errors.AsType[modelagenterrors.ModelUpgradeBlocker](err)
	c.Check(isBlockedErr, tc.IsTrue)
}

// TestUpgradeModelTargetAgentVersionTo is a happy path test for upgrading a
// model to a specific target agent version.
func (s *modelUpgradeSuite) TestUpgradeModelTargetAgentVersionTo(c *tc.C) {
	defer s.setupMocks(c).Finish()

	currentTargetVersion := s.getVersionMinorLess()
	desiredVersion := semversion.MustParse("4.0.1")
	s.controllerState.EXPECT().GetControllerAgentVersionsByArchitecture(
		gomock.Any(),
		[]agentbinary.Architecture{agentbinary.AMD64},
	).Return([]semversion.Number{desiredVersion}, nil)
	s.modelState.EXPECT().GetModelTargetAgentVersion(gomock.Any()).Return(currentTargetVersion, nil)
	s.agentBinaryFinder.EXPECT().HasBinariesForVersion(desiredVersion).Return(true, nil)
	s.modelState.EXPECT().IsControllerModel(gomock.Any()).Return(false, nil)
	s.modelState.EXPECT().GetMachineCountNotUsingBase(gomock.Any(), gomock.Any()).Return(0, nil)
	s.modelState.EXPECT().SetModelTargetAgentVersion(
		gomock.Any(),
		currentTargetVersion,
		desiredVersion,
	).Return(nil)

	svc := NewService(s.agentBinaryFinder, s.modelState, s.controllerState)
	err := svc.UpgradeModelTargetAgentVersionTo(c.Context(), desiredVersion)
	c.Check(err, tc.ErrorIsNil)
}

// TestUpgradeModelTargetAgentVersionStreamToDowngrade is a test that asserts if
// a model upgrade is requested to a specific version and it would be considered
// a downgrade, the caller gets back an error satisfying
// [modelagenterrors.DowngradeNotSupport].
func (s *modelUpgradeSuite) TestUpgradeModelTargetAgentVersionStreamToDowngrade(c *tc.C) {
	defer s.setupMocks(c).Finish()

	version := semversion.MustParse("4.0.0")
	s.modelState.EXPECT().GetModelTargetAgentVersion(gomock.Any()).Return(version, nil)

	upgradeTo := semversion.MustParse("3.6.1")
	svc := NewService(s.agentBinaryFinder, s.modelState, s.controllerState)
	err := svc.UpgradeModelTargetAgentVersionStreamTo(
		c.Context(), upgradeTo, modelagent.AgentStreamDevel,
	)
	c.Check(err, tc.ErrorIs, modelagenterrors.DowngradeNotSupported)
}

// TestUpgradeModelTargetAgentVersionStreamToOverMax is a test that asserts if a
// model upgrade is requested to a version that is greater than the max
// supported version of the controller. The caller gets back an error satisfying
// [modelagenterrors.AgentVersionNotSupported].
func (s *modelUpgradeSuite) TestUpgradeModelTargetAgentVersionStreamToOverMax(c *tc.C) {
	defer s.setupMocks(c).Finish()

	version := semversion.MustParse("4.0.1")
	s.controllerState.EXPECT().GetControllerAgentVersionsByArchitecture(
		gomock.Any(),
		[]agentbinary.Architecture{agentbinary.AMD64},
	).Return([]semversion.Number{version}, nil)
	s.modelState.EXPECT().GetModelTargetAgentVersion(gomock.Any()).Return(version, nil)

	// This is a version that is greater than the max supported version of the
	// controller.
	upgradeTo := version
	upgradeTo.Minor++
	svc := NewService(s.agentBinaryFinder, s.modelState, s.controllerState)
	err := svc.UpgradeModelTargetAgentVersionStreamTo(
		c.Context(), upgradeTo, modelagent.AgentStreamDevel,
	)
	c.Check(err, tc.ErrorIs, modelagenterrors.AgentVersionNotSupported)
}

// TestUpgradeModelTargetAgentVersionStreamToMissingAgentBinaries is a test that
// asserts if a model upgrade is requested to a version that does not have
// agent binaries available, the caller gets back an error satisfying
// [modelagenterrors.MissingAgentBinaries].
func (s *modelUpgradeSuite) TestUpgradeModelTargetAgentVersionStreamToMissingAgentBinaries(c *tc.C) {
	defer s.setupMocks(c).Finish()

	currentTargetVersion := s.getVersionMinorLess()
	desiredVersion := semversion.MustParse("4.0.1")
	s.controllerState.EXPECT().GetControllerAgentVersionsByArchitecture(
		gomock.Any(),
		[]agentbinary.Architecture{agentbinary.AMD64},
	).Return([]semversion.Number{desiredVersion}, nil)
	s.modelState.EXPECT().GetModelTargetAgentVersion(gomock.Any()).Return(currentTargetVersion, nil)
	s.agentBinaryFinder.EXPECT().HasBinariesForVersion(desiredVersion).Return(false, nil)

	svc := NewService(s.agentBinaryFinder, s.modelState, s.controllerState)
	err := svc.UpgradeModelTargetAgentVersionStreamTo(
		c.Context(), desiredVersion, modelagent.AgentStreamDevel,
	)
	c.Check(err, tc.ErrorIs, modelagenterrors.MissingAgentBinaries)
}

// TestUpgradeModelTargetAgentVersionStreamToControllerModel is a test that
// asserts if the controller model is attempted to be upgraded the caller gets
// back an error satisfying [modelagenterrors.CannotUpgradeControllerModel].
func (s *modelUpgradeSuite) TestUpgradeModelTargetAgentVersionStreamToControllerModel(c *tc.C) {
	defer s.setupMocks(c).Finish()

	currentTargetVersion := s.getVersionMinorLess()
	desiredVersion := semversion.MustParse("4.0.1")
	s.controllerState.EXPECT().GetControllerAgentVersionsByArchitecture(
		gomock.Any(),
		[]agentbinary.Architecture{agentbinary.AMD64},
	).Return([]semversion.Number{desiredVersion}, nil)
	s.modelState.EXPECT().GetModelTargetAgentVersion(gomock.Any()).Return(currentTargetVersion, nil)
	s.agentBinaryFinder.EXPECT().HasBinariesForVersion(desiredVersion).Return(true, nil)
	s.modelState.EXPECT().IsControllerModel(gomock.Any()).Return(true, nil)

	svc := NewService(s.agentBinaryFinder, s.modelState, s.controllerState)
	err := svc.UpgradeModelTargetAgentVersionStreamTo(
		c.Context(), desiredVersion, modelagent.AgentStreamDevel,
	)
	c.Check(err, tc.ErrorIs, modelagenterrors.CannotUpgradeControllerModel)
}

// TestUpgradeModelTargetAgentVersionStreamToInvalidStream is a test that
// asserts when upgrade a model to a specific version and stream, if the stream
// supplied is not valid, the caller gets back an error satisfying
// [coreerrors.NotValid].
func (s *modelUpgradeSuite) TestUpgradeModelTargetAgentVersionStreamToInvalidStream(c *tc.C) {
	defer s.setupMocks(c).Finish()

	desiredVersion := semversion.MustParse("4.0.1")
	agentStream := modelagent.AgentStream(-1)

	svc := NewService(s.agentBinaryFinder, s.modelState, s.controllerState)
	err := svc.UpgradeModelTargetAgentVersionStreamTo(
		c.Context(), desiredVersion, agentStream,
	)
	c.Check(err, tc.ErrorIs, coreerrors.NotValid)
}

// TestUpgradeModelTargetAgentVersionToMachineBaseValidation is a test that
// asserts a model cannot be upgraded to a new version when there exists
// machines in the model that are running unsupported bases. This test expects
// that the caller gets back an error satisfying
// [modelagenterrors.ModelUpgradeBlocker].
func (s *modelUpgradeSuite) TestUpgradeModelTargetAgentVersionStreamToMachineBaseValidation(c *tc.C) {
	defer s.setupMocks(c).Finish()

	currentTargetVersion := s.getVersionMinorLess()
	desiredVersion := semversion.MustParse("4.0.1")
	s.controllerState.EXPECT().GetControllerAgentVersionsByArchitecture(
		gomock.Any(),
		[]agentbinary.Architecture{agentbinary.AMD64},
	).Return([]semversion.Number{desiredVersion}, nil)
	s.modelState.EXPECT().GetModelTargetAgentVersion(gomock.Any()).Return(currentTargetVersion, nil)
	s.agentBinaryFinder.EXPECT().HasBinariesForVersion(desiredVersion).Return(true, nil)
	s.modelState.EXPECT().IsControllerModel(gomock.Any()).Return(false, nil)
	s.modelState.EXPECT().GetMachineCountNotUsingBase(gomock.Any(), gomock.Any()).Return(1, nil)

	svc := NewService(s.agentBinaryFinder, s.modelState, s.controllerState)
	err := svc.UpgradeModelTargetAgentVersionStreamTo(
		c.Context(), desiredVersion, modelagent.AgentStreamReleased,
	)
	_, isBlockedErr := errors.AsType[modelagenterrors.ModelUpgradeBlocker](err)
	c.Check(isBlockedErr, tc.IsTrue)
}

// TestUpgradeModelTargetAgentVersionTo is a happy path test for upgrading a
// model to a specific target agent version.
func (s *modelUpgradeSuite) TestUpgradeModelTargetAgentVersionStreamTo(c *tc.C) {
	defer s.setupMocks(c).Finish()

	currentTargetVersion := s.getVersionMinorLess()
	desiredVersion := semversion.MustParse("4.0.1")
	s.controllerState.EXPECT().GetControllerAgentVersionsByArchitecture(
		gomock.Any(),
		[]agentbinary.Architecture{agentbinary.AMD64},
	).Return([]semversion.Number{desiredVersion}, nil)
	s.modelState.EXPECT().GetModelTargetAgentVersion(gomock.Any()).Return(currentTargetVersion, nil)
	s.agentBinaryFinder.EXPECT().HasBinariesForVersion(desiredVersion).Return(true, nil)
	s.modelState.EXPECT().IsControllerModel(gomock.Any()).Return(false, nil)
	s.modelState.EXPECT().GetMachineCountNotUsingBase(gomock.Any(), gomock.Any()).Return(0, nil)
	s.modelState.EXPECT().SetModelTargetAgentVersionAndStream(
		gomock.Any(),
		currentTargetVersion,
		desiredVersion,
		modelagent.AgentStreamProposed,
	).Return(nil)

	svc := NewService(s.agentBinaryFinder, s.modelState, s.controllerState)
	err := svc.UpgradeModelTargetAgentVersionStreamTo(
		c.Context(), desiredVersion, modelagent.AgentStreamProposed,
	)
	c.Check(err, tc.ErrorIsNil)
}
