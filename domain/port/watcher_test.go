// Copyright 2024 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package port_test

import (
	"context"
	"database/sql"
	"fmt"

	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/core/changestream"
	"github.com/juju/juju/core/database"
	"github.com/juju/juju/core/network"
	"github.com/juju/juju/core/watcher/watchertest"
	"github.com/juju/juju/domain"
	"github.com/juju/juju/domain/application"
	"github.com/juju/juju/domain/application/charm"
	applicationerrors "github.com/juju/juju/domain/application/errors"
	applicationstate "github.com/juju/juju/domain/application/state"
	machinestate "github.com/juju/juju/domain/machine/state"
	"github.com/juju/juju/domain/port/service"
	"github.com/juju/juju/domain/port/state"
	changestreamtesting "github.com/juju/juju/internal/changestream/testing"
	"github.com/juju/juju/internal/errors"
	"github.com/juju/juju/internal/logger"
	loggertesting "github.com/juju/juju/internal/logger/testing"
)

type watcherSuite struct {
	changestreamtesting.ModelSuite

	srv *service.WatchableService

	unitCount int

	unitUUIDs [3]string

	appUUIDs [2]string
}

var _ = gc.Suite(&watcherSuite{})

var (
	ssh   = network.PortRange{Protocol: "tcp", FromPort: 22, ToPort: 22}
	http  = network.PortRange{Protocol: "tcp", FromPort: 80, ToPort: 80}
	https = network.PortRange{Protocol: "tcp", FromPort: 443, ToPort: 443}
)

var (
	machineUUIDs = []string{"machine-0-uuid", "machine-1-uuid"}
	netNodeUUIDs = []string{"net-node-0-uuid", "net-node-1-uuid"}
	appNames     = []string{"app-0-name", "app-1-name"}
)

func (s *watcherSuite) SetUpTest(c *gc.C) {
	s.ModelSuite.SetUpTest(c)

	factory := changestream.NewWatchableDBFactoryForNamespace(s.GetWatchableDB, "port_range")
	s.srv = service.NewWatchableService(
		state.NewState(
			func() (database.TxnRunner, error) { return factory() },
		),
		domain.NewWatcherFactory(factory, loggertesting.WrapCheckLog(c)),
	)

	machineSt := machinestate.NewState(s.TxnRunnerFactory(), logger.GetLogger("juju.test.machine"))

	err := machineSt.CreateMachine(context.Background(), "0", netNodeUUIDs[0], machineUUIDs[0])
	c.Assert(err, jc.ErrorIsNil)

	s.unitUUIDs[0], s.appUUIDs[0] = s.createUnit(c, netNodeUUIDs[0], appNames[0])
	s.unitUUIDs[1], s.appUUIDs[1] = s.createUnit(c, netNodeUUIDs[0], appNames[1])

	err = machineSt.CreateMachine(context.Background(), "1", netNodeUUIDs[1], machineUUIDs[1])
	c.Assert(err, jc.ErrorIsNil)

	s.unitUUIDs[2], _ = s.createUnit(c, netNodeUUIDs[1], appNames[1])
}

// createUnit creates a new unit in state and returns its UUID. The unit is assigned
// to the net node with uuid `netNodeUUID`.
func (s *watcherSuite) createUnit(c *gc.C, netNodeUUID, appName string) (string, string) {
	applicationSt := applicationstate.NewApplicationState(s.TxnRunnerFactory(), logger.GetLogger("juju.test.application"))
	_, err := applicationSt.CreateApplication(context.Background(), appName, application.AddApplicationArg{
		Charm: charm.Charm{
			Metadata: charm.Metadata{
				Name: appName,
			},
		},
	})
	c.Assert(err == nil || errors.Is(err, applicationerrors.ApplicationAlreadyExists), jc.IsTrue)

	unitName := fmt.Sprintf("%s/%d", appName, s.unitCount)
	err = applicationSt.AddUnits(context.Background(), appName, application.UpsertUnitArg{UnitName: unitName})
	c.Assert(err, jc.ErrorIsNil)
	s.unitCount++

	var (
		unitUUID string
		appUUID  string
	)
	err = s.TxnRunner().StdTxn(context.Background(), func(ctx context.Context, tx *sql.Tx) error {
		err := tx.QueryRowContext(ctx, "SELECT uuid FROM unit WHERE name = ?", unitName).Scan(&unitUUID)
		if err != nil {
			return err
		}

		err = tx.QueryRowContext(ctx, "SELECT uuid FROM application WHERE name = ?", appName).Scan(&appUUID)
		if err != nil {
			return err
		}

		_, err = tx.ExecContext(ctx, "INSERT INTO net_node VALUES (?) ON CONFLICT DO NOTHING", netNodeUUID)
		c.Assert(err, jc.ErrorIsNil)

		_, err = tx.ExecContext(ctx, "UPDATE unit SET net_node_uuid = ? WHERE name = ?", netNodeUUID, unitName)
		if err != nil {
			return err
		}

		return nil
	})
	c.Assert(err, jc.ErrorIsNil)
	return unitUUID, appUUID
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
 */

func (s *watcherSuite) TestWatchPortRanges(c *gc.C) {
	watcher, err := s.srv.WatchOpenedPorts(context.Background())
	c.Assert(err, jc.ErrorIsNil)

	harness := watchertest.NewHarness(s, watchertest.NewWatcherC(c, watcher))

	// open a port on an empty endpoint on a unit on machine 0
	harness.AddTest(func(c *gc.C) {
		err := s.srv.UpdateUnitPorts(context.Background(), s.unitUUIDs[0], network.GroupedPortRanges{
			"ep0": {ssh},
		}, network.GroupedPortRanges{})
		c.Assert(err, jc.ErrorIsNil)
	}, func(w watchertest.WatcherC[[]string]) {
		w.Check(watchertest.StringSliceAssert(machineUUIDs[0]))
	})

	// open a port on an endpoint with opened ports on a unit on machine 0
	harness.AddTest(func(c *gc.C) {
		err := s.srv.UpdateUnitPorts(context.Background(), s.unitUUIDs[0], network.GroupedPortRanges{
			"ep0": {http},
		}, network.GroupedPortRanges{})
		c.Assert(err, jc.ErrorIsNil)
	}, func(w watchertest.WatcherC[[]string]) {
		w.Check(watchertest.StringSliceAssert(machineUUIDs[0]))
	})

	// open a port on a new endpoint on another unit on machine 0
	harness.AddTest(func(c *gc.C) {
		err := s.srv.UpdateUnitPorts(context.Background(), s.unitUUIDs[1], network.GroupedPortRanges{
			"ep1": {http},
		}, network.GroupedPortRanges{})
		c.Assert(err, jc.ErrorIsNil)
	}, func(w watchertest.WatcherC[[]string]) {
		w.Check(watchertest.StringSliceAssert(machineUUIDs[0]))
	})

	// open a port on a endpoint on a unit on machine 1
	harness.AddTest(func(c *gc.C) {
		err := s.srv.UpdateUnitPorts(context.Background(), s.unitUUIDs[2], network.GroupedPortRanges{
			"ep2": {https},
		}, network.GroupedPortRanges{})
		c.Assert(err, jc.ErrorIsNil)
	}, func(w watchertest.WatcherC[[]string]) {
		w.Check(watchertest.StringSliceAssert(machineUUIDs[1]))
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
		w.Check(watchertest.StringSliceAssert(machineUUIDs[0]))
	})

	// close the final open port of an endpoint for a unit on machine 0
	harness.AddTest(func(c *gc.C) {
		err := s.srv.UpdateUnitPorts(context.Background(), s.unitUUIDs[0], network.GroupedPortRanges{}, network.GroupedPortRanges{
			"ep0": {http},
		})
		c.Assert(err, jc.ErrorIsNil)
	}, func(w watchertest.WatcherC[[]string]) {
		w.Check(watchertest.StringSliceAssert(machineUUIDs[0]))
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
		w.Check(watchertest.StringSliceAssert(machineUUIDs[0], machineUUIDs[1]))
	})

	harness.Run(c, []string{machineUUIDs[0], machineUUIDs[1]})
}

func (s *watcherSuite) TestWatchPortRangesForApplication(c *gc.C) {
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

	harness.Run(c)
}
