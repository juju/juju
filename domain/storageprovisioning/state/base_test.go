// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state

import (
	"context"
	"database/sql"

	coremachine "github.com/juju/juju/core/machine"
	machinetesting "github.com/juju/juju/core/machine/testing"
	"github.com/juju/juju/core/network"
	coreunit "github.com/juju/juju/core/unit"
	domainlife "github.com/juju/juju/domain/life"
	domainnetwork "github.com/juju/juju/domain/network"
	schematesting "github.com/juju/juju/domain/schema/testing"
	"github.com/juju/juju/internal/uuid"
	"github.com/juju/tc"
)

// baseSuite defines a set of common helper methods for storage provisioning
// tests. Base suite does not seed a starting state and does not run any tests.
type baseSuite struct {
	schematesting.ModelSuite
}

// newMachineWithNetNode creates a new machine in the model attached to the
// supplied net node. The newly created machines uuid is returned along with the
// name.
func (s *baseSuite) newMachineWithNetNode(c *tc.C, netNodeUUID string) (string, coremachine.Name) {
	machineUUID := machinetesting.GenUUID(c)
	name := "mfoo-" + machineUUID.String()

	_, err := s.DB().ExecContext(
		c.Context(),
		"INSERT INTO machine (uuid, name, net_node_uuid, life_id) VALUES (?, ?, ?, 0)",
		machineUUID.String(),
		name,
		netNodeUUID,
	)
	c.Assert(err, tc.ErrorIsNil)

	return machineUUID.String(), coremachine.Name(name)
}

// changeMachineLife is a utility function for updating the life value of a
// machine.
func (s *baseSuite) changeMachineLife(c *tc.C, machineUUID string, lifeID domainlife.Life) {
	_, err := s.DB().ExecContext(
		c.Context(),
		"UPDATE machine SET life_id = ? WHERE uuid = ?",
		int(lifeID),
		machineUUID,
	)
	c.Assert(err, tc.ErrorIsNil)
}

// newNetNode creates a new net node in the model for referencing to storage
// entity attachments. The net node is not associated with any machine or units.
func (s *baseSuite) newNetNode(c *tc.C) string {
	nodeUUID, err := domainnetwork.NewNetNodeUUID()
	c.Assert(err, tc.ErrorIsNil)

	_, err = s.DB().ExecContext(
		c.Context(),
		"INSERT INTO net_node VALUES (?)",
		nodeUUID.String(),
	)
	c.Assert(err, tc.ErrorIsNil)

	return nodeUUID.String()
}

// newApplication creates a new application in the model returning the uuid of
// the new application.
func (s *baseSuite) newApplication(c *tc.C, name string) string {
	appUUID, err := uuid.NewUUID()
	c.Assert(err, tc.ErrorIsNil)

	err = s.TxnRunner().StdTxn(c.Context(), func(ctx context.Context, tx *sql.Tx) error {
		_, err := tx.ExecContext(ctx, `
INSERT INTO charm (uuid, source_id, reference_name, revision, architecture_id)
VALUES (?, 0, ?, 1, 0)`, appUUID.String(), name)
		if err != nil {
			return err
		}

		_, err = tx.ExecContext(ctx, `
INSERT INTO charm_metadata (charm_uuid, name)
VALUES (?, 'myapp')`, appUUID.String())
		if err != nil {
			return err
		}

		_, err = tx.ExecContext(ctx, `
INSERT INTO application (uuid, charm_uuid, name, life_id, space_uuid)
VALUES (?, ?, ?, "0", ?)`, appUUID.String(), appUUID.String(), name, network.AlphaSpaceId)
		return err
	})
	c.Assert(err, tc.ErrorIsNil)

	return appUUID.String()
}

// newUnitWithNetNode creates a new unit in the model for the provided
// application uuid. The new unit will use the supplied net node. Returned is
// the new uuid of the unit and the name that was used.
func (s *baseSuite) newUnitWithNetNode(c *tc.C, name, appUUID, netNodeUUID string) (string, coreunit.Name) {
	var charmUUID string
	err := s.DB().QueryRowContext(
		c.Context(),
		"SELECT charm_uuid FROM application WHERE uuid = ?",
		appUUID,
	).Scan(&charmUUID)
	c.Assert(err, tc.ErrorIsNil)

	unitUUID, err := uuid.NewUUID()
	c.Assert(err, tc.ErrorIsNil)

	_, err = s.DB().ExecContext(
		c.Context(), `
INSERT INTO unit (uuid, name, application_uuid, charm_uuid, net_node_uuid, life_id)
VALUES (?, ?, ?, ?, ?, 0)
`,
		unitUUID.String(), name, appUUID, charmUUID, netNodeUUID,
	)
	c.Assert(err, tc.ErrorIsNil)

	return unitUUID.String(), coreunit.Name(name)
}
