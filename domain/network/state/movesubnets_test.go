// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state

import (
	"context"
	"database/sql"
	"testing"

	"github.com/canonical/sqlair"
	"github.com/juju/tc"

	coreapplication "github.com/juju/juju/core/application"
	corecharm "github.com/juju/juju/core/charm"
	"github.com/juju/juju/core/network"
	domainnetwork "github.com/juju/juju/domain/network"
	networkerrors "github.com/juju/juju/domain/network/errors"
	"github.com/juju/juju/internal/charm"
	"github.com/juju/juju/internal/uuid"
)

type spaceMachineSuite struct {
	linkLayerBaseSuite
}

func TestSpaceMachineSuite(t *testing.T) {
	tc.Run(t, &spaceMachineSuite{})
}

// TestMoveSubnetsToSpaceEmptySubnetList verifies that there is no error if
// there is no subnet to move
func (s *spaceMachineSuite) TestMoveSubnetsToSpaceEmptySubnetList(c *tc.C) {
	// Arrange
	ctx := c.Context()

	// Act
	moved, err := s.state.MoveSubnetsToSpace(ctx, []string{}, "test-space", false)

	// Assert
	c.Assert(err, tc.IsNil)
	c.Assert(moved, tc.HasLen, 0)
}

// TestMoveSubnetsToSpaceInvalidSpace verifies that moving subnets to a
// non-existent space results in an appropriate error.
func (s *spaceMachineSuite) TestMoveSubnetsToSpaceInvalidSpace(c *tc.C) {
	// Arrange
	ctx := c.Context()

	// Act
	_, err := s.state.MoveSubnetsToSpace(ctx, []string{"placeholder"}, "test-space", false)

	// Assert
	c.Assert(err, tc.ErrorIs, networkerrors.SpaceNotFound)
}

// TestValidateSubnetsLeavingSpacesNoMachines ensures that moving a subnet from
// one space to another results in no constraint failures when no machines
// are associated.
func (s *spaceMachineSuite) TestValidateSubnetsLeavingSpacesNoMachines(c *tc.C) {
	// Arrange
	ctx := c.Context()

	// Create subnet in a space, and a second space
	subnetUUID := s.addSubnet(c, "192.168.1.0/24", s.addSpace(c, "from-space"))
	s.addSpace(c, "to-space")

	db, err := s.state.DB()
	c.Assert(err, tc.IsNil)

	// Act
	var failures []positiveSpaceConstraintFailure
	err = db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		failures, err = s.state.validateSubnetsLeavingSpaces(ctx, tx,
			[]string{subnetUUID},
			"to-space")
		return err
	})

	// Assert
	c.Assert(err, tc.IsNil)
	c.Assert(failures, tc.HasLen, 0, tc.Commentf("Expected no constraint failures"))
}

// TestValidateSubnetsLeavingSpacesAppBindingSuccess validates that a subnet can be
// moved if a machine bound to the origin space has still an address on
// the origin space.
func (s *spaceMachineSuite) TestValidateSubnetsLeavingSpacesAppBindingSuccess(c *tc.C) {
	// Arrange
	ctx := c.Context()

	// Create spaces
	fromSpaceUUID := s.addSpace(c, "from-space")
	s.addSpace(c, "to-space")

	// Create subnets from space, one will be unmoved
	movedSubnetUUID := s.addSubnet(c, "192.168.1.0/24", fromSpaceUUID)
	okSubnetUUID := s.addSubnet(c, "10.0.0.0/24", fromSpaceUUID)

	// Create a machine with addresses in both spaces
	nodeUUID := s.addNetNode(c)
	deviceUUID := s.addLinkLayerDevice(c, nodeUUID, "eth0", "00:11:22:33:44:55", network.EthernetDevice)
	s.addIPAddressWithSubnet(c, deviceUUID, nodeUUID, movedSubnetUUID, "192.168.1.10")
	s.addIPAddressWithSubnet(c, deviceUUID, nodeUUID, okSubnetUUID, "10.0.0.10")

	// Add an application with a unit on this machine, bind to the source space
	charmUUID := s.addCharm(c)
	appUUID := s.addApplication(c, charmUUID, fromSpaceUUID)
	s.addUnit(c, appUUID, charmUUID, nodeUUID)

	db, err := s.state.DB()
	c.Assert(err, tc.IsNil)

	// Act
	var failures []positiveSpaceConstraintFailure
	err = db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		failures, err = s.state.validateSubnetsLeavingSpaces(ctx, tx,
			[]string{movedSubnetUUID},
			"to-space")
		return err
	})

	// Assert
	c.Assert(err, tc.IsNil)
	c.Assert(failures, tc.HasLen, 0, tc.Commentf("Expected no constraint failures"))
}

// TestValidateSubnetsLeavingSpacesAppEndpointBindingSuccess validates that a subnet
// can be moved if a machine bound to the origin space through endpoint
// has still an address on the origin space
func (s *spaceMachineSuite) TestValidateSubnetsLeavingSpacesAppEndpointBindingSuccess(c *tc.C) {
	// Arrange
	ctx := c.Context()

	// Create spaces
	fromSpaceUUID := s.addSpace(c, "from-space")
	s.addSpace(c, "to-space")

	// Create subnets from space, one will be unmoved
	movedSubnetUUID := s.addSubnet(c, "192.168.1.0/24", fromSpaceUUID)
	okSubnetUUID := s.addSubnet(c, "10.0.0.0/24", fromSpaceUUID)
	alphaSubnetUUID := s.addSubnet(c, "10.0.1.0/24", network.AlphaSpaceId.String())

	// Create a machine with addresses in both spaces
	nodeUUID := s.addNetNode(c)
	deviceUUID := s.addLinkLayerDevice(c, nodeUUID, "eth0", "00:11:22:33:44:55", network.EthernetDevice)
	s.addIPAddressWithSubnet(c, deviceUUID, nodeUUID, movedSubnetUUID, "192.168.1.10")
	s.addIPAddressWithSubnet(c, deviceUUID, nodeUUID, okSubnetUUID, "10.0.0.10")
	s.addIPAddressWithSubnet(c, deviceUUID, nodeUUID, alphaSubnetUUID, "10.0.1.10")

	// Add an application with a unit on this machine, bind to the source space
	charmUUID := s.addCharm(c)
	appUUID := s.addApplication(c, charmUUID, network.AlphaSpaceId.String()) // won't move
	charmRelationUUID := s.addCharmRelation(c, corecharm.ID(charmUUID), charm.Relation{Name: "db",
		Role:      charm.RoleProvider,
		Interface: "mysql", Optional: false, Limit: 1, Scope: charm.ScopeGlobal})
	s.addApplicationEndpoint(c, coreapplication.ID(appUUID), charmRelationUUID, fromSpaceUUID)
	s.addUnit(c, appUUID, charmUUID, nodeUUID)

	db, err := s.state.DB()
	c.Assert(err, tc.IsNil)

	// Act
	var failures []positiveSpaceConstraintFailure
	err = db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		failures, err = s.state.validateSubnetsLeavingSpaces(ctx, tx,
			[]string{movedSubnetUUID},
			"to-space")
		return err
	})

	// Assert
	c.Assert(err, tc.IsNil)
	c.Assert(failures, tc.HasLen, 0, tc.Commentf("Expected no constraint failures"))
}

// TestValidateSubnetsLeavingSpacesPositiveConstraintSuccess validates that a subnet
// can be moved if a machine with a positive constraint to the origin space
// has still an address on the origin space
func (s *spaceMachineSuite) TestValidateSubnetsLeavingSpacesPositiveConstraintSuccess(c *tc.C) {
	// Arrange
	ctx := c.Context()

	// Create spaces
	fromSpaceUUID := s.addSpace(c, "from-space")
	s.addSpace(c, "to-space")

	// Create subnets from space, one will be unmoved
	movedSubnetUUID := s.addSubnet(c, "192.168.1.0/24", fromSpaceUUID)
	okSubnetUUID := s.addSubnet(c, "10.0.0.0/24", fromSpaceUUID)

	// Create a machine with addresses in both spaces
	nodeUUID := s.addNetNode(c)
	deviceUUID := s.addLinkLayerDevice(c, nodeUUID, "eth0", "00:11:22:33:44:55", network.EthernetDevice)
	s.addIPAddressWithSubnet(c, deviceUUID, nodeUUID, movedSubnetUUID, "192.168.1.10")
	s.addIPAddressWithSubnet(c, deviceUUID, nodeUUID, okSubnetUUID, "10.0.0.10")

	// Add a machine with a positive constraint on from-space
	machineUUID := s.addMachine(c, "machine-1", nodeUUID)
	s.addSpaceConstraint(c, machineUUID.String(), "from-space", true)

	db, err := s.state.DB()
	c.Assert(err, tc.IsNil)

	// Act
	var failures []positiveSpaceConstraintFailure
	err = db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		failures, err = s.state.validateSubnetsLeavingSpaces(ctx, tx,
			[]string{movedSubnetUUID},
			"to-space")
		return err
	})

	// Assert
	c.Assert(err, tc.IsNil)
	c.Assert(failures, tc.HasLen, 0, tc.Commentf("Expected no constraint failures"))
}

// TestValidateSubnetsLeavingSpacesAppBindingFailure verifies that moving a subnet
// from one space to another fails if a machine bound by application to
// the space has no more addresses on it.
func (s *spaceMachineSuite) TestValidateSubnetsLeavingSpacesAppBindingFailure(c *tc.C) {
	// Arrange
	ctx := c.Context()

	// Create spaces
	fromSpaceUUID := s.addSpace(c, "from-space")
	s.addSpace(c, "to-space")

	// Create subnet in from-space
	movedSubnetUUID := s.addSubnet(c, "192.168.1.0/24", fromSpaceUUID)

	// Create a machine with address in the subnet that will be moved
	nodeUUID := s.addNetNode(c)
	deviceUUID := s.addLinkLayerDevice(c, nodeUUID, "eth0", "00:11:22:33:44:55", network.EthernetDevice)
	s.addIPAddressWithSubnet(c, deviceUUID, nodeUUID, movedSubnetUUID, "192.168.1.10")

	// Add an application with a unit on this machine, bind to the source space
	charmUUID := s.addCharm(c)
	appUUID := s.addApplication(c, charmUUID, fromSpaceUUID)
	s.addUnit(c, appUUID, charmUUID, nodeUUID)
	s.addMachine(c, "machine-1", nodeUUID)

	db, err := s.state.DB()
	c.Assert(err, tc.IsNil)

	// Act
	var failures []positiveSpaceConstraintFailure
	err = db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		failures, err = s.state.validateSubnetsLeavingSpaces(ctx, tx,
			[]string{movedSubnetUUID},
			"to-space")
		return err
	})

	// Assert
	c.Assert(err, tc.IsNil)
	c.Assert(failures, tc.SameContents, []positiveSpaceConstraintFailure{
		{
			MachineName: "machine-1",
			SpaceName:   "from-space",
		},
	})
}

// TestValidateSubnetsLeavingSpacesAppEndpointBindingFailure verifies that moving a subnet
// from one space to another fails if a machine bound by application endpoint to
// the space has no more addresses on it.
func (s *spaceMachineSuite) TestValidateSubnetsLeavingSpacesAppEndpointBindingFailure(c *tc.C) {
	// Arrange
	ctx := c.Context()

	// Create spaces
	fromSpaceUUID := s.addSpace(c, "from-space")
	s.addSpace(c, "to-space")

	// Create subnet in from-space
	movedSubnetUUID := s.addSubnet(c, "192.168.1.0/24", fromSpaceUUID)
	alphaSubnetUUID := s.addSubnet(c, "10.0.1.0/24", network.AlphaSpaceId.String())

	// Create a machine with address in the subnet that will be moved
	nodeUUID := s.addNetNode(c)
	deviceUUID := s.addLinkLayerDevice(c, nodeUUID, "eth0", "00:11:22:33:44:55", network.EthernetDevice)
	s.addIPAddressWithSubnet(c, deviceUUID, nodeUUID, movedSubnetUUID, "192.168.1.10")
	s.addIPAddressWithSubnet(c, deviceUUID, nodeUUID, alphaSubnetUUID, "10.0.1.10")

	// Add an application with a unit on this machine, bind to the source space
	charmUUID := s.addCharm(c)
	appUUID := s.addApplication(c, charmUUID, network.AlphaSpaceId.String()) // won't move
	charmRelationUUID := s.addCharmRelation(c, corecharm.ID(charmUUID), charm.Relation{Name: "db",
		Role:      charm.RoleProvider,
		Interface: "mysql", Optional: false, Limit: 1, Scope: charm.ScopeGlobal})
	s.addApplicationEndpoint(c, coreapplication.ID(appUUID), charmRelationUUID, fromSpaceUUID)
	s.addUnit(c, appUUID, charmUUID, nodeUUID)
	s.addMachine(c, "machine-1", nodeUUID)

	db, err := s.state.DB()
	c.Assert(err, tc.IsNil)

	// Act
	var failures []positiveSpaceConstraintFailure
	err = db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		failures, err = s.state.validateSubnetsLeavingSpaces(ctx, tx,
			[]string{movedSubnetUUID},
			"to-space")
		return err
	})

	// Assert
	c.Assert(err, tc.IsNil)
	c.Assert(failures, tc.SameContents, []positiveSpaceConstraintFailure{
		{
			MachineName: "machine-1",
			SpaceName:   "from-space",
		},
	})
}

// TestValidateSubnetsLeavingSpacesAppEndpointBindingFailure verifies that moving
// a subnet from one space to another fails if a machine with a positive
// constraint to the space has no more addresses on it.
func (s *spaceMachineSuite) TestValidateSubnetsLeavingSpacesPositiveConstraintFailure(c *tc.C) {
	// Arrange
	ctx := c.Context()

	// Create spaces
	fromSpaceUUID := s.addSpace(c, "from-space")
	s.addSpace(c, "to-space")

	// Create subnet in from-space
	movedSubnetUUID := s.addSubnet(c, "192.168.1.0/24", fromSpaceUUID)

	// Create a machine with address in the subnet that will be moved
	nodeUUID := s.addNetNode(c)
	deviceUUID := s.addLinkLayerDevice(c, nodeUUID, "eth0", "00:11:22:33:44:55", network.EthernetDevice)
	s.addIPAddressWithSubnet(c, deviceUUID, nodeUUID, movedSubnetUUID, "192.168.1.10")

	// Add a machine with a positive constraint on from-space
	machineUUID := s.addMachine(c, "machine-1", nodeUUID)
	s.addSpaceConstraint(c, machineUUID.String(), "from-space", true)

	db, err := s.state.DB()
	c.Assert(err, tc.IsNil)

	// Act
	var failures []positiveSpaceConstraintFailure
	err = db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		failures, err = s.state.validateSubnetsLeavingSpaces(ctx, tx,
			[]string{movedSubnetUUID},
			"to-space")
		return err
	})

	// Assert
	c.Assert(err, tc.IsNil)
	c.Assert(failures, tc.SameContents, []positiveSpaceConstraintFailure{
		{
			MachineName: "machine-1",
			SpaceName:   "from-space",
		},
	})
}

// TestValidateSubnetsLeavingSpacesMultipleFailure verifies the detection of multiple
// failures in machines due to subnet moves.
func (s *spaceMachineSuite) TestValidateSubnetsLeavingSpacesMultipleFailure(c *tc.C) {
	// Arrange
	ctx := c.Context()

	// Create spaces
	fromSpaceUUID1 := s.addSpace(c, "from-space-1")
	fromSpaceUUID2 := s.addSpace(c, "from-space-2")
	fromSpaceUUID3 := s.addSpace(c, "from-space-3")
	s.addSpace(c, "to-space")

	// Create subnets in from-spaces
	movedSubnetUUID1 := s.addSubnet(c, "192.168.1.0/24", fromSpaceUUID1)
	movedSubnetUUID2 := s.addSubnet(c, "192.168.2.0/24", fromSpaceUUID2)
	movedSubnetUUID3 := s.addSubnet(c, "192.168.3.0/24", fromSpaceUUID3)
	alphaSubnetUUID := s.addSubnet(c, "10.0.1.0/24", network.AlphaSpaceId.String())

	// Create machines with addresses in the subnets that will be moved
	nodeUUID1 := s.addNetNode(c)
	nodeUUID2 := s.addNetNode(c)
	nodeUUID3 := s.addNetNode(c)
	deviceUUID1 := s.addLinkLayerDevice(c, nodeUUID1, "eth0", "00:11:22:33:44:55", network.EthernetDevice)
	deviceUUID2 := s.addLinkLayerDevice(c, nodeUUID2, "eth1", "00:11:22:33:44:66", network.EthernetDevice)
	deviceUUID3 := s.addLinkLayerDevice(c, nodeUUID3, "eth2", "00:11:22:33:44:77", network.EthernetDevice)
	s.addIPAddressWithSubnet(c, deviceUUID1, nodeUUID1, movedSubnetUUID1, "192.168.1.10")
	s.addIPAddressWithSubnet(c, deviceUUID2, nodeUUID2, movedSubnetUUID2, "192.168.2.10")
	s.addIPAddressWithSubnet(c, deviceUUID3, nodeUUID3, movedSubnetUUID3, "192.168.3.10")
	s.addIPAddressWithSubnet(c, deviceUUID3, nodeUUID3, alphaSubnetUUID, "10.0.1.10")

	charmUUID := s.addCharm(c)

	// Add a machine with a positive constraint on space-1
	machineUUID := s.addMachine(c, "machine-1", nodeUUID1)
	s.addSpaceConstraint(c, machineUUID.String(), "from-space-1", true)

	// Add an application with a unit on this machine, bind to a source space
	appUUID2 := s.addApplication(c, charmUUID, fromSpaceUUID2) // won't move
	s.addUnit(c, appUUID2, charmUUID, nodeUUID2)
	s.addMachine(c, "machine-2", nodeUUID2)

	// Add an application with a unit on this machine, bind to a source space
	appUUID := s.addApplication(c, charmUUID, network.AlphaSpaceId.String()) // won't move
	charmRelationUUID := s.addCharmRelation(c, corecharm.ID(charmUUID), charm.Relation{Name: "db",
		Role:      charm.RoleProvider,
		Interface: "mysql", Optional: false, Limit: 1, Scope: charm.ScopeGlobal})
	s.addApplicationEndpoint(c, coreapplication.ID(appUUID), charmRelationUUID, fromSpaceUUID3)
	s.addUnit(c, appUUID, charmUUID, nodeUUID3)
	s.addMachine(c, "machine-3", nodeUUID3)

	db, err := s.state.DB()
	c.Assert(err, tc.IsNil)

	// Act
	var failures []positiveSpaceConstraintFailure
	err = db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		failures, err = s.state.validateSubnetsLeavingSpaces(ctx, tx,
			[]string{
				movedSubnetUUID1,
				movedSubnetUUID2,
				movedSubnetUUID3,
			},
			"to-space")
		return err
	})

	// Assert
	c.Assert(err, tc.IsNil)
	c.Assert(failures, tc.SameContents, []positiveSpaceConstraintFailure{
		{
			MachineName: "machine-1",
			SpaceName:   "from-space-1",
		},
		{
			MachineName: "machine-2",
			SpaceName:   "from-space-2",
		},
		{
			MachineName: "machine-3",
			SpaceName:   "from-space-3",
		},
	})
}

// TestValidateSubnetsJoiningSpaceNoMachines ensures that moving a subnet to a new
// space results in no negative constraint failures when no machines
// are associated.
func (s *spaceMachineSuite) TestValidateSubnetsJoiningSpaceNoMachines(c *tc.C) {
	// Arrange
	ctx := c.Context()

	// Create subnet in a space, and a second space
	subnetUUID := s.addSubnet(c, "192.168.1.0/24", s.addSpace(c, "from-space"))
	s.addSpace(c, "to-space")

	db, err := s.state.DB()
	c.Assert(err, tc.IsNil)

	// Act
	var failures []negativeSpaceConstraintFailure
	err = db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		failures, err = s.state.validateSubnetsJoiningSpace(ctx, tx,
			[]string{subnetUUID},
			"to-space")
		return err
	})

	// Assert
	c.Assert(err, tc.IsNil)
	c.Assert(failures, tc.HasLen, 0, tc.Commentf("Expected no constraint failures"))
}

// TestValidateSubnetsJoiningSpaceNoViolations ensures no space constraint violations
// occur when a subnet is moved to a machine with no negative constraint on the
// new space
func (s *spaceMachineSuite) TestValidateSubnetsJoiningSpaceNoViolations(c *tc.C) {
	// Arrange
	ctx := c.Context()

	// Create spaces
	fromSpaceUUID := s.addSpace(c, "from-space")
	s.addSpace(c, "to-space")
	otherSpaceUUID := s.addSpace(c, "other-space")

	// Create subnets in from-space and other-space
	movedSubnetUUID := s.addSubnet(c, "192.168.1.0/24", fromSpaceUUID)
	otherSubnetUUID := s.addSubnet(c, "10.0.0.0/24", otherSpaceUUID)

	// Create a machine with address in other-space
	nodeUUID := s.addNetNode(c)
	deviceUUID := s.addLinkLayerDevice(c, nodeUUID, "eth0", "00:11:22:33:44:55", network.EthernetDevice)
	s.addIPAddressWithSubnet(c, deviceUUID, nodeUUID, otherSubnetUUID, "10.0.0.10")

	// Add a machine with a negative constraint on to-space
	machineUUID := s.addMachine(c, "machine-1", nodeUUID)
	s.addSpaceConstraint(c, machineUUID.String(), "to-space", false)

	db, err := s.state.DB()
	c.Assert(err, tc.IsNil)

	// Act
	var failures []negativeSpaceConstraintFailure
	err = db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		failures, err = s.state.validateSubnetsJoiningSpace(ctx, tx,
			[]string{movedSubnetUUID},
			"to-space")
		return err
	})

	// Assert
	c.Assert(err, tc.IsNil)
	c.Assert(failures, tc.HasLen, 0, tc.Commentf("Expected no constraint failures"))
}

// TestValidateSubnetsJoiningSpaceWithViolations verifies that machines with negative
// space constraints are detected correctly when subnets are moved.
func (s *spaceMachineSuite) TestValidateSubnetsJoiningSpaceWithViolations(c *tc.C) {
	// Arrange
	ctx := c.Context()

	// Create spaces
	fromSpaceUUID := s.addSpace(c, "from-space")
	s.addSpace(c, "to-space")

	// Create subnet in from-space
	movedSubnetUUID := s.addSubnet(c, "192.168.1.0/24", fromSpaceUUID)

	// Create a machine with address in the subnet that will be moved
	nodeUUID := s.addNetNode(c)
	deviceUUID := s.addLinkLayerDevice(c, nodeUUID, "eth0", "00:11:22:33:44:55", network.EthernetDevice)
	addressValue := "192.168.1.10"
	s.addIPAddressWithSubnet(c, deviceUUID, nodeUUID, movedSubnetUUID, addressValue)

	// Add a machine with a negative constraint on to-space
	machineUUID := s.addMachine(c, "machine-1", nodeUUID)
	s.addSpaceConstraint(c, machineUUID.String(), "to-space", false)

	db, err := s.state.DB()
	c.Assert(err, tc.IsNil)

	// Act
	var failures []negativeSpaceConstraintFailure
	err = db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		failures, err = s.state.validateSubnetsJoiningSpace(ctx, tx,
			[]string{movedSubnetUUID},
			"to-space")
		return err
	})

	// Assert
	c.Assert(err, tc.IsNil)
	c.Assert(failures, tc.SameContents, []negativeSpaceConstraintFailure{
		{
			MachineName: "machine-1",
			Address:     addressValue,
		},
	})
}

// TestValidateSubnetsJoiningSpaceMultipleViolations verifies handling of multiple
// violations when moving subnets to an excluded space.
func (s *spaceMachineSuite) TestValidateSubnetsJoiningSpaceMultipleViolations(c *tc.C) {
	// Arrange
	ctx := c.Context()

	// Create spaces
	fromSpaceUUID1 := s.addSpace(c, "from-space-1")
	fromSpaceUUID2 := s.addSpace(c, "from-space-2")
	s.addSpace(c, "to-space")

	// Create subnets in from-spaces
	movedSubnetUUID1 := s.addSubnet(c, "192.168.1.0/24", fromSpaceUUID1)
	movedSubnetUUID2 := s.addSubnet(c, "192.168.2.0/24", fromSpaceUUID2)

	// Create machines with addresses in the subnets that will be moved
	nodeUUID1 := s.addNetNode(c)
	nodeUUID2 := s.addNetNode(c)
	deviceUUID1 := s.addLinkLayerDevice(c, nodeUUID1, "eth0", "00:11:22:33:44:55", network.EthernetDevice)
	deviceUUID2 := s.addLinkLayerDevice(c, nodeUUID2, "eth1", "00:11:22:33:44:66", network.EthernetDevice)
	addressValue1 := "192.168.1.10"
	addressValue2 := "192.168.2.10"
	s.addIPAddressWithSubnet(c, deviceUUID1, nodeUUID1, movedSubnetUUID1, addressValue1)
	s.addIPAddressWithSubnet(c, deviceUUID2, nodeUUID2, movedSubnetUUID2, addressValue2)

	// Add machines with negative constraints on to-space
	machineUUID1 := s.addMachine(c, "machine-1", nodeUUID1)
	machineUUID2 := s.addMachine(c, "machine-2", nodeUUID2)
	s.addSpaceConstraint(c, machineUUID1.String(), "to-space", false)
	s.addSpaceConstraint(c, machineUUID2.String(), "to-space", false)

	db, err := s.state.DB()
	c.Assert(err, tc.IsNil)

	// Act
	var failures []negativeSpaceConstraintFailure
	err = db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		failures, err = s.state.validateSubnetsJoiningSpace(ctx, tx,
			[]string{
				movedSubnetUUID1,
				movedSubnetUUID2,
			},
			"to-space")
		return err
	})

	// Assert
	c.Assert(err, tc.IsNil)
	c.Assert(failures, tc.SameContents, []negativeSpaceConstraintFailure{
		{
			MachineName: "machine-1",
			Address:     addressValue1,
		},
		{
			MachineName: "machine-2",
			Address:     addressValue2,
		},
	})
}

// TestMoveSubnetsToSpaceErrorPath tests the error path where moving subnets would
// violate both positive and negative constraints.
func (s *spaceMachineSuite) TestMoveSubnetsToSpaceErrorPath(c *tc.C) {
	// Arrange
	ctx := c.Context()

	// Create spaces
	fromSpaceUUID1 := s.addSpace(c, "from-space-1")
	fromSpaceUUID2 := s.addSpace(c, "from-space-2")
	s.addSpace(c, "to-space") // Space name is used directly in MoveSubnetsToSpace

	// Create subnets in from-spaces
	movedSubnetUUID1 := s.addSubnet(c, "192.168.1.0/24", fromSpaceUUID1)
	movedSubnetUUID2 := s.addSubnet(c, "192.168.2.0/24", fromSpaceUUID2)

	// Create machines with addresses in the subnets that will be moved
	nodeUUID1 := s.addNetNode(c)
	nodeUUID2 := s.addNetNode(c)
	deviceUUID1 := s.addLinkLayerDevice(c, nodeUUID1, "eth0", "00:11:22:33:44:55", network.EthernetDevice)
	deviceUUID2 := s.addLinkLayerDevice(c, nodeUUID2, "eth1", "00:11:22:33:44:66", network.EthernetDevice)
	addressValue1 := "192.168.1.10"
	addressValue2 := "192.168.2.10"
	s.addIPAddressWithSubnet(c, deviceUUID1, nodeUUID1, movedSubnetUUID1, addressValue1)
	s.addIPAddressWithSubnet(c, deviceUUID2, nodeUUID2, movedSubnetUUID2, addressValue2)

	// Add a machine with a positive constraint on from-space-1 (will fail when moved)
	machineUUID1 := s.addMachine(c, "machine-1", nodeUUID1)
	s.addSpaceConstraint(c, machineUUID1.String(), "from-space-1", true)

	// Add a machine with a negative constraint on to-space (will fail when moved)
	machineUUID2 := s.addMachine(c, "machine-2", nodeUUID2)
	s.addSpaceConstraint(c, machineUUID2.String(), "to-space", false)

	// Act
	_, err := s.state.MoveSubnetsToSpace(ctx,
		[]string{
			movedSubnetUUID1,
			movedSubnetUUID2,
		},
		"to-space",
		false)

	// Assert
	c.Assert(err, tc.NotNil)
	c.Assert(err.Error(), tc.Matches, `machine "machine-1" is missing addresses in space\(s\) "from-space-1"
machine "machine-2" would have 1 addresses in excluded space "to-space" \(192.168.2.10\)`)
	c.Assert(s.fetchSubnetSpace(c), tc.SameContents, []subnetSpace{
		{
			UUID:  movedSubnetUUID1,
			Space: "from-space-1",
		},
		{
			UUID:  movedSubnetUUID2,
			Space: "from-space-2",
		},
	})
}

// TestMoveSubnetsToSpaceForceSuccess tests the happy path where moving subnets would
// violate constraints, but force=true allows the operation to succeed.
func (s *spaceMachineSuite) TestMoveSubnetsToSpaceForceSuccess(c *tc.C) {
	// Arrange
	ctx := c.Context()

	// Create spaces
	fromSpaceUUID1 := s.addSpace(c, "from-space-1")
	fromSpaceUUID2 := s.addSpace(c, "from-space-2")
	s.addSpace(c, "to-space")

	// Create subnets in from-spaces
	movedSubnetUUID1 := s.addSubnet(c, "192.168.1.0/24", fromSpaceUUID1)
	movedSubnetUUID2 := s.addSubnet(c, "192.168.2.0/24", fromSpaceUUID2)

	// Create machines with addresses in the subnets that will be moved
	nodeUUID1 := s.addNetNode(c)
	nodeUUID2 := s.addNetNode(c)
	deviceUUID1 := s.addLinkLayerDevice(c, nodeUUID1, "eth0", "00:11:22:33:44:55", network.EthernetDevice)
	deviceUUID2 := s.addLinkLayerDevice(c, nodeUUID2, "eth1", "00:11:22:33:44:66", network.EthernetDevice)
	addressValue1 := "192.168.1.10"
	addressValue2 := "192.168.2.10"
	s.addIPAddressWithSubnet(c, deviceUUID1, nodeUUID1, movedSubnetUUID1, addressValue1)
	s.addIPAddressWithSubnet(c, deviceUUID2, nodeUUID2, movedSubnetUUID2, addressValue2)

	// Add a machine with a positive constraint on from-space-1 (would fail without force)
	machineUUID1 := s.addMachine(c, "machine-1", nodeUUID1)
	s.addSpaceConstraint(c, machineUUID1.String(), "from-space-1", true)

	// Add a machine with a negative constraint on to-space (would fail without force)
	machineUUID2 := s.addMachine(c, "machine-2", nodeUUID2)
	s.addSpaceConstraint(c, machineUUID2.String(), "to-space", false)

	// Act
	movedSubnets, err := s.state.MoveSubnetsToSpace(ctx,
		[]string{
			movedSubnetUUID1,
			movedSubnetUUID2,
		},
		"to-space",
		true) // Force=true

	// Assert
	c.Assert(err, tc.IsNil)
	c.Assert(movedSubnets, tc.SameContents, []domainnetwork.MovedSubnets{
		{
			UUID:      domainnetwork.SubnetUUID(movedSubnetUUID1),
			FromSpace: "from-space-1",
		},
		{
			UUID:      domainnetwork.SubnetUUID(movedSubnetUUID2),
			FromSpace: "from-space-2",
		},
	})

	// Verify the subnets were actually moved in the database
	c.Assert(s.fetchSubnetSpace(c), tc.SameContents, []subnetSpace{
		{
			UUID:  movedSubnetUUID1,
			Space: "to-space",
		},
		{
			UUID:  movedSubnetUUID2,
			Space: "to-space",
		},
	})
}

// TestMoveSubnetsToSpaceNoViolationsSuccess tests the happy path where moving subnets
// doesn't violate any constraints.
func (s *spaceMachineSuite) TestMoveSubnetsToSpaceNoViolationsSuccess(c *tc.C) {
	// Arrange
	ctx := c.Context()

	// Create spaces
	fromSpaceUUID1 := s.addSpace(c, "from-space-1")
	fromSpaceUUID2 := s.addSpace(c, "from-space-2")
	s.addSpace(c, "to-space")
	otherSpaceUUID := s.addSpace(c, "other-space")

	// Create subnets in from-spaces and other-space
	movedSubnetUUID1 := s.addSubnet(c, "192.168.1.0/24", fromSpaceUUID1)
	movedSubnetUUID2 := s.addSubnet(c, "192.168.2.0/24", fromSpaceUUID2)
	otherSubnetUUID := s.addSubnet(c, "10.0.0.0/24", otherSpaceUUID)

	// Create machines with addresses in the subnets
	nodeUUID1 := s.addNetNode(c)
	nodeUUID2 := s.addNetNode(c)
	deviceUUID1 := s.addLinkLayerDevice(c, nodeUUID1, "eth0", "00:11:22:33:44:55", network.EthernetDevice)
	deviceUUID2 := s.addLinkLayerDevice(c, nodeUUID2, "eth1", "00:11:22:33:44:66", network.EthernetDevice)
	s.addIPAddressWithSubnet(c, deviceUUID1, nodeUUID1, movedSubnetUUID1, "192.168.1.10")
	s.addIPAddressWithSubnet(c, deviceUUID2, nodeUUID2, otherSubnetUUID, "10.0.0.10")

	// Add machines without any constraints that would be violated
	// We don't need to store the machine UUIDs as we're just testing the subnet movement
	s.addMachine(c, "machine-1", nodeUUID1)
	s.addMachine(c, "machine-2", nodeUUID2)

	// Act
	movedSubnets, err := s.state.MoveSubnetsToSpace(ctx,
		[]string{
			movedSubnetUUID1,
			movedSubnetUUID2,
		},
		"to-space",
		false) // Force=false

	// Assert
	c.Assert(err, tc.IsNil)
	c.Assert(movedSubnets, tc.SameContents, []domainnetwork.MovedSubnets{
		{
			UUID:      domainnetwork.SubnetUUID(movedSubnetUUID1),
			FromSpace: "from-space-1",
		},
		{
			UUID:      domainnetwork.SubnetUUID(movedSubnetUUID2),
			FromSpace: "from-space-2",
		},
	})

	// Verify the subnets were actually moved in the database
	c.Assert(s.fetchSubnetSpace(c), tc.SameContents, []subnetSpace{
		{
			UUID:  movedSubnetUUID1,
			Space: "to-space",
		},
		{
			UUID:  movedSubnetUUID2,
			Space: "to-space",
		},
		{
			UUID:  otherSubnetUUID, // not moved
			Space: "other-space",
		},
	})
}

type subnetSpace struct {
	UUID  string
	Space string
}

// fetchSubnetSpace retrieves subnet and space mapping from the database
func (s *spaceMachineSuite) fetchSubnetSpace(c *tc.C) []subnetSpace {
	ctx := c.Context()
	// Verify the subnets were actually moved in the database
	var result []subnetSpace
	err := s.TxnRunner().StdTxn(ctx, func(ctx context.Context, tx *sql.Tx) error {
		rows, err := tx.QueryContext(ctx, `
SELECT subnet.uuid, space.name
FROM subnet
JOIN space ON space.uuid = subnet.space_uuid
`)
		if err != nil {
			return err
		}
		defer rows.Close()
		for rows.Next() {
			var uuid string
			var space string
			err := rows.Scan(&uuid, &space)
			c.Assert(err, tc.IsNil)
			result = append(result, subnetSpace{
				UUID:  uuid,
				Space: space,
			})
		}
		return nil
	})
	c.Assert(err, tc.IsNil)
	return result
}

// addSpace inserts a new space with the given name into the database
// and returns its UUID.
func (s *spaceMachineSuite) addSpace(c *tc.C, name string) string {
	spaceUUID := uuid.MustNewUUID().String()
	s.query(c, `INSERT INTO space (uuid, name) VALUES (?, ?)`,
		spaceUUID, name)
	return spaceUUID
}

// addSpaceConstraint adds a space constraint to a machine with a given UUID,
// associating it with a space name and include/exclude behavior.
// It returns the generated constraint UUID for the added space constraint.
func (s *spaceMachineSuite) addSpaceConstraint(c *tc.C, machineUUID, spaceName string, positive bool) string {
	constraintUUID := uuid.MustNewUUID().String()
	s.query(c, `INSERT INTO "constraint" (uuid) VALUES (?)`, constraintUUID)
	s.query(c, `INSERT INTO machine_constraint (machine_uuid, constraint_uuid) VALUES (?, ?)`, machineUUID,
		constraintUUID)
	s.query(c, `INSERT INTO constraint_space (constraint_uuid, space, exclude) VALUES (?, ?, ?)`,
		constraintUUID, spaceName, !positive)
	return constraintUUID
}
