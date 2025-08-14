// Copyright 2024 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state

import (
	"context"
	"database/sql"
	"testing"

	"github.com/juju/clock"
	"github.com/juju/collections/set"
	"github.com/juju/tc"

	coreapplication "github.com/juju/juju/core/application"
	"github.com/juju/juju/core/machine"
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

	appUUIDs [2]coreapplication.ID
}

func TestWatcherSuite(t *testing.T) {
	tc.Run(t, &watcherSuite{})
}

func (s *watcherSuite) SetUpTest(c *tc.C) {
	s.ModelSuite.SetUpTest(c)

	modelUUID := coremodel.GenUUID(c)
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

func (s *watcherSuite) TestGetMachinesForUnitEndpoints(c *tc.C) {
	st := NewState(s.TxnRunnerFactory())
	ctx := c.Context()

	machineUUIDsForEndpoint, err := st.GetMachineNamesForUnits(ctx, s.unitUUIDs[:])
	c.Assert(err, tc.ErrorIsNil)
	c.Check(machineUUIDsForEndpoint, tc.SameContents, []machine.Name{"0", "1"})

	machineUUIDsForEndpoint, err = st.GetMachineNamesForUnits(ctx, []coreunit.UUID{s.unitUUIDs[0]})
	c.Assert(err, tc.ErrorIsNil)
	c.Check(machineUUIDsForEndpoint, tc.DeepEquals, []machine.Name{"0"})
}

func (s *watcherSuite) TestFilterEndpointForApplication(c *tc.C) {
	st := NewState(s.TxnRunnerFactory())
	ctx := c.Context()

	filteredUnits, err := st.FilterUnitUUIDsForApplication(ctx, s.unitUUIDs[:], s.appUUIDs[0])
	c.Assert(err, tc.ErrorIsNil)
	c.Check(filteredUnits, tc.DeepEquals, set.NewStrings(s.unitUUIDs[0].String()))

	filteredUnits, err = st.FilterUnitUUIDsForApplication(ctx, s.unitUUIDs[:], s.appUUIDs[1])
	c.Assert(err, tc.ErrorIsNil)
	c.Check(filteredUnits, tc.DeepEquals, set.NewStrings(s.unitUUIDs[1].String(), s.unitUUIDs[2].String()))
}
