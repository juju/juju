// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state

import (
	"context"
	"fmt"

	"github.com/canonical/sqlair"
	"github.com/juju/juju/core/machine"
	"github.com/juju/juju/domain/placement"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"
)

func (s *unitStateSuite) TestPlaceNetNodeMachinesUnset(c *gc.C) {
	// Ensure the machine got created.

	var netNode string
	err := s.TxnRunner().Txn(context.Background(), func(ctx context.Context, tx *sqlair.TX) error {
		var err error
		netNode, err = s.state.placeNetNodeMachines(ctx, tx, placement.Placement{
			Type: placement.PlacementTypeUnset,
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
}

func (s *unitStateSuite) TestPlaceNetNodeMachinesUnsetMultipleTimes(c *gc.C) {
	total := 10

	// Ensure that the machines are sequenced correctly.
	// The first machine should be 0, the second 1, and so on.

	var netNodes []string
	err := s.TxnRunner().Txn(context.Background(), func(ctx context.Context, tx *sqlair.TX) error {
		for range total {
			netNode, err := s.state.placeNetNodeMachines(ctx, tx, placement.Placement{
				Type: placement.PlacementTypeUnset,
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
			netNode, err := s.state.getMachineNetNodeUUIDFromName(ctx, tx, machine.Name(fmt.Sprintf("%d", i)))
			if err != nil {
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
		netNode, err = s.state.placeNetNodeMachines(ctx, tx, placement.Placement{
			Type: placement.PlacementTypeUnset,
		})
		return err
	})
	c.Assert(err, jc.ErrorIsNil)

	var resultNetNode string
	err = s.TxnRunner().Txn(context.Background(), func(ctx context.Context, tx *sqlair.TX) error {
		var err error
		resultNetNode, err = s.state.placeNetNodeMachines(ctx, tx, placement.Placement{
			Type:      placement.PlacementTypeMachine,
			Directive: "0",
		})
		return err
	})
	c.Assert(err, jc.ErrorIsNil)
	c.Check(resultNetNode, gc.Equals, netNode)
}
