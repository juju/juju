// Copyright 2024 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package port_test

import (
	"context"
	"database/sql"
	"testing"

	"github.com/juju/clock"
	"github.com/juju/tc"

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
	domainmachine "github.com/juju/juju/domain/machine"
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

func TestWatcherSuite(t *testing.T) {
	tc.Run(t, &watcherSuite{})
}

var (
	ssh   = network.PortRange{Protocol: "tcp", FromPort: 22, ToPort: 22}
	http  = network.PortRange{Protocol: "tcp", FromPort: 80, ToPort: 80}
	https = network.PortRange{Protocol: "tcp", FromPort: 443, ToPort: 443}
)

var (
	machineUUIDs []string
	netNodeUUIDs []string
	appNames     = []string{"app-zero", "app-one"}
)

func (s *watcherSuite) SetUpTest(c *tc.C) {
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
}

func (s *watcherSuite) createApplicationWithRelations(c *tc.C, appName string, relations ...string) coreapplication.ID {
	relationsMap := map[string]charm.Relation{}
	for _, relation := range relations {
		relationsMap[relation] = charm.Relation{
			Name:  relation,
			Role:  charm.RoleRequirer,
			Scope: charm.ScopeGlobal,
		}
	}

	applicationSt := applicationstate.NewState(s.TxnRunnerFactory(), clock.WallClock, loggertesting.WrapCheckLog(c))
	appUUID, _, err := applicationSt.CreateIAASApplication(c.Context(), appName, application.AddIAASApplicationArg{
		BaseAddApplicationArg: application.BaseAddApplicationArg{
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
		},
	}, nil)
	c.Assert(err, tc.ErrorIsNil)
	return appUUID
}

// createUnit creates a new unit in state and returns its UUID. The unit is assigned
// to the net node with uuid `netNodeUUID`.
func (s *watcherSuite) createUnit(c *tc.C, netNodeUUID, appName string) coreunit.UUID {
	applicationSt := applicationstate.NewState(s.TxnRunnerFactory(), clock.WallClock, loggertesting.WrapCheckLog(c))
	ctx := c.Context()

	appID, err := applicationSt.GetApplicationIDByName(ctx, appName)
	c.Assert(err, tc.ErrorIsNil)

	// Ensure that we place the unit on the same machine as the net node.
	var machineName machine.Name
	err = s.TxnRunner().StdTxn(c.Context(), func(ctx context.Context, tx *sql.Tx) error {
		err := tx.QueryRowContext(ctx, "SELECT name FROM machine WHERE net_node_uuid = ?", netNodeUUID).Scan(&machineName)
		return err
	})
	c.Assert(err, tc.ErrorIsNil)

	unitNames, _, err := applicationSt.AddIAASUnits(ctx, appID, application.AddIAASUnitArg{
		AddUnitArg: application.AddUnitArg{
			Placement: deployment.Placement{
				Type:      deployment.PlacementTypeMachine,
				Directive: machineName.String(),
			},
		},
	})
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(unitNames, tc.HasLen, 1)
	unitName := unitNames[0]
	s.unitCount++

	var unitUUID coreunit.UUID
	err = s.TxnRunner().StdTxn(c.Context(), func(ctx context.Context, tx *sql.Tx) error {
		err := tx.QueryRowContext(ctx, "SELECT uuid FROM unit WHERE name = ?", unitName).Scan(&unitUUID)
		return err
	})
	c.Assert(err, tc.ErrorIsNil)
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
func (s *watcherSuite) TestWatchMachinePortRanges(c *tc.C) {
	s.appUUIDs[0] = s.createApplicationWithRelations(c, appNames[0], "ep0", "ep1", "ep2", "ep3")
	s.appUUIDs[1] = s.createApplicationWithRelations(c, appNames[1], "ep0", "ep1", "ep2", "ep3")

	s.unitUUIDs[0] = s.createUnit(c, netNodeUUIDs[0], appNames[0])
	s.unitUUIDs[1] = s.createUnit(c, netNodeUUIDs[0], appNames[1])
	s.unitUUIDs[2] = s.createUnit(c, netNodeUUIDs[1], appNames[1])

	watcher, err := s.srv.WatchMachineOpenedPorts(c.Context())
	c.Assert(err, tc.ErrorIsNil)

	harness := watchertest.NewHarness(s, watchertest.NewWatcherC(c, watcher))

	// open a port on an empty endpoint on a unit on machine 0
	harness.AddTest(c, func(c *tc.C) {
		err := s.srv.UpdateUnitPorts(c.Context(), s.unitUUIDs[0], network.GroupedPortRanges{
			"ep0": {ssh},
		}, network.GroupedPortRanges{})
		c.Assert(err, tc.ErrorIsNil)
	}, func(w watchertest.WatcherC[[]string]) {
		w.Check(watchertest.StringSliceAssert("0"))
	})

	// open a port on an endpoint with opened ports on a unit on machine 0
	harness.AddTest(c, func(c *tc.C) {
		err := s.srv.UpdateUnitPorts(c.Context(), s.unitUUIDs[0], network.GroupedPortRanges{
			"ep0": {http},
		}, network.GroupedPortRanges{})
		c.Assert(err, tc.ErrorIsNil)
	}, func(w watchertest.WatcherC[[]string]) {
		w.Check(watchertest.StringSliceAssert("0"))
	})

	// open a port on a new endpoint on another unit on machine 0
	harness.AddTest(c, func(c *tc.C) {
		err := s.srv.UpdateUnitPorts(c.Context(), s.unitUUIDs[1], network.GroupedPortRanges{
			"ep1": {http},
		}, network.GroupedPortRanges{})
		c.Assert(err, tc.ErrorIsNil)
	}, func(w watchertest.WatcherC[[]string]) {
		w.Check(watchertest.StringSliceAssert("0"))
	})

	// open a port on a endpoint on a unit on machine 1
	harness.AddTest(c, func(c *tc.C) {
		err := s.srv.UpdateUnitPorts(c.Context(), s.unitUUIDs[2], network.GroupedPortRanges{
			"ep2": {https},
		}, network.GroupedPortRanges{})
		c.Assert(err, tc.ErrorIsNil)
	}, func(w watchertest.WatcherC[[]string]) {
		w.Check(watchertest.StringSliceAssert("1"))
	})

	// open a port that's already open
	harness.AddTest(c, func(c *tc.C) {
		err := s.srv.UpdateUnitPorts(c.Context(), s.unitUUIDs[0], network.GroupedPortRanges{
			"ep0": {ssh},
		}, network.GroupedPortRanges{})
		c.Assert(err, tc.ErrorIsNil)
	}, func(w watchertest.WatcherC[[]string]) {
		w.AssertNoChange()
	})

	// close a port on an endpoint on a unit on machine 0
	harness.AddTest(c, func(c *tc.C) {
		err := s.srv.UpdateUnitPorts(c.Context(), s.unitUUIDs[0], network.GroupedPortRanges{}, network.GroupedPortRanges{
			"ep0": {ssh},
		})
		c.Assert(err, tc.ErrorIsNil)
	}, func(w watchertest.WatcherC[[]string]) {
		w.Check(watchertest.StringSliceAssert("0"))
	})

	// close the final open port of an endpoint for a unit on machine 0
	harness.AddTest(c, func(c *tc.C) {
		err := s.srv.UpdateUnitPorts(c.Context(), s.unitUUIDs[0], network.GroupedPortRanges{}, network.GroupedPortRanges{
			"ep0": {http},
		})
		c.Assert(err, tc.ErrorIsNil)
	}, func(w watchertest.WatcherC[[]string]) {
		w.Check(watchertest.StringSliceAssert("0"))
	})

	// close a port range which isn't open
	harness.AddTest(c, func(c *tc.C) {
		err := s.srv.UpdateUnitPorts(c.Context(), s.unitUUIDs[1], network.GroupedPortRanges{}, network.GroupedPortRanges{
			"ep1": {https},
		})
		c.Assert(err, tc.ErrorIsNil)
	}, func(w watchertest.WatcherC[[]string]) {
		w.AssertNoChange()
	})

	// open ports on different machines at the same time
	harness.AddTest(c, func(c *tc.C) {
		err := s.srv.UpdateUnitPorts(c.Context(), s.unitUUIDs[1], network.GroupedPortRanges{
			"ep3": {https},
		}, network.GroupedPortRanges{})
		c.Assert(err, tc.ErrorIsNil)
		err = s.srv.UpdateUnitPorts(c.Context(), s.unitUUIDs[2], network.GroupedPortRanges{
			"ep3": {https},
		}, network.GroupedPortRanges{})
		c.Assert(err, tc.ErrorIsNil)
	}, func(w watchertest.WatcherC[[]string]) {
		w.Check(watchertest.StringSliceAssert("0", "1"))
	})

	harness.Run(c, []string{"0", "1"})
}

func (s *watcherSuite) TestWatchOpenedPortsForApplication(c *tc.C) {
	s.appUUIDs[0] = s.createApplicationWithRelations(c, appNames[0], "ep0", "ep1", "ep2")
	s.appUUIDs[1] = s.createApplicationWithRelations(c, appNames[1], "ep0", "ep1", "ep2")

	s.unitUUIDs[0] = s.createUnit(c, netNodeUUIDs[0], appNames[0])
	s.unitUUIDs[1] = s.createUnit(c, netNodeUUIDs[0], appNames[1])
	s.unitUUIDs[2] = s.createUnit(c, netNodeUUIDs[1], appNames[1])

	watcher, err := s.srv.WatchOpenedPortsForApplication(c.Context(), s.appUUIDs[1])
	c.Assert(err, tc.ErrorIsNil)

	harness := watchertest.NewHarness(s, watchertest.NewWatcherC(c, watcher))

	// open a port on an empty endpoint on a unit the application
	harness.AddTest(c, func(c *tc.C) {
		err := s.srv.UpdateUnitPorts(c.Context(), s.unitUUIDs[1], network.GroupedPortRanges{
			"ep1": {http},
		}, network.GroupedPortRanges{})
		c.Assert(err, tc.ErrorIsNil)
	}, func(w watchertest.WatcherC[struct{}]) {
		w.Check(watchertest.SliceAssert(struct{}{}))
	})

	// open a port on another unit of the application
	harness.AddTest(c, func(c *tc.C) {
		err := s.srv.UpdateUnitPorts(c.Context(), s.unitUUIDs[2], network.GroupedPortRanges{
			"ep2": {https},
		}, network.GroupedPortRanges{})
		c.Assert(err, tc.ErrorIsNil)
	}, func(w watchertest.WatcherC[struct{}]) {
		w.Check(watchertest.SliceAssert(struct{}{}))
	})

	// open a port on another application
	harness.AddTest(c, func(c *tc.C) {
		err := s.srv.UpdateUnitPorts(c.Context(), s.unitUUIDs[0], network.GroupedPortRanges{
			"ep0": {ssh},
		}, network.GroupedPortRanges{})
		c.Assert(err, tc.ErrorIsNil)
	}, func(w watchertest.WatcherC[struct{}]) {
		w.AssertNoChange()
	})

	// open a port that's already open
	harness.AddTest(c, func(c *tc.C) {
		err := s.srv.UpdateUnitPorts(c.Context(), s.unitUUIDs[1], network.GroupedPortRanges{
			"ep1": {http},
		}, network.GroupedPortRanges{})
		c.Assert(err, tc.ErrorIsNil)
	}, func(w watchertest.WatcherC[struct{}]) {
		w.AssertNoChange()
	})

	// close a port on a unit of the application
	harness.AddTest(c, func(c *tc.C) {
		err := s.srv.UpdateUnitPorts(c.Context(), s.unitUUIDs[1], network.GroupedPortRanges{}, network.GroupedPortRanges{
			"ep1": {http},
		})
		c.Assert(err, tc.ErrorIsNil)
	}, func(w watchertest.WatcherC[struct{}]) {
		w.Check(watchertest.SliceAssert(struct{}{}))
	})

	// close the final open port of an endpoint for the application
	harness.AddTest(c, func(c *tc.C) {
		err := s.srv.UpdateUnitPorts(c.Context(), s.unitUUIDs[2], network.GroupedPortRanges{}, network.GroupedPortRanges{
			"ep2": {https},
		})
		c.Assert(err, tc.ErrorIsNil)
	}, func(w watchertest.WatcherC[struct{}]) {
		w.Check(watchertest.SliceAssert(struct{}{}))
	})

	// close a port on another application
	harness.AddTest(c, func(c *tc.C) {
		err := s.srv.UpdateUnitPorts(c.Context(), s.unitUUIDs[0], network.GroupedPortRanges{}, network.GroupedPortRanges{
			"ep0": {ssh},
		})
		c.Assert(err, tc.ErrorIsNil)
	}, func(w watchertest.WatcherC[struct{}]) {
		w.AssertNoChange()
	})

	harness.Run(c, struct{}{})
}
