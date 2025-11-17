// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state

import (
	"context"
	"database/sql"
	"fmt"
	"strconv"
	"testing"

	"github.com/canonical/sqlair"
	"github.com/juju/clock"
	"github.com/juju/tc"

	"github.com/juju/juju/core/arch"
	"github.com/juju/juju/core/instance"
	"github.com/juju/juju/core/machine"
	machinetesting "github.com/juju/juju/core/machine/testing"
	"github.com/juju/juju/domain/application/architecture"
	applicationerrors "github.com/juju/juju/domain/application/errors"
	"github.com/juju/juju/domain/constraints"
	"github.com/juju/juju/domain/deployment"
	domainmachine "github.com/juju/juju/domain/machine"
	machineerrors "github.com/juju/juju/domain/machine/errors"
	domainnetwork "github.com/juju/juju/domain/network"
	schematesting "github.com/juju/juju/domain/schema/testing"
	"github.com/juju/juju/domain/sequence"
	domainstatus "github.com/juju/juju/domain/status"
	"github.com/juju/juju/internal/errors"
	loggertesting "github.com/juju/juju/internal/logger/testing"
)

type placementSuite struct {
	schematesting.ModelSuite
	st *State
}

func TestPlacementSuite(t *testing.T) {
	tc.Run(t, &placementSuite{})
}

func (s *placementSuite) SetUpTest(c *tc.C) {
	s.ModelSuite.SetUpTest(c)
	s.st = NewState(s.TxnRunnerFactory(), clock.WallClock, loggertesting.WrapCheckLog(c))
}

func (s *placementSuite) TestPlaceNetNodeMachinesInvalidPlacement(c *tc.C) {
	err := s.TxnRunner().Txn(c.Context(), func(ctx context.Context, tx *sqlair.TX) error {
		_, err := PlaceMachine(ctx, tx, s.st, clock.WallClock, domainmachine.PlaceMachineArgs{
			Directive: deployment.Placement{
				Type: deployment.PlacementType(666),
			},
		})
		return err
	})
	c.Assert(err, tc.ErrorMatches, `invalid placement type: 666`)
}

func (s *placementSuite) TestPlaceNetNodeMachinesUnset(c *tc.C) {
	netNodeUUID := tc.Must(c, domainnetwork.NewNetNodeUUID)
	machineUUID := machinetesting.GenUUID(c)

	var machineNames []machine.Name
	err := s.TxnRunner().Txn(c.Context(), func(ctx context.Context, tx *sqlair.TX) error {
		var err error
		machineNames, err = PlaceMachine(
			ctx, tx, s.st, clock.WallClock, domainmachine.PlaceMachineArgs{
				Directive: deployment.Placement{
					Type: deployment.PlacementTypeUnset,
				},
				MachineUUID: machineUUID,
				NetNodeUUID: netNodeUUID,
			})
		return err
	})
	c.Assert(err, tc.ErrorIsNil)

	s.checkMachineNetNode(c, machine.Name("0"), netNodeUUID.String())

	s.checkSequenceForMachineNamespace(c, 0)

	s.checkStatusForMachine(c, machine.Name("0"), domainstatus.MachineStatusPending)
	s.checkStatusForMachineInstance(c, machine.Name("0"), domainstatus.InstanceStatusPending)

	s.checkPlatformForMachine(c, machine.Name("0"), deployment.Platform{})
	s.checkContainerTypeForMachine(c, machine.Name("0"), "lxd")

	c.Assert(machineNames, tc.HasLen, 1)
	c.Check(machineNames[0], tc.Equals, machine.Name("0"))
}

func (s *placementSuite) TestPlaceNetNodeMachinesUnsetWithPlatform(c *tc.C) {
	// Ensure the machine got created.
	platform := deployment.Platform{
		OSType:       deployment.Ubuntu,
		Channel:      "22.04",
		Architecture: architecture.ARM64,
	}
	netNodeUUID := tc.Must(c, domainnetwork.NewNetNodeUUID)
	machineUUID := machinetesting.GenUUID(c)

	err := s.TxnRunner().Txn(c.Context(), func(ctx context.Context, tx *sqlair.TX) error {
		_, err := PlaceMachine(
			ctx, tx, s.st, clock.WallClock, domainmachine.PlaceMachineArgs{
				Directive: deployment.Placement{
					Type: deployment.PlacementTypeUnset,
				},
				NetNodeUUID: netNodeUUID,
				MachineUUID: machineUUID,
				Platform:    platform,
			})
		return err
	})
	c.Assert(err, tc.ErrorIsNil)

	s.checkPlatformForMachine(c, machine.Name("0"), platform)
}

func (s *placementSuite) TestPlaceNetNodeMachinesUnsetWithNonce(c *tc.C) {
	nonce := ptr("test-nonce")
	netNodeUUID := tc.Must(c, domainnetwork.NewNetNodeUUID)
	machineUUID := machinetesting.GenUUID(c)

	var (
		machineNames []machine.Name
		err          error
	)
	err = s.TxnRunner().Txn(c.Context(), func(ctx context.Context, tx *sqlair.TX) error {
		machineNames, err = PlaceMachine(ctx, tx, s.st, clock.WallClock, domainmachine.PlaceMachineArgs{
			Directive: deployment.Placement{
				Type: deployment.PlacementTypeUnset,
			},
			MachineUUID: machineUUID,
			NetNodeUUID: netNodeUUID,
			Nonce:       nonce,
		})
		return err
	})
	c.Assert(err, tc.ErrorIsNil)

	s.checkNonceForMachine(c, machineNames[0], nonce)
}

func (s *placementSuite) TestPlaceNetNodeMachinesUnsetWithPlatformMissingArchitecture(c *tc.C) {
	// Ensure the machine got created.
	platform := deployment.Platform{
		OSType:  deployment.Ubuntu,
		Channel: "22.04",
	}
	netNodeUUID := tc.Must(c, domainnetwork.NewNetNodeUUID)
	machineUUID := machinetesting.GenUUID(c)

	err := s.TxnRunner().Txn(c.Context(), func(ctx context.Context, tx *sqlair.TX) error {
		_, err := PlaceMachine(ctx, tx, s.st, clock.WallClock, domainmachine.PlaceMachineArgs{
			Directive: deployment.Placement{
				Type: deployment.PlacementTypeUnset,
			},
			MachineUUID: machineUUID,
			NetNodeUUID: netNodeUUID,
			Platform:    platform,
		})
		return err
	})
	c.Assert(err, tc.ErrorIsNil)

	s.checkPlatformForMachine(c, machine.Name("0"), platform)
}

func (s *placementSuite) TestPlaceNetNodeMachinesUnsetWithPlatformMissingBase(c *tc.C) {
	// Ensure the machine got created.
	platform := deployment.Platform{
		Architecture: architecture.ARM64,
	}
	netNodeUUID := tc.Must(c, domainnetwork.NewNetNodeUUID)
	machineUUID := machinetesting.GenUUID(c)

	err := s.TxnRunner().Txn(c.Context(), func(ctx context.Context, tx *sqlair.TX) error {
		_, err := PlaceMachine(ctx, tx, s.st, clock.WallClock, domainmachine.PlaceMachineArgs{
			Directive: deployment.Placement{
				Type: deployment.PlacementTypeUnset,
			},
			MachineUUID: machineUUID,
			NetNodeUUID: netNodeUUID,
			Platform:    platform,
		})
		return err
	})
	c.Assert(err, tc.ErrorIsNil)

	s.checkPlatformForMachine(c, machine.Name("0"), platform)
}

func (s *placementSuite) TestPlaceNetNodeMachinesUnsetMultipleTimes(c *tc.C) {
	// Ensure that the machines are sequenced correctly.
	// The first machine should be 0, the second 1, and so on.

	total := 10
	netNodes := make([]string, 0, total)
	err := s.TxnRunner().Txn(c.Context(), func(ctx context.Context, tx *sqlair.TX) error {
		for range total {
			netNodeUUID := tc.Must(c, domainnetwork.NewNetNodeUUID)
			_, err := PlaceMachine(ctx, tx, s.st, clock.WallClock, domainmachine.PlaceMachineArgs{
				Directive: deployment.Placement{
					Type: deployment.PlacementTypeUnset,
				},
				MachineUUID: machinetesting.GenUUID(c),
				NetNodeUUID: netNodeUUID,
			})
			if err != nil {
				return err
			}
			netNodes = append(netNodes, netNodeUUID.String())

		}
		return nil
	})
	c.Assert(err, tc.ErrorIsNil)

	for i := range total {
		s.checkMachineNetNode(c, machine.Name(strconv.Itoa(i)), netNodes[i])
	}

	s.checkSequenceForMachineNamespace(c, total-1)
}

func (s *placementSuite) TestPlaceNetNodeMachinesUnsetMultipleTimesWithGaps(c *tc.C) {
	stepTotal := 5

	// Ensure that the machines are sequenced correctly.
	// The first machine should be 0, the second 1, and so on, then delete
	// the the last machine. The following sequence should continue from
	// the last machine, leaving gaps in the sequence.
	// The final sequence should be:
	//
	// 0, 1, 2, 3, 5, 6, 7, 8, 9

	netNodes := make([]string, 0, stepTotal*2)
	createMachines := func() {
		err := s.TxnRunner().Txn(c.Context(), func(ctx context.Context, tx *sqlair.TX) error {
			for range stepTotal {
				netNodeUUID := tc.Must(c, domainnetwork.NewNetNodeUUID)
				_, err := PlaceMachine(ctx, tx, s.st, clock.WallClock, domainmachine.PlaceMachineArgs{
					Directive: deployment.Placement{
						Type: deployment.PlacementTypeUnset,
					},
					MachineUUID: machinetesting.GenUUID(c),
					NetNodeUUID: netNodeUUID,
				})
				if err != nil {
					return err
				}
				netNodes = append(netNodes, netNodeUUID.String())
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
				"machine_constraint",
				"machine_status",
				"machine_cloud_instance_status",
				"machine_cloud_instance",
				"machine_platform",
				"machine_container_type",
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
	s.checkSequenceForMachineNamespace(c, stepTotal-1)

	deleteLastMachine()
	s.checkSequenceForMachineNamespace(c, stepTotal-1)

	createMachines()
	s.checkSequenceForMachineNamespace(c, (stepTotal*2)-1)

	for i := range stepTotal * 2 {
		var nn string
		err := s.TxnRunner().StdTxn(c.Context(), func(ctx context.Context, tx *sql.Tx) error {
			return tx.QueryRow(`
SELECT m.net_node_uuid
FROM machine AS m
WHERE m.name = ?
`, strconv.Itoa(i)).Scan(&nn)
		})
		if errors.Is(err, sqlair.ErrNoRows) && i == stepTotal-1 {
			// This machine was deleted, this is fine.
			c.Logf("machine %d not found, this is expected", i)
			continue
		}
		c.Assert(err, tc.ErrorIsNil)

	}
}

func (s *placementSuite) TestPlaceNetNodeMachinesContainer(c *tc.C) {
	// Ensure the parent and child machine got created.
	var (
		netNodeUUID  = tc.Must(c, domainnetwork.NewNetNodeUUID)
		machineNames []machine.Name
	)
	err := s.TxnRunner().Txn(c.Context(), func(ctx context.Context, tx *sqlair.TX) error {
		var err error
		machineNames, err = PlaceMachine(
			ctx, tx, s.st, clock.WallClock, domainmachine.PlaceMachineArgs{
				Directive: deployment.Placement{
					Type:      deployment.PlacementTypeContainer,
					Container: deployment.ContainerTypeLXD,
				},
				MachineUUID: machinetesting.GenUUID(c),
				NetNodeUUID: netNodeUUID,
				Nonce:       ptr("nonce-ense"),
			})
		return err
	})
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(netNodeUUID, tc.Not(tc.Equals), "")

	s.checkMachineNetNode(c, machine.Name("0/lxd/0"), netNodeUUID.String())

	s.checkSequenceForMachineNamespace(c, 0)
	s.checkSequenceForContainerNamespace(c, "0", 0)

	c.Assert(machineNames, tc.HasLen, 2)
	c.Check(machineNames, tc.DeepEquals, []machine.Name{
		machine.Name("0"),
		machine.Name("0/lxd/0"),
	})

	// Check the nonce.
	s.checkNonceForMachine(c, machine.Name("0"), nil)
	s.checkNonceForMachine(c, machine.Name("0/lxd/0"), ptr("nonce-ense"))
}

func (s *placementSuite) TestPlaceNetNodeMachinesContainerWithDirective(c *tc.C) {
	// Ensure the child machine got created on the parent machine.

	// Insert a machine with no placement, then place a container on it.
	err := s.TxnRunner().Txn(c.Context(), func(ctx context.Context, tx *sqlair.TX) error {
		_, err := PlaceMachine(
			ctx, tx, s.st, clock.WallClock, domainmachine.PlaceMachineArgs{
				Directive: deployment.Placement{
					Type: deployment.PlacementTypeUnset,
				},
				MachineUUID: machinetesting.GenUUID(c),
				NetNodeUUID: tc.Must(c, domainnetwork.NewNetNodeUUID),
			})
		return err
	})
	c.Assert(err, tc.ErrorIsNil)

	var (
		netNodeUUID  = tc.Must(c, domainnetwork.NewNetNodeUUID)
		machineNames []machine.Name
	)
	err = s.TxnRunner().Txn(c.Context(), func(ctx context.Context, tx *sqlair.TX) error {
		var err error
		machineNames, err = PlaceMachine(ctx, tx, s.st, clock.WallClock, domainmachine.PlaceMachineArgs{
			Directive: deployment.Placement{
				Type:      deployment.PlacementTypeContainer,
				Container: deployment.ContainerTypeLXD,
				Directive: "0",
			},
			MachineUUID: machinetesting.GenUUID(c),
			NetNodeUUID: netNodeUUID,
		})
		return err
	})
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(netNodeUUID, tc.Not(tc.Equals), "")

	s.checkMachineNetNode(c, machine.Name("0/lxd/0"), netNodeUUID.String())

	s.checkSequenceForMachineNamespace(c, 0)
	s.checkSequenceForContainerNamespace(c, "0", 0)

	c.Assert(machineNames, tc.HasLen, 2)
	c.Check(machineNames, tc.DeepEquals, []machine.Name{
		machine.Name("0"),
		machine.Name("0/lxd/0"),
	})
}

func (s *placementSuite) TestPlaceNetNodeMachinesContainerInvalidArch(c *tc.C) {
	// Arrange: create a machine with an arch of arm64.
	err := s.TxnRunner().Txn(c.Context(), func(ctx context.Context, tx *sqlair.TX) error {
		_, err := PlaceMachine(ctx, tx, s.st, clock.WallClock, domainmachine.PlaceMachineArgs{
			Directive: deployment.Placement{
				Type: deployment.PlacementTypeUnset,
			},
			Platform: deployment.Platform{
				OSType:       deployment.Ubuntu,
				Channel:      "22.04",
				Architecture: architecture.ARM64,
			},
		})
		return err
	})
	c.Assert(err, tc.ErrorIsNil)

	// Arrange: Mark the machine hw characteristics as arm64. This is needed
	// since this is only filled in when the instance is created.
	err = s.TxnRunner().StdTxn(c.Context(), func(ctx context.Context, tx *sql.Tx) error {
		_, err := tx.ExecContext(ctx, `UPDATE machine_cloud_instance SET arch = "arm64"`)
		return err
	})
	c.Assert(err, tc.ErrorIsNil)

	// Act: place something in a container on the machine, with a constrained arch of amd64.
	err = s.TxnRunner().Txn(c.Context(), func(ctx context.Context, tx *sqlair.TX) error {
		_, err := PlaceMachine(ctx, tx, s.st, clock.WallClock, domainmachine.PlaceMachineArgs{
			Directive: deployment.Placement{
				Type:      deployment.PlacementTypeContainer,
				Container: deployment.ContainerTypeLXD,
				Directive: "0",
			},
			Constraints: constraints.Constraints{
				Arch: ptr(arch.AMD64),
			},
		})
		return err
	})
	// Assert: the placement fails with an error.
	c.Assert(err, tc.ErrorIs, machineerrors.MachineConstraintViolation)
}

func (s *placementSuite) TestPlaceNetNodeMachinesContainerWithDirectiveMachineNotFound(c *tc.C) {
	err := s.TxnRunner().Txn(c.Context(), func(ctx context.Context, tx *sqlair.TX) error {
		_, err := PlaceMachine(ctx, tx, s.st, clock.WallClock, domainmachine.PlaceMachineArgs{
			Directive: deployment.Placement{
				Type:      deployment.PlacementTypeContainer,
				Container: deployment.ContainerTypeLXD,
				Directive: "1",
			},
			MachineUUID: machinetesting.GenUUID(c),
			NetNodeUUID: tc.Must(c, domainnetwork.NewNetNodeUUID),
		})
		return err
	})
	c.Assert(err, tc.ErrorIs, applicationerrors.MachineNotFound)
}

func (s *placementSuite) TestPlaceNetNodeMachinesContainerMultipleTimes(c *tc.C) {
	total := 10

	// Ensure that the machines are sequenced correctly.
	// The first machine should be 0/lxd/0, the second 1/lxd/0, and so on.

	netNodes := make([]string, 0, total)
	err := s.TxnRunner().Txn(c.Context(), func(ctx context.Context, tx *sqlair.TX) error {
		for range total {
			netNodeUUID := tc.Must(c, domainnetwork.NewNetNodeUUID)
			_, err := PlaceMachine(ctx, tx, s.st, clock.WallClock, domainmachine.PlaceMachineArgs{
				Directive: deployment.Placement{
					Type:      deployment.PlacementTypeContainer,
					Container: deployment.ContainerTypeLXD,
				},
				MachineUUID: machinetesting.GenUUID(c),
				NetNodeUUID: netNodeUUID,
			})
			if err != nil {
				return err
			}
			netNodes = append(netNodes, netNodeUUID.String())
		}
		return nil
	})
	c.Assert(err, tc.ErrorIsNil)

	for i := range total {
		name := fmt.Sprintf("%d/lxd/0", i)
		s.checkMachineNetNode(c, machine.Name(name), netNodes[i])
	}

	s.checkSequenceForMachineNamespace(c, total-1)
	for i := range total {
		s.checkSequenceForContainerNamespace(c, machine.Name(strconv.Itoa(i)), 0)
	}
}

func (s *placementSuite) TestPlaceNetNodeMachinesContainerMultipleTimesWithGaps(c *tc.C) {
	stepTotal := 5

	// Ensure that the machines are sequenced correctly. The first machine
	// should be 0, the second 1, and so on, then delete the the last machine.
	// The following sequence should continue from the last machine, leaving
	// gaps in the sequence. The final sequence should be:
	//
	// 0/lxd/0, 1/lxd/0, 2/lxd/0, 3/lxd/0, 5/lxd/0, 6/lxd/0, 7/lxd/0, 8/lxd/0,
	// 9/lxd/0

	netNodes := make([]string, 0, stepTotal*2)
	createMachines := func() {
		err := s.TxnRunner().Txn(c.Context(), func(ctx context.Context, tx *sqlair.TX) error {
			for range stepTotal {
				netNodeUUID := tc.Must(c, domainnetwork.NewNetNodeUUID)
				_, err := PlaceMachine(ctx, tx, s.st, clock.WallClock, domainmachine.PlaceMachineArgs{
					Directive: deployment.Placement{
						Type:      deployment.PlacementTypeContainer,
						Container: deployment.ContainerTypeLXD,
					},
					MachineUUID: machinetesting.GenUUID(c),
					NetNodeUUID: netNodeUUID,
				})
				if err != nil {
					return err
				}
				netNodes = append(netNodes, netNodeUUID.String())
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
				"machine_constraint",
				"machine_status",
				"machine_cloud_instance_status",
				"machine_cloud_instance",
				"machine_platform",
				"machine_container_type",
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
	s.checkSequenceForMachineNamespace(c, stepTotal-1)

	deleteLastMachine()
	s.checkSequenceForMachineNamespace(c, stepTotal-1)

	createMachines()
	s.checkSequenceForMachineNamespace(c, (stepTotal*2)-1)

	for i := range stepTotal * 2 {
		name := fmt.Sprintf("%d/lxd/0", i)
		var nn string
		err := s.TxnRunner().StdTxn(c.Context(), func(ctx context.Context, tx *sql.Tx) error {
			return tx.QueryRow(`
SELECT m.net_node_uuid
FROM machine AS m
WHERE m.name = ?
`, name).Scan(&nn)
		})
		if errors.Is(err, sqlair.ErrNoRows) && i == stepTotal-1 {
			// This machine was deleted, this is fine.
			c.Logf("machine %d not found, this is expected", i)
			continue
		}
		c.Assert(err, tc.ErrorIsNil)
	}
}

func (s *placementSuite) TestPlaceNetNodeMachinesProvider(c *tc.C) {
	// Ensure that the parent placement is correctly set on the
	// machine_placement table.

	netNodeUUID := tc.Must(c, domainnetwork.NewNetNodeUUID)
	err := s.TxnRunner().Txn(c.Context(), func(ctx context.Context, tx *sqlair.TX) error {
		var err error
		_, err = PlaceMachine(ctx, tx, s.st, clock.WallClock, domainmachine.PlaceMachineArgs{
			Directive: deployment.Placement{
				Type:      deployment.PlacementTypeProvider,
				Directive: "zone=eu-west-1",
			},
			NetNodeUUID: netNodeUUID,
		})
		return err
	})
	c.Assert(err, tc.ErrorIsNil)
	s.checkMachineNetNode(c, machine.Name("0"), netNodeUUID.String())

	s.checkSequenceForMachineNamespace(c, 0)

	var directive string
	err = s.TxnRunner().StdTxn(c.Context(), func(ctx context.Context, tx *sql.Tx) error {
		err := tx.QueryRow(`
SELECT mp.directive
FROM machine m
JOIN machine_placement AS mp ON m.uuid = mp.machine_uuid
WHERE m.net_node_uuid = ?
`, netNodeUUID.String()).Scan(&directive)
		if err != nil {
			return err
		}
		return nil
	})
	c.Assert(err, tc.ErrorIsNil)
	c.Check(directive, tc.Equals, "zone=eu-west-1")
}

func (s *placementSuite) TestPlaceMachineWithSpacesConstraint(c *tc.C) {
	// Arrange: Create spaces.
	_, err := s.DB().ExecContext(c.Context(), `
INSERT INTO space (uuid, name) VALUES
	(?, ?),
	(?, ?)`,
		"0", "space1",
		"1", "space2",
	)
	c.Assert(err, tc.ErrorIsNil)

	// Arrange: Create a machine with space constraints.
	err = s.TxnRunner().Txn(c.Context(), func(ctx context.Context, tx *sqlair.TX) error {
		_, err := PlaceMachine(ctx, tx, s.st, clock.WallClock, domainmachine.PlaceMachineArgs{
			Directive: deployment.Placement{
				Type: deployment.PlacementTypeUnset,
			},
			MachineUUID: machinetesting.GenUUID(c),
			NetNodeUUID: tc.Must(c, domainnetwork.NewNetNodeUUID),
			Constraints: constraints.Constraints{
				Spaces: ptr([]constraints.SpaceConstraint{
					{SpaceName: "space1", Exclude: false},
					{SpaceName: "space2", Exclude: true},
				}),
			},
		})
		return err
	})
	// Assert: the placement succeeds.
	c.Assert(err, tc.ErrorIsNil)

	// Act: Try to place a machine with a non-existent space constraint.
	err = s.TxnRunner().Txn(c.Context(), func(ctx context.Context, tx *sqlair.TX) error {
		_, err := PlaceMachine(ctx, tx, s.st, clock.WallClock, domainmachine.PlaceMachineArgs{
			Directive: deployment.Placement{
				Type: deployment.PlacementTypeUnset,
			},
			MachineUUID: machinetesting.GenUUID(c),
			NetNodeUUID: tc.Must(c, domainnetwork.NewNetNodeUUID),
			Constraints: constraints.Constraints{
				Spaces: ptr([]constraints.SpaceConstraint{
					{SpaceName: "nonexistent-space", Exclude: false},
				}),
			},
		})
		return err
	})
	// Assert: the placement fails with an error about the missing space.
	c.Assert(err, tc.ErrorMatches, `.*space "nonexistent-space" does not exist.*`)
	c.Assert(err, tc.ErrorIs, machineerrors.InvalidMachineConstraints)
}

// TestCreateMachineWithName_PopulatesHardwareCharacteristics verifies that machines can be created
// with their hardware characteristics. This is required for the manual provider.
func (s *placementSuite) TestCreateMachineWithName_PopulatesHardwareCharacteristics(c *tc.C) {
	netNodeUUID := tc.Must(c, domainnetwork.NewNetNodeUUID)
	machineUUID1 := machinetesting.GenUUID(c)
	expectedAzUUID := machinetesting.GenUUID(c)

	azName := "amazing-canonical-cloud-1a"
	arch := "arm64"
	mem := uint64(4096)
	rootDisk := uint64(8192)
	rootDiskSource := "local"
	cpuCores := uint64(42)
	cpuPower := uint64(4200)
	virtType := "kvm"

	characteristics := instance.HardwareCharacteristics{
		Arch:             &arch,
		Mem:              &mem,
		RootDisk:         &rootDisk,
		RootDiskSource:   &rootDiskSource,
		CpuCores:         &cpuCores,
		CpuPower:         &cpuPower,
		AvailabilityZone: &azName,
		VirtType:         &virtType,
	}

	// Setup the AZ.
	err := s.TxnRunner().StdTxn(c.Context(), func(ctx context.Context, tx *sql.Tx) error {
		_, err := tx.ExecContext(
			ctx,
			"INSERT INTO availability_zone (name, uuid) VALUES (?, ?)",
			azName,
			expectedAzUUID.String(),
		)
		return err
	})
	c.Assert(err, tc.ErrorIsNil)

	err = s.TxnRunner().Txn(c.Context(), func(ctx context.Context, tx *sqlair.TX) error {
		var err error

		err = CreateMachineWithName(
			ctx,
			tx,
			s.st,
			clock.WallClock,
			"0",
			CreateMachineArgs{
				MachineUUID: machineUUID1.String(),
				NetNodeUUID: netNodeUUID.String(),
				Platform: deployment.Platform{
					Architecture: architecture.ARM64,
				},
				HardwareCharacteristics: characteristics,
			},
		)

		return err
	})
	c.Assert(err, tc.ErrorIsNil)

	// Verify the hardware characteristics were stored. Just verify AZ uuid set,
	// we know rest is fine.
	var azUuid string
	err = s.TxnRunner().StdTxn(c.Context(), func(ctx context.Context, tx *sql.Tx) error {
		err := tx.QueryRow(`
SELECT availability_zone_uuid
FROM machine_cloud_instance m
WHERE m.machine_uuid = ?
`, machineUUID1.String()).Scan(&azUuid)
		if err != nil {
			return err
		}
		return nil
	})
	c.Assert(err, tc.ErrorIsNil)
	c.Check(azUuid, tc.Equals, expectedAzUUID.String())
}

func (s *placementSuite) checkSequenceForMachineNamespace(c *tc.C, expected int) {
	var seq int
	err := s.TxnRunner().StdTxn(c.Context(), func(ctx context.Context, tx *sql.Tx) error {
		namespace := domainmachine.MachineSequenceNamespace
		return tx.QueryRow("SELECT value FROM sequence WHERE namespace = ?", namespace).Scan(&seq)
	})
	c.Assert(err, tc.ErrorIsNil)
	c.Check(seq, tc.Equals, expected)
}

func (s *placementSuite) checkSequenceForContainerNamespace(c *tc.C, parentName machine.Name, expected int) {
	var seq int
	err := s.TxnRunner().StdTxn(c.Context(), func(ctx context.Context, tx *sql.Tx) error {
		namespace := sequence.MakePrefixNamespace(domainmachine.ContainerSequenceNamespace, parentName.String()).String()
		return tx.QueryRow("SELECT value FROM sequence WHERE namespace = ?", namespace).Scan(&seq)
	})
	c.Assert(err, tc.ErrorIsNil)
	c.Check(seq, tc.Equals, expected)
}

func (s *placementSuite) checkStatusForMachine(c *tc.C, name machine.Name, expected domainstatus.MachineStatusType) {
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

func (s *placementSuite) checkStatusForMachineInstance(c *tc.C, name machine.Name, expected domainstatus.InstanceStatusType) {
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

func (s *placementSuite) checkPlatformForMachine(c *tc.C, name machine.Name, expected deployment.Platform) {
	var platform deployment.Platform
	err := s.TxnRunner().StdTxn(c.Context(), func(ctx context.Context, tx *sql.Tx) error {
		return tx.QueryRow(`
SELECT mp.os_id, COALESCE(mp.channel,''), mp.architecture_id
FROM machine AS m
LEFT JOIN machine_platform AS mp ON m.uuid = mp.machine_uuid
WHERE m.name = ?
`, name).Scan(&platform.OSType, &platform.Channel, &platform.Architecture)
	})
	c.Assert(err, tc.ErrorIsNil)
	c.Check(platform, tc.DeepEquals, expected)
}

func (s *placementSuite) checkContainerTypeForMachine(c *tc.C, name machine.Name, expected ...string) {
	var containerTypes []string
	err := s.TxnRunner().StdTxn(c.Context(), func(ctx context.Context, tx *sql.Tx) error {
		rows, err := tx.QueryContext(ctx, `
SELECT ct.value
FROM machine AS m
LEFT JOIN machine_container_type AS mct ON m.uuid = mct.machine_uuid
LEFT JOIN container_type AS ct ON mct.container_type_id = ct.id
WHERE m.name = ?`, name)
		if err != nil {
			return errors.Capture(err)
		}
		defer rows.Close()

		for rows.Next() {
			var ct string
			if err := rows.Scan(&ct); err != nil {
				return errors.Capture(err)
			}
			containerTypes = append(containerTypes, ct)
		}
		if err := rows.Err(); err != nil {
			return errors.Capture(err)
		}
		return nil
	})
	c.Assert(err, tc.ErrorIsNil)
	c.Check(containerTypes, tc.DeepEquals, expected)
}

func (s *placementSuite) checkMachineNetNode(c *tc.C, name machine.Name, expectedNetNodeUUID string) {
	var nn string
	err := s.TxnRunner().StdTxn(c.Context(), func(ctx context.Context, tx *sql.Tx) error {
		return tx.QueryRow(`
SELECT m.net_node_uuid
FROM machine AS m
WHERE m.name = ?
`, name).Scan(&nn)
	})
	c.Assert(err, tc.ErrorIsNil, tc.Commentf("checking net node of machine %s", name))
	c.Check(nn, tc.Equals, expectedNetNodeUUID)
}

func (s *placementSuite) checkNonceForMachine(c *tc.C, name machine.Name, expected *string) {
	var nonce sql.Null[string]
	err := s.TxnRunner().StdTxn(c.Context(), func(ctx context.Context, tx *sql.Tx) error {
		err := tx.QueryRow(`
SELECT nonce
FROM machine
WHERE name = ?
`, name).Scan(&nonce)
		if err != nil && !errors.Is(err, sql.ErrNoRows) {
			return errors.Capture(err)
		}
		return nil
	})
	c.Assert(err, tc.ErrorIsNil)
	if expected == nil {
		c.Check(nonce.Valid, tc.Equals, false)
	} else {
		c.Check(nonce.V, tc.Equals, *expected)
	}
}
