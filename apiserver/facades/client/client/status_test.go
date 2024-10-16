// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package client_test

import (
	"context"
	"fmt"
	"time"

	"github.com/canonical/sqlair"
	"github.com/juju/clock"
	"github.com/juju/names/v5"
	jc "github.com/juju/testing/checkers"
	"go.uber.org/mock/gomock"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/api"
	apiclient "github.com/juju/juju/api/client/client"
	"github.com/juju/juju/apiserver/facades/controller/charmrevisionupdater"
	"github.com/juju/juju/apiserver/facades/controller/charmrevisionupdater/mocks"
	corearch "github.com/juju/juju/core/arch"
	"github.com/juju/juju/core/base"
	corecharm "github.com/juju/juju/core/charm"
	"github.com/juju/juju/core/instance"
	coremachine "github.com/juju/juju/core/machine"
	"github.com/juju/juju/core/migration"
	coremodel "github.com/juju/juju/core/model"
	"github.com/juju/juju/core/network"
	"github.com/juju/juju/core/version"
	applicationservice "github.com/juju/juju/domain/application/service"
	domainmodel "github.com/juju/juju/domain/model"
	modelstate "github.com/juju/juju/domain/model/state"
	"github.com/juju/juju/environs/config"
	"github.com/juju/juju/internal/charm"
	"github.com/juju/juju/internal/charmhub/transport"
	loggertesting "github.com/juju/juju/internal/logger/testing"
	"github.com/juju/juju/internal/storage"
	testfactory "github.com/juju/juju/internal/testing/factory"
	"github.com/juju/juju/internal/uuid"
	"github.com/juju/juju/juju/testing"
	"github.com/juju/juju/rpc/params"
	"github.com/juju/juju/state"
	"github.com/juju/juju/testcharms"
)

type statusSuite struct {
	baseSuite
}

var _ = gc.Suite(&statusSuite{})

func (s *statusSuite) addMachine(c *gc.C) *state.Machine {
	st := s.ControllerModel(c).State()
	machine, err := st.AddMachine(s.modelConfigService(c), state.UbuntuBase("12.10"), state.JobHostUnits)
	c.Assert(err, jc.ErrorIsNil)
	return machine
}

// Complete testing of status functionality happens elsewhere in the codebase,
// these tests just sanity-check the api itself.

func (s *statusSuite) TestFullStatus(c *gc.C) {
	machine := s.addMachine(c)
	st := s.ControllerModel(c).State()
	conn := s.OpenModelAPI(c, st.ModelUUID())
	client := apiclient.NewClient(conn, loggertesting.WrapCheckLog(c))
	status, err := client.Status(context.Background(), nil)
	c.Assert(err, jc.ErrorIsNil)
	c.Check(status.Model.Name, gc.Equals, "controller")
	c.Check(status.Model.Type, gc.Equals, "iaas")
	c.Check(status.Model.CloudTag, gc.Equals, "cloud-dummy")
	c.Check(status.Applications, gc.HasLen, 0)
	c.Check(status.RemoteApplications, gc.HasLen, 0)
	c.Check(status.Offers, gc.HasLen, 0)
	c.Check(status.Machines, gc.HasLen, 1)
	c.Check(status.ControllerTimestamp, gc.NotNil)
	resultMachine, ok := status.Machines[machine.Id()]
	if !ok {
		c.Fatalf("Missing machine with id %q", machine.Id())
	}
	c.Check(resultMachine.Id, gc.Equals, machine.Id())
	c.Check(resultMachine.Base, jc.DeepEquals, params.Base{Name: "ubuntu", Channel: "12.10/stable"})
	c.Check(resultMachine.LXDProfiles, gc.HasLen, 0)
}

func (s *statusSuite) TestFullStatusUnitLeadership(c *gc.C) {
	factory, release := s.NewFactory(c, s.ControllerModelUUID())
	release()
	factory = factory.WithModelConfigService(s.modelConfigService(c))
	unit := factory.MakeUnit(c, nil)
	st := s.ControllerModel(c).State()
	claimer, err := s.LeaseManager.Claimer("application-leadership", st.ModelUUID())
	c.Assert(err, jc.ErrorIsNil)
	err = claimer.Claim(unit.ApplicationName(), unit.Name(), time.Minute)
	c.Assert(err, jc.ErrorIsNil)
	conn := s.OpenModelAPI(c, st.ModelUUID())
	client := apiclient.NewClient(conn, loggertesting.WrapCheckLog(c))
	status, err := client.Status(context.Background(), nil)
	c.Assert(err, jc.ErrorIsNil)
	app, ok := status.Applications[unit.ApplicationName()]
	c.Assert(ok, jc.IsTrue)
	unitStatus, ok := app.Units[unit.Name()]
	c.Assert(ok, jc.IsTrue)
	c.Assert(unitStatus.Leader, jc.IsTrue)
}

func (s *statusSuite) TestFullStatusUnitScaling(c *gc.C) {
	factory, release := s.NewFactory(c, s.ControllerModelUUID())
	release()
	factory = factory.WithModelConfigService(s.modelConfigService(c))
	machine := factory.MakeMachine(c, nil)
	unit := factory.MakeUnit(c, &testfactory.UnitParams{
		Machine: machine,
	})
	st := s.ControllerModel(c).State()
	tracker := st.TrackQueries("FullStatus")

	conn := s.OpenModelAPI(c, st.ModelUUID())
	client := apiclient.NewClient(conn, loggertesting.WrapCheckLog(c))
	_, err := client.Status(context.Background(), nil)
	c.Assert(err, jc.ErrorIsNil)

	queryCount := tracker.ReadCount()
	c.Logf("initial query count: %d", queryCount)

	// Add several more units of the same application to the
	// same machine. We do this because we want to isolate to
	// status handling to just additional units, not additional machines
	// or applications.
	app, err := unit.Application()
	c.Assert(err, jc.ErrorIsNil)

	const numUnits = 5
	for i := 0; i < numUnits; i++ {
		factory.MakeUnit(c, &testfactory.UnitParams{
			Application: app,
			Machine:     machine,
		})
	}

	tracker.Reset()

	_, err = client.Status(context.Background(), nil)
	c.Assert(err, jc.ErrorIsNil)

	// The number of queries should be the same.
	c.Check(tracker.ReadCount(), gc.Equals, queryCount+numUnits*3,
		gc.Commentf("if the query count is not the same, there has been a regression "+
			"in the processing of units, please fix it"))
}

func (s *statusSuite) TestFullStatusMachineScaling(c *gc.C) {
	factory, release := s.NewFactory(c, s.ControllerModelUUID())
	release()
	factory = factory.WithModelConfigService(s.modelConfigService(c))
	factory.MakeMachine(c, nil)
	st := s.ControllerModel(c).State()
	tracker := st.TrackQueries("FullStatus")

	conn := s.OpenModelAPI(c, st.ModelUUID())
	client := apiclient.NewClient(conn, loggertesting.WrapCheckLog(c))
	_, err := client.Status(context.Background(), nil)
	c.Assert(err, jc.ErrorIsNil)

	queryCount := tracker.ReadCount()
	c.Logf("initial query count: %d", queryCount)

	// Add several more machines to the model.
	for i := 0; i < 5; i++ {
		factory.MakeMachine(c, nil)
	}
	tracker.Reset()

	_, err = client.Status(context.Background(), nil)
	c.Assert(err, jc.ErrorIsNil)

	// The number of queries should be the same.
	c.Check(tracker.ReadCount(), gc.Equals, queryCount,
		gc.Commentf("if the query count is not the same, there has been a regression "+
			"in the processing of machines, please fix it"))
}

func (s *statusSuite) TestFullStatusInterfaceScaling(c *gc.C) {
	machine := s.addMachine(c)
	st := s.ControllerModel(c).State()
	tracker := st.TrackQueries("FullStatus")

	conn := s.OpenModelAPI(c, st.ModelUUID())
	client := apiclient.NewClient(conn, loggertesting.WrapCheckLog(c))
	_, err := client.Status(context.Background(), nil)
	c.Assert(err, jc.ErrorIsNil)

	queryCount := tracker.ReadCount()
	c.Logf("initial query count: %d", queryCount)

	// Add a bunch of interfaces to the machine.
	s.createNICWithIP(c, machine, "eth0", "10.0.0.11/24")
	s.createNICWithIP(c, machine, "eth1", "10.20.0.42/24")
	s.createNICWithIP(c, machine, "eth2", "10.30.0.99/24")

	err = machine.SetDevicesAddresses(
		state.LinkLayerDeviceAddress{
			DeviceName:        "eth1",
			ConfigMethod:      network.ConfigStatic,
			ProviderNetworkID: "vpc-abcd",
			ProviderSubnetID:  "prov-ffff",
			CIDRAddress:       "10.20.0.42/24",
		},
		state.LinkLayerDeviceAddress{
			DeviceName:        "eth2",
			ConfigMethod:      network.ConfigStatic,
			ProviderNetworkID: "vpc-abcd",
			ProviderSubnetID:  "prov-abcd",
			CIDRAddress:       "10.30.0.99/24",
		},
	)
	c.Assert(err, jc.ErrorIsNil)

	tracker.Reset()

	_, err = client.Status(context.Background(), nil)
	c.Assert(err, jc.ErrorIsNil)

	// The number of queries should be the same.
	c.Check(tracker.ReadCount(), gc.Equals, queryCount,
		gc.Commentf("if the query count is not the same, there has been a regression "+
			"in the way the addresses are processed"))
}

func (s *statusSuite) createNICWithIP(c *gc.C, machine *state.Machine, deviceName, cidrAddress string) {
	err := machine.SetLinkLayerDevices(
		state.LinkLayerDeviceArgs{
			Name:       deviceName,
			Type:       network.EthernetDevice,
			ParentName: "",
			IsUp:       true,
		},
	)
	c.Assert(err, jc.ErrorIsNil)
	err = machine.SetDevicesAddresses(
		state.LinkLayerDeviceAddress{
			DeviceName:   deviceName,
			CIDRAddress:  cidrAddress,
			ConfigMethod: network.ConfigStatic,
		},
	)
	c.Assert(err, jc.ErrorIsNil)
}

var _ = gc.Suite(&statusUnitTestSuite{})

type statusUnitTestSuite struct {
	baseSuite
}

func (s *statusUnitTestSuite) TestProcessMachinesWithOneMachineAndOneContainer(c *gc.C) {
	factory, release := s.NewFactory(c, s.ControllerModelUUID())
	release()
	factory = factory.WithModelConfigService(s.modelConfigService(c))
	host := factory.MakeMachine(c, &testfactory.MachineParams{InstanceId: "0"})
	container := factory.MakeMachineNested(c, host.Id(), nil)

	conn := s.OpenControllerModelAPI(c)
	client := apiclient.NewClient(conn, loggertesting.WrapCheckLog(c))
	status, err := client.Status(context.Background(), nil)
	c.Assert(err, jc.ErrorIsNil)

	c.Check(status.Machines, gc.HasLen, 1)
	mStatus, ok := status.Machines[host.Id()]
	c.Check(ok, jc.IsTrue)
	c.Check(mStatus.Containers, gc.HasLen, 1)

	_, ok = mStatus.Containers[container.Id()]
	c.Check(ok, jc.IsTrue)
}

func (s *statusUnitTestSuite) TestProcessMachinesWithEmbeddedContainers(c *gc.C) {
	factory, release := s.NewFactory(c, s.ControllerModelUUID())
	release()
	factory = factory.WithModelConfigService(s.modelConfigService(c))
	host := factory.MakeMachine(c, &testfactory.MachineParams{InstanceId: "1"})
	factory.MakeMachineNested(c, host.Id(), nil)
	lxdHost := factory.MakeMachineNested(c, host.Id(), nil)
	factory.MakeMachineNested(c, lxdHost.Id(), nil)

	conn := s.OpenControllerModelAPI(c)
	client := apiclient.NewClient(conn, loggertesting.WrapCheckLog(c))
	status, err := client.Status(context.Background(), nil)
	c.Assert(err, jc.ErrorIsNil)

	c.Check(status.Machines, gc.HasLen, 1)
	mStatus, ok := status.Machines[host.Id()]
	c.Check(ok, jc.IsTrue)
	c.Check(mStatus.Containers, gc.HasLen, 2)

	mStatus, ok = mStatus.Containers[lxdHost.Id()]
	c.Check(ok, jc.IsTrue)

	c.Check(mStatus.Containers, gc.HasLen, 1)
}

func (s *statusUnitTestSuite) TestApplicationWithExposedEndpoints(c *gc.C) {
	factory, release := s.NewFactory(c, s.ControllerModelUUID())
	release()
	factory = factory.WithModelConfigService(s.modelConfigService(c))
	charm := factory.MakeCharm(c, &testfactory.CharmParams{Name: "wordpress", URL: "ch:amd64/wordpress"})
	app := factory.MakeApplication(c, &testfactory.ApplicationParams{Charm: charm})
	err := app.MergeExposeSettings(map[string]state.ExposedEndpoint{
		"": {
			ExposeToSpaceIDs: []string{network.AlphaSpaceId},
			ExposeToCIDRs:    []string{"10.0.0.0/24", "192.168.0.0/24"},
		},
	})
	c.Assert(err, jc.ErrorIsNil)

	conn := s.OpenControllerModelAPI(c)
	client := apiclient.NewClient(conn, loggertesting.WrapCheckLog(c))
	status, err := client.Status(context.Background(), nil)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(status, gc.NotNil)
	appStatus, ok := status.Applications[app.Name()]
	c.Assert(ok, gc.Equals, true)

	c.Assert(appStatus.ExposedEndpoints, gc.DeepEquals, map[string]params.ExposedEndpoint{
		"": {
			ExposeToSpaces: []string{network.AlphaSpaceName},
			ExposeToCIDRs:  []string{"10.0.0.0/24", "192.168.0.0/24"},
		},
	})
}

func defaultCharmOrigin(curlStr string) *state.CharmOrigin {
	// Use ParseURL here in test until either the charm and/or application
	// can easily provide the same data.
	curl, _ := charm.ParseURL(curlStr)
	var source string
	var channel *state.Channel
	if charm.CharmHub.Matches(curl.Schema) {
		source = corecharm.CharmHub.String()
		channel = &state.Channel{
			Risk: "stable",
		}
	} else if charm.Local.Matches(curl.Schema) {
		source = corecharm.Local.String()
	}

	b := base.MustParseBaseFromString("ubuntu@22.04")

	platform := &state.Platform{
		Architecture: corearch.DefaultArchitecture,
		OS:           b.OS,
		Channel:      b.Channel.String(),
	}

	return &state.CharmOrigin{
		Source:   source,
		Type:     "charm",
		Revision: intPtr(curl.Revision),
		Channel:  channel,
		Platform: platform,
	}
}

func intPtr(i int) *int {
	return &i
}

func (s *statusUnitTestSuite) TestSubordinateUpgradingFrom(c *gc.C) {
	factory, release := s.NewFactory(c, s.ControllerModelUUID())
	release()
	factory = factory.WithModelConfigService(s.modelConfigService(c))
	principalCharm := factory.MakeCharm(c, &testfactory.CharmParams{Name: "mysql", URL: "ch:amd64/mysql"})
	subordCharm := factory.MakeCharm(c, &testfactory.CharmParams{Name: "logging", URL: "ch:amd64/logging-1"})
	subordCharmNew := factory.MakeCharm(c, &testfactory.CharmParams{Name: "logging", URL: "ch:amd64/logging-2"})
	app := factory.MakeApplication(c, &testfactory.ApplicationParams{
		Charm: principalCharm,
		Name:  "principal",
	})
	pu := factory.MakeUnit(c, &testfactory.UnitParams{
		Application: app,
	})
	subordApp := factory.MakeApplication(c, &testfactory.ApplicationParams{
		Charm:       subordCharm,
		CharmOrigin: defaultCharmOrigin(subordCharm.URL()),
		Name:        "subord",
	})

	subEndpoint, err := subordApp.Endpoint("info")
	c.Assert(err, jc.ErrorIsNil)
	principalEndpoint, err := app.Endpoint("juju-info")
	c.Assert(err, jc.ErrorIsNil)
	st := s.ControllerModel(c).State()
	rel, err := st.AddRelation(subEndpoint, principalEndpoint)
	c.Assert(err, jc.ErrorIsNil)
	ru, err := rel.Unit(pu)
	c.Assert(err, jc.ErrorIsNil)
	err = ru.EnterScope(s.modelConfigService(c), nil)
	c.Assert(err, jc.ErrorIsNil)
	subordUnit, err := st.Unit("subord/0")
	c.Assert(err, jc.ErrorIsNil)
	err = subordUnit.SetCharmURL(subordCharm.URL())
	c.Assert(err, jc.ErrorIsNil)

	conn := s.OpenControllerModelAPI(c)
	client := apiclient.NewClient(conn, loggertesting.WrapCheckLog(c))
	status, err := client.Status(context.Background(), nil)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(status, gc.NotNil)
	unitStatus, ok := status.Applications["principal"].Units["principal/0"].Subordinates["subord/0"]
	c.Assert(ok, gc.Equals, true)
	c.Assert(unitStatus.Charm, gc.Equals, "")

	err = subordApp.SetCharm(s.modelConfigService(c), state.SetCharmConfig{
		Charm:       subordCharmNew,
		CharmOrigin: defaultCharmOrigin(subordCharmNew.URL()),
	}, testing.NewObjectStore(c, s.ControllerModelUUID()))
	c.Assert(err, jc.ErrorIsNil)

	status, err = client.Status(context.Background(), nil)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(status, gc.NotNil)
	unitStatus, ok = status.Applications["principal"].Units["principal/0"].Subordinates["subord/0"]
	c.Assert(ok, gc.Equals, true)
	c.Assert(unitStatus.Charm, gc.Equals, "ch:amd64/logging-1")
}

func addUnitWithVersion(
	c *gc.C,
	modelConfigService state.ModelConfigService,
	application *state.Application,
	version string,
) *state.Unit {
	unit, err := application.AddUnit(modelConfigService, state.AddUnitParams{})
	c.Assert(err, jc.ErrorIsNil)
	// Ensure that the timestamp on this version record is different
	// from the previous one.
	// TODO(babbageclunk): when Application and Unit have clocks, change
	// that instead of sleeping (lp:1558657)
	time.Sleep(time.Millisecond * 1)
	err = unit.SetWorkloadVersion(version)
	c.Assert(err, jc.ErrorIsNil)
	return unit
}

func (s *statusUnitTestSuite) checkAppVersion(c *gc.C, application *state.Application, expectedVersion string,
) params.ApplicationStatus {
	conn := s.OpenControllerModelAPI(c)
	client := apiclient.NewClient(conn, loggertesting.WrapCheckLog(c))
	status, err := client.Status(context.Background(), nil)
	c.Assert(err, jc.ErrorIsNil)
	appStatus, found := status.Applications[application.Name()]
	c.Assert(found, jc.IsTrue)
	c.Check(appStatus.WorkloadVersion, gc.Equals, expectedVersion)
	return appStatus
}

func checkUnitVersion(c *gc.C, appStatus params.ApplicationStatus, unit *state.Unit, expectedVersion string) {
	unitStatus, found := appStatus.Units[unit.Name()]
	c.Check(found, jc.IsTrue)
	c.Check(unitStatus.WorkloadVersion, gc.Equals, expectedVersion)
}

func (s *statusUnitTestSuite) TestWorkloadVersionLastWins(c *gc.C) {
	factory, release := s.NewFactory(c, s.ControllerModelUUID())
	release()
	factory = factory.WithModelConfigService(s.modelConfigService(c))
	application := factory.MakeApplication(c, nil)
	unit1 := addUnitWithVersion(c, s.modelConfigService(c), application, "voltron")
	unit2 := addUnitWithVersion(c, s.modelConfigService(c), application, "voltron")
	unit3 := addUnitWithVersion(c, s.modelConfigService(c), application, "zarkon")

	appStatus := s.checkAppVersion(c, application, "zarkon")
	checkUnitVersion(c, appStatus, unit1, "voltron")
	checkUnitVersion(c, appStatus, unit2, "voltron")
	checkUnitVersion(c, appStatus, unit3, "zarkon")
}

func (s *statusUnitTestSuite) TestWorkloadVersionSimple(c *gc.C) {
	factory, release := s.NewFactory(c, s.ControllerModelUUID())
	release()
	factory = factory.WithModelConfigService(s.modelConfigService(c))
	application := factory.MakeApplication(c, nil)
	unit1 := addUnitWithVersion(c, s.modelConfigService(c), application, "voltron")

	appStatus := s.checkAppVersion(c, application, "voltron")
	checkUnitVersion(c, appStatus, unit1, "voltron")
}

func (s *statusUnitTestSuite) TestWorkloadVersionBlanksCanWin(c *gc.C) {
	factory, release := s.NewFactory(c, s.ControllerModelUUID())
	release()
	factory = factory.WithModelConfigService(s.modelConfigService(c))
	application := factory.MakeApplication(c, nil)
	unit1 := addUnitWithVersion(c, s.modelConfigService(c), application, "voltron")
	unit2 := addUnitWithVersion(c, s.modelConfigService(c), application, "")

	appStatus := s.checkAppVersion(c, application, "")
	checkUnitVersion(c, appStatus, unit1, "voltron")
	checkUnitVersion(c, appStatus, unit2, "")
}

func (s *statusUnitTestSuite) TestWorkloadVersionNoUnits(c *gc.C) {
	factory, release := s.NewFactory(c, s.ControllerModelUUID())
	release()
	factory = factory.WithModelConfigService(s.modelConfigService(c))
	application := factory.MakeApplication(c, nil)
	s.checkAppVersion(c, application, "")
}

func (s *statusUnitTestSuite) TestWorkloadVersionOkWithUnset(c *gc.C) {
	factory, release := s.NewFactory(c, s.ControllerModelUUID())
	release()
	factory = factory.WithModelConfigService(s.modelConfigService(c))
	application := factory.MakeApplication(c, nil)
	unit, err := application.AddUnit(s.modelConfigService(c), state.AddUnitParams{})
	c.Assert(err, jc.ErrorIsNil)
	appStatus := s.checkAppVersion(c, application, "")
	checkUnitVersion(c, appStatus, unit, "")
}

func (s *statusUnitTestSuite) TestMigrationInProgress(c *gc.C) {
	// Create a host model because controller models can't be migrated.
	factory, release := s.NewFactory(c, s.ControllerModelUUID())
	release()
	state2 := factory.MakeModel(c, nil)
	defer state2.Close()

	model2, err := state2.Model()
	c.Assert(err, jc.ErrorIsNil)

	// Double-write model information to dqlite.
	// Add the model to the model database.
	err = s.ModelTxnRunner(c, model2.UUID()).Txn(context.Background(), func(ctx context.Context, tx *sqlair.TX) error {
		return modelstate.CreateReadOnlyModel(ctx, domainmodel.ReadOnlyModelCreationArgs{
			UUID:         coremodel.UUID(model2.UUID()),
			Name:         model2.Name(),
			Cloud:        "dummy",
			AgentVersion: version.Current,
		}, preparer{}, tx)
	})
	c.Assert(err, jc.ErrorIsNil)

	// Get API connection to hosted model.
	apiInfo := s.ModelApiInfo(model2.UUID())
	apiInfo.Tag = testing.AdminUser
	apiInfo.Password = testing.AdminSecret

	conn, err := api.Open(context.Background(), apiInfo, api.DialOpts{})
	c.Assert(err, jc.ErrorIsNil)
	client := apiclient.NewClient(conn, loggertesting.WrapCheckLog(c))

	checkMigStatus := func(expected string) {
		status, err := client.Status(context.Background(), nil)
		c.Assert(err, jc.ErrorIsNil)
		if expected != "" {
			expected = "migrating: " + expected
		}
		c.Check(status.Model.ModelStatus.Info, gc.Equals, expected)
	}

	// Migration status should be empty when no migration is happening.
	checkMigStatus("")

	// Start it migrating.
	mig, err := state2.CreateMigration(state.MigrationSpec{
		InitiatedBy: names.NewUserTag("admin"),
		TargetInfo: migration.TargetInfo{
			ControllerTag: names.NewControllerTag(uuid.MustNewUUID().String()),
			Addrs:         []string{"1.2.3.4:5555", "4.3.2.1:6666"},
			CACert:        "cert",
			AuthTag:       names.NewUserTag("user"),
			Password:      "password",
		},
	})
	c.Assert(err, jc.ErrorIsNil)

	// Check initial message.
	checkMigStatus("starting")

	// Check status is reported when set.
	setAndCheckMigStatus := func(message string) {
		err := mig.SetStatusMessage(message)
		c.Assert(err, jc.ErrorIsNil)
		checkMigStatus(message)
	}
	setAndCheckMigStatus("proceeding swimmingly")
	setAndCheckMigStatus("oh noes")
}

func (s *statusUnitTestSuite) TestRelationFiltered(c *gc.C) {
	factory, release := s.NewFactory(c, s.ControllerModelUUID())
	release()
	factory = factory.WithModelConfigService(s.modelConfigService(c))
	// make application 1 with endpoint 1
	wordpress := factory.MakeApplication(c, &testfactory.ApplicationParams{
		Name: "abc",
		Charm: factory.MakeCharm(c, &testfactory.CharmParams{
			Name: "wordpress",
		}),
	})
	wordpressEndpoint, err := wordpress.Endpoint("db")
	c.Assert(err, jc.ErrorIsNil)

	// make application 2 with endpoint 2
	mysql := factory.MakeApplication(c, &testfactory.ApplicationParams{
		Name: "def",
		Charm: factory.MakeCharm(c, &testfactory.CharmParams{
			Name: "mysql",
		}),
	})
	mysqlEndpoint, err := mysql.Endpoint("server")
	c.Assert(err, jc.ErrorIsNil)

	// create relation between wordpress and mysql
	wordpressMysqlRelation := factory.MakeRelation(c, &testfactory.RelationParams{
		Endpoints: []state.Endpoint{wordpressEndpoint, mysqlEndpoint},
	})
	c.Assert(wordpressMysqlRelation, gc.NotNil)

	// create another application 3 with an endpoint 3
	logging := factory.MakeApplication(c, &testfactory.ApplicationParams{
		Charm: factory.MakeCharm(c, &testfactory.CharmParams{Name: "logging"}),
	})
	loggingEndpoint, err := logging.Endpoint("info")
	c.Assert(err, jc.ErrorIsNil)

	// create endpoint 4 on application 1
	wordpressJujuInfo, err := wordpress.Endpoint("juju-info")
	c.Assert(err, jc.ErrorIsNil)
	wordpressLoggingRelation := factory.MakeRelation(c, &testfactory.RelationParams{
		Endpoints: []state.Endpoint{loggingEndpoint, wordpressJujuInfo},
	})
	c.Assert(wordpressLoggingRelation, gc.NotNil)

	// Test status filtering with application 1: should get both relations
	conn := s.OpenControllerModelAPI(c)
	client := apiclient.NewClient(conn, loggertesting.WrapCheckLog(c))
	status, err := client.Status(context.Background(),
		&apiclient.StatusArgs{
			Patterns: []string{wordpress.Name()},
		})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(status, gc.NotNil)
	assertApplicationRelations(c, wordpress.Name(), 2, status.Relations)

	// test status filtering with application 3: should get 1 relation
	status, err = client.Status(
		context.Background(),
		&apiclient.StatusArgs{
			Patterns: []string{logging.Name()},
		})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(status, gc.NotNil)
	assertApplicationRelations(c, logging.Name(), 1, status.Relations)
}

// TestApplicationFilterIndependentOfAlphabeticUnitOrdering ensures we
// do not regress and are carrying forward fix for lp#1592872.
func (s *statusUnitTestSuite) TestApplicationFilterIndependentOfAlphabeticUnitOrdering(c *gc.C) {
	factory, release := s.NewFactory(c, s.ControllerModelUUID())
	release()
	factory = factory.WithModelConfigService(s.modelConfigService(c))
	// Application A has no touch points with application C
	// but will have a unit on the same machine is a unit of an application B.
	applicationA := factory.MakeApplication(c, &testfactory.ApplicationParams{
		Charm: factory.MakeCharm(c, &testfactory.CharmParams{
			Name: "mysql",
		}),
		Name: "abc",
	})

	// Application B will have a unit on the same machine as a unit of an application A
	// and will have a relation to an application C.
	applicationB := factory.MakeApplication(c, &testfactory.ApplicationParams{
		Charm: factory.MakeCharm(c, &testfactory.CharmParams{
			Name: "wordpress",
		}),
		Name: "def",
	})

	// Put a unit from each, application A and B, on the same machine.
	// This will be enough to ensure that the application B qualifies to be
	// in the status result filtered by the application A.
	machine := factory.MakeMachine(c, &testfactory.MachineParams{
		Jobs: []state.MachineJob{state.JobHostUnits},
	})
	factory.MakeUnit(c, &testfactory.UnitParams{
		Application: applicationA,
		Machine:     machine,
	})
	factory.MakeUnit(c, &testfactory.UnitParams{
		Application: applicationB,
		Machine:     machine,
	})

	conn := s.OpenControllerModelAPI(c)
	client := apiclient.NewClient(conn, loggertesting.WrapCheckLog(c))
	for i := 0; i < 20; i++ {
		c.Logf("run %d", i)
		status, err := client.Status(
			context.Background(),
			&apiclient.StatusArgs{
				Patterns: []string{applicationA.Name()},
			})
		c.Assert(err, jc.ErrorIsNil)
		c.Check(status.Applications, gc.HasLen, 2)
	}
}

// TestFilterOutRelationsForRelatedApplicationsThatDoNotMatchCriteriaDirectly
// tests scenario where applications are returned as part of the status because
// they are related to an application that matches given filter.
// However, the relations for these applications should not be returned.
// In other words, if there are two applications, A and B, such that:
//
// * an application A matches the supplied filter directly;
// * an application B has units on the same machine as units of an application A and, thus,
// qualifies to be returned by the status result;
//
// application B's relations should not be returned.
func (s *statusUnitTestSuite) TestFilterOutRelationsForRelatedApplicationsThatDoNotMatchCriteriaDirectly(c *gc.C) {
	factory, release := s.NewFactory(c, s.ControllerModelUUID())
	release()
	factory = factory.WithModelConfigService(s.modelConfigService(c))
	// Application A has no touch points with application C
	// but will have a unit on the same machine is a unit of an application B.
	applicationA := factory.MakeApplication(c, &testfactory.ApplicationParams{
		Charm: factory.MakeCharm(c, &testfactory.CharmParams{
			Name: "mysql",
		}),
	})

	// Application B will have a unit on the same machine as a unit of an application A
	// and will have a relation to an application C.
	applicationB := factory.MakeApplication(c, &testfactory.ApplicationParams{
		Charm: factory.MakeCharm(c, &testfactory.CharmParams{
			Name: "wordpress",
		}),
	})
	endpoint1, err := applicationB.Endpoint("juju-info")
	c.Assert(err, jc.ErrorIsNil)

	// Application C has a relation to application B but has no touch points with
	// an application A.
	applicationC := factory.MakeApplication(c, &testfactory.ApplicationParams{
		Charm: factory.MakeCharm(c, &testfactory.CharmParams{Name: "logging"}),
	})
	endpoint2, err := applicationC.Endpoint("info")
	c.Assert(err, jc.ErrorIsNil)
	factory.MakeRelation(c, &testfactory.RelationParams{
		Endpoints: []state.Endpoint{endpoint2, endpoint1},
	})

	// Put a unit from each, application A and B, on the same machine.
	// This will be enough to ensure that the application B qualifies to be
	// in the status result filtered by the application A.
	machine := factory.MakeMachine(c, &testfactory.MachineParams{
		Jobs: []state.MachineJob{state.JobHostUnits},
	})
	factory.MakeUnit(c, &testfactory.UnitParams{
		Application: applicationA,
		Machine:     machine,
	})
	factory.MakeUnit(c, &testfactory.UnitParams{
		Application: applicationB,
		Machine:     machine,
	})

	// Filtering status on application A should get:
	// * no relations;
	// * two applications.
	conn := s.OpenControllerModelAPI(c)
	client := apiclient.NewClient(conn, loggertesting.WrapCheckLog(c))
	status, err := client.Status(
		context.Background(),
		&apiclient.StatusArgs{
			Patterns: []string{applicationA.Name()},
		})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(status, gc.NotNil)
	c.Check(status.Applications, gc.HasLen, 2)
	c.Check(status.Relations, gc.HasLen, 0)
}

func (s *statusUnitTestSuite) TestMachineWithNoDisplayNameHasItsEmptyDisplayNameSent(c *gc.C) {
	factory, release := s.NewFactory(c, s.ControllerModelUUID())
	release()
	factory = factory.WithModelConfigService(s.modelConfigService(c))
	machine := factory.MakeMachine(c, &testfactory.MachineParams{
		InstanceId: "i-123",
	})

	conn := s.OpenControllerModelAPI(c)
	client := apiclient.NewClient(conn, loggertesting.WrapCheckLog(c))
	status, err := client.Status(context.Background(), nil)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(status.Machines, gc.HasLen, 1)
	c.Assert(status.Machines[machine.Id()].DisplayName, gc.Equals, "")
}

func (s *statusUnitTestSuite) TestMachineWithDisplayNameHasItsDisplayNameSent(c *gc.C) {
	factory, release := s.NewFactory(c, s.ControllerModelUUID())
	release()
	factory = factory.WithModelConfigService(s.modelConfigService(c))
	machine := factory.MakeMachine(c, &testfactory.MachineParams{
		InstanceId:  "i-123",
		DisplayName: "snowflake",
	})
	machineService := s.ControllerDomainServices(c).Machine()
	machineUUID, err := machineService.CreateMachine(context.Background(), coremachine.Name("0"))
	c.Assert(err, jc.ErrorIsNil)
	err = machineService.SetMachineCloudInstance(context.Background(), machineUUID, instance.Id("i-123"), "snowflake", nil)
	c.Assert(err, jc.ErrorIsNil)

	conn := s.OpenControllerModelAPI(c)
	client := apiclient.NewClient(conn, loggertesting.WrapCheckLog(c))
	status, err := client.Status(context.Background(), nil)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(status.Machines, gc.HasLen, 1)
	c.Assert(status.Machines[machine.Id()].DisplayName, gc.Equals, "snowflake")
}

func assertApplicationRelations(c *gc.C, appName string, expectedNumber int, relations []params.RelationStatus) {
	c.Check(relations, gc.HasLen, expectedNumber)
	for _, relation := range relations {
		belongs := false
		for _, endpoint := range relation.Endpoints {
			if endpoint.ApplicationName == appName {
				belongs = true
				continue
			}
		}
		if !belongs {
			c.Fatalf("application %v is not part of the relation %v as expected", appName, relation.Id)
		}
	}
}

func (s *statusUnitTestSuite) TestUnitsWithOpenedPortsSent(c *gc.C) {
	factory, release := s.NewFactory(c, s.ControllerModelUUID())
	release()
	factory = factory.WithModelConfigService(s.modelConfigService(c))

	app := factory.MakeApplication(c, &testfactory.ApplicationParams{
		Charm: factory.MakeCharm(c, &testfactory.CharmParams{
			Name: "wordpress",
		}),
	})
	_ = factory.MakeUnit(c, &testfactory.UnitParams{
		Application: app,
	})

	appService := s.ControllerDomainServices(c).Application(applicationservice.ApplicationServiceParams{
		StorageRegistry: storage.NotImplementedProviderRegistry{},
		Secrets:         applicationservice.NotImplementedSecretService{},
	})

	unitUUID, err := appService.GetUnitUUID(context.Background(), "wordpress/0")
	c.Assert(err, jc.ErrorIsNil)

	portService := s.ControllerDomainServices(c).Port()
	err = portService.UpdateUnitPorts(context.Background(), unitUUID, network.GroupedPortRanges{
		"":    []network.PortRange{network.MustParsePortRange("1000/tcp")},
		"foo": []network.PortRange{network.MustParsePortRange("2000/tcp")},
	}, network.GroupedPortRanges{})
	c.Assert(err, jc.ErrorIsNil)

	conn := s.OpenControllerModelAPI(c)
	client := apiclient.NewClient(conn, loggertesting.WrapCheckLog(c))
	status, err := client.Status(context.Background(), &apiclient.StatusArgs{})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(status, gc.NotNil)
	c.Assert(status.Applications, gc.HasLen, 1)
	c.Assert(status.Applications["wordpress"].Units, gc.HasLen, 1)
	c.Assert(status.Applications["wordpress"].Units["wordpress/0"].OpenedPorts, gc.DeepEquals, []string{"1000/tcp", "2000/tcp"})
}

type statusUpgradeUnitSuite struct {
	testing.ApiServerSuite

	charms               map[string]*state.Charm
	charmrevisionupdater *charmrevisionupdater.CharmRevisionUpdaterAPI
	ctrl                 *gomock.Controller
}

var _ = gc.Suite(&statusUpgradeUnitSuite{})

// modelConfigService is a convenience function to get the controller model's
// model config service inside a test.
func (s *statusUpgradeUnitSuite) modelConfigService(c *gc.C) state.ModelConfigService {
	return s.ControllerDomainServices(c).Config()
}

func (s *statusUpgradeUnitSuite) SetUpTest(c *gc.C) {
	s.ApiServerSuite.WithLeaseManager = true
	s.ApiServerSuite.SetUpTest(c)
	s.charms = make(map[string]*state.Charm)

	state := charmrevisionupdater.StateShim{State: s.ControllerModel(c).State()}
	s.ctrl = gomock.NewController(c)
	charmhubClient := mocks.NewMockCharmhubRefreshClient(s.ctrl)
	charmhubClient.EXPECT().RefreshWithRequestMetrics(gomock.Any(), gomock.Any(),
		gomock.Any()).Return([]transport.RefreshResponse{
		{Entity: transport.RefreshEntity{Revision: 42}},
	}, nil)
	newCharmhubClient := func(context.Context) (charmrevisionupdater.CharmhubRefreshClient, error) {
		return charmhubClient, nil
	}
	modelConfigService := mocks.NewMockModelConfigService(s.ctrl)
	cfg, err := config.New(config.UseDefaults, map[string]interface{}{
		"name": "model",
		"type": "type",
		"uuid": s.DefaultModelUUID,
	})
	c.Assert(err, jc.ErrorIsNil)
	modelConfigService.EXPECT().ModelConfig(gomock.Any()).Return(cfg, nil)

	s.charmrevisionupdater, err = charmrevisionupdater.NewCharmRevisionUpdaterAPIState(
		state,
		testing.NewObjectStore(c, s.ControllerModelUUID()),
		clock.WallClock,
		modelConfigService,
		newCharmhubClient, loggertesting.WrapCheckLog(c))
	c.Assert(err, jc.ErrorIsNil)
}

func (s *statusUpgradeUnitSuite) TearDownTest(c *gc.C) {
	s.ApiServerSuite.TearDownTest(c)
	s.ctrl.Finish()
}

// AddMachine adds a new machine to state.
func (s *statusUpgradeUnitSuite) AddMachine(c *gc.C, machineId string, job state.MachineJob) {
	st := s.ControllerModel(c).State()
	machine, err := st.AddOneMachine(s.modelConfigService(c), state.MachineTemplate{
		Base: state.UbuntuBase("12.10"),
		Jobs: []state.MachineJob{job},
	})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(machine.Id(), gc.Equals, machineId)
}

// AddCharmhubCharmWithRevision adds a charmhub charm with the specified revision to state.
func (s *statusUpgradeUnitSuite) AddCharmhubCharmWithRevision(c *gc.C, charmName string, rev int) *state.Charm {
	ch := testcharms.Hub.CharmDir(charmName)
	name := ch.Meta().Name
	curl := fmt.Sprintf("ch:amd64/%s-%d", name, rev)
	info := state.CharmInfo{
		Charm:       ch,
		ID:          curl,
		StoragePath: "dummy-path",
		SHA256:      fmt.Sprintf("%s-%d-sha256", name, rev),
	}
	dummy, err := s.ControllerModel(c).State().AddCharm(info)
	c.Assert(err, jc.ErrorIsNil)
	s.charms[name] = dummy
	return dummy
}

// AddApplication adds an application for the specified charm to state.
func (s *statusUpgradeUnitSuite) AddApplication(c *gc.C, charmName, applicationName string) {
	ch, ok := s.charms[charmName]
	c.Assert(ok, jc.IsTrue)
	revision := ch.Revision()

	st := s.ControllerModel(c).State()
	_, err := st.AddApplication(s.modelConfigService(c), state.AddApplicationArgs{
		Name:  applicationName,
		Charm: ch,
		CharmOrigin: &state.CharmOrigin{
			ID:     "mycharmhubid",
			Hash:   "mycharmhash",
			Source: "charm-hub",
			Platform: &state.Platform{
				Architecture: "amd64",
				OS:           "ubuntu",
				Channel:      "12.10/stable",
			},
			Revision: &revision,
			Channel: &state.Channel{
				Track: "latest",
				Risk:  "stable",
			},
		},
	}, testing.NewObjectStore(c, s.ControllerModelUUID()))
	c.Assert(err, jc.ErrorIsNil)
}

// AddUnit adds a new unit for application to the specified machine.
func (s *statusUpgradeUnitSuite) AddUnit(c *gc.C, appName, machineId string) {
	app, err := s.ControllerModel(c).State().Application(appName)
	c.Assert(err, jc.ErrorIsNil)
	unit, err := app.AddUnit(s.modelConfigService(c), state.AddUnitParams{})
	c.Assert(err, jc.ErrorIsNil)
	machine, err := s.ControllerModel(c).State().Machine(machineId)
	c.Assert(err, jc.ErrorIsNil)
	err = unit.AssignToMachine(s.modelConfigService(c), machine)
	c.Assert(err, jc.ErrorIsNil)
}

// SetUnitRevision sets the unit's charm to the specified revision.
func (s *statusUpgradeUnitSuite) SetUnitRevision(c *gc.C, unitName string, rev int) {
	unit, err := s.ControllerModel(c).State().Unit(unitName)
	c.Assert(err, jc.ErrorIsNil)
	svc, err := unit.Application()
	c.Assert(err, jc.ErrorIsNil)
	curl := fmt.Sprintf("ch:amd64/%s-%d", svc.Name(), rev)
	err = unit.SetCharmURL(curl)
	c.Assert(err, jc.ErrorIsNil)
}

// SetupScenario adds some machines and applications to state.
// It assumes a controller machine has already been created.
func (s *statusUpgradeUnitSuite) SetupScenario(c *gc.C) {
	s.AddMachine(c, "1", state.JobHostUnits)
	s.AddMachine(c, "2", state.JobHostUnits)
	s.AddMachine(c, "3", state.JobHostUnits)

	// mysql is out of date
	s.AddCharmhubCharmWithRevision(c, "mysql", 22)
	s.AddApplication(c, "mysql", "mysql")
	s.AddUnit(c, "mysql", "1")

	// wordpress is up to date
	s.AddCharmhubCharmWithRevision(c, "wordpress", 26)
	s.AddApplication(c, "wordpress", "wordpress")
	s.AddUnit(c, "wordpress", "2")
	s.AddUnit(c, "wordpress", "2")
	// wordpress/0 has a version, wordpress/1 is unknown
	s.SetUnitRevision(c, "wordpress/0", 26)

	// varnish is a charm that does not have a version in the mock store.
	s.AddCharmhubCharmWithRevision(c, "varnish", 5)
	s.AddApplication(c, "varnish", "varnish")
	s.AddUnit(c, "varnish", "3")
}

func (s *statusUpgradeUnitSuite) TestUpdateRevisionsCharmhub(c *gc.C) {
	s.AddMachine(c, "0", state.JobManageModel)
	s.AddMachine(c, "1", state.JobHostUnits)
	s.AddCharmhubCharmWithRevision(c, "charmhubby", 41)
	s.AddApplication(c, "charmhubby", "charmhubby")
	s.AddUnit(c, "charmhubby", "1")

	conn := s.OpenControllerModelAPI(c)
	client := apiclient.NewClient(conn, loggertesting.WrapCheckLog(c))
	status, _ := client.Status(context.Background(), nil)

	appStatus, ok := status.Applications["charmhubby"]
	c.Assert(ok, gc.Equals, true)
	c.Assert(appStatus.CanUpgradeTo, gc.Equals, "")

	// Update to the latest available charm revision.
	result, err := s.charmrevisionupdater.UpdateLatestRevisions(context.Background())
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(result.Error, gc.IsNil)

	// Check if CanUpgradeTo suggests the latest revision.
	status, _ = client.Status(context.Background(), nil)
	appStatus, ok = status.Applications["charmhubby"]
	c.Assert(ok, gc.Equals, true)
	c.Assert(appStatus.CanUpgradeTo, gc.Equals, "ch:amd64/charmhubby-42")
}

type preparer struct{}

func (p preparer) Prepare(query string, args ...any) (*sqlair.Statement, error) {
	return sqlair.Prepare(query, args...)
}
