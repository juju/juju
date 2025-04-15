// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state

import (
	"context"
	"database/sql"
	"fmt"
	"strconv"

	"github.com/canonical/sqlair"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/core/machine"
	domainapplication "github.com/juju/juju/domain/application"
	applicationerrors "github.com/juju/juju/domain/application/errors"
	"github.com/juju/juju/domain/deployment"
	"github.com/juju/juju/domain/sequence"
	"github.com/juju/juju/internal/errors"
)

func (s *unitStateSuite) TestPlaceNetNodeMachinesInvalidPlacement(c *gc.C) {
	err := s.TxnRunner().Txn(context.Background(), func(ctx context.Context, tx *sqlair.TX) error {
		_, err := s.state.placeMachine(ctx, tx, deployment.Placement{
			Type: deployment.PlacementType(666),
		})
		return err
	})
	c.Assert(err, gc.ErrorMatches, `invalid placement type: 666`)
}

func (s *unitStateSuite) TestPlaceNetNodeMachinesUnset(c *gc.C) {
	// Ensure the machine got created.

	var netNode string
	err := s.TxnRunner().Txn(context.Background(), func(ctx context.Context, tx *sqlair.TX) error {
		var err error
		netNode, err = s.state.placeMachine(ctx, tx, deployment.Placement{
			Type: deployment.PlacementTypeUnset,
		})
		return err
	})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(netNode, gc.Not(gc.Equals), "")

	var resultNetNode string
	err = s.TxnRunner().Txn(context.Background(), func(ctx context.Context, tx *sqlair.TX) error {
		var err error
		resultNetNode, err = s.state.getMachineNetNodeUUIDFromName(ctx, tx, machine.Name("0"))
		return err
	})
	c.Assert(err, jc.ErrorIsNil)
	c.Check(resultNetNode, gc.Equals, netNode)

	s.ensureSequenceForMachineNamespace(c, 0)
}

func (s *unitStateSuite) TestPlaceNetNodeMachinesUnsetMultipleTimes(c *gc.C) {
	total := 10

	// Ensure that the machines are sequenced correctly.
	// The first machine should be 0, the second 1, and so on.

	var netNodes []string
	err := s.TxnRunner().Txn(context.Background(), func(ctx context.Context, tx *sqlair.TX) error {
		for range total {
			netNode, err := s.state.placeMachine(ctx, tx, deployment.Placement{
				Type: deployment.PlacementTypeUnset,
			})
			if err != nil {
				return err
			}
			netNodes = append(netNodes, netNode)

		}
		return nil
	})
	c.Assert(err, jc.ErrorIsNil)

	var resultNetNodes []string
	err = s.TxnRunner().Txn(context.Background(), func(ctx context.Context, tx *sqlair.TX) error {
		for i := range total {
			netNode, err := s.state.getMachineNetNodeUUIDFromName(ctx, tx, machine.Name(strconv.Itoa(i)))
			if err != nil {
				return err
			}
			resultNetNodes = append(resultNetNodes, netNode)
		}
		return nil
	})
	c.Assert(err, jc.ErrorIsNil)
	c.Check(resultNetNodes, gc.DeepEquals, netNodes)

	s.ensureSequenceForMachineNamespace(c, total-1)
}

func (s *unitStateSuite) TestPlaceNetNodeMachinesUnsetMultipleTimesWithGaps(c *gc.C) {
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
		err := s.TxnRunner().Txn(context.Background(), func(ctx context.Context, tx *sqlair.TX) error {
			for range stepTotal {
				netNode, err := s.state.placeMachine(ctx, tx, deployment.Placement{
					Type: deployment.PlacementTypeUnset,
				})
				if err != nil {
					return err
				}
				netNodes = append(netNodes, netNode)
			}
			return nil
		})
		c.Assert(err, jc.ErrorIsNil)
	}
	deleteLastMachine := func() {
		err := s.TxnRunner().StdTxn(context.Background(), func(ctx context.Context, tx *sql.Tx) error {
			_, err := tx.Exec("DELETE FROM machine WHERE net_node_uuid = ?", netNodes[len(netNodes)-1])
			return err
		})
		c.Assert(err, jc.ErrorIsNil)

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
	err := s.TxnRunner().Txn(context.Background(), func(ctx context.Context, tx *sqlair.TX) error {
		for i := range stepTotal * 2 {
			netNode, err := s.state.getMachineNetNodeUUIDFromName(ctx, tx, machine.Name(strconv.Itoa(i)))
			if errors.Is(err, applicationerrors.MachineNotFound) {
				if i == stepTotal-1 {
					// This machine was deleted, this is fine.
					c.Logf("machine %d not found, this is expected", i)
					continue
				}
				return err
			} else if err != nil {
				return err
			}

			resultNetNodes = append(resultNetNodes, netNode)
		}
		return nil
	})
	c.Assert(err, jc.ErrorIsNil)
	c.Check(resultNetNodes, gc.DeepEquals, netNodes)
}

func (s *unitStateSuite) TestPlaceNetNodeMachinesExistingMachine(c *gc.C) {
	// Create the machine, then try to place it on the same machine.

	var netNode string
	err := s.TxnRunner().Txn(context.Background(), func(ctx context.Context, tx *sqlair.TX) error {
		var err error
		netNode, err = s.state.placeMachine(ctx, tx, deployment.Placement{
			Type: deployment.PlacementTypeUnset,
		})
		return err
	})
	c.Assert(err, jc.ErrorIsNil)

	var resultNetNode string
	err = s.TxnRunner().Txn(context.Background(), func(ctx context.Context, tx *sqlair.TX) error {
		var err error
		resultNetNode, err = s.state.placeMachine(ctx, tx, deployment.Placement{
			Type:      deployment.PlacementTypeMachine,
			Directive: "0",
		})
		return err
	})
	c.Assert(err, jc.ErrorIsNil)
	c.Check(resultNetNode, gc.Equals, netNode)
}

func (s *unitStateSuite) TestPlaceNetNodeMachinesExistingMachineNotFound(c *gc.C) {
	// Try and place a machine that doesn't exist.

	err := s.TxnRunner().Txn(context.Background(), func(ctx context.Context, tx *sqlair.TX) error {
		_, err := s.state.placeMachine(ctx, tx, deployment.Placement{
			Type:      deployment.PlacementTypeMachine,
			Directive: "0",
		})
		return err
	})
	c.Assert(err, jc.ErrorIs, applicationerrors.MachineNotFound)
}

func (s *unitStateSuite) TestPlaceNetNodeMachinesContainer(c *gc.C) {
	// Ensure the parent and child machine got created.

	var netNode string
	err := s.TxnRunner().Txn(context.Background(), func(ctx context.Context, tx *sqlair.TX) error {
		var err error
		netNode, err = s.state.placeMachine(ctx, tx, deployment.Placement{
			Type:      deployment.PlacementTypeContainer,
			Container: deployment.ContainerTypeLXD,
		})
		return err
	})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(netNode, gc.Not(gc.Equals), "")

	var resultNetNode string
	err = s.TxnRunner().Txn(context.Background(), func(ctx context.Context, tx *sqlair.TX) error {
		var err error
		resultNetNode, err = s.state.getMachineNetNodeUUIDFromName(ctx, tx, machine.Name("0/lxd/0"))
		return err
	})
	c.Assert(err, jc.ErrorIsNil)
	c.Check(resultNetNode, gc.Equals, netNode)

	s.ensureSequenceForMachineNamespace(c, 0)
	s.ensureSequenceForContainerNamespace(c, machine.Name("0"), 0)
}

func (s *unitStateSuite) TestPlaceNetNodeMachinesContainerMultipleTimes(c *gc.C) {
	total := 10

	// Ensure that the machines are sequenced correctly.
	// The first machine should be 0/lxd/0, the second 1/lxd/0, and so on.

	var netNodes []string
	err := s.TxnRunner().Txn(context.Background(), func(ctx context.Context, tx *sqlair.TX) error {
		for range total {
			netNode, err := s.state.placeMachine(ctx, tx, deployment.Placement{
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
	c.Assert(err, jc.ErrorIsNil)

	var resultNetNodes []string
	err = s.TxnRunner().Txn(context.Background(), func(ctx context.Context, tx *sqlair.TX) error {
		for i := range total {
			name := fmt.Sprintf("%d/lxd/0", i)
			netNode, err := s.state.getMachineNetNodeUUIDFromName(ctx, tx, machine.Name(name))
			if err != nil {
				return err
			}
			resultNetNodes = append(resultNetNodes, netNode)
		}
		return nil
	})
	c.Assert(err, jc.ErrorIsNil)
	c.Check(resultNetNodes, gc.DeepEquals, netNodes)

	s.ensureSequenceForMachineNamespace(c, total-1)
	for i := range total {
		s.ensureSequenceForContainerNamespace(c, machine.Name(strconv.Itoa(i)), 0)
	}
}

func (s *unitStateSuite) TestPlaceNetNodeMachinesContainerMultipleTimesWithGaps(c *gc.C) {
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
		err := s.TxnRunner().Txn(context.Background(), func(ctx context.Context, tx *sqlair.TX) error {
			for range stepTotal {
				netNode, err := s.state.placeMachine(ctx, tx, deployment.Placement{
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
		c.Assert(err, jc.ErrorIsNil)
	}
	deleteLastMachine := func() {
		// Delete the parent and the child.
		err := s.TxnRunner().StdTxn(context.Background(), func(ctx context.Context, tx *sql.Tx) error {
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
		c.Assert(err, jc.ErrorIsNil)

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
	err := s.TxnRunner().Txn(context.Background(), func(ctx context.Context, tx *sqlair.TX) error {
		for i := range stepTotal * 2 {
			name := fmt.Sprintf("%d/lxd/0", i)
			netNode, err := s.state.getMachineNetNodeUUIDFromName(ctx, tx, machine.Name(name))
			if errors.Is(err, applicationerrors.MachineNotFound) {
				if i == stepTotal-1 {
					// This machine was deleted, this is fine.
					c.Logf("machine %d not found, this is expected", i)
					continue
				}
				return err
			} else if err != nil {
				return err
			}

			resultNetNodes = append(resultNetNodes, netNode)
		}
		return nil
	})
	c.Assert(err, jc.ErrorIsNil)
	c.Check(resultNetNodes, gc.DeepEquals, netNodes)
}

func (s *unitStateSuite) TestPlaceNetNodeMachinesProvider(c *gc.C) {
	// Ensure that the parent placement is correctly set on the
	// machine_placement table.

	var netNode string
	err := s.TxnRunner().Txn(context.Background(), func(ctx context.Context, tx *sqlair.TX) error {
		var err error
		netNode, err = s.state.placeMachine(ctx, tx, deployment.Placement{
			Type:      deployment.PlacementTypeProvider,
			Directive: "zone=eu-west-1",
		})
		return err
	})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(netNode, gc.Not(gc.Equals), "")

	var resultNetNode string
	err = s.TxnRunner().Txn(context.Background(), func(ctx context.Context, tx *sqlair.TX) error {
		var err error
		resultNetNode, err = s.state.getMachineNetNodeUUIDFromName(ctx, tx, machine.Name("0"))
		return err
	})
	c.Assert(err, jc.ErrorIsNil)
	c.Check(resultNetNode, gc.Equals, netNode)

	s.ensureSequenceForMachineNamespace(c, 0)

	var directive string
	err = s.TxnRunner().StdTxn(context.Background(), func(ctx context.Context, tx *sql.Tx) error {
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
	c.Assert(err, jc.ErrorIsNil)
	c.Check(directive, gc.Equals, "zone=eu-west-1")
}

func (s *unitStateSuite) ensureSequenceForMachineNamespace(c *gc.C, expected int) {
	var seq int
	err := s.TxnRunner().StdTxn(context.Background(), func(ctx context.Context, tx *sql.Tx) error {
		namespace := domainapplication.MachineSequenceNamespace
		return tx.QueryRow("SELECT value FROM sequence WHERE namespace = ?", namespace).Scan(&seq)
	})
	c.Assert(err, jc.ErrorIsNil)
	c.Check(seq, gc.Equals, expected)
}

func (s *unitStateSuite) ensureSequenceForContainerNamespace(c *gc.C, parentName machine.Name, expected int) {
	var seq int
	err := s.TxnRunner().StdTxn(context.Background(), func(ctx context.Context, tx *sql.Tx) error {
		namespace := sequence.MakePrefixNamespace(domainapplication.ContainerSequenceNamespace, parentName.String()).String()
		return tx.QueryRow("SELECT value FROM sequence WHERE namespace = ?", namespace).Scan(&seq)
	})
	c.Assert(err, jc.ErrorIsNil)
	c.Check(seq, gc.Equals, expected)
}
