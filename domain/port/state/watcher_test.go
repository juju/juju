// Copyright 2024 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state

import (
	"context"
	"database/sql"

	"github.com/juju/clock"
	"github.com/juju/collections/set"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

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

var _ = gc.Suite(&watcherSuite{})

func (s *watcherSuite) SetUpTest(c *gc.C) {
	s.ModelSuite.SetUpTest(c)

	modelUUID := modeltesting.GenModelUUID(c)
	err := s.TxnRunner().StdTxn(context.Background(), func(ctx context.Context, tx *sql.Tx) error {
		_, err := tx.ExecContext(ctx, `
			INSERT INTO model (uuid, controller_uuid, name, type, cloud, cloud_type)
			VALUES (?, ?, "test", "iaas", "test-model", "ec2")
		`, modelUUID.String(), coretesting.ControllerTag.Id())
		return err
	})
	c.Assert(err, jc.ErrorIsNil)

	machineSt := machinestate.NewState(s.TxnRunnerFactory(), clock.WallClock, logger.GetLogger("juju.test.machine"))

	err = machineSt.CreateMachine(context.Background(), "0", netNodeUUIDs[0], machine.UUID(machineUUIDs[0]))
	c.Assert(err, jc.ErrorIsNil)
	err = machineSt.CreateMachine(context.Background(), "1", netNodeUUIDs[1], machine.UUID(machineUUIDs[1]))
	c.Assert(err, jc.ErrorIsNil)

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

func (s *watcherSuite) TestGetMachinesForUnitEndpoints(c *gc.C) {
	st := NewState(s.TxnRunnerFactory())
	ctx := context.Background()

	machineUUIDsForEndpoint, err := st.GetMachineNamesForUnits(ctx, s.unitUUIDs[:])
	c.Assert(err, jc.ErrorIsNil)
	c.Check(machineUUIDsForEndpoint, jc.SameContents, []machine.Name{"0", "1"})

	machineUUIDsForEndpoint, err = st.GetMachineNamesForUnits(ctx, []coreunit.UUID{s.unitUUIDs[0]})
	c.Assert(err, jc.ErrorIsNil)
	c.Check(machineUUIDsForEndpoint, jc.DeepEquals, []machine.Name{"0"})
}

func (s *watcherSuite) TestFilterEndpointForApplication(c *gc.C) {
	st := NewState(s.TxnRunnerFactory())
	ctx := context.Background()

	filteredUnits, err := st.FilterUnitUUIDsForApplication(ctx, s.unitUUIDs[:], s.appUUIDs[0])
	c.Assert(err, jc.ErrorIsNil)
	c.Check(filteredUnits, jc.DeepEquals, set.NewStrings(s.unitUUIDs[0].String()))

	filteredUnits, err = st.FilterUnitUUIDsForApplication(ctx, s.unitUUIDs[:], s.appUUIDs[1])
	c.Assert(err, jc.ErrorIsNil)
	c.Check(filteredUnits, jc.DeepEquals, set.NewStrings(s.unitUUIDs[1].String(), s.unitUUIDs[2].String()))
}
