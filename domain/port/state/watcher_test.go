// Copyright 2024 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state

import (
	"context"

	"github.com/canonical/sqlair"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	coreapplication "github.com/juju/juju/core/application"
	"github.com/juju/juju/core/machine"
	"github.com/juju/juju/core/network"
	coreunit "github.com/juju/juju/core/unit"
	"github.com/juju/juju/domain"
	machinestate "github.com/juju/juju/domain/machine/state"
	"github.com/juju/juju/internal/logger"
)

type watcherSuite struct {
	baseSuite

	unitUUIDs [3]coreunit.UUID

	appUUIDs [2]coreapplication.ID
}

var _ = gc.Suite(&watcherSuite{})

var (
	ssh   = network.PortRange{Protocol: "tcp", FromPort: 22, ToPort: 22}
	http  = network.PortRange{Protocol: "tcp", FromPort: 80, ToPort: 80}
	https = network.PortRange{Protocol: "tcp", FromPort: 443, ToPort: 443}
)

func (s *watcherSuite) SetUpTest(c *gc.C) {
	s.ModelSuite.SetUpTest(c)

	machineSt := machinestate.NewState(s.TxnRunnerFactory(), logger.GetLogger("juju.test.machine"))

	err := machineSt.CreateMachine(context.Background(), "0", netNodeUUIDs[0], machineUUIDs[0])
	c.Assert(err, jc.ErrorIsNil)

	s.unitUUIDs[0], _, s.appUUIDs[0] = s.createUnit(c, netNodeUUIDs[0], appNames[0])
	s.unitUUIDs[1], _, s.appUUIDs[1] = s.createUnit(c, netNodeUUIDs[0], appNames[1])

	err = machineSt.CreateMachine(context.Background(), "1", netNodeUUIDs[1], machineUUIDs[1])
	c.Assert(err, jc.ErrorIsNil)

	s.unitUUIDs[2], _, _ = s.createUnit(c, netNodeUUIDs[1], appNames[1])
}

func (s *watcherSuite) initialiseOpenPorts(c *gc.C, st *State) ([]string, map[string]string) {
	err := st.RunAtomic(context.Background(), func(ctx domain.AtomicContext) error {
		err := st.UpdateUnitPorts(ctx, s.unitUUIDs[0], network.GroupedPortRanges{
			"ep0": {ssh},
		}, network.GroupedPortRanges{})
		if err != nil {
			return err
		}
		err = st.UpdateUnitPorts(ctx, s.unitUUIDs[1], network.GroupedPortRanges{
			"ep1": {http},
		}, network.GroupedPortRanges{})
		if err != nil {
			return err
		}
		return st.UpdateUnitPorts(ctx, s.unitUUIDs[2], network.GroupedPortRanges{
			"ep2": {https},
		}, network.GroupedPortRanges{})
	})
	c.Assert(err, jc.ErrorIsNil)

	query, err := sqlair.Prepare(`
SELECT &endpoint.*
FROM unit_endpoint
`, endpoint{})
	c.Assert(err, jc.ErrorIsNil)

	var endpoints []endpoint
	err = s.TxnRunner().Txn(context.Background(), func(ctx context.Context, tx *sqlair.TX) error {
		return tx.Query(ctx, query).GetAll(&endpoints)
	})
	c.Assert(err, jc.ErrorIsNil)

	endpointToUUIDMap := make(map[string]string)
	endpointUUIDs := make([]string, len(endpoints))
	for i, ep := range endpoints {
		endpointToUUIDMap[ep.Endpoint] = ep.UUID
		endpointUUIDs[i] = ep.UUID
	}

	return endpointUUIDs, endpointToUUIDMap
}

/*
* The following tests are set up to run with the following context:
* - 3 units are deployed (with uuids stored in s.unitUUIDs)
* - 2 machines are deployed (with uuids stored in machineUUIDs)
*   - machine 0 hosts units 0 & 1
*   - machine 1 hosts unit 2
* - on 2 applications (with names stored in appNames; uuids s.appUUIDs)
*   - unit 0 is deployed to app 0
*   - units 1 & 2 are deployed to app 1
*
* - The following ports are open:
*   - ssh is open on endpoint 0 on unit 0
*   - http is open on endpoint 1 on unit 1
*   - https is open on endpoint 2 on unit 2
 */

func (s *watcherSuite) TestGetMachinesForUnitEndpoints(c *gc.C) {
	st := NewState(s.TxnRunnerFactory())
	ctx := context.Background()
	endpointUUIDs, endpointToUUIDMap := s.initialiseOpenPorts(c, st)

	machineUUIDsForUnitEndpoints, err := st.GetMachineNamesForUnitEndpoints(ctx, endpointUUIDs)
	c.Assert(err, jc.ErrorIsNil)
	c.Check(machineUUIDsForUnitEndpoints, jc.SameContents, []machine.Name{"0", "1"})

	machineUUIDsForUnitEndpoints, err = st.GetMachineNamesForUnitEndpoints(ctx, []string{endpointToUUIDMap["ep0"]})
	c.Assert(err, jc.ErrorIsNil)
	c.Check(machineUUIDsForUnitEndpoints, jc.DeepEquals, []machine.Name{"0"})

	machineUUIDsForUnitEndpoints, err = st.GetMachineNamesForUnitEndpoints(ctx, []string{endpointToUUIDMap["ep0"], endpointToUUIDMap["ep1"]})
	c.Assert(err, jc.ErrorIsNil)
	c.Check(machineUUIDsForUnitEndpoints, jc.DeepEquals, []machine.Name{"0"})
}

func (s *watcherSuite) TestGetApplicationForUnitEndpoints(c *gc.C) {
	st := NewState(s.TxnRunnerFactory())
	ctx := context.Background()
	endpointUUIDs, endpointToUUIDMap := s.initialiseOpenPorts(c, st)

	appNamesForUnitEndpoints, err := st.GetApplicationNamesForUnitEndpoints(ctx, endpointUUIDs)
	c.Assert(err, jc.ErrorIsNil)
	c.Check(appNamesForUnitEndpoints, jc.SameContents, []string{appNames[0], appNames[1]})

	appNamesForUnitEndpoints, err = st.GetApplicationNamesForUnitEndpoints(ctx, []string{endpointToUUIDMap["ep0"]})
	c.Assert(err, jc.ErrorIsNil)
	c.Check(appNamesForUnitEndpoints, jc.SameContents, []string{appNames[0]})

	appNamesForUnitEndpoints, err = st.GetApplicationNamesForUnitEndpoints(ctx, []string{endpointToUUIDMap["ep1"], endpointToUUIDMap["ep2"]})
	c.Assert(err, jc.ErrorIsNil)
	c.Check(appNamesForUnitEndpoints, jc.SameContents, []string{appNames[1]})
}
