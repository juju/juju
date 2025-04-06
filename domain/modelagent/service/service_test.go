// Copyright 2024 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package service

import (
	"context"

	jc "github.com/juju/testing/checkers"
	"go.uber.org/mock/gomock"
	gc "gopkg.in/check.v1"

	coreagentbinary "github.com/juju/juju/core/agentbinary"
	corearch "github.com/juju/juju/core/arch"
	coreerrors "github.com/juju/juju/core/errors"
	coremachine "github.com/juju/juju/core/machine"
	"github.com/juju/juju/core/semversion"
	coreunit "github.com/juju/juju/core/unit"
	unittesting "github.com/juju/juju/core/unit/testing"
	applicationerrors "github.com/juju/juju/domain/application/errors"
	machineerrors "github.com/juju/juju/domain/machine/errors"
	modelagenterrors "github.com/juju/juju/domain/modelagent/errors"
	"github.com/juju/juju/internal/uuid"
)

type suite struct {
	state *MockState
}

var _ = gc.Suite(&suite{})

func (s *suite) setupMocks(c *gc.C) *gomock.Controller {
	ctrl := gomock.NewController(c)
	s.state = NewMockState(ctrl)
	return ctrl
}

// TestGetModelAgentVersionSuccess tests the happy path for
// Service.GetModelAgentVersion.
func (s *suite) TestGetModelAgentVersionSuccess(c *gc.C) {
	defer s.setupMocks(c).Finish()

	expectedVersion, err := semversion.Parse("4.21.65")
	c.Assert(err, jc.ErrorIsNil)
	s.state.EXPECT().GetModelTargetAgentVersion(gomock.Any()).Return(expectedVersion, nil)

	svc := NewService(s.state)
	ver, err := svc.GetModelTargetAgentVersion(context.Background())
	c.Check(err, jc.ErrorIsNil)
	c.Check(ver, jc.DeepEquals, expectedVersion)
}

// TestGetModelAgentVersionNotFound tests that Service.GetModelAgentVersion
// returns an appropriate error when the agent version cannot be found.
func (s *suite) TestGetModelAgentVersionModelNotFound(c *gc.C) {
	defer s.setupMocks(c).Finish()

	s.state.EXPECT().GetModelTargetAgentVersion(gomock.Any()).Return(semversion.Zero, modelagenterrors.AgentVersionNotFound)

	svc := NewService(s.state)
	_, err := svc.GetModelTargetAgentVersion(context.Background())
	c.Check(err, jc.ErrorIs, modelagenterrors.AgentVersionNotFound)
}

// TestGetMachineTargetAgentVersion is asserting the happy path for getting
// a machine's target agent version.
func (s *suite) TestGetMachineTargetAgentVersion(c *gc.C) {
	defer s.setupMocks(c).Finish()

	machineName := coremachine.Name("0")
	uuid := uuid.MustNewUUID().String()
	ver := coreagentbinary.Version{
		Number: semversion.MustParse("4.0.0"),
		Arch:   "amd64",
	}

	s.state.EXPECT().GetMachineUUIDByName(gomock.Any(), machineName).Return(uuid, nil)
	s.state.EXPECT().GetMachineTargetAgentVersion(gomock.Any(), uuid).Return(ver, nil)

	rval, err := NewService(s.state).GetMachineTargetAgentVersion(context.Background(), machineName)
	c.Check(err, jc.ErrorIsNil)
	c.Check(rval, gc.Equals, ver)
}

// TestGetMachineTargetAgentVersionNotFound is testing that the service
// returns a [machineerrors.MachineNotFound] error when no machine exists for
// a given name.
func (s *suite) TestGetMachineTargetAgentVersionNotFound(c *gc.C) {
	defer s.setupMocks(c).Finish()

	s.state.EXPECT().GetMachineUUIDByName(gomock.Any(), coremachine.Name("0")).Return(
		"", machineerrors.MachineNotFound,
	)

	_, err := NewService(s.state).GetMachineTargetAgentVersion(
		context.Background(),
		coremachine.Name("0"),
	)
	c.Check(err, jc.ErrorIs, machineerrors.MachineNotFound)
}

// TestGetUnitTargetAgentVersion is asserting the happy path for getting
// a unit's target agent version.
func (s *suite) TestGetUnitTargetAgentVersion(c *gc.C) {
	defer s.setupMocks(c).Finish()

	ver := coreagentbinary.Version{
		Number: semversion.MustParse("4.0.0"),
		Arch:   "amd64",
	}

	uuid := unittesting.GenUnitUUID(c)
	s.state.EXPECT().GetUnitUUIDByName(gomock.Any(), coreunit.Name("foo/0")).Return(uuid, nil)
	s.state.EXPECT().GetUnitTargetAgentVersion(gomock.Any(), uuid).Return(ver, nil)

	rval, err := NewService(s.state).GetUnitTargetAgentVersion(context.Background(), "foo/0")
	c.Check(err, jc.ErrorIsNil)
	c.Check(rval, gc.Equals, ver)
}

// TestGetUnitTargetAgentVersionNotFound is testing that the service
// returns a [applicationerrors.UnitNotFound] error when no unit exists for
// a given name.
func (s *suite) TestGetUnitTargetAgentVersionNotFound(c *gc.C) {
	defer s.setupMocks(c).Finish()

	s.state.EXPECT().GetUnitUUIDByName(gomock.Any(), coreunit.Name("foo/0")).Return(
		"", applicationerrors.UnitNotFound,
	)

	_, err := NewService(s.state).GetUnitTargetAgentVersion(
		context.Background(),
		"foo/0",
	)
	c.Check(err, jc.ErrorIs, applicationerrors.UnitNotFound)
}

// TestWatchUnitTargetAgentVersionNotFound is testing that the service
// returns a [applicationerrors.UnitNotFound] error when no unit exists for
// a given name.
func (s *suite) TestWatchUnitTargetAgentVersionNotFound(c *gc.C) {
	defer s.setupMocks(c).Finish()

	s.state.EXPECT().GetUnitUUIDByName(gomock.Any(), coreunit.Name("foo/0")).Return(
		"", applicationerrors.UnitNotFound,
	)

	_, err := NewWatchableService(s.state, nil).WatchUnitTargetAgentVersion(
		context.Background(),
		"foo/0",
	)
	c.Check(err, jc.ErrorIs, applicationerrors.UnitNotFound)
}

// TestWatchMachineTargetAgentVersionNotFound is testing that the service
// returns a [machineerrors.MachineNotFound] error when no machine exists for
// a given name.
func (s *suite) TestWatchMachineTargetAgentVersionNotFound(c *gc.C) {
	defer s.setupMocks(c).Finish()

	s.state.EXPECT().GetMachineUUIDByName(gomock.Any(), coremachine.Name("0")).Return(
		"", machineerrors.MachineNotFound,
	)

	_, err := NewWatchableService(s.state, nil).WatchMachineTargetAgentVersion(context.Background(), "0")
	c.Check(err, jc.ErrorIs, machineerrors.MachineNotFound)
}

// TestSetMachineReportedAgentVersionInvalid is here to assert that if pass a
// junk agent binary version to [Service.SetMachineReportedAgentVersion] we get
// back an error that satisfies [coreerrors.NotValid].
func (s *suite) TestSetMachineReportedAgentVersionInvalid(c *gc.C) {
	defer s.setupMocks(c).Finish()

	err := NewService(s.state).SetMachineReportedAgentVersion(
		context.Background(),
		coremachine.Name("0"),
		coreagentbinary.Version{
			Number: semversion.Zero,
		},
	)
	c.Check(err, jc.ErrorIs, coreerrors.NotValid)
}

// TestSetMachineReportedAgentVersionSuccess asserts that if we try to set the
// reported agent version for a machine that doesn't exist we get an error
// satisfying [machineerrors.MachineNotFound]. Because the service relied on
// state for producing this error we need to simulate this in two different
// locations to assert the full functionality.
func (s *suite) TestSetMachineReportedAgentVersionNotFound(c *gc.C) {
	defer s.setupMocks(c).Finish()

	// MachineNotFound error location 1.
	s.state.EXPECT().GetMachineUUIDByName(gomock.Any(), coremachine.Name("0")).Return(
		"", machineerrors.MachineNotFound,
	)

	err := NewService(s.state).SetMachineReportedAgentVersion(
		context.Background(),
		coremachine.Name("0"),
		coreagentbinary.Version{
			Number: semversion.MustParse("1.2.3"),
			Arch:   corearch.ARM64,
		},
	)
	c.Check(err, jc.ErrorIs, machineerrors.MachineNotFound)

	// MachineNotFound error location 2.
	machineUUID, err := uuid.NewUUID()
	c.Assert(err, jc.ErrorIsNil)

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
		context.Background(),
		coremachine.Name("0"),
		coreagentbinary.Version{
			Number: semversion.MustParse("1.2.3"),
			Arch:   corearch.ARM64,
		},
	)
	c.Check(err, jc.ErrorIs, machineerrors.MachineNotFound)
}

func (s *suite) TestSetMachineReportedAgentVersionDead(c *gc.C) {
	defer s.setupMocks(c).Finish()

	machineUUID, err := uuid.NewUUID()
	c.Assert(err, jc.ErrorIsNil)

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
		context.Background(),
		coremachine.Name("0"),
		coreagentbinary.Version{
			Number: semversion.MustParse("1.2.3"),
			Arch:   corearch.ARM64,
		},
	)
	c.Check(err, jc.ErrorIs, machineerrors.MachineIsDead)
}

// TestSetMachineReportedAgentVersion asserts the happy path of
// [Service.SetMachineReportedAgentVersion].
func (s *suite) TestSetMachineReportedAgentVersion(c *gc.C) {
	defer s.setupMocks(c).Finish()

	machineUUID, err := uuid.NewUUID()
	c.Assert(err, jc.ErrorIsNil)

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
		context.Background(),
		coremachine.Name("0"),
		coreagentbinary.Version{
			Number: semversion.MustParse("1.2.3"),
			Arch:   corearch.ARM64,
		},
	)
	c.Check(err, jc.ErrorIsNil)
}

// TestSetReportedUnitAgentVersionInvalid is here to assert that if pass a
// junk agent binary version to [Service.SetReportedUnitAgentVersion] we get
// back an error that satisfies [coreerrors.NotValid].
func (s *suite) TestSetReportedUnitAgentVersionInvalid(c *gc.C) {
	defer s.setupMocks(c).Finish()

	err := NewService(s.state).SetUnitReportedAgentVersion(
		context.Background(),
		coreunit.Name("foo/666"),
		coreagentbinary.Version{
			Number: semversion.Zero,
		},
	)
	c.Check(err, jc.ErrorIs, coreerrors.NotValid)
}

// TestSetReportedUnitAgentVersionNotFound asserts that if we try to set the
// reported agent version for a unit that doesn't exist we get an error
// satisfying [applicationerrors.UnitNotFound]. Because the service relied on
// state for producing this error we need to simulate this in two different
// locations to assert the full functionality.
func (s *suite) TestSetReportedUnitAgentVersionNotFound(c *gc.C) {
	defer s.setupMocks(c).Finish()

	// UnitNotFound error location 1.
	s.state.EXPECT().GetUnitUUIDByName(gomock.Any(), coreunit.Name("foo/666")).Return(
		"", applicationerrors.UnitNotFound,
	)

	err := NewService(s.state).SetUnitReportedAgentVersion(
		context.Background(),
		coreunit.Name("foo/666"),
		coreagentbinary.Version{
			Number: semversion.MustParse("1.2.3"),
			Arch:   corearch.ARM64,
		},
	)
	c.Check(err, jc.ErrorIs, applicationerrors.UnitNotFound)

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
		context.Background(),
		coreunit.Name("foo/666"),
		coreagentbinary.Version{
			Number: semversion.MustParse("1.2.3"),
			Arch:   corearch.ARM64,
		},
	)
	c.Check(err, jc.ErrorIs, applicationerrors.UnitNotFound)
}

// TestSetReportedUnitAgentVersionDead asserts that if we try to set the
// reported agent version for a dead unit we get an error satisfying
// [applicationerrors.UnitIsDead].
func (s *suite) TestSetReportedUnitAgentVersionDead(c *gc.C) {
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
		context.Background(),
		coreunit.Name("foo/666"),
		coreagentbinary.Version{
			Number: semversion.MustParse("1.2.3"),
			Arch:   corearch.ARM64,
		},
	)
	c.Check(err, jc.ErrorIs, applicationerrors.UnitIsDead)
}

// TestSetReportedUnitAgentVersion asserts the happy path of
// [Service.SetReportedUnitAgentVersion].
func (s *suite) TestSetReportedUnitAgentVersion(c *gc.C) {
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
		context.Background(),
		coreunit.Name("foo/666"),
		coreagentbinary.Version{
			Number: semversion.MustParse("1.2.3"),
			Arch:   corearch.ARM64,
		},
	)
	c.Check(err, jc.ErrorIsNil)
}

// TestGetMachineReportedAgentVersionMachineNotFound asserts that if we ask for
// the reported agent version of a machine and the machine does not exist we get
// back an error that satisfies [machineerrors.MachineNotFound].
func (s *suite) TestGetMachineReportedAgentVersionMachineNotFound(c *gc.C) {
	defer s.setupMocks(c).Finish()

	machineName := coremachine.Name("0")

	// First test of MachineNotFound when translating from name to uuid.
	s.state.EXPECT().GetMachineUUIDByName(gomock.Any(), machineName).Return(
		"", machineerrors.MachineNotFound)

	svc := NewService(s.state)
	_, err := svc.GetMachineReportedAgentVersion(context.Background(), machineName)
	c.Check(err, jc.ErrorIs, machineerrors.MachineNotFound)

	// Section test of MachineNotFound when using the uuid to fetch the running
	// version.
	uuid := uuid.MustNewUUID().String()
	s.state.EXPECT().GetMachineUUIDByName(gomock.Any(), machineName).Return(uuid, nil)
	s.state.EXPECT().GetMachineRunningAgentBinaryVersion(gomock.Any(), uuid).Return(
		coreagentbinary.Version{}, machineerrors.MachineNotFound,
	)

	_, err = svc.GetMachineReportedAgentVersion(context.Background(), machineName)
	c.Check(err, jc.ErrorIs, machineerrors.MachineNotFound)
}

// TestGetMachineReportedAgentVersionAgentVersionNotFound asserts that if we ask
// for the reported agent version of a machine and one has not been set that an
// error statisfying [modelagenterrors.AgentVersionNotFound].
func (s *suite) TestGetMachineReportedAgentVersionAgentVersionNotFound(c *gc.C) {
	defer s.setupMocks(c).Finish()

	machineName := coremachine.Name("0")

	uuid := uuid.MustNewUUID().String()
	s.state.EXPECT().GetMachineUUIDByName(gomock.Any(), machineName).Return(uuid, nil)
	s.state.EXPECT().GetMachineRunningAgentBinaryVersion(gomock.Any(), uuid).Return(
		coreagentbinary.Version{}, modelagenterrors.AgentVersionNotFound,
	)

	svc := NewService(s.state)
	_, err := svc.GetMachineReportedAgentVersion(context.Background(), machineName)
	c.Check(err, jc.ErrorIs, modelagenterrors.AgentVersionNotFound)
}

// TestGetMachineReportedAgentVersion is a happy path test of
// [Service.GetMachineReportedAgentVersion].
func (s *suite) TestGetMachineReportedAgentVersion(c *gc.C) {
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
	ver, err := svc.GetMachineReportedAgentVersion(context.Background(), machineName)
	c.Check(err, jc.ErrorIsNil)
	c.Check(ver, jc.DeepEquals, coreagentbinary.Version{
		Number: semversion.MustParse("4.1.1"),
		Arch:   corearch.ARM64,
	})
}

// TestGetUnitReportedAgentVersionUnitNotFound asserts that if we ask for
// the reported agent version of a unit and the unit does not exist we get
// back an error that satisfies [applicationerrors.UnitNotFound].
func (s *suite) TestGetUnitReportedAgentVersionUnitNotFound(c *gc.C) {
	defer s.setupMocks(c).Finish()

	unitName := unittesting.GenNewName(c, "foo/0")

	// First test of UnitNotFound when translating from name to uuid.
	s.state.EXPECT().GetUnitUUIDByName(gomock.Any(), unitName).Return(
		"", applicationerrors.UnitNotFound)

	svc := NewService(s.state)
	_, err := svc.GetUnitReportedAgentVersion(context.Background(), unitName)
	c.Check(err, jc.ErrorIs, applicationerrors.UnitNotFound)

	// Section test of UnitNotFound when using the uuid to fetch the running
	// version.
	uuid := unittesting.GenUnitUUID(c)
	s.state.EXPECT().GetUnitUUIDByName(gomock.Any(), unitName).Return(uuid, nil)
	s.state.EXPECT().GetUnitRunningAgentBinaryVersion(gomock.Any(), uuid).Return(
		coreagentbinary.Version{}, applicationerrors.UnitNotFound,
	)

	_, err = svc.GetUnitReportedAgentVersion(context.Background(), unitName)
	c.Check(err, jc.ErrorIs, applicationerrors.UnitNotFound)
}

// TestGetUnitReportedAgentVersionAgentVersionNotFound asserts that if we ask
// for the reported agent version of a unit and one has not been set that an
// error statisfying [modelagenterrors.AgentVersionNotFound].
func (s *suite) TestGetUnitReportedAgentVersionAgentVersionNotFound(c *gc.C) {
	defer s.setupMocks(c).Finish()

	unitName := unittesting.GenNewName(c, "foo/0")
	unitUUID := unittesting.GenUnitUUID(c)

	s.state.EXPECT().GetUnitUUIDByName(gomock.Any(), unitName).Return(unitUUID, nil)
	s.state.EXPECT().GetUnitRunningAgentBinaryVersion(gomock.Any(), unitUUID).Return(
		coreagentbinary.Version{}, modelagenterrors.AgentVersionNotFound,
	)

	svc := NewService(s.state)
	_, err := svc.GetUnitReportedAgentVersion(context.Background(), unitName)
	c.Check(err, jc.ErrorIs, modelagenterrors.AgentVersionNotFound)
}

// TestGetUnitReportedAgentVersion is a happy path test of
// [Service.GetMachineReportedAgentVersion].
func (s *suite) TestGetUnitReportedAgentVersion(c *gc.C) {
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
	ver, err := svc.GetUnitReportedAgentVersion(context.Background(), unitName)
	c.Check(err, jc.ErrorIsNil)
	c.Check(ver, jc.DeepEquals, coreagentbinary.Version{
		Number: semversion.MustParse("4.1.1"),
		Arch:   corearch.ARM64,
	})
}
