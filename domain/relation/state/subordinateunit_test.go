// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state

import (
	"context"
	"database/sql"
	"testing"

	"github.com/canonical/gomock/gomock"
	"github.com/canonical/sqlair"
	"github.com/juju/clock"
	"github.com/juju/tc"

	coreapplication "github.com/juju/juju/core/application"
	corecharm "github.com/juju/juju/core/charm"
	"github.com/juju/juju/core/life"
	coremachine "github.com/juju/juju/core/machine"
	corerelation "github.com/juju/juju/core/relation"
	corestatus "github.com/juju/juju/core/status"
	coreunit "github.com/juju/juju/core/unit"
	coreunittesting "github.com/juju/juju/core/unit/testing"
	domainapplication "github.com/juju/juju/domain/application"
	applicationerrors "github.com/juju/juju/domain/application/errors"
	"github.com/juju/juju/domain/deployment"
	"github.com/juju/juju/domain/deployment/charm"
	"github.com/juju/juju/domain/network"
	"github.com/juju/juju/domain/relation/errors"
	"github.com/juju/juju/domain/relation/internal"
	"github.com/juju/juju/domain/status"
	loggertesting "github.com/juju/juju/internal/logger/testing"
)

type subordinateUnitSuite struct {
	baseRelationSuite

	insertIAASUnitState *MockInsertIAASUnitState
}

func (s *subordinateUnitSuite) SetUpTest(c *tc.C) {
	s.ModelSuite.SetUpTest(c)
}

func (s *subordinateUnitSuite) setupMocks(c *tc.C) *gomock.Controller {
	ctrl := gomock.NewController(c)

	s.insertIAASUnitState = NewMockInsertIAASUnitState(ctrl)
	s.state = NewState(s.TxnRunnerFactory(), clock.WallClock, loggertesting.WrapCheckLog(c), s.insertIAASUnitState)

	c.Cleanup(func() {
		s.insertIAASUnitState = nil
		s.state = nil
	})

	return ctrl
}

func TestSubordinateUnitSuite(t *testing.T) {
	tc.Run(t, &subordinateUnitSuite{})
}

func (s *subordinateUnitSuite) TestAddSubordinateUnit(c *tc.C) {
	c.Skip("skipping till dependency problem from application domain fixed")
	defer s.setupMocks(c).Finish()

	// Arrange: add principal application with 1 unit on a machine
	principalCharmUUID := s.addCharm(c)
	s.addCharmMetadata(c, principalCharmUUID, false)
	principalApplicationUUID := s.addApplication(c, principalCharmUUID, "pri")
	principalUnitName := coreunittesting.GenNewName(c, "pri/0")
	principalUnitUUID := s.addUnit(c, principalUnitName, principalApplicationUUID, principalCharmUUID)
	principalMachineName, principalMachineUUID := s.addMachineToUnit(c, principalUnitUUID.String())
	principalUnitNetNode := s.getUnitNetNode(c, principalUnitUUID.String())

	// Arrange: add a subordinate application with 1 unit
	subordinateCharmUUID := s.addCharm(c)
	s.addCharmMetadata(c, subordinateCharmUUID, true)
	subordinateApplicationUUID := s.addApplication(c, subordinateCharmUUID, "sub")
	subordinateUnitName := coreunittesting.GenNewName(c, "sub/0")
	subordinateUnitUUID := s.addUnit(c, subordinateUnitName, subordinateApplicationUUID, subordinateCharmUUID)

	// Arrange: relate the principal and subordinate applications
	relationUUID, principalRelationEndpointUUID, _ := s.addContainerScopedRelation(c, principalApplicationUUID, principalCharmUUID, subordinateApplicationUUID, subordinateCharmUUID)
	relationUnitUUID := s.addRelationUnit(c, principalUnitUUID, principalRelationEndpointUUID)

	// Arrange: expect the call to InsertIAASUnit
	args := domainapplication.AddIAASUnitArg{
		MachineNetNodeUUID: principalUnitNetNode,
		MachineUUID:        principalMachineUUID,
		AddUnitArg: domainapplication.AddUnitArg{
			UnitStatusArg: domainapplication.UnitStatusArg{
				AgentStatus: &status.StatusInfo[status.UnitAgentStatusType]{
					Status: status.UnitAgentStatusAllocating,
				},
				WorkloadStatus: &status.StatusInfo[status.WorkloadStatusType]{
					Status:  status.WorkloadStatusWaiting,
					Message: corestatus.MessageWaitForMachine,
				},
			},
			Placement: deployment.Placement{
				Type:      deployment.PlacementTypeMachine,
				Directive: principalMachineName.String(),
			},
			NetNodeUUID: principalUnitNetNode,
		},
	}
	s.insertIAASUnitState.EXPECT().InsertIAASUnit(
		gomock.Any(), gomock.Any(), subordinateApplicationUUID.String(), subordinateCharmUUID.String(), addIAASUnitArgMatcher{
			c:        c,
			expected: args,
		}).Return(
		subordinateUnitName, []coremachine.Name{principalMachineName}, nil)

	// Act
	var (
		err          error
		obtainedData internal.SubordinateUnitStatusHistoryData
	)
	err = s.TxnRunner().Txn(c.Context(), func(ctx context.Context, tx *sqlair.TX) error {
		obtainedData, err = s.state.addSubordinateUnit(ctx, tx, relationUUID.String(), relationUnitUUID.String(), principalUnitUUID.String())
		return err
	})

	// Assert
	c.Assert(err, tc.ErrorIsNil)
	mc := tc.NewMultiChecker()
	mc.AddExpr("_.UnitStatus.AgentStatus.Since", tc.Ignore)
	mc.AddExpr("_.UnitStatus.WorkloadStatus.Since", tc.Ignore)
	c.Check(obtainedData, mc, internal.SubordinateUnitStatusHistoryData{
		UnitName:   subordinateUnitName.String(),
		UnitStatus: args.UnitStatusArg,
	})
	s.checkUnitPrincipal(c, principalUnitUUID.String(), subordinateUnitUUID.String())
}

func (s *subordinateUnitSuite) TestAddSubordinateUnitNotContainerScope(c *tc.C) {
	defer s.setupMocks(c).Finish()

	// Arrange: add principal application with 1 unit
	principalCharmUUID := s.addCharm(c)
	s.addCharmMetadata(c, principalCharmUUID, false)
	principalApplicationUUID := s.addApplication(c, principalCharmUUID, "pri")
	principalUnitName := coreunittesting.GenNewName(c, "pri/0")
	principalUnitUUID := s.addUnit(c, principalUnitName, principalApplicationUUID, principalCharmUUID)

	// Arrange: add a subordinate application
	subordinateCharmUUID := s.addCharm(c)
	s.addCharmMetadata(c, subordinateCharmUUID, true)
	subordinateApplicationUUID := s.addApplication(c, subordinateCharmUUID, "sub")

	// Arrange: relate the principal and subordinate applications
	relationUUID, principalRelationEndpointUUID, _ := s.addGlobalScopedRelation(c, principalApplicationUUID, principalCharmUUID, subordinateApplicationUUID, subordinateCharmUUID)
	relationUnitUUID := s.addRelationUnit(c, principalUnitUUID, principalRelationEndpointUUID)

	// Act
	var (
		err          error
		obtainedData internal.SubordinateUnitStatusHistoryData
	)
	err = s.TxnRunner().Txn(c.Context(), func(ctx context.Context, tx *sqlair.TX) error {
		obtainedData, err = s.state.addSubordinateUnit(ctx, tx, relationUUID.String(), relationUnitUUID.String(), principalUnitUUID.String())
		return err
	})

	// Assert
	c.Assert(err, tc.ErrorIsNil)
	c.Check(obtainedData.SubordinateCreated(), tc.Equals, false)
}

func (s *subordinateUnitSuite) TestAddSubordinateUnitAlreadyRelated(c *tc.C) {
	defer s.setupMocks(c).Finish()

	// Arrange: add an application with 1 unit
	principalCharmUUID := s.addCharm(c)
	s.addCharmMetadata(c, principalCharmUUID, false)
	principalApplicationUUID := s.addApplication(c, principalCharmUUID, "pri")
	principalUnitName := coreunittesting.GenNewName(c, "pri/0")
	principalUnitUUID := s.addUnit(c, principalUnitName, principalApplicationUUID, principalCharmUUID)

	// Arrange: add a second application
	subordinateCharmUUID := s.addCharm(c)
	s.addCharmMetadata(c, subordinateCharmUUID, true)
	subordinateApplicationUUID := s.addApplication(c, subordinateCharmUUID, "sub")
	subordinateUnitName := coreunittesting.GenNewName(c, "sub/0")
	subordinateUnitUUID := s.addUnit(c, subordinateUnitName, subordinateApplicationUUID, subordinateCharmUUID)

	// Arrange: relate the principal and subordinate applications
	relationUUID, principalRelationEndpointUUID, _ := s.addContainerScopedRelation(c, principalApplicationUUID, principalCharmUUID, subordinateApplicationUUID, subordinateCharmUUID)
	relationUnitUUID := s.addRelationUnit(c, principalUnitUUID, principalRelationEndpointUUID)

	var (
		err          error
		obtainedData internal.SubordinateUnitStatusHistoryData
	)
	err = s.TxnRunner().Txn(c.Context(), func(ctx context.Context, tx *sqlair.TX) error {
		// Arrange: ensure the unit_principle row exists, as if the
		// subordinate was already created.
		err = s.state.recordUnitPrincipal(ctx, tx, principalUnitUUID.String(), subordinateUnitUUID.String())
		if err != nil {
			return err
		}

		// Act
		obtainedData, err = s.state.addSubordinateUnit(ctx, tx, relationUUID.String(), relationUnitUUID.String(), principalUnitUUID.String())
		return err
	})

	// Assert
	c.Assert(err, tc.ErrorIsNil)
	c.Check(obtainedData.SubordinateCreated(), tc.Equals, false)
}

// TestAddSubordinateUnitEnteringUnitIsSubordinate ensures that when a
// subordinate unit enters scope in a container scoped relation with another
// subordinate application, the created unit is keyed to the principal of the
// entering unit, and not to the entering subordinate unit itself. Otherwise
// the relation-joined hook of the newly created unit spawns a new unit of
// the other application, and so on without ever terminating.
// See https://github.com/juju/juju/issues/23049.
func (s *subordinateUnitSuite) TestAddSubordinateUnitEnteringUnitIsSubordinate(c *tc.C) {
	defer s.setupMocks(c).Finish()

	// Arrange: add principal application with 1 unit on a machine.
	principalCharmUUID := s.addCharm(c)
	s.addCharmMetadata(c, principalCharmUUID, false)
	principalApplicationUUID := s.addApplication(c, principalCharmUUID, "pri")
	principalUnitName := coreunittesting.GenNewName(c, "pri/0")
	principalUnitUUID := s.addUnit(c, principalUnitName, principalApplicationUUID, principalCharmUUID)
	principalMachineName, principalMachineUUID := s.addMachineToUnit(c, principalUnitUUID.String())
	principalUnitNetNode := s.getUnitNetNode(c, principalUnitUUID.String())

	// Arrange: add a first subordinate application, with a unit already
	// attached to the principal unit, as if it was created by its own
	// container scoped relation with the principal application.
	subordinate1CharmUUID := s.addCharm(c)
	s.addCharmMetadata(c, subordinate1CharmUUID, true)
	subordinate1ApplicationUUID := s.addApplication(c, subordinate1CharmUUID, "sub1")
	subordinate1UnitName := coreunittesting.GenNewName(c, "sub1/0")
	subordinate1UnitUUID := s.addUnit(c, subordinate1UnitName, subordinate1ApplicationUUID, subordinate1CharmUUID)
	// The subordinate unit lives on the machine of the principal unit, so it
	// shares its net node.
	s.setUnitNetNode(c, subordinate1UnitUUID.String(), principalUnitNetNode)
	s.query(c, `
INSERT INTO unit_principal (unit_uuid, principal_uuid)
VALUES (?, ?)
`, subordinate1UnitUUID, principalUnitUUID)

	// Arrange: add a second subordinate application, with no unit yet.
	subordinate2CharmUUID := s.addCharm(c)
	s.addCharmMetadata(c, subordinate2CharmUUID, true)
	subordinate2ApplicationUUID := s.addApplication(c, subordinate2CharmUUID, "sub2")
	subordinate2UnitName := coreunittesting.GenNewName(c, "sub2/0")

	// Arrange: relate the two subordinate applications in a container
	// scoped relation, and make the first subordinate unit enter scope.
	relationUUID, subordinate1RelationEndpointUUID, _ := s.addContainerScopedRelation(
		c, subordinate1ApplicationUUID, subordinate1CharmUUID,
		subordinate2ApplicationUUID, subordinate2CharmUUID)
	relationUnitUUID := s.addRelationUnit(c, subordinate1UnitUUID, subordinate1RelationEndpointUUID)

	// Arrange: expect the call to InsertIAASUnit, placed on the machine of
	// the entering unit's principal unit.
	args := domainapplication.AddIAASUnitArg{
		MachineNetNodeUUID: principalUnitNetNode,
		MachineUUID:        principalMachineUUID,
		AddUnitArg: domainapplication.AddUnitArg{
			UnitStatusArg: domainapplication.UnitStatusArg{
				AgentStatus: &status.StatusInfo[status.UnitAgentStatusType]{
					Status: status.UnitAgentStatusAllocating,
				},
				WorkloadStatus: &status.StatusInfo[status.WorkloadStatusType]{
					Status:  status.WorkloadStatusWaiting,
					Message: corestatus.MessageWaitForMachine,
				},
			},
			Placement: deployment.Placement{
				Type:      deployment.PlacementTypeMachine,
				Directive: principalMachineName.String(),
			},
			NetNodeUUID: principalUnitNetNode,
		},
	}
	s.insertIAASUnitState.EXPECT().InsertIAASUnit(
		gomock.Any(), gomock.Any(), subordinate2ApplicationUUID.String(), subordinate2CharmUUID.String(), addIAASUnitArgMatcher{
			c:        c,
			expected: args,
		}).DoAndReturn(
		func(ctx context.Context, tx *sqlair.TX, appUUID, charmUUID string, arg domainapplication.AddIAASUnitArg) (coreunit.Name, []coremachine.Name, error) {
			// Mirror the real state behavior: the unit row is inserted with
			// the uuid and net node passed in by the caller, so that the
			// principal-subordinate relationship can be recorded.
			if err := s.insertUnitInTx(ctx, tx, arg.UnitUUID.String(), subordinate2UnitName.String(), appUUID, charmUUID, arg.NetNodeUUID); err != nil {
				return "", nil, err
			}
			return subordinate2UnitName, []coremachine.Name{principalMachineName}, nil
		})

	// Act
	var (
		err          error
		obtainedData internal.SubordinateUnitStatusHistoryData
	)
	err = s.TxnRunner().Txn(c.Context(), func(ctx context.Context, tx *sqlair.TX) error {
		obtainedData, err = s.state.addSubordinateUnit(ctx, tx, relationUUID.String(), relationUnitUUID.String(), subordinate1UnitUUID.String())
		return err
	})

	// Assert
	c.Assert(err, tc.ErrorIsNil)
	c.Check(obtainedData.SubordinateCreated(), tc.IsTrue)
	// The new unit must be keyed to the principal of the entering unit, on
	// top of the existing first subordinate unit.
	s.checkUnitPrincipalCount(c, principalUnitUUID.String(), 2)
	// ... and not to the entering subordinate unit.
	s.checkUnitPrincipalCount(c, subordinate1UnitUUID.String(), 0)
}

// TestAddSubordinateUnitAlreadyExistsUnderEnteringUnitPrincipal ensures that
// no subordinate unit is created when a unit of the related subordinate
// application already exists under the principal of the unit entering scope,
// even when the entering unit is itself a subordinate.
// See https://github.com/juju/juju/issues/23049.
func (s *subordinateUnitSuite) TestAddSubordinateUnitAlreadyExistsUnderEnteringUnitPrincipal(c *tc.C) {
	defer s.setupMocks(c).Finish()

	// Arrange: add principal application with 1 unit on a machine.
	principalCharmUUID := s.addCharm(c)
	s.addCharmMetadata(c, principalCharmUUID, false)
	principalApplicationUUID := s.addApplication(c, principalCharmUUID, "pri")
	principalUnitName := coreunittesting.GenNewName(c, "pri/0")
	principalUnitUUID := s.addUnit(c, principalUnitName, principalApplicationUUID, principalCharmUUID)
	principalUnitNetNode := s.getUnitNetNode(c, principalUnitUUID.String())
	s.addMachineToUnit(c, principalUnitUUID.String())

	// Arrange: add a first subordinate application, with a unit already
	// attached to the principal unit.
	subordinate1CharmUUID := s.addCharm(c)
	s.addCharmMetadata(c, subordinate1CharmUUID, true)
	subordinate1ApplicationUUID := s.addApplication(c, subordinate1CharmUUID, "sub1")
	subordinate1UnitName := coreunittesting.GenNewName(c, "sub1/0")
	subordinate1UnitUUID := s.addUnit(c, subordinate1UnitName, subordinate1ApplicationUUID, subordinate1CharmUUID)
	// The subordinate unit lives on the machine of the principal unit, so it
	// shares its net node.
	s.setUnitNetNode(c, subordinate1UnitUUID.String(), principalUnitNetNode)
	s.query(c, `
INSERT INTO unit_principal (unit_uuid, principal_uuid)
VALUES (?, ?)
`, subordinate1UnitUUID, principalUnitUUID)

	// Arrange: add a second subordinate application, with a unit already
	// attached to the principal unit as well.
	subordinate2CharmUUID := s.addCharm(c)
	s.addCharmMetadata(c, subordinate2CharmUUID, true)
	subordinate2ApplicationUUID := s.addApplication(c, subordinate2CharmUUID, "sub2")
	subordinate2UnitName := coreunittesting.GenNewName(c, "sub2/0")
	subordinate2UnitUUID := s.addUnit(c, subordinate2UnitName, subordinate2ApplicationUUID, subordinate2CharmUUID)
	s.query(c, `
INSERT INTO unit_principal (unit_uuid, principal_uuid)
VALUES (?, ?)
`, subordinate2UnitUUID, principalUnitUUID)

	// Arrange: relate the two subordinate applications in a container
	// scoped relation, and make the first subordinate unit enter scope.
	relationUUID, subordinate1RelationEndpointUUID, _ := s.addContainerScopedRelation(
		c, subordinate1ApplicationUUID, subordinate1CharmUUID,
		subordinate2ApplicationUUID, subordinate2CharmUUID)
	relationUnitUUID := s.addRelationUnit(c, subordinate1UnitUUID, subordinate1RelationEndpointUUID)

	// No call to InsertIAASUnit is expected.

	// Act
	var (
		err          error
		obtainedData internal.SubordinateUnitStatusHistoryData
	)
	err = s.TxnRunner().Txn(c.Context(), func(ctx context.Context, tx *sqlair.TX) error {
		obtainedData, err = s.state.addSubordinateUnit(ctx, tx, relationUUID.String(), relationUnitUUID.String(), subordinate1UnitUUID.String())
		return err
	})

	// Assert
	c.Assert(err, tc.ErrorIsNil)
	c.Check(obtainedData.SubordinateCreated(), tc.IsFalse)
}

func (s *subordinateUnitSuite) TestAddSubordinateUnitSubordinateNotAlive(c *tc.C) {
	defer s.setupMocks(c).Finish()

	// Arrange: add an application with 1 unit
	principalCharmUUID := s.addCharm(c)
	s.addCharmMetadata(c, principalCharmUUID, false)
	principalApplicationUUID := s.addApplication(c, principalCharmUUID, "pri")
	principalUnitName := coreunittesting.GenNewName(c, "pri/0")
	principalUnitUUID := s.addUnit(c, principalUnitName, principalApplicationUUID, principalCharmUUID)

	// Arrange: add a second application
	subordinateCharmUUID := s.addCharm(c)
	s.addCharmMetadata(c, subordinateCharmUUID, true)
	subordinateApplicationUUID := s.addApplication(c, subordinateCharmUUID, "sub")
	subordinateUnitName := coreunittesting.GenNewName(c, "sub/0")
	subordinateUnitUUID := s.addUnitWithLife(c, subordinateUnitName, subordinateApplicationUUID, subordinateCharmUUID, life.Dying)

	// Arrange: relate the principal and subordinate applications
	relationUUID, principalRelationEndpointUUID, _ := s.addContainerScopedRelation(c, principalApplicationUUID, principalCharmUUID, subordinateApplicationUUID, subordinateCharmUUID)
	relationUnitUUID := s.addRelationUnit(c, principalUnitUUID, principalRelationEndpointUUID)

	var (
		err          error
		obtainedData internal.SubordinateUnitStatusHistoryData
	)
	err = s.TxnRunner().Txn(c.Context(), func(ctx context.Context, tx *sqlair.TX) error {
		// Arrange: ensure the unit_principle row exists, as if the
		// subordinate was already created.
		err = s.state.recordUnitPrincipal(ctx, tx, principalUnitUUID.String(), subordinateUnitUUID.String())
		if err != nil {
			return err
		}

		// Act
		obtainedData, err = s.state.addSubordinateUnit(ctx, tx, relationUUID.String(), relationUnitUUID.String(), principalUnitUUID.String())
		return err
	})

	// Assert
	c.Assert(err, tc.ErrorIs, errors.CannotEnterScopeSubordinateNotAlive)
	c.Check(obtainedData.SubordinateCreated(), tc.Equals, false)
}

// TestGetUnitMachineUUIDandNetNodeUnitNotFound wants to see that when a caller
// calls [State.getUnitMachineIdentifier] with a unit uuid that does not
// exist in the model the caller gets back an error satisfying
// [applicationerrors.UnitNotFound].
func (s *subordinateUnitSuite) TestGetUnitMachineIdentifiersSubDoesNotExist(c *tc.C) {
	s.state = NewState(s.TxnRunnerFactory(), clock.WallClock, loggertesting.WrapCheckLog(c), nil)
	c.Cleanup(func() {
		s.state = nil
	})

	unitUUID := tc.Must(c, coreunit.NewUUID).String()

	err := s.TxnRunner().Txn(c.Context(), func(ctx context.Context, tx *sqlair.TX) error {
		_, err := s.state.getUnitMachineIdentifier(
			ctx, tx, unitUUID,
		)
		return err
	})
	c.Check(err, tc.ErrorIs, applicationerrors.UnitMachineNotAssigned)
}

// TestGetUnitMachineUUIDandNetNodeUnit is a happy path test for
// [State.getUnitMachineIdentifier].
func (s *subordinateUnitSuite) TestGetUnitMachineIdentifiers(c *tc.C) {
	s.state = NewState(s.TxnRunnerFactory(), clock.WallClock, loggertesting.WrapCheckLog(c), nil)
	c.Cleanup(func() {
		s.state = nil
	})

	// Arrange
	charmUUID := s.addCharm(c)
	s.addCharmMetadata(c, charmUUID, false)
	principalApplicationUUID := s.addApplication(c, charmUUID, "app1")

	unitName := coreunittesting.GenNewName(c, "app1/0")
	principalUnitUUID := s.addUnit(c, unitName, principalApplicationUUID, charmUUID).String()

	machineName, machineUUID := s.addMachineToUnit(c, principalUnitUUID)
	netNodeUUID := s.getUnitNetNode(c, principalUnitUUID).String()

	// Act
	var receivedIdentifier machineIdentifier
	err := s.TxnRunner().Txn(c.Context(), func(ctx context.Context, tx *sqlair.TX) error {
		var err error
		receivedIdentifier, err = s.state.getUnitMachineIdentifier(
			ctx, tx, principalUnitUUID,
		)
		return err
	})

	// Assert
	c.Check(err, tc.ErrorIsNil)
	c.Check(receivedIdentifier, tc.Equals, machineIdentifier{
		Name:        machineName.String(),
		NetNodeUUID: netNodeUUID,
		UUID:        machineUUID.String(),
	})
}

func (s *subordinateUnitSuite) addMachineToUnit(c *tc.C, unitUUID string) (coremachine.Name, coremachine.UUID) {
	machineUUID := tc.Must(c, coremachine.NewUUID).String()
	machineName := "0"
	s.query(c, `
INSERT INTO machine (uuid, name, life_id, net_node_uuid)
SELECT ?, ?, ?, net_node_uuid
FROM unit
WHERE uuid = ?
`, machineUUID, machineName, 0 /* alive */, unitUUID)
	return coremachine.Name(machineName), coremachine.UUID(machineUUID)
}

func (s *subordinateUnitSuite) addContainerScopedRelation(
	c *tc.C,
	app1ID coreapplication.UUID,
	charm1UUID corecharm.ID,
	app2ID coreapplication.UUID,
	charm2UUID corecharm.ID,
) (corerelation.UUID, string, string) {
	// Arrange: Add two endpoints
	endpoint1 := charm.Relation{
		Name:      "fake-endpoint-name-1",
		Role:      charm.RoleProvider,
		Interface: "database",
		Scope:     charm.ScopeContainer,
	}
	endpoint2 := charm.Relation{
		Name:      "fake-endpoint-name-2",
		Role:      charm.RoleRequirer,
		Interface: "database",
		Scope:     charm.ScopeContainer,
	}
	charmRelationUUID1 := s.addCharmRelation(c, charm1UUID, endpoint1)
	charmRelationUUID2 := s.addCharmRelation(c, charm2UUID, endpoint2)
	applicationEndpointUUID1 := s.addApplicationEndpoint(c, app1ID, charmRelationUUID1)
	applicationEndpointUUID2 := s.addApplicationEndpoint(c, app2ID, charmRelationUUID2)
	relationUUID := s.addRelationWithScope(c, charm.ScopeContainer)
	relationEndpointUUID1 := s.addRelationEndpoint(c, relationUUID, applicationEndpointUUID1)
	relationEndpointUUID2 := s.addRelationEndpoint(c, relationUUID, applicationEndpointUUID2)

	return relationUUID, relationEndpointUUID1, relationEndpointUUID2
}

func (s *subordinateUnitSuite) addGlobalScopedRelation(
	c *tc.C,
	app1ID coreapplication.UUID,
	charm1UUID corecharm.ID,
	app2ID coreapplication.UUID,
	charm2UUID corecharm.ID) (corerelation.UUID, string, string) {
	// Arrange: Add two endpoints
	endpoint1 := charm.Relation{
		Name:      "fake-endpoint-name-1",
		Role:      charm.RoleProvider,
		Interface: "database",
		Scope:     charm.ScopeGlobal,
	}
	endpoint2 := charm.Relation{
		Name:      "fake-endpoint-name-2",
		Role:      charm.RoleRequirer,
		Interface: "database",
		Scope:     charm.ScopeGlobal,
	}
	charmRelationUUID1 := s.addCharmRelation(c, charm1UUID, endpoint1)
	charmRelationUUID2 := s.addCharmRelation(c, charm2UUID, endpoint2)
	applicationEndpointUUID1 := s.addApplicationEndpoint(c, app1ID, charmRelationUUID1)
	applicationEndpointUUID2 := s.addApplicationEndpoint(c, app2ID, charmRelationUUID2)
	relationUUID := s.addRelationWithScope(c, charm.ScopeGlobal)
	relationEndpointUUID1 := s.addRelationEndpoint(c, relationUUID, applicationEndpointUUID1)
	relationEndpointUUID2 := s.addRelationEndpoint(c, relationUUID, applicationEndpointUUID2)

	return relationUUID, relationEndpointUUID1, relationEndpointUUID2
}

func (s *subordinateUnitSuite) getUnitNetNode(c *tc.C, unitUUID string) network.NetNodeUUID {
	var netNodeUUID string
	err := s.TxnRunner().StdTxn(c.Context(), func(ctx context.Context, tx *sql.Tx) error {
		row := tx.QueryRowContext(
			ctx, "SELECT net_node_uuid FROM unit WHERE uuid = ?", unitUUID)
		if row.Err() != nil {
			return row.Err()
		}

		if err := row.Scan(&netNodeUUID); err != nil {
			return err
		}

		return nil
	})
	c.Assert(err, tc.ErrorIsNil)
	return network.NetNodeUUID(netNodeUUID)
}

func (s *subordinateUnitSuite) checkUnitPrincipal(c *tc.C, principal, subordinate string) {
	qry := `SELECT count(*) FROM unit_principal WHERE principal_uuid = ? AND unit_uuid = ?`
	row := s.DB().QueryRow(qry, principal, subordinate)
	var count int
	err := row.Scan(&count)
	c.Check(err, tc.ErrorIsNil)
	c.Check(count, tc.Equals, 1, tc.Commentf("q: %s", qry))
}

// checkUnitPrincipalCount checks the number of units keyed to the given
// principal unit in the unit_principal table.
func (s *subordinateUnitSuite) checkUnitPrincipalCount(c *tc.C, principal string, expected int) {
	qry := `SELECT count(*) FROM unit_principal WHERE principal_uuid = ?`
	row := s.DB().QueryRow(qry, principal)
	var count int
	err := row.Scan(&count)
	c.Assert(err, tc.ErrorIsNil)
	c.Check(count, tc.Equals, expected, tc.Commentf("q: %s", qry))
}

// setUnitNetNode updates the net node of a unit.
func (s *subordinateUnitSuite) setUnitNetNode(c *tc.C, unitUUID string, netNodeUUID network.NetNodeUUID) {
	s.query(c, `
UPDATE unit SET net_node_uuid = ? WHERE uuid = ?
`, netNodeUUID, unitUUID)
}

// insertUnitInTx inserts a unit row within the given transaction, mirroring
// what the real InsertIAASUnit state does. The net node must already exist.
func (s *subordinateUnitSuite) insertUnitInTx(
	ctx context.Context, tx *sqlair.TX,
	unitUUID, unitName, appUUID, charmUUID string,
	netNodeUUID network.NetNodeUUID,
) error {
	type unitRow struct {
		UnitUUID        string `db:"unit_uuid"`
		Name            string `db:"name"`
		ApplicationUUID string `db:"application_uuid"`
		CharmUUID       string `db:"charm_uuid"`
		NetNodeUUID     string `db:"net_node_uuid"`
	}
	arg := unitRow{
		UnitUUID:        unitUUID,
		Name:            unitName,
		ApplicationUUID: appUUID,
		CharmUUID:       charmUUID,
		NetNodeUUID:     netNodeUUID.String(),
	}
	stmt, err := s.state.Prepare(`
INSERT INTO unit (uuid, name, life_id, application_uuid, charm_uuid, net_node_uuid)
VALUES ($unitRow.unit_uuid, $unitRow.name, 0, $unitRow.application_uuid, $unitRow.charm_uuid, $unitRow.net_node_uuid)
`, arg)
	if err != nil {
		return err
	}
	return tx.Query(ctx, stmt, arg).Run()
}

type addIAASUnitArgMatcher struct {
	c        *tc.C
	expected domainapplication.AddIAASUnitArg
}

func (m addIAASUnitArgMatcher) Matches(x any) bool {
	obtained, ok := x.(domainapplication.AddIAASUnitArg)
	if !ok {
		return false
	}
	mc := tc.NewMultiChecker()
	mc.AddExpr("_.AddUnitArg.UnitStatusArg.AgentStatus.Since", tc.Ignore)
	mc.AddExpr("_.AddUnitArg.UnitStatusArg.WorkloadStatus.Since", tc.Ignore)
	mc.AddExpr("_.AddUnitArg.UnitUUID", tc.NotNil)
	m.c.Check(obtained, mc, m.expected)
	return true
}

func (addIAASUnitArgMatcher) String() string {
	return "matches AddIAASUnitArg, modulo status since value"
}
