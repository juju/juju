// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state

import (
	"context"
	"testing"

	"github.com/canonical/sqlair"
	"github.com/juju/tc"

	coremachine "github.com/juju/juju/core/machine"
	machinetesting "github.com/juju/juju/core/machine/testing"
	domainlife "github.com/juju/juju/domain/life"
	machineerrors "github.com/juju/juju/domain/machine/errors"
	domainnetwork "github.com/juju/juju/domain/network"
	schematesting "github.com/juju/juju/domain/schema/testing"
)

// ddlAssumptionsSuite provides a test suite for testing assumption relied on in
// the DDL of the model. These tests exist to break when relied upon assumptions
// in the DDL change over time. When a test in this suite fails, it means code
// in this domain needs to be updated.
type ddlAssumptionsSuite struct {
	schematesting.ModelSuite
}

// stateSuite provides a test suite for testing the commonality parts of [State].
type stateSuite struct {
	baseSuite
}

// TestStateSuite registers and runs all of the tests located in [stateSuite].
func TestStateSuite(t *testing.T) {
	tc.Run(t, &stateSuite{})
}

// TestCheckMachineIsDeadTrue tests that the [State.CheckMachineIsDead] method
// returns true when the machine is dead.
func (s *stateSuite) TestCheckMachineIsDeadTrue(c *tc.C) {
	netNode := s.newNetNode(c)
	machineUUID, _ := s.newMachineWithNetNode(c, netNode)
	s.changeMachineLife(c, machineUUID, domainlife.Dead)

	st := NewState(s.TxnRunnerFactory())
	isDead, err := st.CheckMachineIsDead(
		c.Context(), coremachine.UUID(machineUUID),
	)
	c.Check(err, tc.ErrorIsNil)
	c.Check(isDead, tc.IsTrue)
}

// TestCheckMachineIsDeadTrue tests that the [State.CheckMachineIsDead] method
// returns false when the machine is not dead.
func (s *stateSuite) TestCheckMachineIsDeadFalse(c *tc.C) {
	netNode := s.newNetNode(c)
	machineUUID, _ := s.newMachineWithNetNode(c, netNode)

	st := NewState(s.TxnRunnerFactory())
	isDead, err := st.CheckMachineIsDead(
		c.Context(), coremachine.UUID(machineUUID),
	)
	c.Check(err, tc.ErrorIsNil)
	c.Check(isDead, tc.IsFalse)
}

// TestCheckMachineIsDeadNotFound tests that check if a non-existent machine
// is dead results in a [machineerrors.MachineNotFound] error to the caller.
func (s *stateSuite) TestCheckMachineIsDeadNotFound(c *tc.C) {
	machineUUID := machinetesting.GenUUID(c)
	st := NewState(s.TxnRunnerFactory())
	_, err := st.CheckMachineIsDead(
		c.Context(), machineUUID,
	)
	c.Check(err, tc.ErrorIs, machineerrors.MachineNotFound)
}

// TestCheckNetNodeNotExist tests that the [State.checkNetNodeExists] method
// returns false when the net node does not exist.
func (s *stateSuite) TestCheckNetNodeNotExist(c *tc.C) {
	netNodeUUID, err := domainnetwork.NewNetNodeUUID()
	c.Assert(err, tc.ErrorIsNil)

	st := NewState(s.TxnRunnerFactory())

	var exists bool
	err = s.TxnRunner().Txn(c.Context(), func(ctx context.Context, tx *sqlair.TX) error {
		var err error
		exists, err = st.checkNetNodeExists(ctx, tx, netNodeUUID)
		return err
	})
	c.Check(err, tc.ErrorIsNil)
	c.Check(exists, tc.Equals, false)
}

// TestCheckNetNodeExists tests that when a net node exists
// [State.checkNetNodeExists] returns true.
func (s *stateSuite) TestCheckNetNodeExists(c *tc.C) {
	netNodeUUID, err := domainnetwork.NewNetNodeUUID()
	c.Assert(err, tc.ErrorIsNil)

	_, err = s.DB().ExecContext(
		c.Context(),
		"INSERT INTO net_node (uuid) VALUES (?)",
		netNodeUUID.String(),
	)
	c.Assert(err, tc.ErrorIsNil)

	st := NewState(s.TxnRunnerFactory())

	var exists bool
	err = s.TxnRunner().Txn(c.Context(), func(ctx context.Context, tx *sqlair.TX) error {
		var err error
		exists, err = st.checkNetNodeExists(ctx, tx, netNodeUUID)
		return err
	})
	c.Check(err, tc.ErrorIsNil)
	c.Check(exists, tc.Equals, true)
}

// TestGetMachineNetNodeUUID tests that the [State.GetMachineNetNodeUUID]
// returns the correct net node uuid for a machine.
func (s *stateSuite) TestGetMachineNetNodeUUID(c *tc.C) {
	netNodeUUID := s.newNetNode(c)
	machineUUID, _ := s.newMachineWithNetNode(c, netNodeUUID)

	st := NewState(s.TxnRunnerFactory())
	rval, err := st.GetMachineNetNodeUUID(
		c.Context(), coremachine.UUID(machineUUID),
	)
	c.Check(err, tc.ErrorIsNil)
	c.Check(rval, tc.Equals, domainnetwork.NetNodeUUID(netNodeUUID))
}

// TestGetMachineNetNodeUUIDNotFound tests that asking for the net node of a
// machine that does not exist returns a [machineerrors.MachineNotFound] error
// to the caller.
func (s *stateSuite) TestGetMachineNetNodeUUIDNotFound(c *tc.C) {
	machineUUID := machinetesting.GenUUID(c)

	st := NewState(s.TxnRunnerFactory())
	_, err := st.GetMachineNetNodeUUID(
		c.Context(), coremachine.UUID(machineUUID),
	)
	c.Check(err, tc.ErrorIs, machineerrors.MachineNotFound)
}

// TestMachineProvisionScopeValue tests that the value of machine provision
// scope in the storage_provision_scope table is 1. This is an assumption that
// is made in this state layer.
func (s *ddlAssumptionsSuite) TestMachineProvisionScopeValue(c *tc.C) {
	var idVal int
	err := s.DB().QueryRowContext(
		c.Context(),
		"SELECT id from storage_provision_scope WHERE scope = 'machine",
	).Scan(&idVal)

	c.Check(err, tc.ErrorIsNil)
	c.Check(idVal, tc.Equals, 1)
}

// TestModelProvisionScopeValue tests that the value of model provision
// scope in the storage_provision_scope table is 0. This is an assumption that
// is made in this state layer.
func (s *ddlAssumptionsSuite) TestModelProvisionScopeValue(c *tc.C) {
	var idVal int
	err := s.DB().QueryRowContext(
		c.Context(),
		"SELECT id from storage_provision_scope WHERE scope = 'model",
	).Scan(&idVal)

	c.Check(err, tc.ErrorIsNil)
	c.Check(idVal, tc.Equals, 0)
}
