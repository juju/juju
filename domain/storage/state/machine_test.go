// Copyright 2026 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state

import (
	"context"
	"testing"

	"github.com/canonical/sqlair"
	"github.com/juju/tc"

	coremachine "github.com/juju/juju/core/machine"
)

// machineSuite is a test suite for asserting interfaces of this state layer
// for getting information about machines in the model.
type machineSuite struct {
	baseSuite
}

// TestMachineSuite is responsible for running the tests contained in
// [machineSuite].
func TestMachineSuite(t *testing.T) {
	tc.Run(t, &machineSuite{})
}

// TestCheckEmptyMachineUUIDListExists tests that check an empty list of machine
// uuids exists returns true.
func (m *machineSuite) TestCheckEmptyMachinesUUIDListExists(c *tc.C) {
	var exists bool
	err := m.TxnRunner().Txn(
		c.Context(), func(ctx context.Context, tx *sqlair.TX) error {
			var err error
			exists, err = checkMachinesExist(ctx, preparer{}, tx, nil)
			return err
		},
	)
	c.Check(err, tc.ErrorIsNil)
	c.Check(exists, tc.IsTrue)
}

// TestCheckMachineUUIDDoesNotExist tests that checking of a single machine uuid
// that does not exists reports false to the caller.
func (m *machineSuite) TestCheckMachinesUUIDDoesNotExist(c *tc.C) {
	machineNotFoundUUID := tc.Must(c, coremachine.NewUUID)
	var exists bool

	err := m.TxnRunner().Txn(
		c.Context(), func(ctx context.Context, tx *sqlair.TX) error {
			var err error
			exists, err = checkMachinesExist(
				ctx, preparer{}, tx, machineUUIDs{
					machineNotFoundUUID.String(),
				},
			)
			return err
		},
	)
	c.Check(err, tc.ErrorIsNil)
	c.Check(exists, tc.IsFalse)
}

// TestCheckMachineUUIDDoesNotExistWithOtherExisting tests that
// [checkMachineExists] return false when supplied with a list of machine uuids
// and one or more of the uuids does not exist in the model.
func (m *machineSuite) TestCheckMachinesUUIDDoesNotExistWithOtherExisting(c *tc.C) {
	machineNotFoundUUID := tc.Must(c, coremachine.NewUUID)
	machine1UUID := m.newMachine(c)

	var exists bool
	err := m.TxnRunner().Txn(
		c.Context(), func(ctx context.Context, tx *sqlair.TX) error {
			var err error
			exists, err = checkMachinesExist(
				ctx, preparer{}, tx, machineUUIDs{
					machine1UUID.String(),
					machineNotFoundUUID.String(),
				},
			)
			return err
		},
	)
	c.Check(err, tc.ErrorIsNil)
	c.Check(exists, tc.IsFalse)
}

// TestCheckMachinesExist is a happy path test of [checkMachinesExist].
func (m *machineSuite) TestCheckMachinesExist(c *tc.C) {
	machine1UUID := m.newMachine(c)
	machine2UUID := m.newMachine(c)
	machine3UUID := m.newMachine(c)

	var exists bool
	err := m.TxnRunner().Txn(
		c.Context(), func(ctx context.Context, tx *sqlair.TX) error {
			var err error
			exists, err = checkMachinesExist(
				ctx, preparer{}, tx, machineUUIDs{
					machine2UUID.String(),
					machine3UUID.String(),
					machine1UUID.String(),
				},
			)
			return err
		},
	)
	c.Check(err, tc.ErrorIsNil)
	c.Check(exists, tc.IsTrue)
}
