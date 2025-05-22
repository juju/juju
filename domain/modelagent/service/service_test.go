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
	applicationerrors "github.com/juju/juju/domain/application/errors"
	machineerrors "github.com/juju/juju/domain/machine/errors"
	"github.com/juju/juju/domain/modelagent"
	modelagenterrors "github.com/juju/juju/domain/modelagent/errors"
	"github.com/juju/juju/internal/uuid"
)

type suite struct {
	state *MockState
}

func TestSuite(t *testing.T) {
	tc.Run(t, &suite{})
}

func (s *suite) setupMocks(c *tc.C) *gomock.Controller {
	ctrl := gomock.NewController(c)
	s.state = NewMockState(ctrl)
	return ctrl
}

// TestGetModelAgentVersionSuccess tests the happy path for
// Service.GetModelAgentVersion.
func (s *suite) TestGetModelAgentVersionSuccess(c *tc.C) {
	defer s.setupMocks(c).Finish()

	expectedVersion, err := semversion.Parse("4.21.65")
	c.Assert(err, tc.ErrorIsNil)
	s.state.EXPECT().GetModelTargetAgentVersion(gomock.Any()).Return(expectedVersion, nil)

	svc := NewService(s.state)
	ver, err := svc.GetModelTargetAgentVersion(c.Context())
	c.Check(err, tc.ErrorIsNil)
	c.Check(ver, tc.DeepEquals, expectedVersion)
}

// TestGetModelAgentVersionNotFound tests that Service.GetModelAgentVersion
// returns an appropriate error when the agent version cannot be found.
func (s *suite) TestGetModelAgentVersionModelNotFound(c *tc.C) {
	defer s.setupMocks(c).Finish()

	s.state.EXPECT().GetModelTargetAgentVersion(gomock.Any()).Return(semversion.Zero, modelagenterrors.AgentVersionNotFound)

	svc := NewService(s.state)
	_, err := svc.GetModelTargetAgentVersion(c.Context())
	c.Check(err, tc.ErrorIs, modelagenterrors.AgentVersionNotFound)
}

// TestGetMachineTargetAgentVersion is asserting the happy path for getting
// a machine's target agent version.
func (s *suite) TestGetMachineTargetAgentVersion(c *tc.C) {
	defer s.setupMocks(c).Finish()

	machineName := coremachine.Name("0")
	uuid := uuid.MustNewUUID().String()
	ver := coreagentbinary.Version{
		Number: semversion.MustParse("4.0.0"),
		Arch:   "amd64",
	}

	s.state.EXPECT().GetMachineUUIDByName(gomock.Any(), machineName).Return(uuid, nil)
	s.state.EXPECT().GetMachineTargetAgentVersion(gomock.Any(), uuid).Return(ver, nil)

	rval, err := NewService(s.state).GetMachineTargetAgentVersion(c.Context(), machineName)
	c.Check(err, tc.ErrorIsNil)
	c.Check(rval, tc.Equals, ver)
}

// TestGetMachineTargetAgentVersionNotFound is testing that the service
// returns a [machineerrors.MachineNotFound] error when no machine exists for
// a given name.
func (s *suite) TestGetMachineTargetAgentVersionNotFound(c *tc.C) {
	defer s.setupMocks(c).Finish()

	s.state.EXPECT().GetMachineUUIDByName(gomock.Any(), coremachine.Name("0")).Return(
		"", machineerrors.MachineNotFound,
	)

	_, err := NewService(s.state).GetMachineTargetAgentVersion(
		c.Context(),
		coremachine.Name("0"),
	)
	c.Check(err, tc.ErrorIs, machineerrors.MachineNotFound)
}

// TestGetUnitTargetAgentVersion is asserting the happy path for getting
// a unit's target agent version.
func (s *suite) TestGetUnitTargetAgentVersion(c *tc.C) {
	defer s.setupMocks(c).Finish()

	ver := coreagentbinary.Version{
		Number: semversion.MustParse("4.0.0"),
		Arch:   "amd64",
	}

	uuid := unittesting.GenUnitUUID(c)
	s.state.EXPECT().GetUnitUUIDByName(gomock.Any(), coreunit.Name("foo/0")).Return(uuid, nil)
	s.state.EXPECT().GetUnitTargetAgentVersion(gomock.Any(), uuid).Return(ver, nil)

	rval, err := NewService(s.state).GetUnitTargetAgentVersion(c.Context(), "foo/0")
	c.Check(err, tc.ErrorIsNil)
	c.Check(rval, tc.Equals, ver)
}

// TestGetUnitTargetAgentVersionNotFound is testing that the service
// returns a [applicationerrors.UnitNotFound] error when no unit exists for
// a given name.
func (s *suite) TestGetUnitTargetAgentVersionNotFound(c *tc.C) {
	defer s.setupMocks(c).Finish()

	s.state.EXPECT().GetUnitUUIDByName(gomock.Any(), coreunit.Name("foo/0")).Return(
		"", applicationerrors.UnitNotFound,
	)

	_, err := NewService(s.state).GetUnitTargetAgentVersion(
		c.Context(),
		"foo/0",
	)
	c.Check(err, tc.ErrorIs, applicationerrors.UnitNotFound)
}

// TestWatchUnitTargetAgentVersionNotFound is testing that the service
// returns a [applicationerrors.UnitNotFound] error when no unit exists for
// a given name.
func (s *suite) TestWatchUnitTargetAgentVersionNotFound(c *tc.C) {
	defer s.setupMocks(c).Finish()

	s.state.EXPECT().GetUnitUUIDByName(gomock.Any(), coreunit.Name("foo/0")).Return(
		"", applicationerrors.UnitNotFound,
	)

	_, err := NewWatchableService(s.state, nil).WatchUnitTargetAgentVersion(
		c.Context(),
		"foo/0",
	)
	c.Check(err, tc.ErrorIs, applicationerrors.UnitNotFound)
}

// TestWatchMachineTargetAgentVersionNotFound is testing that the service
// returns a [machineerrors.MachineNotFound] error when no machine exists for
// a given name.
func (s *suite) TestWatchMachineTargetAgentVersionNotFound(c *tc.C) {
	defer s.setupMocks(c).Finish()

	s.state.EXPECT().GetMachineUUIDByName(gomock.Any(), coremachine.Name("0")).Return(
		"", machineerrors.MachineNotFound,
	)

	_, err := NewWatchableService(s.state, nil).WatchMachineTargetAgentVersion(c.Context(), "0")
	c.Check(err, tc.ErrorIs, machineerrors.MachineNotFound)
}

// TestSetMachineReportedAgentVersionInvalid is here to assert that if pass a
// junk agent binary version to [Service.SetMachineReportedAgentVersion] we get
// back an error that satisfies [coreerrors.NotValid].
func (s *suite) TestSetMachineReportedAgentVersionInvalid(c *tc.C) {
	defer s.setupMocks(c).Finish()

	err := NewService(s.state).SetMachineReportedAgentVersion(
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
// state for producing this error we need to simulate this in two different
// locations to assert the full functionality.
func (s *suite) TestSetMachineReportedAgentVersionNotFound(c *tc.C) {
	defer s.setupMocks(c).Finish()

	// MachineNotFound error location 1.
	s.state.EXPECT().GetMachineUUIDByName(gomock.Any(), coremachine.Name("0")).Return(
		"", machineerrors.MachineNotFound,
	)

	err := NewService(s.state).SetMachineReportedAgentVersion(
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

	s.state.EXPECT().GetMachineUUIDByName(gomock.Any(), coremachine.Name("0")).Return(
		machineUUID.String(), nil,
	)

	s.state.EXPECT().SetMachineRunningAgentBinaryVersion(
		gomock.Any(),
		machineUUID.String(),
		coreagentbinary.Version{
			Number: semversion.MustParse("1.2.3"),
			Arch:   corearch.ARM64,
		},
	).Return(machineerrors.MachineNotFound)

	err = NewService(s.state).SetMachineReportedAgentVersion(
		c.Context(),
		coremachine.Name("0"),
		coreagentbinary.Version{
			Number: semversion.MustParse("1.2.3"),
			Arch:   corearch.ARM64,
		},
	)
	c.Check(err, tc.ErrorIs, machineerrors.MachineNotFound)
}

func (s *suite) TestSetMachineReportedAgentVersionDead(c *tc.C) {
	defer s.setupMocks(c).Finish()

	machineUUID, err := uuid.NewUUID()
	c.Assert(err, tc.ErrorIsNil)

	s.state.EXPECT().GetMachineUUIDByName(gomock.Any(), coremachine.Name("0")).Return(
		machineUUID.String(), nil,
	)

	s.state.EXPECT().SetMachineRunningAgentBinaryVersion(
		gomock.Any(),
		machineUUID.String(),
		coreagentbinary.Version{
			Number: semversion.MustParse("1.2.3"),
			Arch:   corearch.ARM64,
		},
	).Return(machineerrors.MachineIsDead)

	err = NewService(s.state).SetMachineReportedAgentVersion(
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
func (s *suite) TestSetMachineReportedAgentVersion(c *tc.C) {
	defer s.setupMocks(c).Finish()

	machineUUID, err := uuid.NewUUID()
	c.Assert(err, tc.ErrorIsNil)

	s.state.EXPECT().GetMachineUUIDByName(gomock.Any(), coremachine.Name("0")).Return(
		machineUUID.String(), nil,
	)
	s.state.EXPECT().SetMachineRunningAgentBinaryVersion(
		gomock.Any(),
		machineUUID.String(),
		coreagentbinary.Version{
			Number: semversion.MustParse("1.2.3"),
			Arch:   corearch.ARM64,
		},
	).Return(nil)

	err = NewService(s.state).SetMachineReportedAgentVersion(
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
func (s *suite) TestSetReportedUnitAgentVersionInvalid(c *tc.C) {
	defer s.setupMocks(c).Finish()

	err := NewService(s.state).SetUnitReportedAgentVersion(
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
// state for producing this error we need to simulate this in two different
// locations to assert the full functionality.
func (s *suite) TestSetReportedUnitAgentVersionNotFound(c *tc.C) {
	defer s.setupMocks(c).Finish()

	// UnitNotFound error location 1.
	s.state.EXPECT().GetUnitUUIDByName(gomock.Any(), coreunit.Name("foo/666")).Return(
		"", applicationerrors.UnitNotFound,
	)

	err := NewService(s.state).SetUnitReportedAgentVersion(
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

	s.state.EXPECT().GetUnitUUIDByName(gomock.Any(), coreunit.Name("foo/666")).Return(
		unitUUID, nil,
	)

	s.state.EXPECT().SetUnitRunningAgentBinaryVersion(
		gomock.Any(),
		unitUUID,
		coreagentbinary.Version{
			Number: semversion.MustParse("1.2.3"),
			Arch:   corearch.ARM64,
		},
	).Return(applicationerrors.UnitNotFound)

	err = NewService(s.state).SetUnitReportedAgentVersion(
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
func (s *suite) TestSetReportedUnitAgentVersionDead(c *tc.C) {
	defer s.setupMocks(c).Finish()

	unitUUID := unittesting.GenUnitUUID(c)

	s.state.EXPECT().GetUnitUUIDByName(gomock.Any(), coreunit.Name("foo/666")).Return(
		coreunit.UUID(unitUUID.String()), nil,
	)

	s.state.EXPECT().SetUnitRunningAgentBinaryVersion(
		gomock.Any(),
		coreunit.UUID(unitUUID.String()),
		coreagentbinary.Version{
			Number: semversion.MustParse("1.2.3"),
			Arch:   corearch.ARM64,
		},
	).Return(applicationerrors.UnitIsDead)

	err := NewService(s.state).SetUnitReportedAgentVersion(
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
func (s *suite) TestSetReportedUnitAgentVersion(c *tc.C) {
	defer s.setupMocks(c).Finish()

	unitUUID := unittesting.GenUnitUUID(c)

	s.state.EXPECT().GetUnitUUIDByName(gomock.Any(), coreunit.Name("foo/666")).Return(
		coreunit.UUID(unitUUID.String()), nil,
	)

	s.state.EXPECT().SetUnitRunningAgentBinaryVersion(
		gomock.Any(),
		coreunit.UUID(unitUUID.String()),
		coreagentbinary.Version{
			Number: semversion.MustParse("1.2.3"),
			Arch:   corearch.ARM64,
		},
	).Return(nil)

	err := NewService(s.state).SetUnitReportedAgentVersion(
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
func (s *suite) TestGetMachineReportedAgentVersionMachineNotFound(c *tc.C) {
	defer s.setupMocks(c).Finish()

	machineName := coremachine.Name("0")

	// First test of MachineNotFound when translating from name to uuid.
	s.state.EXPECT().GetMachineUUIDByName(gomock.Any(), machineName).Return(
		"", machineerrors.MachineNotFound)

	svc := NewService(s.state)
	_, err := svc.GetMachineReportedAgentVersion(c.Context(), machineName)
	c.Check(err, tc.ErrorIs, machineerrors.MachineNotFound)

	// Section test of MachineNotFound when using the uuid to fetch the running
	// version.
	uuid := uuid.MustNewUUID().String()
	s.state.EXPECT().GetMachineUUIDByName(gomock.Any(), machineName).Return(uuid, nil)
	s.state.EXPECT().GetMachineRunningAgentBinaryVersion(gomock.Any(), uuid).Return(
		coreagentbinary.Version{}, machineerrors.MachineNotFound,
	)

	_, err = svc.GetMachineReportedAgentVersion(c.Context(), machineName)
	c.Check(err, tc.ErrorIs, machineerrors.MachineNotFound)
}

// TestGetMachineReportedAgentVersionAgentVersionNotFound asserts that if we ask
// for the reported agent version of a machine and one has not been set that an
// error statisfying [modelagenterrors.AgentVersionNotFound].
func (s *suite) TestGetMachineReportedAgentVersionAgentVersionNotFound(c *tc.C) {
	defer s.setupMocks(c).Finish()

	machineName := coremachine.Name("0")

	uuid := uuid.MustNewUUID().String()
	s.state.EXPECT().GetMachineUUIDByName(gomock.Any(), machineName).Return(uuid, nil)
	s.state.EXPECT().GetMachineRunningAgentBinaryVersion(gomock.Any(), uuid).Return(
		coreagentbinary.Version{}, modelagenterrors.AgentVersionNotFound,
	)

	svc := NewService(s.state)
	_, err := svc.GetMachineReportedAgentVersion(c.Context(), machineName)
	c.Check(err, tc.ErrorIs, modelagenterrors.AgentVersionNotFound)
}

// TestGetMachineReportedAgentVersion is a happy path test of
// [Service.GetMachineReportedAgentVersion].
func (s *suite) TestGetMachineReportedAgentVersion(c *tc.C) {
	defer s.setupMocks(c).Finish()

	machineName := coremachine.Name("0")

	uuid := uuid.MustNewUUID().String()
	s.state.EXPECT().GetMachineUUIDByName(gomock.Any(), machineName).Return(uuid, nil)
	s.state.EXPECT().GetMachineRunningAgentBinaryVersion(gomock.Any(), uuid).Return(
		coreagentbinary.Version{
			Number: semversion.MustParse("4.1.1"),
			Arch:   corearch.ARM64,
		}, nil,
	)

	svc := NewService(s.state)
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
func (s *suite) TestGetUnitReportedAgentVersionUnitNotFound(c *tc.C) {
	defer s.setupMocks(c).Finish()

	unitName := unittesting.GenNewName(c, "foo/0")

	// First test of UnitNotFound when translating from name to uuid.
	s.state.EXPECT().GetUnitUUIDByName(gomock.Any(), unitName).Return(
		"", applicationerrors.UnitNotFound)

	svc := NewService(s.state)
	_, err := svc.GetUnitReportedAgentVersion(c.Context(), unitName)
	c.Check(err, tc.ErrorIs, applicationerrors.UnitNotFound)

	// Section test of UnitNotFound when using the uuid to fetch the running
	// version.
	uuid := unittesting.GenUnitUUID(c)
	s.state.EXPECT().GetUnitUUIDByName(gomock.Any(), unitName).Return(uuid, nil)
	s.state.EXPECT().GetUnitRunningAgentBinaryVersion(gomock.Any(), uuid).Return(
		coreagentbinary.Version{}, applicationerrors.UnitNotFound,
	)

	_, err = svc.GetUnitReportedAgentVersion(c.Context(), unitName)
	c.Check(err, tc.ErrorIs, applicationerrors.UnitNotFound)
}

// TestGetUnitReportedAgentVersionAgentVersionNotFound asserts that if we ask
// for the reported agent version of a unit and one has not been set that an
// error statisfying [modelagenterrors.AgentVersionNotFound].
func (s *suite) TestGetUnitReportedAgentVersionAgentVersionNotFound(c *tc.C) {
	defer s.setupMocks(c).Finish()

	unitName := unittesting.GenNewName(c, "foo/0")
	unitUUID := unittesting.GenUnitUUID(c)

	s.state.EXPECT().GetUnitUUIDByName(gomock.Any(), unitName).Return(unitUUID, nil)
	s.state.EXPECT().GetUnitRunningAgentBinaryVersion(gomock.Any(), unitUUID).Return(
		coreagentbinary.Version{}, modelagenterrors.AgentVersionNotFound,
	)

	svc := NewService(s.state)
	_, err := svc.GetUnitReportedAgentVersion(c.Context(), unitName)
	c.Check(err, tc.ErrorIs, modelagenterrors.AgentVersionNotFound)
}

// TestGetUnitReportedAgentVersion is a happy path test of
// [Service.GetMachineReportedAgentVersion].
func (s *suite) TestGetUnitReportedAgentVersion(c *tc.C) {
	defer s.setupMocks(c).Finish()

	unitName := unittesting.GenNewName(c, "foo/0")
	unitUUID := unittesting.GenUnitUUID(c)

	s.state.EXPECT().GetUnitUUIDByName(gomock.Any(), unitName).Return(unitUUID, nil)
	s.state.EXPECT().GetUnitRunningAgentBinaryVersion(gomock.Any(), unitUUID).Return(
		coreagentbinary.Version{
			Number: semversion.MustParse("4.1.1"),
			Arch:   corearch.ARM64,
		}, nil,
	)

	svc := NewService(s.state)
	ver, err := svc.GetUnitReportedAgentVersion(c.Context(), unitName)
	c.Check(err, tc.ErrorIsNil)
	c.Check(ver, tc.DeepEquals, coreagentbinary.Version{
		Number: semversion.MustParse("4.1.1"),
		Arch:   corearch.ARM64,
	})
}

// TestGetMachinesReportedAgentVersionAgentVersionNotSet asserts error
// pass through on state of modelagenterrors.AgentVersionNotSet to
// satisfy contract.
func (s *suite) TestGetMachinesReportedAgentVersionAgentVersionNotSet(c *tc.C) {
	defer s.setupMocks(c).Finish()
	s.state.EXPECT().GetMachinesAgentBinaryMetadata(gomock.Any()).Return(
		nil, modelagenterrors.AgentVersionNotSet,
	)
	svc := NewService(s.state)
	_, err := svc.GetMachinesAgentBinaryMetadata(c.Context())
	c.Check(err, tc.ErrorIs, modelagenterrors.AgentVersionNotSet)
}

// TestGetMachinesReportedAgentVersionMissingAgentBinaries asserts error pass
// through on state of modelagenterrors.MissingAgentBinaries to satisfy
// contract.
func (s *suite) TestGetMachinesReportedAgentVersionMissingAgentBinaries(c *tc.C) {
	defer s.setupMocks(c).Finish()
	s.state.EXPECT().GetMachinesAgentBinaryMetadata(gomock.Any()).Return(
		nil, modelagenterrors.MissingAgentBinaries,
	)
	svc := NewService(s.state)
	_, err := svc.GetMachinesAgentBinaryMetadata(c.Context())
	c.Check(err, tc.ErrorIs, modelagenterrors.MissingAgentBinaries)
}

// TestGetUnitReportedAgentVersionAgentVersionNotSet asserts error pass
// through on state of modelagenterrors.AgentVersionNotSet to satisfy
// contract.
func (s *suite) TestGetUnitReportedAgentVersionAgentVersionNotSet(c *tc.C) {
	defer s.setupMocks(c).Finish()
	s.state.EXPECT().GetUnitsAgentBinaryMetadata(gomock.Any()).Return(
		nil, modelagenterrors.AgentVersionNotSet,
	)
	svc := NewService(s.state)
	_, err := svc.GetUnitsAgentBinaryMetadata(c.Context())
	c.Check(err, tc.ErrorIs, modelagenterrors.AgentVersionNotSet)
}

// TestGetUnitReportedAgentVersionMissingAgentBinaries asserts error pass
// through on state of modelagenterrors.MissingAgentBinaries to satisfy
// contract.
func (s *suite) TestGetUnitReportedAgentVersionMissingAgentBinaries(c *tc.C) {
	defer s.setupMocks(c).Finish()
	s.state.EXPECT().GetUnitsAgentBinaryMetadata(gomock.Any()).Return(
		nil, modelagenterrors.MissingAgentBinaries,
	)
	svc := NewService(s.state)
	_, err := svc.GetUnitsAgentBinaryMetadata(c.Context())
	c.Check(err, tc.ErrorIs, modelagenterrors.MissingAgentBinaries)
}

// TestSetAgentStreamNotValidAgentStream is testing that if we supply an
// unknown agent stream to [Service.SetModelAgentStream] we get back an error
// satisfying [coreerrors.NotValid].
func (s *suite) TestSetAgentStreamNotValidAgentStream(c *tc.C) {
	defer s.setupMocks(c).Finish()

	// This is a fake stream that doesn't exist.
	agentStream := coreagentbinary.AgentStream("bad value")

	err := NewService(s.state).SetModelAgentStream(
		c.Context(),
		agentStream,
	)
	c.Check(err, tc.ErrorIs, coreerrors.NotValid)
}

// TestSetAgentStream is testing the happy path of setting the model's agent
// stream.
func (s *suite) TestSetAgentStream(c *tc.C) {
	defer s.setupMocks(c).Finish()

	s.state.EXPECT().SetModelAgentStream(
		gomock.Any(),
		modelagent.AgentStreamTesting,
	).Return(nil)

	err := NewService(s.state).SetModelAgentStream(
		c.Context(),
		coreagentbinary.AgentStreamTesting,
	)
	c.Check(err, tc.ErrorIsNil)
}
