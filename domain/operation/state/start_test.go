// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state

import (
	"context"
	"database/sql"
	"testing"
	"time"

	"github.com/juju/tc"

	"github.com/juju/juju/core/machine"
	corestatus "github.com/juju/juju/core/status"
	"github.com/juju/juju/core/unit"
	"github.com/juju/juju/domain/operation"
	internaluuid "github.com/juju/juju/internal/uuid"
)

type startSuite struct {
	baseSuite
}

func TestStartSuite(t *testing.T) {
	tc.Run(t, &startSuite{})
}

func (s *startSuite) SetUpTest(c *tc.C) {
	s.baseSuite.SetUpTest(c)
}

// addApplication creates a new application and returns its UUID
func (s *startSuite) addApplication(c *tc.C, charmUUID, appName string) string {
	appUUID := internaluuid.MustNewUUID().String()
	s.query(c, `INSERT INTO application (uuid, name, life_id, charm_uuid, space_uuid) VALUES (?, ?, ?, ?, ?)`,
		appUUID, appName, 0, charmUUID, "656b4a82-e28c-53d6-a014-f0dd53417eb6")
	return appUUID
}

// addUnitToApplication creates a unit for an existing application
func (s *startSuite) addUnitToApplication(c *tc.C, charmUUID, appUUID, unitName string) string {
	nodeUUID := internaluuid.MustNewUUID().String()
	unitUUID := internaluuid.MustNewUUID().String()
	s.query(c, `INSERT INTO net_node (uuid) VALUES (?)`, nodeUUID)
	s.query(c, `INSERT INTO unit (uuid, name, life_id, application_uuid, charm_uuid, net_node_uuid) VALUES (?, ?, ?, ?, ?, ?)`,
		unitUUID, unitName, 0, appUUID, charmUUID, nodeUUID)
	return unitUUID
}

func (s *startSuite) TestAddExecOperationWithMachinesOnly(c *tc.C) {
	machineUUID := s.addMachine(c, "0")
	_ = s.addMachine(c, "1")

	operationUUID := internaluuid.MustNewUUID()
	target := operation.ReceiversWithoutLeader{
		Machines: []machine.Name{"0", "1"},
	}
	args := operation.ExecArgs{
		Command:        "echo hello",
		Timeout:        time.Minute,
		Parallel:       true,
		ExecutionGroup: "test-group",
	}

	result, err := s.state.AddExecOperation(c.Context(), operationUUID, target, args)
	c.Assert(err, tc.IsNil)
	c.Check(result.OperationID, tc.Not(tc.Equals), "")
	c.Check(len(result.Machines), tc.Equals, 2)
	c.Check(len(result.Units), tc.Equals, 0)

	// Check first machine result.
	c.Check(result.Machines[0].ReceiverName, tc.Equals, machine.Name("0"))
	c.Check(result.Machines[0].TaskInfo.ID, tc.Not(tc.Equals), "")
	c.Check(result.Machines[0].TaskInfo.Status, tc.Equals, corestatus.Pending)
	c.Check(result.Machines[0].TaskInfo.Error, tc.IsNil)

	// Verify operation was stored in database.
	var opCount int
	err = s.TxnRunner().StdTxn(c.Context(), func(ctx context.Context, tx *sql.Tx) error {
		return tx.QueryRow("SELECT COUNT(*) FROM operation WHERE uuid = ?", operationUUID.String()).Scan(&opCount)
	})
	c.Assert(err, tc.IsNil)
	c.Check(opCount, tc.Equals, 1)

	// Verify machine tasks were created.
	var machineTaskCount int
	err = s.TxnRunner().StdTxn(c.Context(), func(ctx context.Context, tx *sql.Tx) error {
		return tx.QueryRow("SELECT COUNT(*) FROM operation_machine_task WHERE machine_uuid = ?", machineUUID).Scan(&machineTaskCount)
	})
	c.Assert(err, tc.IsNil)
	c.Check(machineTaskCount, tc.Equals, 1)
}

func (s *startSuite) TestAddExecOperationWithUnitsOnly(c *tc.C) {
	unitUUID := s.addUnitWithName(c, s.addCharm(c), "unit-exec/0")

	operationUUID := internaluuid.MustNewUUID()
	target := operation.ReceiversWithoutLeader{
		Units: []unit.Name{"unit-exec/0"},
	}
	args := operation.ExecArgs{
		Command:        "restart service",
		Timeout:        2 * time.Minute,
		Parallel:       false,
		ExecutionGroup: "maintenance",
	}

	result, err := s.state.AddExecOperation(c.Context(), operationUUID, target, args)
	c.Assert(err, tc.IsNil)
	c.Check(result.OperationID, tc.Not(tc.Equals), "")
	c.Check(len(result.Machines), tc.Equals, 0)
	c.Check(len(result.Units), tc.Equals, 1)

	// Check unit result
	c.Check(result.Units[0].ReceiverName, tc.Equals, unit.Name("unit-exec/0"))
	c.Check(result.Units[0].TaskInfo.ID, tc.Not(tc.Equals), "")
	c.Check(result.Units[0].TaskInfo.Status, tc.Equals, corestatus.Pending)
	c.Check(result.Units[0].TaskInfo.Error, tc.IsNil)

	// Verify unit task was created.
	var unitTaskCount int
	err = s.TxnRunner().StdTxn(c.Context(), func(ctx context.Context, tx *sql.Tx) error {
		return tx.QueryRow("SELECT COUNT(*) FROM operation_unit_task WHERE unit_uuid = ?", unitUUID).Scan(&unitTaskCount)
	})
	c.Assert(err, tc.IsNil)
	c.Check(unitTaskCount, tc.Equals, 1)
}

func (s *startSuite) TestAddExecOperationWithApplications(c *tc.C) {
	charmUUID := s.addCharm(c)
	appUUID := s.addApplication(c, charmUUID, "test-exec-apps")
	_ = s.addUnitToApplication(c, charmUUID, appUUID, "test-exec-apps/0")
	_ = s.addUnitToApplication(c, charmUUID, appUUID, "test-exec-apps/1")

	operationUUID := internaluuid.MustNewUUID()
	target := operation.ReceiversWithoutLeader{
		Applications: []string{"test-exec-apps"},
	}
	args := operation.ExecArgs{
		Command:        "app-wide command",
		Parallel:       true,
		ExecutionGroup: "app-ops",
	}

	result, err := s.state.AddExecOperation(c.Context(), operationUUID, target, args)
	c.Assert(err, tc.IsNil)
	c.Check(result.OperationID, tc.Not(tc.Equals), "")
	c.Check(len(result.Units), tc.Equals, 2) // Both units in the app
	c.Check(len(result.Machines), tc.Equals, 0)

	// Verify both units are included.
	unitNames := make(map[string]bool)
	for _, unit := range result.Units {
		unitNames[unit.ReceiverName.String()] = true
		c.Check(unit.TaskInfo.Error, tc.IsNil)
		c.Check(unit.TaskInfo.Status, tc.Equals, corestatus.Pending)
	}
	c.Check(unitNames["test-exec-apps/0"], tc.Equals, true)
	c.Check(unitNames["test-exec-apps/1"], tc.Equals, true)
}

func (s *startSuite) TestAddExecOperationMixedTargets(c *tc.C) {
	_ = s.addMachine(c, "0")
	_ = s.addUnitWithName(c, s.addCharm(c), "mixed-exec/0")

	operationUUID := internaluuid.MustNewUUID()
	target := operation.ReceiversWithoutLeader{
		Machines: []machine.Name{"0"},
		Units:    []unit.Name{"mixed-exec/0"},
	}
	args := operation.ExecArgs{
		Command:        "mixed command",
		Parallel:       true,
		ExecutionGroup: "mixed",
	}

	result, err := s.state.AddExecOperation(c.Context(), operationUUID, target, args)
	c.Assert(err, tc.IsNil)
	c.Check(result.OperationID, tc.Not(tc.Equals), "")
	c.Check(len(result.Machines), tc.Equals, 1)
	c.Check(len(result.Units), tc.Equals, 1)

	c.Check(result.Machines[0].ReceiverName, tc.Equals, machine.Name("0"))
	c.Check(result.Units[0].ReceiverName, tc.Equals, unit.Name("mixed-exec/0"))
}

func (s *startSuite) TestAddExecOperationEmptyTarget(c *tc.C) {
	operationUUID := internaluuid.MustNewUUID()
	target := operation.ReceiversWithoutLeader{}
	args := operation.ExecArgs{
		Command: "empty target command",
	}

	result, err := s.state.AddExecOperation(c.Context(), operationUUID, target, args)
	c.Assert(err, tc.IsNil)
	c.Check(result.OperationID, tc.Not(tc.Equals), "")
	c.Check(len(result.Machines), tc.Equals, 0)
	c.Check(len(result.Units), tc.Equals, 0)

	// Operation should still be created even with no targets.
	var opCount int
	err = s.TxnRunner().StdTxn(c.Context(), func(ctx context.Context, tx *sql.Tx) error {
		return tx.QueryRow("SELECT COUNT(*) FROM operation WHERE uuid = ?", operationUUID.String()).Scan(&opCount)
	})
	c.Assert(err, tc.IsNil)
	c.Check(opCount, tc.Equals, 1)
}

func (s *startSuite) TestAddExecOperationMachineNotFound(c *tc.C) {
	operationUUID := internaluuid.MustNewUUID()
	target := operation.ReceiversWithoutLeader{
		Machines: []machine.Name{"nonexistent"},
	}
	args := operation.ExecArgs{
		Command: "test command",
	}

	result, err := s.state.AddExecOperation(c.Context(), operationUUID, target, args)
	c.Assert(err, tc.IsNil) // Operation should succeed
	c.Check(len(result.Machines), tc.Equals, 1)

	// The machine task should have an error.
	c.Check(result.Machines[0].TaskInfo.Error, tc.ErrorMatches, ".*machine UUID.*")
	c.Check(result.Machines[0].ReceiverName, tc.Equals, machine.Name("nonexistent"))
}

func (s *startSuite) TestAddExecOperationUnitNotFound(c *tc.C) {
	operationUUID := internaluuid.MustNewUUID()
	target := operation.ReceiversWithoutLeader{
		Units: []unit.Name{"nonexistent/0"},
	}
	args := operation.ExecArgs{
		Command: "test command",
	}

	result, err := s.state.AddExecOperation(c.Context(), operationUUID, target, args)
	c.Assert(err, tc.IsNil) // Operation should succeed
	c.Check(len(result.Units), tc.Equals, 1)

	// The unit task should have an error.
	c.Check(result.Units[0].TaskInfo.Error, tc.ErrorMatches, ".*unit UUID.*")
	c.Check(result.Units[0].ReceiverName, tc.Equals, unit.Name("nonexistent/0"))
}

func (s *startSuite) TestAddExecOperationApplicationNotFound(c *tc.C) {
	operationUUID := internaluuid.MustNewUUID()
	target := operation.ReceiversWithoutLeader{
		Applications: []string{"nonexistent-app"},
	}
	args := operation.ExecArgs{
		Command: "test command",
	}

	_, err := s.state.AddExecOperation(c.Context(), operationUUID, target, args)
	c.Assert(err, tc.ErrorMatches, ".*units for application.*")
}

func (s *startSuite) TestAddExecOperationParametersStored(c *tc.C) {
	_ = s.addMachine(c, "0")

	operationUUID := internaluuid.MustNewUUID()
	target := operation.ReceiversWithoutLeader{
		Machines: []machine.Name{"0"},
	}
	args := operation.ExecArgs{
		Command: "echo hello world",
		Timeout: 5 * time.Minute,
	}

	_, err := s.state.AddExecOperation(c.Context(), operationUUID, target, args)
	c.Assert(err, tc.IsNil)

	// Verify command and timeout parameters were stored.
	var commandValue, timeoutValue string
	err = s.TxnRunner().StdTxn(c.Context(), func(ctx context.Context, tx *sql.Tx) error {
		err := tx.QueryRow("SELECT value FROM operation_parameter WHERE operation_uuid = ? AND key = 'command'",
			operationUUID.String()).Scan(&commandValue)
		if err != nil {
			return err
		}
		return tx.QueryRow("SELECT value FROM operation_parameter WHERE operation_uuid = ? AND key = 'timeout'",
			operationUUID.String()).Scan(&timeoutValue)
	})
	c.Assert(err, tc.IsNil)
	c.Check(commandValue, tc.Equals, "echo hello world")
	c.Check(timeoutValue, tc.Equals, "5m0s")
}

func (s *startSuite) TestAddExecOperationOnAllMachines(c *tc.C) {
	_ = s.addMachine(c, "0")
	_ = s.addMachine(c, "1")

	operationUUID := internaluuid.MustNewUUID()
	args := operation.ExecArgs{
		Command:        "update all",
		Parallel:       true,
		ExecutionGroup: "maintenance",
	}

	result, err := s.state.AddExecOperationOnAllMachines(c.Context(), operationUUID, args)
	c.Assert(err, tc.IsNil)
	c.Check(result.OperationID, tc.Not(tc.Equals), "")
	c.Check(len(result.Machines), tc.Equals, 2)
	c.Check(len(result.Units), tc.Equals, 0)

	// Check that both machines are included.
	machineNames := make(map[string]bool)
	for _, machine := range result.Machines {
		machineNames[string(machine.ReceiverName)] = true
		c.Check(machine.TaskInfo.Status, tc.Equals, corestatus.Pending)
		c.Check(machine.TaskInfo.Error, tc.IsNil)
	}
	c.Check(machineNames["0"], tc.Equals, true)
	c.Check(machineNames["1"], tc.Equals, true)
}

func (s *startSuite) TestAddExecOperationOnAllMachinesNoMachines(c *tc.C) {
	operationUUID := internaluuid.MustNewUUID()
	args := operation.ExecArgs{
		Command: "update all",
	}

	result, err := s.state.AddExecOperationOnAllMachines(c.Context(), operationUUID, args)
	c.Assert(err, tc.IsNil)
	c.Check(result.OperationID, tc.Not(tc.Equals), "")
	c.Check(len(result.Machines), tc.Equals, 0)
	c.Check(len(result.Units), tc.Equals, 0)

	var opCount int
	err = s.TxnRunner().StdTxn(c.Context(), func(ctx context.Context, tx *sql.Tx) error {
		return tx.QueryRow("SELECT COUNT(*) FROM operation WHERE uuid = ?", operationUUID.String()).Scan(&opCount)
	})
	c.Assert(err, tc.IsNil)
	c.Check(opCount, tc.Equals, 1)
}

func (s *startSuite) TestAddExecOperationOnAllMachinesWithDeadMachine(c *tc.C) {
	_ = s.addMachine(c, "0")

	// Add a dead machine (life_id = 1)
	netNodeUUID := internaluuid.MustNewUUID().String()
	s.query(c, `INSERT INTO net_node (uuid) VALUES (?)`, netNodeUUID)
	deadMachineUUID := internaluuid.MustNewUUID()
	s.query(c, `INSERT INTO machine (uuid, name, life_id, net_node_uuid) VALUES (?, ?, ?, ?)`,
		deadMachineUUID.String(), "1", 1, netNodeUUID) // life_id = 1 (dead)

	operationUUID := internaluuid.MustNewUUID()
	args := operation.ExecArgs{
		Command: "update all",
	}

	result, err := s.state.AddExecOperationOnAllMachines(c.Context(), operationUUID, args)
	c.Assert(err, tc.IsNil)
	c.Check(len(result.Machines), tc.Equals, 1) // Only alive machine should be included
	c.Check(result.Machines[0].ReceiverName, tc.Equals, machine.Name("0"))
}

func (s *startSuite) TestAddActionOperationSingleUnit(c *tc.C) {
	charmUUID := s.addCharm(c)
	s.addCharmAction(c, charmUUID)
	_ = s.addUnitWithName(c, charmUUID, "single-app/0")

	operationUUID := internaluuid.MustNewUUID()
	targetUnits := []unit.Name{"single-app/0"}
	args := operation.TaskArgs{
		ActionName:     "test-action",
		ExecutionGroup: "actions",
		IsParallel:     false,
		Parameters: map[string]any{
			"param1": "value1",
			"param2": 42,
		},
	}

	result, err := s.state.AddActionOperation(c.Context(), operationUUID, targetUnits, args)
	c.Assert(err, tc.IsNil)
	c.Check(result.OperationID, tc.Not(tc.Equals), "")
	c.Check(len(result.Units), tc.Equals, 1)
	c.Check(len(result.Machines), tc.Equals, 0)

	// Check unit result
	c.Check(result.Units[0].ReceiverName, tc.Equals, unit.Name("single-app/0"))
	c.Check(result.Units[0].TaskInfo.ID, tc.Not(tc.Equals), "")
	c.Check(result.Units[0].TaskInfo.Status, tc.Equals, corestatus.Pending)
	c.Check(result.Units[0].TaskInfo.Error, tc.IsNil)

	// Verify operation action was stored.
	var actionCount int
	err = s.TxnRunner().StdTxn(c.Context(), func(ctx context.Context, tx *sql.Tx) error {
		return tx.QueryRow("SELECT COUNT(*) FROM operation_action WHERE operation_uuid = ?",
			operationUUID.String()).Scan(&actionCount)
	})
	c.Assert(err, tc.IsNil)
	c.Check(actionCount, tc.Equals, 1)
}

func (s *startSuite) TestAddActionOperationMultipleUnits(c *tc.C) {
	charmUUID := s.addCharm(c)
	s.addCharmAction(c, charmUUID)
	appUUID := s.addApplication(c, charmUUID, "test-action-multi")
	_ = s.addUnitToApplication(c, charmUUID, appUUID, "test-action-multi/0")
	_ = s.addUnitToApplication(c, charmUUID, appUUID, "test-action-multi/1")

	operationUUID := internaluuid.MustNewUUID()
	targetUnits := []unit.Name{"test-action-multi/0", "test-action-multi/1"}
	args := operation.TaskArgs{
		ActionName: "test-action",
		IsParallel: true,
		Parameters: map[string]any{
			"multi-param": "multi-value",
		},
	}

	result, err := s.state.AddActionOperation(c.Context(), operationUUID, targetUnits, args)
	c.Assert(err, tc.IsNil)
	c.Check(result.OperationID, tc.Not(tc.Equals), "")
	c.Check(len(result.Units), tc.Equals, 2)

	// Check both units have valid results.
	unitNames := make(map[string]bool)
	for _, unit := range result.Units {
		unitNames[unit.ReceiverName.String()] = true
		c.Check(unit.TaskInfo.Error, tc.IsNil)
		c.Check(unit.TaskInfo.Status, tc.Equals, corestatus.Pending)
		c.Check(unit.TaskInfo.ID, tc.Not(tc.Equals), "")
	}
	c.Check(unitNames["test-action-multi/0"], tc.Equals, true)
	c.Check(unitNames["test-action-multi/1"], tc.Equals, true)
}

func (s *startSuite) TestAddActionOperationEmptyTargetUnits(c *tc.C) {
	operationUUID := internaluuid.MustNewUUID()
	targetUnits := []unit.Name{}
	args := operation.TaskArgs{
		ActionName: "test-action",
	}

	_, err := s.state.AddActionOperation(c.Context(), operationUUID, targetUnits, args)
	c.Assert(err, tc.ErrorMatches, "no target units provided")
}

func (s *startSuite) TestAddActionOperationUnitNotFound(c *tc.C) {
	charmUUID := s.addCharm(c)
	s.addCharmAction(c, charmUUID)

	operationUUID := internaluuid.MustNewUUID()
	targetUnits := []unit.Name{"nonexistent/0"}
	args := operation.TaskArgs{
		ActionName: "test-action",
	}

	_, err := s.state.AddActionOperation(c.Context(), operationUUID, targetUnits, args)
	c.Assert(err, tc.ErrorMatches, "no valid unit found for the action test-action")
}

func (s *startSuite) TestAddActionOperationCharmNotFound(c *tc.C) {
	charmUUID := s.addCharm(c)
	_ = s.addUnitWithName(c, charmUUID, "test-charm-not-found/0")

	operationUUID := internaluuid.MustNewUUID()
	targetUnits := []unit.Name{"test-charm-not-found/0"}
	args := operation.TaskArgs{
		ActionName: "nonexistent-action",
	}

	_, err := s.state.AddActionOperation(c.Context(), operationUUID, targetUnits, args)
	c.Assert(err, tc.ErrorMatches, ".*FOREIGN KEY constraint failed.*")
}

func (s *startSuite) TestAddActionOperationParametersStored(c *tc.C) {
	charmUUID := s.addCharm(c)
	s.addCharmAction(c, charmUUID)
	_ = s.addUnitWithName(c, charmUUID, "params-app/0")

	operationUUID := internaluuid.MustNewUUID()
	targetUnits := []unit.Name{"params-app/0"}
	args := operation.TaskArgs{
		ActionName: "test-action",
		Parameters: map[string]any{
			"param1": "value1",
			"param2": 42,
			"param3": true,
		},
	}

	_, err := s.state.AddActionOperation(c.Context(), operationUUID, targetUnits, args)
	c.Assert(err, tc.IsNil)

	// Verify all parameters were stored.
	var paramCount int
	err = s.TxnRunner().StdTxn(c.Context(), func(ctx context.Context, tx *sql.Tx) error {
		return tx.QueryRow("SELECT COUNT(*) FROM operation_parameter WHERE operation_uuid = ?",
			operationUUID.String()).Scan(&paramCount)
	})
	c.Assert(err, tc.IsNil)
	c.Check(paramCount, tc.Equals, 3)

	// Verify specific parameter values.
	var param1Value, param2Value, param3Value string
	err = s.TxnRunner().StdTxn(c.Context(), func(ctx context.Context, tx *sql.Tx) error {
		err := tx.QueryRow("SELECT value FROM operation_parameter WHERE operation_uuid = ? AND key = 'param1'",
			operationUUID.String()).Scan(&param1Value)
		if err != nil {
			return err
		}
		err = tx.QueryRow("SELECT value FROM operation_parameter WHERE operation_uuid = ? AND key = 'param2'",
			operationUUID.String()).Scan(&param2Value)
		if err != nil {
			return err
		}
		return tx.QueryRow("SELECT value FROM operation_parameter WHERE operation_uuid = ? AND key = 'param3'",
			operationUUID.String()).Scan(&param3Value)
	})
	c.Assert(err, tc.IsNil)
	c.Check(param1Value, tc.Equals, "value1")
	c.Check(param2Value, tc.Equals, "42")
	c.Check(param3Value, tc.Equals, "true")
}

func (s *startSuite) TestAddActionOperationMixedSuccessAndErrors(c *tc.C) {
	charmUUID := s.addCharm(c)
	s.addCharmAction(c, charmUUID)
	_ = s.addUnitWithName(c, charmUUID, "test-action-mixed/0")

	operationUUID := internaluuid.MustNewUUID()
	targetUnits := []unit.Name{"test-action-mixed/0", "nonexistent/0"}
	args := operation.TaskArgs{
		ActionName: "test-action",
	}

	result, err := s.state.AddActionOperation(c.Context(), operationUUID, targetUnits, args)
	c.Assert(err, tc.IsNil)
	c.Check(len(result.Units), tc.Equals, 2)

	// First unit should succeed.
	c.Check(result.Units[0].ReceiverName, tc.Equals, unit.Name("test-action-mixed/0"))
	c.Check(result.Units[0].TaskInfo.Error, tc.IsNil)
	c.Check(result.Units[0].TaskInfo.ID, tc.Not(tc.Equals), "")

	// Second unit should have error.
	c.Check(result.Units[1].ReceiverName, tc.Equals, unit.Name("nonexistent/0"))
	c.Check(result.Units[1].TaskInfo.Error, tc.ErrorMatches, ".*unit UUID.*")
}

// Sequence tests

func (s *startSuite) TestOperationAndTaskSequenceIncremental(c *tc.C) {
	_ = s.addMachine(c, "0")

	target := operation.ReceiversWithoutLeader{
		Machines: []machine.Name{"0"},
	}
	args := operation.ExecArgs{
		Command: "first operation",
	}

	result1, err := s.state.AddExecOperation(c.Context(), internaluuid.MustNewUUID(), target, args)
	c.Assert(err, tc.IsNil)

	result2, err := s.state.AddExecOperation(c.Context(), internaluuid.MustNewUUID(), target, args)
	c.Assert(err, tc.IsNil)

	// Operation IDs should be sequential.
	c.Check(result1.OperationID, tc.Not(tc.Equals), result2.OperationID)

	// Task IDs should also be sequential and different from operation IDs.
	c.Check(result1.Machines[0].TaskInfo.ID, tc.Not(tc.Equals), result2.Machines[0].TaskInfo.ID)
	c.Check(result1.Machines[0].TaskInfo.ID, tc.Not(tc.Equals), result1.OperationID)
}

func (s *startSuite) TestOperationSummaryGeneration(c *tc.C) {
	_ = s.addMachine(c, "0")

	operationUUID := internaluuid.MustNewUUID()
	target := operation.ReceiversWithoutLeader{
		Machines: []machine.Name{"0"},
	}
	args := operation.ExecArgs{
		Command: "test command",
	}

	result, err := s.state.AddExecOperation(c.Context(), operationUUID, target, args)
	c.Assert(err, tc.IsNil)

	// Verify operation summary was generated correctly.
	var summary string
	err = s.TxnRunner().StdTxn(c.Context(), func(ctx context.Context, tx *sql.Tx) error {
		return tx.QueryRow("SELECT summary FROM operation WHERE operation_id = ?",
			result.OperationID).Scan(&summary)
	})
	c.Assert(err, tc.IsNil)
	c.Check(summary, tc.Equals, "exec test command")

	// Test action operation summary.
	actionCharmUUID := s.addCharm(c)
	s.addCharmAction(c, actionCharmUUID)
	_ = s.addUnitWithName(c, actionCharmUUID, "summary-app/0")

	actionOperationUUID := internaluuid.MustNewUUID()
	targetUnits := []unit.Name{"summary-app/0"}
	actionArgs := operation.TaskArgs{
		ActionName: "test-action",
	}

	actionResult, err := s.state.AddActionOperation(c.Context(), actionOperationUUID, targetUnits, actionArgs)
	c.Assert(err, tc.IsNil)

	var actionSummary string
	err = s.TxnRunner().StdTxn(c.Context(), func(ctx context.Context, tx *sql.Tx) error {
		return tx.QueryRow("SELECT summary FROM operation WHERE operation_id = ?",
			actionResult.OperationID).Scan(&actionSummary)
	})
	c.Assert(err, tc.IsNil)
	c.Check(actionSummary, tc.Equals, "action test-action")
}

func (s *startSuite) TestTaskStatusAndLinksCreated(c *tc.C) {
	machineUUID := s.addMachine(c, "0")
	unitUUID := s.addUnitWithName(c, s.addCharm(c), "links-app/0")

	operationUUID := internaluuid.MustNewUUID()
	target := operation.ReceiversWithoutLeader{
		Machines: []machine.Name{"0"},
		Units:    []unit.Name{"links-app/0"},
	}
	args := operation.ExecArgs{
		Command: "test command",
	}

	result, err := s.state.AddExecOperation(c.Context(), operationUUID, target, args)
	c.Assert(err, tc.IsNil)
	c.Check(len(result.Machines), tc.Equals, 1)
	c.Check(len(result.Units), tc.Equals, 1)

	// Verify task status records were created.
	var machineTaskStatusCount, unitTaskStatusCount int
	err = s.TxnRunner().StdTxn(c.Context(), func(ctx context.Context, tx *sql.Tx) error {
		// Count machine task statuses.
		err := tx.QueryRow(`
			SELECT COUNT(*) FROM operation_task_status ots
			JOIN operation_task ot ON ots.task_uuid = ot.uuid
			JOIN operation_machine_task omt ON ot.uuid = omt.task_uuid
			WHERE omt.machine_uuid = ?`, machineUUID).Scan(&machineTaskStatusCount)
		if err != nil {
			return err
		}
		// Count unit task statuses.
		return tx.QueryRow(`
			SELECT COUNT(*) FROM operation_task_status ots
			JOIN operation_task ot ON ots.task_uuid = ot.uuid
			JOIN operation_unit_task out ON ot.uuid = out.task_uuid
			WHERE out.unit_uuid = ?`, unitUUID).Scan(&unitTaskStatusCount)
	})
	c.Assert(err, tc.IsNil)
	c.Check(machineTaskStatusCount, tc.Equals, 1)
	c.Check(unitTaskStatusCount, tc.Equals, 1)

	// Verify links were created.
	var machineLinks, unitLinks int
	err = s.TxnRunner().StdTxn(c.Context(), func(ctx context.Context, tx *sql.Tx) error {
		err := tx.QueryRow("SELECT COUNT(*) FROM operation_machine_task WHERE machine_uuid = ?", machineUUID).Scan(&machineLinks)
		if err != nil {
			return err
		}
		return tx.QueryRow("SELECT COUNT(*) FROM operation_unit_task WHERE unit_uuid = ?", unitUUID).Scan(&unitLinks)
	})
	c.Assert(err, tc.IsNil)
	c.Check(machineLinks, tc.Equals, 1)
	c.Check(unitLinks, tc.Equals, 1)
}
