// Copyright 2024 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state

import (
	"context"
	"database/sql"

	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/core/network"
	"github.com/juju/juju/domain/application"
	"github.com/juju/juju/domain/application/charm"
	applicationstate "github.com/juju/juju/domain/application/state"
	"github.com/juju/juju/internal/changestream/testing"
	"github.com/juju/juju/internal/logger"
)

type stateSuite struct {
	testing.ModelSuite

	unitUUID string
}

var _ = gc.Suite(&stateSuite{})

func ptr[T any](v T) *T {
	return &v
}

func (s *stateSuite) SetUpTest(c *gc.C) {
	s.ModelSuite.SetUpTest(c)
	applicationSt := applicationstate.NewApplicationState(s.TxnRunnerFactory(), logger.GetLogger("juju.test.application"))
	_, err := applicationSt.CreateApplication(context.Background(), "app", application.AddApplicationArg{
		Charm: charm.Charm{
			Metadata: charm.Metadata{
				Name: "app",
			},
		},
	}, application.UpsertUnitArg{UnitName: ptr("app/0")})
	c.Assert(err, jc.ErrorIsNil)

	err = s.TxnRunner().StdTxn(context.Background(), func(ctx context.Context, tx *sql.Tx) error {
		err := tx.QueryRowContext(ctx, "SELECT uuid FROM unit").Scan(&s.unitUUID)
		c.Assert(err, jc.ErrorIsNil)
		return nil
	})
	c.Assert(err, jc.ErrorIsNil)
}

func (s *stateSuite) initialiseOpenPort(c *gc.C, st *State) {
	ctx := context.Background()
	err := st.UpdateUnitPorts(ctx, s.unitUUID, network.GroupedPortRanges{
		"endpoint": {
			{Protocol: "tcp", FromPort: 80, ToPort: 80},
			{Protocol: "udp", FromPort: 1000, ToPort: 1500},
		},
		"misc": {
			{Protocol: "tcp", FromPort: 8080, ToPort: 8080},
		},
	}, network.GroupedPortRanges{})
	c.Assert(err, jc.ErrorIsNil)
}

func (s *stateSuite) TestGetOpenedPortsBlankDB(c *gc.C) {
	st := NewState(s.TxnRunnerFactory())
	ctx := context.Background()

	groupedPortRanges, err := st.GetOpenedPorts(ctx, s.unitUUID)
	c.Assert(err, jc.ErrorIsNil)
	c.Check(groupedPortRanges, gc.HasLen, 0)

	groupedPortRanges, err = st.GetOpenedPorts(ctx, "non-existent")
	c.Assert(err, jc.ErrorIsNil)
	c.Check(groupedPortRanges, gc.HasLen, 0)
}

func (s *stateSuite) TestGetOpenedPorts(c *gc.C) {
	st := NewState(s.TxnRunnerFactory())
	ctx := context.Background()
	s.initialiseOpenPort(c, st)

	groupedPortRanges, err := st.GetOpenedPorts(ctx, s.unitUUID)
	c.Assert(err, jc.ErrorIsNil)
	c.Check(groupedPortRanges, gc.HasLen, 2)

	c.Check(groupedPortRanges["endpoint"], gc.HasLen, 2)
	c.Check(groupedPortRanges["endpoint"][0], jc.DeepEquals, network.PortRange{Protocol: "tcp", FromPort: 80, ToPort: 80})
	c.Check(groupedPortRanges["endpoint"][1], jc.DeepEquals, network.PortRange{Protocol: "udp", FromPort: 1000, ToPort: 1500})

	c.Check(groupedPortRanges["misc"], gc.HasLen, 1)
	c.Check(groupedPortRanges["misc"][0], jc.DeepEquals, network.PortRange{Protocol: "tcp", FromPort: 8080, ToPort: 8080})
}

func (s *stateSuite) TestUpdateUnitPortsOpenPort(c *gc.C) {
	st := NewState(s.TxnRunnerFactory())
	ctx := context.Background()
	s.initialiseOpenPort(c, st)

	err := st.UpdateUnitPorts(ctx, s.unitUUID, network.GroupedPortRanges{"endpoint": {{Protocol: "tcp", FromPort: 1000, ToPort: 1500}}}, network.GroupedPortRanges{})
	c.Assert(err, jc.ErrorIsNil)

	groupedPortRanges, err := st.GetOpenedPorts(ctx, s.unitUUID)
	c.Assert(err, jc.ErrorIsNil)
	c.Check(groupedPortRanges, gc.HasLen, 2)
	c.Check(groupedPortRanges["endpoint"], gc.HasLen, 3)
	c.Check(groupedPortRanges["endpoint"][0], jc.DeepEquals, network.PortRange{Protocol: "tcp", FromPort: 80, ToPort: 80})
	c.Check(groupedPortRanges["endpoint"][1], jc.DeepEquals, network.PortRange{Protocol: "tcp", FromPort: 1000, ToPort: 1500})
	c.Check(groupedPortRanges["endpoint"][2], jc.DeepEquals, network.PortRange{Protocol: "udp", FromPort: 1000, ToPort: 1500})

	c.Check(groupedPortRanges["misc"], gc.HasLen, 1)
	c.Check(groupedPortRanges["misc"][0], jc.DeepEquals, network.PortRange{Protocol: "tcp", FromPort: 8080, ToPort: 8080})
}

func (s *stateSuite) TestUpdateUnitPortsClosePort(c *gc.C) {
	st := NewState(s.TxnRunnerFactory())
	ctx := context.Background()
	s.initialiseOpenPort(c, st)

	groupedPortRanges, err := st.GetOpenedPorts(ctx, s.unitUUID)
	c.Assert(err, jc.ErrorIsNil)
	c.Check(groupedPortRanges, gc.HasLen, 2)
	c.Check(groupedPortRanges["endpoint"], gc.HasLen, 2)
	c.Check(groupedPortRanges["endpoint"][0], jc.DeepEquals, network.PortRange{Protocol: "tcp", FromPort: 80, ToPort: 80})
	c.Check(groupedPortRanges["endpoint"][1], jc.DeepEquals, network.PortRange{Protocol: "udp", FromPort: 1000, ToPort: 1500})

	c.Check(groupedPortRanges["misc"], gc.HasLen, 1)
	c.Check(groupedPortRanges["misc"][0], jc.DeepEquals, network.PortRange{Protocol: "tcp", FromPort: 8080, ToPort: 8080})

	err = st.UpdateUnitPorts(ctx, s.unitUUID, network.GroupedPortRanges{}, network.GroupedPortRanges{"endpoint": {{Protocol: "tcp", FromPort: 80, ToPort: 80}}})
	c.Assert(err, jc.ErrorIsNil)

	groupedPortRanges, err = st.GetOpenedPorts(ctx, s.unitUUID)
	c.Assert(err, jc.ErrorIsNil)
	c.Check(groupedPortRanges, gc.HasLen, 2)
	c.Check(groupedPortRanges["endpoint"], gc.HasLen, 1)
	c.Check(groupedPortRanges["endpoint"][0], jc.DeepEquals, network.PortRange{Protocol: "udp", FromPort: 1000, ToPort: 1500})

	c.Check(groupedPortRanges["misc"], gc.HasLen, 1)
	c.Check(groupedPortRanges["misc"][0], jc.DeepEquals, network.PortRange{Protocol: "tcp", FromPort: 8080, ToPort: 8080})
}

func (s *stateSuite) TestUpdateUnitPortsOpenPortRangeAdjacent(c *gc.C) {
	st := NewState(s.TxnRunnerFactory())
	ctx := context.Background()
	s.initialiseOpenPort(c, st)

	err := st.UpdateUnitPorts(ctx, s.unitUUID, network.GroupedPortRanges{"endpoint": {{Protocol: "udp", FromPort: 1501, ToPort: 2000}}}, network.GroupedPortRanges{})
	c.Assert(err, jc.ErrorIsNil)

	groupedPortRanges, err := st.GetOpenedPorts(ctx, s.unitUUID)
	c.Assert(err, jc.ErrorIsNil)
	c.Check(groupedPortRanges, gc.HasLen, 2)
	c.Check(groupedPortRanges["endpoint"], gc.HasLen, 2)
	c.Check(groupedPortRanges["endpoint"][0], jc.DeepEquals, network.PortRange{Protocol: "tcp", FromPort: 80, ToPort: 80})
	c.Check(groupedPortRanges["endpoint"][1], jc.DeepEquals, network.PortRange{Protocol: "udp", FromPort: 1000, ToPort: 2000})

	c.Check(groupedPortRanges["misc"], gc.HasLen, 1)
	c.Check(groupedPortRanges["misc"][0], jc.DeepEquals, network.PortRange{Protocol: "tcp", FromPort: 8080, ToPort: 8080})
}

func (s *stateSuite) TestUpdateUnitPortsOpenPortRangeOverlapping(c *gc.C) {
	st := NewState(s.TxnRunnerFactory())
	ctx := context.Background()
	s.initialiseOpenPort(c, st)

	err := st.UpdateUnitPorts(ctx, s.unitUUID, network.GroupedPortRanges{"endpoint": {{Protocol: "udp", FromPort: 1250, ToPort: 2000}}}, network.GroupedPortRanges{})
	c.Assert(err, jc.ErrorIsNil)

	groupedPortRanges, err := st.GetOpenedPorts(ctx, s.unitUUID)
	c.Assert(err, jc.ErrorIsNil)
	c.Check(groupedPortRanges, gc.HasLen, 2)
	c.Check(groupedPortRanges["endpoint"], gc.HasLen, 2)
	c.Check(groupedPortRanges["endpoint"][0], jc.DeepEquals, network.PortRange{Protocol: "tcp", FromPort: 80, ToPort: 80})
	c.Check(groupedPortRanges["endpoint"][1], jc.DeepEquals, network.PortRange{Protocol: "udp", FromPort: 1000, ToPort: 2000})

	c.Check(groupedPortRanges["misc"], gc.HasLen, 1)
	c.Check(groupedPortRanges["misc"][0], jc.DeepEquals, network.PortRange{Protocol: "tcp", FromPort: 8080, ToPort: 8080})
}

func (s *stateSuite) TestUpdateUnitPortsClosePortRange(c *gc.C) {
	st := NewState(s.TxnRunnerFactory())
	ctx := context.Background()
	s.initialiseOpenPort(c, st)

	err := st.UpdateUnitPorts(ctx, s.unitUUID, network.GroupedPortRanges{}, network.GroupedPortRanges{"endpoint": {{Protocol: "udp", FromPort: 1000, ToPort: 1500}}})
	c.Assert(err, jc.ErrorIsNil)

	groupedPortRanges, err := st.GetOpenedPorts(ctx, s.unitUUID)
	c.Assert(err, jc.ErrorIsNil)
	c.Check(groupedPortRanges, gc.HasLen, 2)
	c.Check(groupedPortRanges["endpoint"], gc.HasLen, 1)
	c.Check(groupedPortRanges["endpoint"][0], jc.DeepEquals, network.PortRange{Protocol: "tcp", FromPort: 80, ToPort: 80})

	c.Check(groupedPortRanges["misc"], gc.HasLen, 1)
	c.Check(groupedPortRanges["misc"][0], jc.DeepEquals, network.PortRange{Protocol: "tcp", FromPort: 8080, ToPort: 8080})
}

func (s *stateSuite) TestUpdateUnitPortsClosePortEndpoint(c *gc.C) {
	st := NewState(s.TxnRunnerFactory())
	ctx := context.Background()
	s.initialiseOpenPort(c, st)

	err := st.UpdateUnitPorts(ctx, s.unitUUID, network.GroupedPortRanges{}, network.GroupedPortRanges{
		"endpoint": {
			{Protocol: "tcp", FromPort: 80, ToPort: 80},
			{Protocol: "udp", FromPort: 1000, ToPort: 1500},
		},
	})
	c.Assert(err, jc.ErrorIsNil)

	groupedPortRanges, err := st.GetOpenedPorts(ctx, s.unitUUID)
	c.Assert(err, jc.ErrorIsNil)
	c.Check(groupedPortRanges, gc.HasLen, 1)

	c.Check(groupedPortRanges["misc"], gc.HasLen, 1)
	c.Check(groupedPortRanges["misc"][0], jc.DeepEquals, network.PortRange{Protocol: "tcp", FromPort: 8080, ToPort: 8080})
}

func (s *stateSuite) TestUpdateUnitPortsClosePortRangeSuperset(c *gc.C) {
	st := NewState(s.TxnRunnerFactory())
	ctx := context.Background()
	s.initialiseOpenPort(c, st)

	err := st.UpdateUnitPorts(ctx, s.unitUUID, network.GroupedPortRanges{}, network.GroupedPortRanges{"endpoint": {{Protocol: "udp", FromPort: 750, ToPort: 1750}}})
	c.Assert(err, jc.ErrorIsNil)

	groupedPortRanges, err := st.GetOpenedPorts(ctx, s.unitUUID)
	c.Assert(err, jc.ErrorIsNil)
	c.Check(groupedPortRanges, gc.HasLen, 2)
	c.Check(groupedPortRanges["endpoint"], gc.HasLen, 1)
	c.Check(groupedPortRanges["endpoint"][0], jc.DeepEquals, network.PortRange{Protocol: "tcp", FromPort: 80, ToPort: 80})

	c.Check(groupedPortRanges["misc"], gc.HasLen, 1)
	c.Check(groupedPortRanges["misc"][0], jc.DeepEquals, network.PortRange{Protocol: "tcp", FromPort: 8080, ToPort: 8080})
}

func (s *stateSuite) TestUpdateUnitPortsClosePortRangeSubset(c *gc.C) {
	st := NewState(s.TxnRunnerFactory())
	ctx := context.Background()
	s.initialiseOpenPort(c, st)

	err := st.UpdateUnitPorts(ctx, s.unitUUID, network.GroupedPortRanges{}, network.GroupedPortRanges{"endpoint": {{Protocol: "udp", FromPort: 1200, ToPort: 1300}}})
	c.Assert(err, jc.ErrorIsNil)

	groupedPortRanges, err := st.GetOpenedPorts(ctx, s.unitUUID)
	c.Assert(err, jc.ErrorIsNil)
	c.Check(groupedPortRanges, gc.HasLen, 2)
	c.Check(groupedPortRanges["endpoint"], gc.HasLen, 3)
	c.Check(groupedPortRanges["endpoint"][0], jc.DeepEquals, network.PortRange{Protocol: "tcp", FromPort: 80, ToPort: 80})
	c.Check(groupedPortRanges["endpoint"][1], jc.DeepEquals, network.PortRange{Protocol: "udp", FromPort: 1000, ToPort: 1199})
	c.Check(groupedPortRanges["endpoint"][2], jc.DeepEquals, network.PortRange{Protocol: "udp", FromPort: 1301, ToPort: 1500})

	c.Check(groupedPortRanges["misc"], gc.HasLen, 1)
	c.Check(groupedPortRanges["misc"][0], jc.DeepEquals, network.PortRange{Protocol: "tcp", FromPort: 8080, ToPort: 8080})
}

func (s *stateSuite) TestUpdateUnitPortsOpenCloseICMP(c *gc.C) {
	st := NewState(s.TxnRunnerFactory())
	ctx := context.Background()
	s.initialiseOpenPort(c, st)

	err := st.UpdateUnitPorts(ctx, s.unitUUID, network.GroupedPortRanges{"endpoint": {{Protocol: "icmp"}}}, network.GroupedPortRanges{})
	c.Assert(err, jc.ErrorIsNil)

	groupedPortRanges, err := st.GetOpenedPorts(ctx, s.unitUUID)
	c.Assert(err, jc.ErrorIsNil)
	c.Check(groupedPortRanges, gc.HasLen, 2)
	c.Check(groupedPortRanges["endpoint"], gc.HasLen, 3)
	c.Check(groupedPortRanges["endpoint"][0], jc.DeepEquals, network.PortRange{Protocol: "icmp"})
	c.Check(groupedPortRanges["endpoint"][1], jc.DeepEquals, network.PortRange{Protocol: "tcp", FromPort: 80, ToPort: 80})
	c.Check(groupedPortRanges["endpoint"][2], jc.DeepEquals, network.PortRange{Protocol: "udp", FromPort: 1000, ToPort: 1500})

	c.Check(groupedPortRanges["misc"], gc.HasLen, 1)
	c.Check(groupedPortRanges["misc"][0], jc.DeepEquals, network.PortRange{Protocol: "tcp", FromPort: 8080, ToPort: 8080})

	err = st.UpdateUnitPorts(ctx, s.unitUUID, network.GroupedPortRanges{}, network.GroupedPortRanges{"endpoint": {{Protocol: "icmp"}}})
	c.Check(err, jc.ErrorIsNil)

	groupedPortRanges, err = st.GetOpenedPorts(ctx, s.unitUUID)
	c.Assert(err, jc.ErrorIsNil)
	c.Check(groupedPortRanges, gc.HasLen, 2)
	c.Check(groupedPortRanges["endpoint"], gc.HasLen, 2)
	c.Check(groupedPortRanges["endpoint"][0], jc.DeepEquals, network.PortRange{Protocol: "tcp", FromPort: 80, ToPort: 80})
	c.Check(groupedPortRanges["endpoint"][1], jc.DeepEquals, network.PortRange{Protocol: "udp", FromPort: 1000, ToPort: 1500})

	c.Check(groupedPortRanges["misc"], gc.HasLen, 1)
	c.Check(groupedPortRanges["misc"][0], jc.DeepEquals, network.PortRange{Protocol: "tcp", FromPort: 8080, ToPort: 8080})
}

func (s *stateSuite) TestUpdateUnitPortsOpenPortRangeMixedEndpoints(c *gc.C) {
	st := NewState(s.TxnRunnerFactory())
	ctx := context.Background()
	s.initialiseOpenPort(c, st)

	err := st.UpdateUnitPorts(ctx, s.unitUUID, network.GroupedPortRanges{
		"endpoint":       {{Protocol: "udp", FromPort: 1500, ToPort: 2000}},
		"other-endpoint": {{Protocol: "udp", FromPort: 2000, ToPort: 2500}},
	}, network.GroupedPortRanges{})
	c.Assert(err, jc.ErrorIsNil)

	groupedPortRanges, err := st.GetOpenedPorts(ctx, s.unitUUID)
	c.Assert(err, jc.ErrorIsNil)
	c.Check(groupedPortRanges, gc.HasLen, 3)

	c.Check(groupedPortRanges["endpoint"], gc.HasLen, 2)
	c.Check(groupedPortRanges["endpoint"][0], jc.DeepEquals, network.PortRange{Protocol: "tcp", FromPort: 80, ToPort: 80})
	c.Check(groupedPortRanges["endpoint"][1], jc.DeepEquals, network.PortRange{Protocol: "udp", FromPort: 1000, ToPort: 2000})

	c.Check(groupedPortRanges["other-endpoint"], gc.HasLen, 1)
	c.Check(groupedPortRanges["other-endpoint"][0], jc.DeepEquals, network.PortRange{Protocol: "udp", FromPort: 2000, ToPort: 2500})

	c.Check(groupedPortRanges["misc"], gc.HasLen, 1)
	c.Check(groupedPortRanges["misc"][0], jc.DeepEquals, network.PortRange{Protocol: "tcp", FromPort: 8080, ToPort: 8080})
}

func (s *stateSuite) TestUpdateUnitPortsClosePortRangeMixedEndpoints(c *gc.C) {
	st := NewState(s.TxnRunnerFactory())
	ctx := context.Background()
	s.initialiseOpenPort(c, st)
	err := st.UpdateUnitPorts(ctx, s.unitUUID, network.GroupedPortRanges{
		"other-endpoint": {{Protocol: "udp", FromPort: 2000, ToPort: 2500}},
	}, network.GroupedPortRanges{})
	c.Assert(err, jc.ErrorIsNil)

	err = st.UpdateUnitPorts(ctx, s.unitUUID, network.GroupedPortRanges{}, network.GroupedPortRanges{
		"endpoint":       {{Protocol: "udp", FromPort: 1100, ToPort: 1400}},
		"other-endpoint": {{Protocol: "udp", FromPort: 2100, ToPort: 2400}},
	})
	c.Assert(err, jc.ErrorIsNil)

	groupedPortRanges, err := st.GetOpenedPorts(ctx, s.unitUUID)
	c.Assert(err, jc.ErrorIsNil)
	c.Check(groupedPortRanges, gc.HasLen, 3)

	c.Check(groupedPortRanges["endpoint"], gc.HasLen, 3)
	c.Check(groupedPortRanges["endpoint"][0], jc.DeepEquals, network.PortRange{Protocol: "tcp", FromPort: 80, ToPort: 80})
	c.Check(groupedPortRanges["endpoint"][1], jc.DeepEquals, network.PortRange{Protocol: "udp", FromPort: 1000, ToPort: 1099})
	c.Check(groupedPortRanges["endpoint"][2], jc.DeepEquals, network.PortRange{Protocol: "udp", FromPort: 1401, ToPort: 1500})

	c.Check(groupedPortRanges["other-endpoint"], gc.HasLen, 2)
	c.Check(groupedPortRanges["other-endpoint"][0], jc.DeepEquals, network.PortRange{Protocol: "udp", FromPort: 2000, ToPort: 2099})
	c.Check(groupedPortRanges["other-endpoint"][1], jc.DeepEquals, network.PortRange{Protocol: "udp", FromPort: 2401, ToPort: 2500})

	c.Check(groupedPortRanges["misc"], gc.HasLen, 1)
	c.Check(groupedPortRanges["misc"][0], jc.DeepEquals, network.PortRange{Protocol: "tcp", FromPort: 8080, ToPort: 8080})
}
