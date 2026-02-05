// Copyright 2024 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state

import (
	"context"
	"database/sql"
	"testing"

	"github.com/juju/clock"
	"github.com/juju/collections/set"
	"github.com/juju/collections/transform"
	"github.com/juju/tc"

	coreapplication "github.com/juju/juju/core/application"
	coremodel "github.com/juju/juju/core/model"
	coreunit "github.com/juju/juju/core/unit"
	"github.com/juju/juju/domain/deployment"
	domainmachine "github.com/juju/juju/domain/machine"
	machinestate "github.com/juju/juju/domain/machine/state"
	"github.com/juju/juju/internal/logger"
	coretesting "github.com/juju/juju/internal/testing"
)

type watcherSuite struct {
	baseSuite

	unitUUIDs [3]coreunit.UUID

	appUUIDs [2]coreapplication.UUID
}

func TestWatcherSuite(t *testing.T) {
	tc.Run(t, &watcherSuite{})
}

func (s *watcherSuite) SetUpTest(c *tc.C) {
	s.ModelSuite.SetUpTest(c)

	modelUUID := tc.Must0(c, coremodel.NewUUID)
	err := s.TxnRunner().StdTxn(c.Context(), func(ctx context.Context, tx *sql.Tx) error {
		_, err := tx.ExecContext(ctx, `
INSERT INTO model (uuid, controller_uuid, name, qualifier, type, cloud, cloud_type)
VALUES (?, ?, "test", "prod", "iaas", "test-model", "ec2")
		`, modelUUID.String(), coretesting.ControllerTag.Id())
		return err
	})
	c.Assert(err, tc.ErrorIsNil)

	machineSt := machinestate.NewState(s.TxnRunnerFactory(), clock.WallClock, logger.GetLogger("juju.test.machine"))

	netNodeUUID0, machineNames0, err := machineSt.AddMachine(c.Context(), domainmachine.AddMachineArgs{
		Platform: deployment.Platform{
			Channel: "24.04",
			OSType:  deployment.Ubuntu,
		},
	})
	c.Assert(err, tc.ErrorIsNil)
	machineUUID0, err := machineSt.GetMachineUUID(c.Context(), machineNames0[0])
	c.Assert(err, tc.ErrorIsNil)
	netNodeUUID1, machineNames1, err := machineSt.AddMachine(c.Context(), domainmachine.AddMachineArgs{
		Platform: deployment.Platform{
			Channel: "24.04",
			OSType:  deployment.Ubuntu,
		},
	})
	c.Assert(err, tc.ErrorIsNil)
	machineUUID1, err := machineSt.GetMachineUUID(c.Context(), machineNames1[0])
	c.Assert(err, tc.ErrorIsNil)

	machineUUIDs = []string{machineUUID0.String(), machineUUID1.String()}
	netNodeUUIDs = []string{netNodeUUID0, netNodeUUID1}

	s.appUUIDs[0] = s.createApplicationWithRelations(c, appNames[0], "ep0", "ep1", "ep2")
	s.appUUIDs[1] = s.createApplicationWithRelations(c, appNames[1], "ep0", "ep1", "ep2")

	s.unitUUIDs[0], _ = s.createUnit(c, netNodeUUIDs[0], appNames[0])
	s.unitUUIDs[1], _ = s.createUnit(c, netNodeUUIDs[0], appNames[1])
	s.unitUUIDs[2], _ = s.createUnit(c, netNodeUUIDs[1], appNames[1])
}

/*
* The following tests will run with the following context:
* - 3 units are deployed (with uuids stored in s.unitUUIDs)
* - 2 machines are deployed (with uuids stored in machineUUIDs)
*   - machine 0 hosts units 0 & 1
*   - machine 1 hosts unit 2
* - on 2 applications (with names stored in appNames; uuids s.appUUIDs)
*   - unit 0 is deployed to app 0
*   - units 1 & 2 are deployed to app 1
 */

func (s *watcherSuite) TestFilterEndpointForApplication(c *tc.C) {
	st := NewState(s.TxnRunnerFactory())

	filteredUnits, err := st.FilterUnitUUIDsForApplication(c.Context(), s.unitUUIDs[:], s.appUUIDs[0])
	c.Assert(err, tc.ErrorIsNil)
	c.Check(filteredUnits, tc.DeepEquals, set.NewStrings(s.unitUUIDs[0].String()))

	filteredUnits, err = st.FilterUnitUUIDsForApplication(c.Context(), s.unitUUIDs[:], s.appUUIDs[1])
	c.Assert(err, tc.ErrorIsNil)
	c.Check(filteredUnits, tc.DeepEquals, set.NewStrings(s.unitUUIDs[1].String(), s.unitUUIDs[2].String()))
}

func (s *watcherSuite) TestInitialWatchOpenedPortsStatement(c *tc.C) {
	st := NewState(s.TxnRunnerFactory())
	_, statement := st.InitialWatchOpenedPortsStatement()
	s.assertUnits(c, statement, s.unitUUIDs[:])

	// Create a unit that doesn't have an associated machine and assert that it
	// isn't included in the results of the initial statement.

	netNode := s.createNetNode(c)
	appUUID := s.createApplicationWithRelations(c, "inferi", "ep0", "ep1", "ep2")
	s.createUnitWithoutMachine(c, netNode, "inferi", appUUID.String())

	// Notice that the results are unchanged, which implies that the unit
	// without a machine is not included in the results of the initial
	// statement.
	s.assertUnits(c, statement, s.unitUUIDs[:])
}

func (s *watcherSuite) assertUnits(c *tc.C, stmt string, expected []coreunit.UUID) {
	var unitUUIDs []string
	err := s.TxnRunner().StdTxn(c.Context(), func(ctx context.Context, tx *sql.Tx) error {
		rows, err := tx.QueryContext(ctx, stmt)
		if err != nil {
			return err
		}
		defer rows.Close()

		for rows.Next() {
			var unitUUID string
			if err := rows.Scan(&unitUUID); err != nil {
				return err
			}

			unitUUIDs = append(unitUUIDs, unitUUID)
		}

		return nil
	})
	c.Assert(err, tc.ErrorIsNil)

	c.Check(unitUUIDs, tc.SameContents, transform.Slice(expected, func(u coreunit.UUID) string {
		return u.String()
	}))
}

func (s *watcherSuite) createNetNode(c *tc.C) string {
	netNodeUUID := tc.Must0(c, coreunit.NewUUID).String()
	err := s.TxnRunner().StdTxn(c.Context(), func(ctx context.Context, tx *sql.Tx) error {
		_, err := tx.ExecContext(ctx, `INSERT INTO net_node (uuid) VALUES (?)`, netNodeUUID)
		return err
	})
	c.Assert(err, tc.ErrorIsNil)

	return netNodeUUID
}

// createUnit creates a new unit in state and returns its UUID. The unit is assigned
// to the net node with uuid `netNodeUUID` and application with name `appName`.
func (s *watcherSuite) createUnitWithoutMachine(c *tc.C, netNodeUUID, appName, appUUID string) {
	unitUUID := tc.Must0(c, coreunit.NewUUID).String()
	unitName := appName + "/0"

	err := s.TxnRunner().StdTxn(c.Context(), func(ctx context.Context, tx *sql.Tx) error {
		// Get the charm UUID from the application name.
		var charmUUID string
		err := tx.QueryRowContext(ctx, `SELECT charm_uuid FROM application WHERE uuid = ?`, appUUID).Scan(&charmUUID)
		if err != nil {
			return err
		}

		// Insert the unit without an associated machine.
		_, err = tx.ExecContext(ctx, `
INSERT INTO unit (uuid, name, application_uuid, net_node_uuid, life_id, charm_uuid)
VALUES (?, ?, ?, ?, 0, ?)
		`, unitUUID, unitName, appUUID, netNodeUUID, charmUUID)
		return err
	})
	c.Assert(err, tc.ErrorIsNil)
}
