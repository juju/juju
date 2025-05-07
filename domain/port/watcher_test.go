// Copyright 2024 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package port_test

import (
	"context"
	"database/sql"

	"github.com/juju/clock"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	coreapplication "github.com/juju/juju/core/application"
	"github.com/juju/juju/core/changestream"
	"github.com/juju/juju/core/database"
	"github.com/juju/juju/core/machine"
	modeltesting "github.com/juju/juju/core/model/testing"
	"github.com/juju/juju/core/network"
	coreunit "github.com/juju/juju/core/unit"
	"github.com/juju/juju/core/watcher/watchertest"
	"github.com/juju/juju/domain"
	"github.com/juju/juju/domain/application"
	"github.com/juju/juju/domain/application/architecture"
	"github.com/juju/juju/domain/application/charm"
	applicationstate "github.com/juju/juju/domain/application/state"
	"github.com/juju/juju/domain/deployment"
	machinestate "github.com/juju/juju/domain/machine/state"
	"github.com/juju/juju/domain/port/service"
	"github.com/juju/juju/domain/port/state"
	changestreamtesting "github.com/juju/juju/internal/changestream/testing"
	"github.com/juju/juju/internal/logger"
	loggertesting "github.com/juju/juju/internal/logger/testing"
	coretesting "github.com/juju/juju/internal/testing"
)

type watcherSuite struct {
	changestreamtesting.ModelSuite

	srv *service.WatchableService

	unitCount int

	unitUUIDs [3]coreunit.UUID

	appUUIDs [2]coreapplication.ID
}

var _ = gc.Suite(&watcherSuite{})

var (
	ssh   = network.PortRange{Protocol: "tcp", FromPort: 22, ToPort: 22}
	http  = network.PortRange{Protocol: "tcp", FromPort: 80, ToPort: 80}
	https = network.PortRange{Protocol: "tcp", FromPort: 443, ToPort: 443}
)

var (
	machineUUIDs = []machine.UUID{"machine-0-uuid", "machine-1-uuid"}
	netNodeUUIDs = []string{"net-node-0-uuid", "net-node-1-uuid"}
	appNames     = []string{"app-zero", "app-one"}
)

func (s *watcherSuite) SetUpTest(c *gc.C) {
	s.ModelSuite.SetUpTest(c)

	factory := changestream.NewWatchableDBFactoryForNamespace(s.GetWatchableDB, "port_range")
	s.srv = service.NewWatchableService(
		state.NewState(
			func() (database.TxnRunner, error) { return factory() },
		),
		domain.NewWatcherFactory(factory, loggertesting.WrapCheckLog(c)),
		logger.GetLogger("juju.test.port"),
	)

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

	err = machineSt.CreateMachine(context.Background(), "0", netNodeUUIDs[0], machineUUIDs[0])
	c.Assert(err, jc.ErrorIsNil)
	err = machineSt.CreateMachine(context.Background(), "1", netNodeUUIDs[1], machineUUIDs[1])
	c.Assert(err, jc.ErrorIsNil)
}

func (s *watcherSuite) createApplicationWithRelations(c *gc.C, appName string, relations ...string) coreapplication.ID {
	relationsMap := map[string]charm.Relation{}
	for _, relation := range relations {
		relationsMap[relation] = charm.Relation{
			Name:  relation,
			Role:  charm.RoleRequirer,
			Scope: charm.ScopeGlobal,
		}
	}

	applicationSt := applicationstate.NewState(s.TxnRunnerFactory(), clock.WallClock, loggertesting.WrapCheckLog(c))
	appUUID, err := applicationSt.CreateApplication(context.Background(), appName, application.AddApplicationArg{
		Charm: charm.Charm{
			Metadata: charm.Metadata{
				Name:     appName,
				Requires: relationsMap,
			},
			Manifest: charm.Manifest{
				Bases: []charm.Base{{
					Name:          "ubuntu",
					Channel:       charm.Channel{Risk: charm.RiskStable},
					Architectures: []string{"amd64"},
				}},
			},
			ReferenceName: appName,
			Architecture:  architecture.AMD64,
			Revision:      1,
			Source:        charm.LocalSource,
		},
	}, nil)
	c.Assert(err, jc.ErrorIsNil)
	return appUUID
}

// createUnit creates a new unit in state and returns its UUID. The unit is assigned
// to the net node with uuid `netNodeUUID`.
func (s *watcherSuite) createUnit(c *gc.C, netNodeUUID, appName string) coreunit.UUID {
	applicationSt := applicationstate.NewState(s.TxnRunnerFactory(), clock.WallClock, loggertesting.WrapCheckLog(c))
	ctx := context.Background()

	appID, err := applicationSt.GetApplicationIDByName(ctx, appName)
	c.Assert(err, jc.ErrorIsNil)

	charmUUID, err := applicationSt.GetCharmIDByApplicationName(ctx, appName)
	c.Assert(err, jc.ErrorIsNil)

	// Ensure that we place the unit on the same machine as the net node.
	var machineName machine.Name
	err = s.TxnRunner().StdTxn(context.Background(), func(ctx context.Context, tx *sql.Tx) error {
		err := tx.QueryRowContext(ctx, "SELECT name FROM machine WHERE net_node_uuid = ?", netNodeUUID).Scan(&machineName)
		return err
	})
	c.Assert(err, jc.ErrorIsNil)

	unitNames, err := applicationSt.AddIAASUnits(ctx, appID, charmUUID, application.AddUnitArg{
		Placement: deployment.Placement{
			Type:      deployment.PlacementTypeMachine,
			Directive: machineName.String(),
		},
	})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(unitNames, gc.HasLen, 1)
	unitName := unitNames[0]
	s.unitCount++

	var unitUUID coreunit.UUID
	err = s.TxnRunner().StdTxn(context.Background(), func(ctx context.Context, tx *sql.Tx) error {
		err := tx.QueryRowContext(ctx, "SELECT uuid FROM unit WHERE name = ?", unitName).Scan(&unitUUID)
		return err
	})
	c.Assert(err, jc.ErrorIsNil)
	return unitUUID
}

// The following tests will run with the following context:
// - 3 units are deployed (with uuids stored in s.unitUUIDs)
// - 2 machines are deployed (with uuids stored in machineUUIDs)
//   - machine 0 hosts units 0 & 1
//   - machine 1 hosts unit 2
//
// - on 2 applications (with names stored in appNames; uuids s.appUUIDs)
//   - unit 0 is deployed to app 0
//   - units 1 & 2 are deployed to app 1
func (s *watcherSuite) TestWatchMachinePortRanges(c *gc.C) {
	s.appUUIDs[0] = s.createApplicationWithRelations(c, appNames[0], "ep0", "ep1", "ep2", "ep3")
	s.appUUIDs[1] = s.createApplicationWithRelations(c, appNames[1], "ep0", "ep1", "ep2", "ep3")

	s.unitUUIDs[0] = s.createUnit(c, netNodeUUIDs[0], appNames[0])
	s.unitUUIDs[1] = s.createUnit(c, netNodeUUIDs[0], appNames[1])
	s.unitUUIDs[2] = s.createUnit(c, netNodeUUIDs[1], appNames[1])

	watcher, err := s.srv.WatchMachineOpenedPorts(context.Background())
	c.Assert(err, jc.ErrorIsNil)

	harness := watchertest.NewHarness(s, watchertest.NewWatcherC(c, watcher))

	// open a port on an empty endpoint on a unit on machine 0
	harness.AddTest(func(c *gc.C) {
		err := s.srv.UpdateUnitPorts(context.Background(), s.unitUUIDs[0], network.GroupedPortRanges{
			"ep0": {ssh},
		}, network.GroupedPortRanges{})
		c.Assert(err, jc.ErrorIsNil)
	}, func(w watchertest.WatcherC[[]string]) {
		w.Check(watchertest.StringSliceAssert("0"))
	})

	// open a port on an endpoint with opened ports on a unit on machine 0
	harness.AddTest(func(c *gc.C) {
		err := s.srv.UpdateUnitPorts(context.Background(), s.unitUUIDs[0], network.GroupedPortRanges{
			"ep0": {http},
		}, network.GroupedPortRanges{})
		c.Assert(err, jc.ErrorIsNil)
	}, func(w watchertest.WatcherC[[]string]) {
		w.Check(watchertest.StringSliceAssert("0"))
	})

	// open a port on a new endpoint on another unit on machine 0
	harness.AddTest(func(c *gc.C) {
		err := s.srv.UpdateUnitPorts(context.Background(), s.unitUUIDs[1], network.GroupedPortRanges{
			"ep1": {http},
		}, network.GroupedPortRanges{})
		c.Assert(err, jc.ErrorIsNil)
	}, func(w watchertest.WatcherC[[]string]) {
		w.Check(watchertest.StringSliceAssert("0"))
	})

	// open a port on a endpoint on a unit on machine 1
	harness.AddTest(func(c *gc.C) {
		err := s.srv.UpdateUnitPorts(context.Background(), s.unitUUIDs[2], network.GroupedPortRanges{
			"ep2": {https},
		}, network.GroupedPortRanges{})
		c.Assert(err, jc.ErrorIsNil)
	}, func(w watchertest.WatcherC[[]string]) {
		w.Check(watchertest.StringSliceAssert("1"))
	})

	// open a port that's already open
	harness.AddTest(func(c *gc.C) {
		err := s.srv.UpdateUnitPorts(context.Background(), s.unitUUIDs[0], network.GroupedPortRanges{
			"ep0": {ssh},
		}, network.GroupedPortRanges{})
		c.Assert(err, jc.ErrorIsNil)
	}, func(w watchertest.WatcherC[[]string]) {
		w.AssertNoChange()
	})

	// close a port on an endpoint on a unit on machine 0
	harness.AddTest(func(c *gc.C) {
		err := s.srv.UpdateUnitPorts(context.Background(), s.unitUUIDs[0], network.GroupedPortRanges{}, network.GroupedPortRanges{
			"ep0": {ssh},
		})
		c.Assert(err, jc.ErrorIsNil)
	}, func(w watchertest.WatcherC[[]string]) {
		w.Check(watchertest.StringSliceAssert("0"))
	})

	// close the final open port of an endpoint for a unit on machine 0
	harness.AddTest(func(c *gc.C) {
		err := s.srv.UpdateUnitPorts(context.Background(), s.unitUUIDs[0], network.GroupedPortRanges{}, network.GroupedPortRanges{
			"ep0": {http},
		})
		c.Assert(err, jc.ErrorIsNil)
	}, func(w watchertest.WatcherC[[]string]) {
		w.Check(watchertest.StringSliceAssert("0"))
	})

	// close a port range which isn't open
	harness.AddTest(func(c *gc.C) {
		err := s.srv.UpdateUnitPorts(context.Background(), s.unitUUIDs[1], network.GroupedPortRanges{}, network.GroupedPortRanges{
			"ep1": {https},
		})
		c.Assert(err, jc.ErrorIsNil)
	}, func(w watchertest.WatcherC[[]string]) {
		w.AssertNoChange()
	})

	// open ports on different machines at the same time
	harness.AddTest(func(c *gc.C) {
		err := s.srv.UpdateUnitPorts(context.Background(), s.unitUUIDs[1], network.GroupedPortRanges{
			"ep3": {https},
		}, network.GroupedPortRanges{})
		c.Assert(err, jc.ErrorIsNil)
		err = s.srv.UpdateUnitPorts(context.Background(), s.unitUUIDs[2], network.GroupedPortRanges{
			"ep3": {https},
		}, network.GroupedPortRanges{})
		c.Assert(err, jc.ErrorIsNil)
	}, func(w watchertest.WatcherC[[]string]) {
		w.Check(watchertest.StringSliceAssert("0", "1"))
	})

	harness.Run(c, []string{"0", "1"})
}

func (s *watcherSuite) TestWatchOpenedPortsForApplication(c *gc.C) {
	s.appUUIDs[0] = s.createApplicationWithRelations(c, appNames[0], "ep0", "ep1", "ep2")
	s.appUUIDs[1] = s.createApplicationWithRelations(c, appNames[1], "ep0", "ep1", "ep2")

	s.unitUUIDs[0] = s.createUnit(c, netNodeUUIDs[0], appNames[0])
	s.unitUUIDs[1] = s.createUnit(c, netNodeUUIDs[0], appNames[1])
	s.unitUUIDs[2] = s.createUnit(c, netNodeUUIDs[1], appNames[1])

	watcher, err := s.srv.WatchOpenedPortsForApplication(context.Background(), s.appUUIDs[1])
	c.Assert(err, jc.ErrorIsNil)

	harness := watchertest.NewHarness(s, watchertest.NewWatcherC(c, watcher))

	// open a port on an empty endpoint on a unit the application
	harness.AddTest(func(c *gc.C) {
		err := s.srv.UpdateUnitPorts(context.Background(), s.unitUUIDs[1], network.GroupedPortRanges{
			"ep1": {http},
		}, network.GroupedPortRanges{})
		c.Assert(err, jc.ErrorIsNil)
	}, func(w watchertest.WatcherC[struct{}]) {
		w.Check(watchertest.SliceAssert(struct{}{}))
	})

	// open a port on another unit of the application
	harness.AddTest(func(c *gc.C) {
		err := s.srv.UpdateUnitPorts(context.Background(), s.unitUUIDs[2], network.GroupedPortRanges{
			"ep2": {https},
		}, network.GroupedPortRanges{})
		c.Assert(err, jc.ErrorIsNil)
	}, func(w watchertest.WatcherC[struct{}]) {
		w.Check(watchertest.SliceAssert(struct{}{}))
	})

	// open a port on another application
	harness.AddTest(func(c *gc.C) {
		err := s.srv.UpdateUnitPorts(context.Background(), s.unitUUIDs[0], network.GroupedPortRanges{
			"ep0": {ssh},
		}, network.GroupedPortRanges{})
		c.Assert(err, jc.ErrorIsNil)
	}, func(w watchertest.WatcherC[struct{}]) {
		w.AssertNoChange()
	})

	// open a port that's already open
	harness.AddTest(func(c *gc.C) {
		err := s.srv.UpdateUnitPorts(context.Background(), s.unitUUIDs[1], network.GroupedPortRanges{
			"ep1": {http},
		}, network.GroupedPortRanges{})
		c.Assert(err, jc.ErrorIsNil)
	}, func(w watchertest.WatcherC[struct{}]) {
		w.AssertNoChange()
	})

	// close a port on a unit of the application
	harness.AddTest(func(c *gc.C) {
		err := s.srv.UpdateUnitPorts(context.Background(), s.unitUUIDs[1], network.GroupedPortRanges{}, network.GroupedPortRanges{
			"ep1": {http},
		})
		c.Assert(err, jc.ErrorIsNil)
	}, func(w watchertest.WatcherC[struct{}]) {
		w.Check(watchertest.SliceAssert(struct{}{}))
	})

	// close the final open port of an endpoint for the application
	harness.AddTest(func(c *gc.C) {
		err := s.srv.UpdateUnitPorts(context.Background(), s.unitUUIDs[2], network.GroupedPortRanges{}, network.GroupedPortRanges{
			"ep2": {https},
		})
		c.Assert(err, jc.ErrorIsNil)
	}, func(w watchertest.WatcherC[struct{}]) {
		w.Check(watchertest.SliceAssert(struct{}{}))
	})

	// close a port on another application
	harness.AddTest(func(c *gc.C) {
		err := s.srv.UpdateUnitPorts(context.Background(), s.unitUUIDs[0], network.GroupedPortRanges{}, network.GroupedPortRanges{
			"ep0": {ssh},
		})
		c.Assert(err, jc.ErrorIsNil)
	}, func(w watchertest.WatcherC[struct{}]) {
		w.AssertNoChange()
	})

	harness.Run(c, struct{}{})
}
