// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state

import (
	"context"
	"database/sql"
	"fmt"
	"strconv"

	"github.com/canonical/sqlair"
	"github.com/juju/tc"

	"github.com/juju/juju/core/machine"
	domainapplication "github.com/juju/juju/domain/application"
	applicationerrors "github.com/juju/juju/domain/application/errors"
	"github.com/juju/juju/domain/deployment"
	"github.com/juju/juju/domain/sequence"
	domainstatus "github.com/juju/juju/domain/status"
	"github.com/juju/juju/internal/errors"
)

func (s *unitStateSuite) TestPlaceNetNodeMachinesInvalidPlacement(c *tc.C) {
	err := s.TxnRunner().Txn(c.Context(), func(ctx context.Context, tx *sqlair.TX) error {
		_, _, err := s.state.placeMachine(ctx, tx, deployment.Placement{
			Type: deployment.PlacementType(666),
		})
		return err
	})
	c.Assert(err, tc.ErrorMatches, `invalid placement type: 666`)
}

func (s *unitStateSuite) TestPlaceNetNodeMachinesUnset(c *tc.C) {
	// Ensure the machine got created.

	var (
		netNode      string
		machineNames []machine.Name
	)
	err := s.TxnRunner().Txn(c.Context(), func(ctx context.Context, tx *sqlair.TX) error {
		var err error
		netNode, machineNames, err = s.state.placeMachine(ctx, tx, deployment.Placement{
			Type: deployment.PlacementTypeUnset,
		})
		return err
	})
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(netNode, tc.Not(tc.Equals), "")

	resultNetNode, err := s.state.GetMachineNetNodeUUIDFromName(c.Context(), machine.Name("0"))
	c.Assert(err, tc.ErrorIsNil)
	c.Check(resultNetNode, tc.Equals, netNode)

	s.ensureSequenceForMachineNamespace(c, 0)

	s.ensureStatusForMachine(c, machine.Name("0"), domainstatus.MachineStatusPending)
	s.ensureStatusForMachineInstance(c, machine.Name("0"), domainstatus.InstanceStatusPending)

	c.Assert(machineNames, tc.HasLen, 1)
	c.Check(machineNames[0], tc.Equals, machine.Name("0"))
}

func (s *unitStateSuite) TestPlaceNetNodeMachinesUnsetMultipleTimes(c *tc.C) {
	total := 10

	// Ensure that the machines are sequenced correctly.
	// The first machine should be 0, the second 1, and so on.

	var netNodes []string
	err := s.TxnRunner().Txn(c.Context(), func(ctx context.Context, tx *sqlair.TX) error {
		for range total {
			netNode, _, err := s.state.placeMachine(ctx, tx, deployment.Placement{
				Type: deployment.PlacementTypeUnset,
			})
			if err != nil {
				return err
			}
			netNodes = append(netNodes, netNode)

		}
		return nil
	})
	c.Assert(err, tc.ErrorIsNil)

	var resultNetNodes []string
	for i := range total {
		netNode, err := s.state.GetMachineNetNodeUUIDFromName(c.Context(), machine.Name(strconv.Itoa(i)))
		c.Assert(err, tc.ErrorIsNil)
		resultNetNodes = append(resultNetNodes, netNode)
	}
	c.Check(resultNetNodes, tc.DeepEquals, netNodes)

	s.ensureSequenceForMachineNamespace(c, total-1)
}

func (s *unitStateSuite) TestPlaceNetNodeMachinesUnsetMultipleTimesWithGaps(c *tc.C) {
	stepTotal := 5

	// Ensure that the machines are sequenced correctly.
	// The first machine should be 0, the second 1, and so on, then delete
	// the the last machine. The following sequence should continue from
	// the last machine, leaving gaps in the sequence.
	// The final sequence should be:
	//
	// 0, 1, 2, 3, 5, 6, 7, 8, 9

	var netNodes []string
	createMachines := func() {
		err := s.TxnRunner().Txn(c.Context(), func(ctx context.Context, tx *sqlair.TX) error {
			for range stepTotal {
				netNode, _, err := s.state.placeMachine(ctx, tx, deployment.Placement{
					Type: deployment.PlacementTypeUnset,
				})
				if err != nil {
					return err
				}
				netNodes = append(netNodes, netNode)
			}
			return nil
		})
		c.Assert(err, tc.ErrorIsNil)
	}
	deleteLastMachine := func() {
		err := s.TxnRunner().StdTxn(c.Context(), func(ctx context.Context, tx *sql.Tx) error {
			var child string
			err := tx.QueryRowContext(ctx, `
SELECT m.uuid
FROM machine m 
WHERE m.net_node_uuid = ?
`, netNodes[len(netNodes)-1]).Scan(&child)
			if err != nil {
				return errors.Capture(err)
			}

			for _, table := range []string{
				"machine_status",
				"machine_cloud_instance_status",
				"machine_cloud_instance",
			} {
				_, err := tx.ExecContext(ctx, fmt.Sprintf(`DELETE FROM %q WHERE machine_uuid = ?`, table), child)
				if err != nil {
					return errors.Capture(err)
				}
			}

			_, err = tx.Exec("DELETE FROM machine WHERE net_node_uuid = ?", netNodes[len(netNodes)-1])
			return err
		})
		c.Assert(err, tc.ErrorIsNil)

		netNodes = netNodes[:len(netNodes)-1]
	}

	// Ensure the sequence hasn't been changed during the delete.

	createMachines()
	s.ensureSequenceForMachineNamespace(c, stepTotal-1)

	deleteLastMachine()
	s.ensureSequenceForMachineNamespace(c, stepTotal-1)

	createMachines()
	s.ensureSequenceForMachineNamespace(c, (stepTotal*2)-1)

	var resultNetNodes []string
	for i := range stepTotal * 2 {
		netNode, err := s.state.GetMachineNetNodeUUIDFromName(c.Context(), machine.Name(strconv.Itoa(i)))
		if errors.Is(err, applicationerrors.MachineNotFound) && i == stepTotal-1 {
			// This machine was deleted, this is fine.
			c.Logf("machine %d not found, this is expected", i)
			continue
		}
		c.Assert(err, tc.ErrorIsNil)
		resultNetNodes = append(resultNetNodes, netNode)
	}
	c.Check(resultNetNodes, tc.DeepEquals, netNodes)
}

func (s *unitStateSuite) TestPlaceNetNodeMachinesExistingMachine(c *tc.C) {
	// Create the machine, then try to place it on the same machine.

	var netNode string
	err := s.TxnRunner().Txn(c.Context(), func(ctx context.Context, tx *sqlair.TX) error {
		var err error
		netNode, _, err = s.state.placeMachine(ctx, tx, deployment.Placement{
			Type: deployment.PlacementTypeUnset,
		})
		return err
	})
	c.Assert(err, tc.ErrorIsNil)

	var (
		resultNetNode string
		machineNames  []machine.Name
	)
	err = s.TxnRunner().Txn(c.Context(), func(ctx context.Context, tx *sqlair.TX) error {
		var err error
		resultNetNode, machineNames, err = s.state.placeMachine(ctx, tx, deployment.Placement{
			Type:      deployment.PlacementTypeMachine,
			Directive: "0",
		})
		return err
	})
	c.Assert(err, tc.ErrorIsNil)
	c.Check(resultNetNode, tc.Equals, netNode)
	c.Assert(machineNames, tc.HasLen, 0)
}

func (s *unitStateSuite) TestPlaceNetNodeMachinesExistingMachineNotFound(c *tc.C) {
	// Try and place a machine that doesn't exist.

	err := s.TxnRunner().Txn(c.Context(), func(ctx context.Context, tx *sqlair.TX) error {
		_, _, err := s.state.placeMachine(ctx, tx, deployment.Placement{
			Type:      deployment.PlacementTypeMachine,
			Directive: "0",
		})
		return err
	})
	c.Assert(err, tc.ErrorIs, applicationerrors.MachineNotFound)
}

func (s *unitStateSuite) TestPlaceNetNodeMachinesContainer(c *tc.C) {
	// Ensure the parent and child machine got created.

	var (
		netNode      string
		machineNames []machine.Name
	)
	err := s.TxnRunner().Txn(c.Context(), func(ctx context.Context, tx *sqlair.TX) error {
		var err error
		netNode, machineNames, err = s.state.placeMachine(ctx, tx, deployment.Placement{
			Type:      deployment.PlacementTypeContainer,
			Container: deployment.ContainerTypeLXD,
		})
		return err
	})
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(netNode, tc.Not(tc.Equals), "")

	resultNetNode, err := s.state.GetMachineNetNodeUUIDFromName(c.Context(), "0/lxd/0")
	c.Assert(err, tc.ErrorIsNil)
	c.Check(resultNetNode, tc.Equals, netNode)

	s.ensureSequenceForMachineNamespace(c, 0)
	s.ensureSequenceForContainerNamespace(c, "0", 0)

	c.Assert(machineNames, tc.HasLen, 2)
	c.Check(machineNames, tc.DeepEquals, []machine.Name{
		machine.Name("0"),
		machine.Name("0/lxd/0"),
	})
}

func (s *unitStateSuite) TestPlaceNetNodeMachinesContainerWithDirective(c *tc.C) {
	// Ensure the child machine got created on the parent machine.

	// Insert a machine with no placement, then place a container on it.
	err := s.TxnRunner().Txn(c.Context(), func(ctx context.Context, tx *sqlair.TX) error {
		_, _, err := s.state.placeMachine(ctx, tx, deployment.Placement{
			Type: deployment.PlacementTypeUnset,
		})
		return err
	})
	c.Assert(err, tc.ErrorIsNil)

	var (
		netNode      string
		machineNames []machine.Name
	)
	err = s.TxnRunner().Txn(c.Context(), func(ctx context.Context, tx *sqlair.TX) error {
		var err error
		netNode, machineNames, err = s.state.placeMachine(ctx, tx, deployment.Placement{
			Type:      deployment.PlacementTypeContainer,
			Container: deployment.ContainerTypeLXD,
			Directive: "0",
		})
		return err
	})
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(netNode, tc.Not(tc.Equals), "")

	resultNetNode, err := s.state.GetMachineNetNodeUUIDFromName(c.Context(), "0/lxd/0")
	c.Assert(err, tc.ErrorIsNil)
	c.Check(resultNetNode, tc.Equals, netNode)

	s.ensureSequenceForMachineNamespace(c, 0)
	s.ensureSequenceForContainerNamespace(c, "0", 0)

	c.Assert(machineNames, tc.HasLen, 2)
	c.Check(machineNames, tc.DeepEquals, []machine.Name{
		machine.Name("0"),
		machine.Name("0/lxd/0"),
	})
}

func (s *unitStateSuite) TestPlaceNetNodeMachinesContainerWithDirectiveMachineNotFound(c *tc.C) {
	err := s.TxnRunner().Txn(c.Context(), func(ctx context.Context, tx *sqlair.TX) error {
		_, _, err := s.state.placeMachine(ctx, tx, deployment.Placement{
			Type:      deployment.PlacementTypeContainer,
			Container: deployment.ContainerTypeLXD,
			Directive: "1",
		})
		return err
	})
	c.Assert(err, tc.ErrorIs, applicationerrors.MachineNotFound)
}

func (s *unitStateSuite) TestPlaceNetNodeMachinesContainerMultipleTimes(c *tc.C) {
	total := 10

	// Ensure that the machines are sequenced correctly.
	// The first machine should be 0/lxd/0, the second 1/lxd/0, and so on.

	var netNodes []string
	err := s.TxnRunner().Txn(c.Context(), func(ctx context.Context, tx *sqlair.TX) error {
		for range total {
			netNode, _, err := s.state.placeMachine(ctx, tx, deployment.Placement{
				Type:      deployment.PlacementTypeContainer,
				Container: deployment.ContainerTypeLXD,
			})
			if err != nil {
				return err
			}
			netNodes = append(netNodes, netNode)

		}
		return nil
	})
	c.Assert(err, tc.ErrorIsNil)

	var resultNetNodes []string
	for i := range total {
		name := fmt.Sprintf("%d/lxd/0", i)
		netNode, err := s.state.GetMachineNetNodeUUIDFromName(c.Context(), machine.Name(name))
		c.Assert(err, tc.ErrorIsNil)
		resultNetNodes = append(resultNetNodes, netNode)
	}
	c.Check(resultNetNodes, tc.DeepEquals, netNodes)

	s.ensureSequenceForMachineNamespace(c, total-1)
	for i := range total {
		s.ensureSequenceForContainerNamespace(c, machine.Name(strconv.Itoa(i)), 0)
	}
}

func (s *unitStateSuite) TestPlaceNetNodeMachinesContainerMultipleTimesWithGaps(c *tc.C) {
	stepTotal := 5

	// Ensure that the machines are sequenced correctly. The first machine
	// should be 0, the second 1, and so on, then delete the the last machine.
	// The following sequence should continue from the last machine, leaving
	// gaps in the sequence. The final sequence should be:
	//
	// 0/lxd/0, 1/lxd/0, 2/lxd/0, 3/lxd/0, 5/lxd/0, 6/lxd/0, 7/lxd/0, 8/lxd/0,
	// 9/lxd/0

	var netNodes []string
	createMachines := func() {
		err := s.TxnRunner().Txn(c.Context(), func(ctx context.Context, tx *sqlair.TX) error {
			for range stepTotal {
				netNode, _, err := s.state.placeMachine(ctx, tx, deployment.Placement{
					Type:      deployment.PlacementTypeContainer,
					Container: deployment.ContainerTypeLXD,
				})
				if err != nil {
					return err
				}
				netNodes = append(netNodes, netNode)
			}
			return nil
		})
		c.Assert(err, tc.ErrorIsNil)
	}
	deleteLastMachine := func() {
		// Delete the parent and the child.
		err := s.TxnRunner().StdTxn(c.Context(), func(ctx context.Context, tx *sql.Tx) error {
			var parent, child string
			err := tx.QueryRowContext(ctx, `
SELECT mp.parent_uuid, mp.machine_uuid
FROM machine m 
JOIN machine_parent AS mp ON m.uuid = mp.machine_uuid
WHERE m.net_node_uuid = ?
`, netNodes[len(netNodes)-1]).Scan(&parent, &child)
			if err != nil {
				return errors.Capture(err)
			}

			_, err = tx.ExecContext(ctx, "DELETE FROM machine_parent WHERE machine_uuid = ?", child)
			if err != nil {
				return errors.Capture(err)
			}

			for _, table := range []string{
				"machine_status",
				"machine_cloud_instance_status",
				"machine_cloud_instance",
			} {
				_, err = tx.ExecContext(ctx, fmt.Sprintf(`DELETE FROM %q WHERE machine_uuid = ?`, table), child)
				if err != nil {
					return errors.Capture(err)
				}
				_, err = tx.ExecContext(ctx, fmt.Sprintf(`DELETE FROM %q WHERE machine_uuid = ?`, table), parent)
				if err != nil {
					return errors.Capture(err)
				}
			}

			_, err = tx.ExecContext(ctx, "DELETE FROM machine WHERE uuid = ?", child)
			if err != nil {
				return errors.Capture(err)
			}
			_, err = tx.ExecContext(ctx, "DELETE FROM machine WHERE uuid = ?", parent)
			if err != nil {
				return errors.Capture(err)
			}

			return nil
		})
		c.Assert(err, tc.ErrorIsNil)

		netNodes = netNodes[:len(netNodes)-1]
	}

	// Ensure the sequence hasn't been changed during the delete.

	createMachines()
	s.ensureSequenceForMachineNamespace(c, stepTotal-1)

	deleteLastMachine()
	s.ensureSequenceForMachineNamespace(c, stepTotal-1)

	createMachines()
	s.ensureSequenceForMachineNamespace(c, (stepTotal*2)-1)

	var resultNetNodes []string
	for i := range stepTotal * 2 {
		name := fmt.Sprintf("%d/lxd/0", i)
		netNode, err := s.state.GetMachineNetNodeUUIDFromName(c.Context(), machine.Name(name))
		if errors.Is(err, applicationerrors.MachineNotFound) && i == stepTotal-1 {
			// This machine was deleted, this is fine.
			c.Logf("machine %d not found, this is expected", i)
			continue
		}
		c.Assert(err, tc.ErrorIsNil)
		resultNetNodes = append(resultNetNodes, netNode)
	}
	c.Check(resultNetNodes, tc.DeepEquals, netNodes)
}

func (s *unitStateSuite) TestPlaceNetNodeMachinesProvider(c *tc.C) {
	// Ensure that the parent placement is correctly set on the
	// machine_placement table.

	var netNode string
	err := s.TxnRunner().Txn(c.Context(), func(ctx context.Context, tx *sqlair.TX) error {
		var err error
		netNode, _, err = s.state.placeMachine(ctx, tx, deployment.Placement{
			Type:      deployment.PlacementTypeProvider,
			Directive: "zone=eu-west-1",
		})
		return err
	})
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(netNode, tc.Not(tc.Equals), "")

	resultNetNode, err := s.state.GetMachineNetNodeUUIDFromName(c.Context(), "0")
	c.Assert(err, tc.ErrorIsNil)
	c.Check(resultNetNode, tc.Equals, netNode)

	s.ensureSequenceForMachineNamespace(c, 0)

	var directive string
	err = s.TxnRunner().StdTxn(c.Context(), func(ctx context.Context, tx *sql.Tx) error {
		err := tx.QueryRow(`
SELECT mp.directive
FROM machine m
JOIN machine_placement AS mp ON m.uuid = mp.machine_uuid
WHERE m.net_node_uuid = ?
`, resultNetNode).Scan(&directive)
		if err != nil {
			return err
		}
		return nil
	})
	c.Assert(err, tc.ErrorIsNil)
	c.Check(directive, tc.Equals, "zone=eu-west-1")
}

func (s *unitStateSuite) ensureSequenceForMachineNamespace(c *tc.C, expected int) {
	var seq int
	err := s.TxnRunner().StdTxn(c.Context(), func(ctx context.Context, tx *sql.Tx) error {
		namespace := domainapplication.MachineSequenceNamespace
		return tx.QueryRow("SELECT value FROM sequence WHERE namespace = ?", namespace).Scan(&seq)
	})
	c.Assert(err, tc.ErrorIsNil)
	c.Check(seq, tc.Equals, expected)
}

func (s *unitStateSuite) ensureSequenceForContainerNamespace(c *tc.C, parentName machine.Name, expected int) {
	var seq int
	err := s.TxnRunner().StdTxn(c.Context(), func(ctx context.Context, tx *sql.Tx) error {
		namespace := sequence.MakePrefixNamespace(domainapplication.ContainerSequenceNamespace, parentName.String()).String()
		return tx.QueryRow("SELECT value FROM sequence WHERE namespace = ?", namespace).Scan(&seq)
	})
	c.Assert(err, tc.ErrorIsNil)
	c.Check(seq, tc.Equals, expected)
}

func (s *unitStateSuite) ensureStatusForMachine(c *tc.C, name machine.Name, expected domainstatus.MachineStatusType) {
	var status int
	err := s.TxnRunner().StdTxn(c.Context(), func(ctx context.Context, tx *sql.Tx) error {
		return tx.QueryRow(`
SELECT ms.status_id
FROM machine AS m
JOIN machine_status AS ms ON m.uuid = ms.machine_uuid
WHERE m.name = ?
`, name).Scan(&status)
	})
	c.Assert(err, tc.ErrorIsNil)

	statusValue, err := domainstatus.EncodeMachineStatus(expected)
	c.Assert(err, tc.ErrorIsNil)
	c.Check(status, tc.Equals, statusValue)
}

func (s *unitStateSuite) ensureStatusForMachineInstance(c *tc.C, name machine.Name, expected domainstatus.InstanceStatusType) {
	var status int
	err := s.TxnRunner().StdTxn(c.Context(), func(ctx context.Context, tx *sql.Tx) error {
		return tx.QueryRow(`
SELECT ms.status_id
FROM machine AS m
JOIN machine_cloud_instance_status AS ms ON m.uuid = ms.machine_uuid
WHERE m.name = ?
`, name).Scan(&status)
	})
	c.Assert(err, tc.ErrorIsNil)

	statusValue, err := domainstatus.EncodeCloudInstanceStatus(expected)
	c.Assert(err, tc.ErrorIsNil)
	c.Check(status, tc.Equals, statusValue)
}
