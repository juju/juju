// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state

import (
	"testing"

	"github.com/juju/tc"

	coremachine "github.com/juju/juju/core/machine"
	coreunit "github.com/juju/juju/core/unit"
	applicationerrors "github.com/juju/juju/domain/application/errors"
	machineerrors "github.com/juju/juju/domain/machine/errors"
	internaluuid "github.com/juju/juju/internal/uuid"
)

type stateSuite struct {
	baseSuite
}

func TestStateSuite(t *testing.T) {
	tc.Run(t, &stateSuite{})
}

func (s *stateSuite) SetUpTest(c *tc.C) {
	s.baseSuite.SetUpTest(c)
}

func (s *stateSuite) TestGetUnitUUIDByName(c *tc.C) {
	// Arrange
	unitUUID := s.addUnit(c)

	// Act
	result, err := s.state.GetUnitUUIDByName(c.Context(), coreunit.Name("test-app/0"))

	// Assert
	c.Assert(err, tc.ErrorIsNil)
	c.Check(result, tc.Equals, unitUUID.String())
}

func (s *stateSuite) TestGetUnitUUIDByNameNotFound(c *tc.C) {
	// Act
	_, err := s.state.GetUnitUUIDByName(c.Context(), coreunit.Name("non-existent/0"))

	// Assert
	c.Assert(err, tc.ErrorMatches, `getting unit UUID for "non-existent/0": unit "non-existent/0" not found`)
	c.Check(err, tc.ErrorIs, applicationerrors.UnitNotFound)
}

func (s *stateSuite) TestGetMachineUUIDByName(c *tc.C) {
	// Arrange
	machineUUID := s.addMachine(c)

	// Act
	result, err := s.state.GetMachineUUIDByName(c.Context(), coremachine.Name("0"))

	// Assert
	c.Assert(err, tc.ErrorIsNil)
	c.Check(result, tc.Equals, machineUUID.String())
}

func (s *stateSuite) TestGetMachineUUIDByNameNotFound(c *tc.C) {
	// Act
	_, err := s.state.GetMachineUUIDByName(c.Context(), coremachine.Name("999"))

	// Assert
	c.Assert(err, tc.ErrorMatches, `getting machine UUID for "999": machine "999" not found`)
	c.Check(err, tc.ErrorIs, machineerrors.MachineNotFound)
}

func (s *stateSuite) TestFilterTaskUUIDsForUnit(c *tc.C) {
	// Arrange
	operationUUID := s.addOperation(c)
	unitUUID := s.addUnit(c)

	// Add tasks with different statuses
	taskUUID1 := s.addOperationTaskWithID(c, operationUUID, "task-1", "1") // running
	taskUUID2 := s.addOperationTaskWithID(c, operationUUID, "task-2", "2") // pending
	taskUUID3 := s.addOperationTaskWithID(c, operationUUID, "task-3", "5") // completed

	// Link tasks to unit
	s.addOperationUnitTask(c, taskUUID1, unitUUID)
	s.addOperationUnitTask(c, taskUUID2, unitUUID)
	s.addOperationUnitTask(c, taskUUID3, unitUUID)

	taskUUIDs := []string{taskUUID1.String(), taskUUID2.String(), taskUUID3.String()}

	// Act
	result, err := s.state.FilterTaskUUIDsForUnit(c.Context(), taskUUIDs, unitUUID.String())

	// Assert
	c.Assert(err, tc.ErrorIsNil)
	// Should only return non-pending tasks (task-1 and task-3)
	c.Check(len(result), tc.Equals, 2)
	// pending task should not be included
	c.Check(result, tc.SameContents, []string{"task-1", "task-3"})
}

func (s *stateSuite) TestFilterTaskUUIDsForUnitEmptyList(c *tc.C) {
	// Arrange
	unitUUID := s.addUnit(c)

	// Act
	result, err := s.state.FilterTaskUUIDsForUnit(c.Context(), []string{}, unitUUID.String())

	// Assert
	c.Assert(err, tc.ErrorIsNil)
	c.Check(len(result), tc.Equals, 0)
}

func (s *stateSuite) TestFilterTaskUUIDsForUnitNoMatchingTasks(c *tc.C) {
	// Arrange
	operationUUID := s.addOperation(c)
	unitUUID := s.addUnit(c)

	// Add task but don't link to unit
	taskUUID := s.addOperationTaskWithID(c, operationUUID, "task-1", "1")

	taskUUIDs := []string{taskUUID.String()}

	// Act
	result, err := s.state.FilterTaskUUIDsForUnit(c.Context(), taskUUIDs, unitUUID.String())

	// Assert
	c.Assert(err, tc.ErrorIsNil)
	c.Check(len(result), tc.Equals, 0)
}

func (s *stateSuite) TestFilterTaskUUIDsForUnitNonExistentUUIDs(c *tc.C) {
	// Arrange
	unitUUID := s.addUnit(c)
	nonExistentUUIDs := []string{internaluuid.MustNewUUID().String(), internaluuid.MustNewUUID().String()}

	// Act
	result, err := s.state.FilterTaskUUIDsForUnit(c.Context(), nonExistentUUIDs, unitUUID.String())

	// Assert
	c.Assert(err, tc.ErrorIsNil)
	c.Check(len(result), tc.Equals, 0)
}

func (s *stateSuite) TestFilterTaskUUIDsForMachine(c *tc.C) {
	// Arrange
	operationUUID := s.addOperation(c)
	machineUUID := s.addMachine(c)

	// Add tasks (machine tasks include pending ones)
	taskUUID1 := s.addOperationTaskWithID(c, operationUUID, "task-1", "1") // running
	taskUUID2 := s.addOperationTaskWithID(c, operationUUID, "task-2", "2") // pending
	taskUUID3 := s.addOperationTaskWithID(c, operationUUID, "task-3", "5") // completed

	// Link tasks to machine
	s.addOperationMachineTask(c, taskUUID1, machineUUID)
	s.addOperationMachineTask(c, taskUUID2, machineUUID)
	s.addOperationMachineTask(c, taskUUID3, machineUUID)

	taskUUIDs := []string{taskUUID1.String(), taskUUID2.String(), taskUUID3.String()}

	// Act
	result, err := s.state.FilterTaskUUIDsForMachine(c.Context(), taskUUIDs, machineUUID.String())

	// Assert
	c.Assert(err, tc.ErrorIsNil)
	// Should return all tasks including pending ones
	c.Check(len(result), tc.Equals, 3)
	// pending task should be included for machines
	c.Check(result, tc.SameContents, []string{"task-1", "task-2", "task-3"})
}

func (s *stateSuite) TestFilterTaskUUIDsForMachineEmptyList(c *tc.C) {
	// Arrange
	machineUUID := s.addMachine(c)

	// Act
	result, err := s.state.FilterTaskUUIDsForMachine(c.Context(), []string{}, machineUUID.String())

	// Assert
	c.Assert(err, tc.ErrorIsNil)
	c.Check(len(result), tc.Equals, 0)
}

func (s *stateSuite) TestFilterTaskUUIDsForMachineNoMatchingTasks(c *tc.C) {
	// Arrange
	operationUUID := s.addOperation(c)
	machineUUID := s.addMachine(c)

	// Add task but don't link to machine
	taskUUID := s.addOperationTaskWithID(c, operationUUID, "task-1", "1")

	taskUUIDs := []string{taskUUID.String()}

	// Act
	result, err := s.state.FilterTaskUUIDsForMachine(c.Context(), taskUUIDs, machineUUID.String())

	// Assert
	c.Assert(err, tc.ErrorIsNil)
	c.Check(len(result), tc.Equals, 0)
}

func (s *stateSuite) TestFilterTaskUUIDsForMachineNonExistentUUIDs(c *tc.C) {
	// Arrange
	machineUUID := s.addMachine(c)
	nonExistentUUIDs := []string{internaluuid.MustNewUUID().String(), internaluuid.MustNewUUID().String()}

	// Act
	result, err := s.state.FilterTaskUUIDsForMachine(c.Context(), nonExistentUUIDs, machineUUID.String())

	// Assert
	c.Assert(err, tc.ErrorIsNil)
	c.Check(len(result), tc.Equals, 0)
}
