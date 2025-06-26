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
	modeltesting "github.com/juju/juju/core/model/testing"
	coreunit "github.com/juju/juju/core/unit"
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

	modelUUID := modeltesting.GenModelUUID(c)
	err := s.TxnRunner().StdTxn(c.Context(), func(ctx context.Context, tx *sql.Tx) error {
		_, err := tx.ExecContext(ctx, `
			INSERT INTO model (uuid, controller_uuid, name, qualifier, type, cloud, cloud_type)
			VALUES (?, ?, "test", "prod", "iaas", "test-model", "ec2")
		`, modelUUID.String(), coretesting.ControllerTag.Id())
		return err
	})
	c.Assert(err, tc.ErrorIsNil)

	machineSt := machinestate.NewState(s.TxnRunnerFactory(), clock.WallClock, logger.GetLogger("juju.test.machine"))

	err = machineSt.CreateMachine(c.Context(), "0", netNodeUUIDs[0], machine.UUID(machineUUIDs[0]), nil)
	c.Assert(err, tc.ErrorIsNil)
	err = machineSt.CreateMachine(c.Context(), "1", netNodeUUIDs[1], machine.UUID(machineUUIDs[1]), nil)
	c.Assert(err, tc.ErrorIsNil)

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
