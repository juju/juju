// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package client_test

import (
	"time"

	"github.com/juju/names/v4"
	jc "github.com/juju/testing/checkers"
	"github.com/juju/utils"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/api"
	"github.com/juju/juju/apiserver/common"
	"github.com/juju/juju/apiserver/facade/facadetest"
	"github.com/juju/juju/apiserver/facades/client/client"
	"github.com/juju/juju/apiserver/facades/controller/charmrevisionupdater"
	"github.com/juju/juju/apiserver/facades/controller/charmrevisionupdater/testing"
	"github.com/juju/juju/apiserver/params"
	apiservertesting "github.com/juju/juju/apiserver/testing"
	"github.com/juju/juju/core/instance"
	"github.com/juju/juju/core/migration"
	"github.com/juju/juju/core/network"
	"github.com/juju/juju/core/status"
	"github.com/juju/juju/feature"
	jujutesting "github.com/juju/juju/juju/testing"
	"github.com/juju/juju/state"
	coretesting "github.com/juju/juju/testing"
	"github.com/juju/juju/testing/factory"
)

type statusSuite struct {
	baseSuite
}

var _ = gc.Suite(&statusSuite{})

func (s *statusSuite) addMachine(c *gc.C) *state.Machine {
	machine, err := s.State.AddMachine("quantal", state.JobHostUnits)
	c.Assert(err, jc.ErrorIsNil)
	return machine
}

// Complete testing of status functionality happens elsewhere in the codebase,
// these tests just sanity-check the api itself.

func (s *statusSuite) TestFullStatus(c *gc.C) {
	machine := s.addMachine(c)
	c.Assert(s.State.SetSLA("essential", "test-user", []byte("")), jc.ErrorIsNil)
	c.Assert(s.State.SetModelMeterStatus("GREEN", "goo"), jc.ErrorIsNil)
	client := s.APIState.Client()
	status, err := client.Status(nil)
	c.Assert(err, jc.ErrorIsNil)
	c.Check(status.Model.Name, gc.Equals, "controller")
	c.Check(status.Model.Type, gc.Equals, "iaas")
	c.Check(status.Model.CloudTag, gc.Equals, "cloud-dummy")
	c.Check(status.Model.SLA, gc.Equals, "essential")
	c.Check(status.Model.MeterStatus.Color, gc.Equals, "green")
	c.Check(status.Model.MeterStatus.Message, gc.Equals, "goo")
	c.Check(status.Applications, gc.HasLen, 0)
	c.Check(status.RemoteApplications, gc.HasLen, 0)
	c.Check(status.Offers, gc.HasLen, 0)
	c.Check(status.Machines, gc.HasLen, 1)
	c.Check(status.ControllerTimestamp, gc.NotNil)
	c.Check(status.Branches, gc.HasLen, 0)
	resultMachine, ok := status.Machines[machine.Id()]
	if !ok {
		c.Fatalf("Missing machine with id %q", machine.Id())
	}
	c.Check(resultMachine.Id, gc.Equals, machine.Id())
	c.Check(resultMachine.Series, gc.Equals, machine.Series())
	c.Check(resultMachine.LXDProfiles, gc.HasLen, 0)
}

func (s *statusSuite) TestUnsupportedNoModelMeterStatus(c *gc.C) {
	s.addMachine(c)
	c.Assert(s.State.SetSLA("unsupported", "test-user", []byte("")), jc.ErrorIsNil)
	c.Assert(s.State.SetModelMeterStatus("RED", "nope"), jc.ErrorIsNil)
	client := s.APIState.Client()
	status, err := client.Status(nil)
	c.Assert(err, jc.ErrorIsNil)
	c.Check(status.Model.SLA, gc.Equals, "unsupported")
	c.Check(status.Model.MeterStatus.Color, gc.Equals, "")
	c.Check(status.Model.MeterStatus.Message, gc.Equals, "")
}

func (s *statusSuite) TestFullStatusUnitLeadership(c *gc.C) {
	u := s.Factory.MakeUnit(c, nil)
	claimer, err := s.LeaseManager.Claimer("application-leadership", s.State.ModelUUID())
	c.Assert(err, jc.ErrorIsNil)
	err = claimer.Claim(u.ApplicationName(), u.Name(), time.Minute)
	c.Assert(err, jc.ErrorIsNil)
	client := s.APIState.Client()
	status, err := client.Status(nil)
	c.Assert(err, jc.ErrorIsNil)
	app, ok := status.Applications[u.ApplicationName()]
	c.Assert(ok, jc.IsTrue)
	unit, ok := app.Units[u.Name()]
	c.Assert(ok, jc.IsTrue)
	c.Assert(unit.Leader, jc.IsTrue)
}

func (s *statusSuite) TestFullStatusUnitScaling(c *gc.C) {
	machine := s.Factory.MakeMachine(c, nil)
	unit := s.Factory.MakeUnit(c, &factory.UnitParams{
		Machine: machine,
	})

	s.WaitForModelWatchersIdle(c, s.State.ModelUUID())
	tracker := s.State.TrackQueries()

	client := s.APIState.Client()
	_, err := client.Status(nil)
	c.Assert(err, jc.ErrorIsNil)

	queryCount := tracker.ReadCount()

	// Add several more units of the same application to the
	// same machine. We do this because we want to isolate to
	// status handling to just additional units, not additional machines
	// or applications.
	app, err := unit.Application()
	c.Assert(err, jc.ErrorIsNil)
	for i := 0; i < 5; i++ {
		s.Factory.MakeUnit(c, &factory.UnitParams{
			Application: app,
			Machine:     machine,
		})
	}

	s.WaitForModelWatchersIdle(c, s.State.ModelUUID())
	tracker.Reset()

	_, err = client.Status(nil)
	c.Assert(err, jc.ErrorIsNil)

	// The number of queries should be the same.
	c.Check(tracker.ReadCount(), gc.Equals, queryCount,
		gc.Commentf("if the query count is not the same, there has been a regression "+
			"in the processing of units, please fix it"))
}

func (s *statusSuite) TestFullStatusMachineScaling(c *gc.C) {
	s.Factory.MakeMachine(c, nil)

	s.WaitForModelWatchersIdle(c, s.State.ModelUUID())
	tracker := s.State.TrackQueries()

	client := s.APIState.Client()
	_, err := client.Status(nil)
	c.Assert(err, jc.ErrorIsNil)

	queryCount := tracker.ReadCount()

	// Add several more machines to the model.
	for i := 0; i < 5; i++ {
		s.Factory.MakeMachine(c, nil)
	}

	s.WaitForModelWatchersIdle(c, s.State.ModelUUID())
	tracker.Reset()

	_, err = client.Status(nil)
	c.Assert(err, jc.ErrorIsNil)

	// The number of queries should be the same.
	c.Check(tracker.ReadCount(), gc.Equals, queryCount,
		gc.Commentf("if the query count is not the same, there has been a regression "+
			"in the processing of machines, please fix it"))
}

func (s *statusSuite) TestFullStatusInterfaceScaling(c *gc.C) {
	machine := s.addMachine(c)
	s.createSpaceAndSubnetWithProviderID(c, "public", "10.0.0.0/24", "prov-0000")
	s.createSpaceAndSubnetWithProviderID(c, "private", "10.20.0.0/24", "prov-ffff")
	s.createSpaceAndSubnetWithProviderID(c, "dmz", "10.30.0.0/24", "prov-abcd")

	s.WaitForModelWatchersIdle(c, s.State.ModelUUID())
	tracker := s.State.TrackQueries()

	client := s.APIState.Client()
	_, err := client.Status(nil)
	c.Assert(err, jc.ErrorIsNil)

	queryCount := tracker.ReadCount()

	// Add a bunch of interfaces to the machine.
	s.createNICWithIP(c, machine, "eth0", "10.0.0.11/24")
	s.createNICWithIP(c, machine, "eth1", "10.20.0.42/24")
	s.createNICWithIP(c, machine, "eth2", "10.30.0.99/24")

	err = machine.SetDevicesAddresses(
		state.LinkLayerDeviceAddress{
			DeviceName:        "eth1",
			ConfigMethod:      network.StaticAddress,
			ProviderNetworkID: "vpc-abcd",
			ProviderSubnetID:  "prov-ffff",
			CIDRAddress:       "10.20.0.42/24",
		},
		state.LinkLayerDeviceAddress{
			DeviceName:        "eth2",
			ConfigMethod:      network.StaticAddress,
			ProviderNetworkID: "vpc-abcd",
			ProviderSubnetID:  "prov-abcd",
			CIDRAddress:       "10.30.0.99/24",
		},
	)
	c.Assert(err, jc.ErrorIsNil)

	s.WaitForModelWatchersIdle(c, s.State.ModelUUID())
	tracker.Reset()

	_, err = client.Status(nil)
	c.Assert(err, jc.ErrorIsNil)

	// The number of queries should be the same.
	c.Check(tracker.ReadCount(), gc.Equals, queryCount,
		gc.Commentf("if the query count is not the same, there has been a regression "+
			"in the way the addresses are processed"))
}

func (s *statusSuite) createSpaceAndSubnetWithProviderID(c *gc.C, spaceName, CIDR, providerSubnetID string) {
	space, err := s.State.AddSpace(spaceName, network.Id(spaceName), nil, true)
	c.Assert(err, jc.ErrorIsNil)

	_, err = s.State.AddSubnet(network.SubnetInfo{
		CIDR:       CIDR,
		SpaceID:    space.Id(),
		ProviderId: network.Id(providerSubnetID),
	})
	c.Assert(err, jc.ErrorIsNil)
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
			ConfigMethod: network.StaticAddress,
		},
	)
	c.Assert(err, jc.ErrorIsNil)
}

var _ = gc.Suite(&statusUnitTestSuite{})

type statusUnitTestSuite struct {
	baseSuite
}

func (s *statusUnitTestSuite) TestProcessMachinesWithOneMachineAndOneContainer(c *gc.C) {
	host := s.Factory.MakeMachine(c, &factory.MachineParams{InstanceId: instance.Id("0")})
	container := s.Factory.MakeMachineNested(c, host.Id(), nil)

	client := s.APIState.Client()
	status, err := client.Status(nil)
	c.Assert(err, jc.ErrorIsNil)

	c.Check(status.Machines, gc.HasLen, 1)
	mStatus, ok := status.Machines[host.Id()]
	c.Check(ok, jc.IsTrue)
	c.Check(mStatus.Containers, gc.HasLen, 1)

	_, ok = mStatus.Containers[container.Id()]
	c.Check(ok, jc.IsTrue)
}

func (s *statusUnitTestSuite) TestProcessMachinesWithEmbeddedContainers(c *gc.C) {
	host := s.Factory.MakeMachine(c, &factory.MachineParams{InstanceId: instance.Id("1")})
	s.Factory.MakeMachineNested(c, host.Id(), nil)
	lxdHost := s.Factory.MakeMachineNested(c, host.Id(), nil)
	s.Factory.MakeMachineNested(c, lxdHost.Id(), nil)

	client := s.APIState.Client()
	status, err := client.Status(nil)
	c.Assert(err, jc.ErrorIsNil)

	c.Check(status.Machines, gc.HasLen, 1)
	mStatus, ok := status.Machines[host.Id()]
	c.Check(ok, jc.IsTrue)
	c.Check(mStatus.Containers, gc.HasLen, 2)

	mStatus, ok = mStatus.Containers[lxdHost.Id()]
	c.Check(ok, jc.IsTrue)

	c.Check(mStatus.Containers, gc.HasLen, 1)
}

var testUnits = []struct {
	unitName       string
	setStatus      *state.MeterStatus
	expectedStatus *params.MeterStatus
}{{
	setStatus:      &state.MeterStatus{Code: state.MeterGreen, Info: "test information"},
	expectedStatus: &params.MeterStatus{Color: "green", Message: "test information"},
}, {
	setStatus:      &state.MeterStatus{Code: state.MeterAmber, Info: "test information"},
	expectedStatus: &params.MeterStatus{Color: "amber", Message: "test information"},
}, {
	setStatus:      &state.MeterStatus{Code: state.MeterRed, Info: "test information"},
	expectedStatus: &params.MeterStatus{Color: "red", Message: "test information"},
}, {
	setStatus:      &state.MeterStatus{Code: state.MeterGreen, Info: "test information"},
	expectedStatus: &params.MeterStatus{Color: "green", Message: "test information"},
}, {},
}

func (s *statusUnitTestSuite) TestModelMeterStatus(c *gc.C) {
	c.Assert(s.State.SetSLA("advanced", "test-user", nil), jc.ErrorIsNil)
	c.Assert(s.State.SetModelMeterStatus("RED", "thing"), jc.ErrorIsNil)

	client := s.APIState.Client()
	status, err := client.Status(nil)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(status, gc.NotNil)
	modelMeterStatus := status.Model.MeterStatus
	c.Assert(modelMeterStatus.Color, gc.Equals, "red")
	c.Assert(modelMeterStatus.Message, gc.Equals, "thing")
}

func (s *statusUnitTestSuite) TestMeterStatus(c *gc.C) {
	meteredCharm := s.Factory.MakeCharm(c, &factory.CharmParams{Name: "metered", URL: "cs:quantal/metered"})
	service := s.Factory.MakeApplication(c, &factory.ApplicationParams{Charm: meteredCharm})

	units, err := service.AllUnits()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(units, gc.HasLen, 0)

	for i, unit := range testUnits {
		u, err := service.AddUnit(state.AddUnitParams{})
		testUnits[i].unitName = u.Name()
		c.Assert(err, jc.ErrorIsNil)
		if unit.setStatus != nil {
			err := u.SetMeterStatus(unit.setStatus.Code.String(), unit.setStatus.Info)
			c.Assert(err, jc.ErrorIsNil)
		}
	}

	client := s.APIState.Client()
	status, err := client.Status(nil)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(status, gc.NotNil)
	serviceStatus, ok := status.Applications[service.Name()]
	c.Assert(ok, gc.Equals, true)

	c.Assert(serviceStatus.MeterStatuses, gc.HasLen, len(testUnits)-1)
	for _, unit := range testUnits {
		unitStatus, ok := serviceStatus.MeterStatuses[unit.unitName]

		if unit.expectedStatus != nil {
			c.Assert(ok, gc.Equals, true)
			c.Assert(&unitStatus, gc.DeepEquals, unit.expectedStatus)
		} else {
			c.Assert(ok, gc.Equals, false)
		}
	}
}

func (s *statusUnitTestSuite) TestNoMeterStatusWhenNotRequired(c *gc.C) {
	service := s.Factory.MakeApplication(c, nil)

	units, err := service.AllUnits()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(units, gc.HasLen, 0)

	for i, unit := range testUnits {
		u, err := service.AddUnit(state.AddUnitParams{})
		testUnits[i].unitName = u.Name()
		c.Assert(err, jc.ErrorIsNil)
		if unit.setStatus != nil {
			err := u.SetMeterStatus(unit.setStatus.Code.String(), unit.setStatus.Info)
			c.Assert(err, jc.ErrorIsNil)
		}
	}

	client := s.APIState.Client()
	status, err := client.Status(nil)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(status, gc.NotNil)
	serviceStatus, ok := status.Applications[service.Name()]
	c.Assert(ok, gc.Equals, true)

	c.Assert(serviceStatus.MeterStatuses, gc.HasLen, 0)
}

func (s *statusUnitTestSuite) TestMeterStatusWithCredentials(c *gc.C) {
	service := s.Factory.MakeApplication(c, nil)
	c.Assert(service.SetMetricCredentials([]byte("magic-ticket")), jc.ErrorIsNil)

	units, err := service.AllUnits()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(units, gc.HasLen, 0)

	for i, unit := range testUnits {
		u, err := service.AddUnit(state.AddUnitParams{})
		testUnits[i].unitName = u.Name()
		c.Assert(err, jc.ErrorIsNil)
		if unit.setStatus != nil {
			err := u.SetMeterStatus(unit.setStatus.Code.String(), unit.setStatus.Info)
			c.Assert(err, jc.ErrorIsNil)
		}
	}

	client := s.APIState.Client()
	status, err := client.Status(nil)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(status, gc.NotNil)
	serviceStatus, ok := status.Applications[service.Name()]
	c.Assert(ok, gc.Equals, true)

	c.Assert(serviceStatus.MeterStatuses, gc.HasLen, len(testUnits)-1)
	for _, unit := range testUnits {
		unitStatus, ok := serviceStatus.MeterStatuses[unit.unitName]

		if unit.expectedStatus != nil {
			c.Assert(ok, gc.Equals, true)
			c.Assert(&unitStatus, gc.DeepEquals, unit.expectedStatus)
		} else {
			c.Assert(ok, gc.Equals, false)
		}
	}
}

func addUnitWithVersion(c *gc.C, application *state.Application, version string) *state.Unit {
	unit, err := application.AddUnit(state.AddUnitParams{})
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

func (s *statusUnitTestSuite) checkAppVersion(c *gc.C, application *state.Application, expectedVersion string) params.ApplicationStatus {
	client := s.APIState.Client()
	status, err := client.Status(nil)
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
	application := s.Factory.MakeApplication(c, nil)
	unit1 := addUnitWithVersion(c, application, "voltron")
	unit2 := addUnitWithVersion(c, application, "voltron")
	unit3 := addUnitWithVersion(c, application, "zarkon")

	appStatus := s.checkAppVersion(c, application, "zarkon")
	checkUnitVersion(c, appStatus, unit1, "voltron")
	checkUnitVersion(c, appStatus, unit2, "voltron")
	checkUnitVersion(c, appStatus, unit3, "zarkon")
}

func (s *statusUnitTestSuite) TestWorkloadVersionSimple(c *gc.C) {
	application := s.Factory.MakeApplication(c, nil)
	unit1 := addUnitWithVersion(c, application, "voltron")

	appStatus := s.checkAppVersion(c, application, "voltron")
	checkUnitVersion(c, appStatus, unit1, "voltron")
}

func (s *statusUnitTestSuite) TestWorkloadVersionBlanksCanWin(c *gc.C) {
	application := s.Factory.MakeApplication(c, nil)
	unit1 := addUnitWithVersion(c, application, "voltron")
	unit2 := addUnitWithVersion(c, application, "")

	appStatus := s.checkAppVersion(c, application, "")
	checkUnitVersion(c, appStatus, unit1, "voltron")
	checkUnitVersion(c, appStatus, unit2, "")
}

func (s *statusUnitTestSuite) TestWorkloadVersionNoUnits(c *gc.C) {
	application := s.Factory.MakeApplication(c, nil)
	s.checkAppVersion(c, application, "")
}

func (s *statusUnitTestSuite) TestWorkloadVersionOkWithUnset(c *gc.C) {
	application := s.Factory.MakeApplication(c, nil)
	unit, err := application.AddUnit(state.AddUnitParams{})
	c.Assert(err, jc.ErrorIsNil)
	appStatus := s.checkAppVersion(c, application, "")
	checkUnitVersion(c, appStatus, unit, "")
}

func (s *statusUnitTestSuite) TestMigrationInProgress(c *gc.C) {
	setGenerationsControllerConfig(c, s.State)
	// Create a host model because controller models can't be migrated.
	state2 := s.Factory.MakeModel(c, nil)
	defer state2.Close()

	model2, err := state2.Model()
	c.Assert(err, jc.ErrorIsNil)

	s.State.StartSync()
	s.WaitForModelWatchersIdle(c, model2.UUID())

	// Get API connection to hosted model.
	apiInfo := s.APIInfo(c)
	apiInfo.ModelTag = model2.ModelTag()
	// To avoid the race between the cache on the model creation,
	// make sure the cache has the model before progressing.
	s.EnsureCachedModel(c, model2.UUID())

	conn, err := api.Open(apiInfo, api.DialOpts{})
	c.Assert(err, jc.ErrorIsNil)
	client := conn.Client()

	checkMigStatus := func(expected string) {
		status, err := client.Status(nil)
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
			ControllerTag: names.NewControllerTag(utils.MustNewUUID().String()),
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
	// make application 1 with endpoint 1
	a1 := s.Factory.MakeApplication(c, &factory.ApplicationParams{
		Name: "abc",
		Charm: s.Factory.MakeCharm(c, &factory.CharmParams{
			Name: "wordpress",
		}),
	})
	e1, err := a1.Endpoint("db")
	c.Assert(err, jc.ErrorIsNil)

	// make application 2 with endpoint 2
	a2 := s.Factory.MakeApplication(c, &factory.ApplicationParams{
		Name: "def",
		Charm: s.Factory.MakeCharm(c, &factory.CharmParams{
			Name: "mysql",
		}),
	})
	e2, err := a2.Endpoint("server")
	c.Assert(err, jc.ErrorIsNil)

	// create relation between a1 and a2
	r12 := s.Factory.MakeRelation(c, &factory.RelationParams{
		Endpoints: []state.Endpoint{e1, e2},
	})
	c.Assert(r12, gc.NotNil)

	// create another application 3 with an endpoint 3
	a3 := s.Factory.MakeApplication(c, &factory.ApplicationParams{
		Charm: s.Factory.MakeCharm(c, &factory.CharmParams{Name: "logging"}),
	})
	e3, err := a3.Endpoint("info")
	c.Assert(err, jc.ErrorIsNil)

	// create endpoint 4 on application 1
	e4, err := a1.Endpoint("juju-info")
	c.Assert(err, jc.ErrorIsNil)
	r13 := s.Factory.MakeRelation(c, &factory.RelationParams{
		Endpoints: []state.Endpoint{e3, e4},
	})
	c.Assert(r13, gc.NotNil)

	// Test status filtering with application 1: should get both relations
	client := s.APIState.Client()
	status, err := client.Status([]string{a1.Name()})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(status, gc.NotNil)
	assertApplicationRelations(c, a1.Name(), 2, status.Relations)

	// test status filtering with application 3: should get 1 relation
	status, err = client.Status([]string{a3.Name()})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(status, gc.NotNil)
	assertApplicationRelations(c, a3.Name(), 1, status.Relations)
}

// TestApplicationFilterIndependentOfAlphabeticUnitOrdering ensures we
// do not regress and are carrying forward fix for lp#1592872.
func (s *statusUnitTestSuite) TestApplicationFilterIndependentOfAlphabeticUnitOrdering(c *gc.C) {
	// Application A has no touch points with application C
	// but will have a unit on the same machine is a unit of an application B.
	applicationA := s.Factory.MakeApplication(c, &factory.ApplicationParams{
		Charm: s.Factory.MakeCharm(c, &factory.CharmParams{
			Name: "mysql",
		}),
		Name: "abc",
	})

	// Application B will have a unit on the same machine as a unit of an application A
	// and will have a relation to an application C.
	applicationB := s.Factory.MakeApplication(c, &factory.ApplicationParams{
		Charm: s.Factory.MakeCharm(c, &factory.CharmParams{
			Name: "wordpress",
		}),
		Name: "def",
	})

	// Put a unit from each, application A and B, on the same machine.
	// This will be enough to ensure that the application B qualifies to be
	// in the status result filtered by the application A.
	machine := s.Factory.MakeMachine(c, &factory.MachineParams{
		Jobs: []state.MachineJob{state.JobHostUnits},
	})
	s.Factory.MakeUnit(c, &factory.UnitParams{
		Application: applicationA,
		Machine:     machine,
	})
	s.Factory.MakeUnit(c, &factory.UnitParams{
		Application: applicationB,
		Machine:     machine,
	})

	client := s.APIState.Client()
	for i := 0; i < 20; i++ {
		c.Logf("run %d", i)
		status, err := client.Status([]string{applicationA.Name()})
		c.Assert(err, jc.ErrorIsNil)
		c.Assert(status.Applications, gc.HasLen, 2)
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
	// Application A has no touch points with application C
	// but will have a unit on the same machine is a unit of an application B.
	applicationA := s.Factory.MakeApplication(c, &factory.ApplicationParams{
		Charm: s.Factory.MakeCharm(c, &factory.CharmParams{
			Name: "mysql",
		}),
	})

	// Application B will have a unit on the same machine as a unit of an application A
	// and will have a relation to an application C.
	applicationB := s.Factory.MakeApplication(c, &factory.ApplicationParams{
		Charm: s.Factory.MakeCharm(c, &factory.CharmParams{
			Name: "wordpress",
		}),
	})
	endpoint1, err := applicationB.Endpoint("juju-info")
	c.Assert(err, jc.ErrorIsNil)

	// Application C has a relation to application B but has no touch points with
	// an application A.
	applicationC := s.Factory.MakeApplication(c, &factory.ApplicationParams{
		Charm: s.Factory.MakeCharm(c, &factory.CharmParams{Name: "logging"}),
	})
	endpoint2, err := applicationC.Endpoint("info")
	c.Assert(err, jc.ErrorIsNil)
	s.Factory.MakeRelation(c, &factory.RelationParams{
		Endpoints: []state.Endpoint{endpoint2, endpoint1},
	})

	// Put a unit from each, application A and B, on the same machine.
	// This will be enough to ensure that the application B qualifies to be
	// in the status result filtered by the application A.
	machine := s.Factory.MakeMachine(c, &factory.MachineParams{
		Jobs: []state.MachineJob{state.JobHostUnits},
	})
	s.Factory.MakeUnit(c, &factory.UnitParams{
		Application: applicationA,
		Machine:     machine,
	})
	s.Factory.MakeUnit(c, &factory.UnitParams{
		Application: applicationB,
		Machine:     machine,
	})

	// Filtering status on application A should get:
	// * no relations;
	// * two applications.
	client := s.APIState.Client()
	status, err := client.Status([]string{applicationA.Name()})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(status, gc.NotNil)
	c.Assert(status.Applications, gc.HasLen, 2)
	c.Assert(status.Relations, gc.HasLen, 0)
}

func (s *statusUnitTestSuite) TestMachineWithNoDisplayNameHasItsEmptyDisplayNameSent(c *gc.C) {
	machine := s.Factory.MakeMachine(c, &factory.MachineParams{
		InstanceId: instance.Id("i-123"),
	})

	client := s.APIState.Client()
	status, err := client.Status(nil)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(status.Machines, gc.HasLen, 1)
	c.Assert(status.Machines[machine.Id()].DisplayName, gc.Equals, "")
}

func (s *statusUnitTestSuite) TestMachineWithDisplayNameHasItsDisplayNameSent(c *gc.C) {
	machine := s.Factory.MakeMachine(c, &factory.MachineParams{
		InstanceId:  instance.Id("i-123"),
		DisplayName: "snowflake",
	})

	client := s.APIState.Client()
	status, err := client.Status(nil)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(status.Machines, gc.HasLen, 1)
	c.Assert(status.Machines[machine.Id()].DisplayName, gc.Equals, "snowflake")
}

func assertApplicationRelations(c *gc.C, appName string, expectedNumber int, relations []params.RelationStatus) {
	c.Assert(relations, gc.HasLen, expectedNumber)
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

type statusUpgradeUnitSuite struct {
	testing.CharmSuite
	jujutesting.JujuConnSuite

	charmrevisionupdater *charmrevisionupdater.CharmRevisionUpdaterAPI
	resources            *common.Resources
	authoriser           apiservertesting.FakeAuthorizer
}

var _ = gc.Suite(&statusUpgradeUnitSuite{})

func (s *statusUpgradeUnitSuite) SetUpSuite(c *gc.C) {
	s.JujuConnSuite.SetUpSuite(c)
	s.CharmSuite.SetUpSuite(c, &s.JujuConnSuite)
}

func (s *statusUpgradeUnitSuite) SetUpTest(c *gc.C) {
	s.JujuConnSuite.SetUpTest(c)
	s.CharmSuite.SetUpTest(c)
	s.resources = common.NewResources()
	s.AddCleanup(func(_ *gc.C) { s.resources.StopAll() })
	s.authoriser = apiservertesting.FakeAuthorizer{
		Controller: true,
	}
	var err error
	s.charmrevisionupdater, err = charmrevisionupdater.NewCharmRevisionUpdaterAPI(s.State, s.resources, s.authoriser)
	c.Assert(err, jc.ErrorIsNil)
}

func (s *statusUpgradeUnitSuite) TestUpdateRevisions(c *gc.C) {
	s.AddMachine(c, "0", state.JobManageModel)
	s.SetupScenario(c)
	client := s.APIState.Client()
	status, _ := client.Status(nil)

	serviceStatus, ok := status.Applications["mysql"]
	c.Assert(ok, gc.Equals, true)
	c.Assert(serviceStatus.CanUpgradeTo, gc.Equals, "")

	// Update to the latest available charm revision.
	result, err := s.charmrevisionupdater.UpdateLatestRevisions()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(result.Error, gc.IsNil)

	// Check if CanUpgradeTo suggest the latest revision.
	status, _ = client.Status(nil)
	serviceStatus, ok = status.Applications["mysql"]
	c.Assert(ok, gc.Equals, true)
	c.Assert(serviceStatus.CanUpgradeTo, gc.Equals, "cs:quantal/mysql-23")
}

type CAASStatusSuite struct {
	baseSuite

	app *state.Application
}

var _ = gc.Suite(&CAASStatusSuite{})

func (s *CAASStatusSuite) SetUpTest(c *gc.C) {
	s.baseSuite.SetUpTest(c)

	// Set up a CAAS model to replace the IAAS one.
	st := s.Factory.MakeCAASModel(c, nil)
	s.CleanupSuite.AddCleanup(func(*gc.C) { st.Close() })
	s.State = st
	s.Factory = factory.NewFactory(s.State, nil)
	m, err := st.Model()
	c.Assert(err, jc.ErrorIsNil)
	s.Model = m

	hp, err := st.APIHostPortsForClients()
	c.Assert(err, jc.ErrorIsNil)
	var addrs []network.SpaceAddress
	for _, server := range hp {
		for _, nhp := range server {
			addrs = append(addrs, nhp.SpaceAddress)
		}
	}

	apiAddrs := network.SpaceAddressesWithPort(addrs, s.ControllerConfig.APIPort()).HostPorts().Strings()
	modelTag := names.NewModelTag(st.ModelUUID())
	apiInfo := &api.Info{Addrs: apiAddrs, CACert: coretesting.CACert, ModelTag: modelTag}
	apiInfo.Tag = s.AdminUserTag(c)
	apiInfo.Password = jujutesting.AdminSecret
	s.APIState, err = api.Open(apiInfo, api.DialOpts{})
	c.Assert(err, jc.ErrorIsNil)

	ch := s.Factory.MakeCharm(c, &factory.CharmParams{
		Series: "kubernetes",
	})
	s.app = s.Factory.MakeApplication(c, &factory.ApplicationParams{
		Charm: ch,
	})
	s.Factory.MakeUnit(c, &factory.UnitParams{Application: s.app})
}

func (s *CAASStatusSuite) TestStatusOperatorNotReady(c *gc.C) {
	client := s.APIState.Client()

	status, err := client.Status(nil)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(status.Applications, gc.HasLen, 1)
	clearSinceTimes(status)
	s.assertUnitStatus(c, status.Applications[s.app.Name()], "waiting", "agent initializing")
}

func (s *CAASStatusSuite) TestStatusPodSpecNotSet(c *gc.C) {
	client := s.APIState.Client()
	err := s.app.SetOperatorStatus(status.StatusInfo{Status: status.Active})
	c.Assert(err, jc.ErrorIsNil)

	status, err := client.Status(nil)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(status.Applications, gc.HasLen, 1)
	clearSinceTimes(status)
	s.assertUnitStatus(c, status.Applications[s.app.Name()], "waiting", "agent initializing")
}

func (s *CAASStatusSuite) TestStatusPodSpecSet(c *gc.C) {
	client := s.APIState.Client()
	err := s.app.SetOperatorStatus(status.StatusInfo{Status: status.Active})
	c.Assert(err, jc.ErrorIsNil)
	cm, err := s.Model.CAASModel()
	c.Assert(err, jc.ErrorIsNil)

	spec := `
containers:
  - name: gitlab
    image: gitlab/latest
`[1:]
	err = cm.SetPodSpec(nil, s.app.ApplicationTag(), &spec)
	c.Assert(err, jc.ErrorIsNil)

	status, err := client.Status(nil)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(status.Applications, gc.HasLen, 1)
	clearSinceTimes(status)
	s.assertUnitStatus(c, status.Applications[s.app.Name()], "waiting", "waiting for container")
}

func (s *CAASStatusSuite) TestStatusCloudContainerSet(c *gc.C) {
	client := s.APIState.Client()
	err := s.app.SetOperatorStatus(status.StatusInfo{Status: status.Active})
	c.Assert(err, jc.ErrorIsNil)

	u, err := s.app.AllUnits()
	c.Assert(err, jc.ErrorIsNil)
	var updateUnits state.UpdateUnitsOperation
	updateUnits.Updates = []*state.UpdateUnitOperation{
		u[0].UpdateOperation(state.UnitUpdateProperties{
			CloudContainerStatus: &status.StatusInfo{Status: status.Blocked, Message: "blocked"},
		})}
	err = s.app.UpdateUnits(&updateUnits)
	c.Assert(err, jc.ErrorIsNil)

	status, err := client.Status(nil)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(status.Applications, gc.HasLen, 1)
	clearSinceTimes(status)
	s.assertUnitStatus(c, status.Applications[s.app.Name()], "blocked", "blocked")
}

func (s *CAASStatusSuite) assertUnitStatus(c *gc.C, appStatus params.ApplicationStatus, status, info string) {
	curl, _ := s.app.CharmURL()
	workloadVersion := ""
	if info != "agent initializing" && info != "blocked" {
		workloadVersion = "gitlab/latest"
	}
	c.Assert(appStatus, jc.DeepEquals, params.ApplicationStatus{
		Charm:           curl.String(),
		Series:          "kubernetes",
		WorkloadVersion: workloadVersion,
		Relations:       map[string][]string{},
		SubordinateTo:   []string{},
		Units: map[string]params.UnitStatus{
			s.app.Name() + "/0": {
				AgentStatus: params.DetailedStatus{
					Status: "allocating",
				},
				WorkloadStatus: params.DetailedStatus{
					Status: status,
					Info:   info,
				},
			},
		},
		Status: params.DetailedStatus{
			Status: status,
			Info:   info,
		},
		EndpointBindings: map[string]string{
			"":             network.AlphaSpaceName,
			"server":       network.AlphaSpaceName,
			"server-admin": network.AlphaSpaceName,
		},
	})
}

type filteringBranchesSuite struct {
	baseSuite

	appA string
	appB string
	subB string
}

var _ = gc.Suite(&filteringBranchesSuite{})

func (s *filteringBranchesSuite) SetUpTest(c *gc.C) {
	s.baseSuite.SetUpTest(c)
	setGenerationsControllerConfig(c, s.State)

	s.appA = "mysql"
	s.appB = "wordpress"
	s.subB = "logging"

	// Application A has no touch points with application C
	// but will have a unit on the same machine is a unit of an application B.
	applicationA := s.Factory.MakeApplication(c, &factory.ApplicationParams{
		Charm: s.Factory.MakeCharm(c, &factory.CharmParams{
			Name: s.appA,
		}),
	})

	// Application B will have a unit on the same machine as a unit of an application A
	// and will have a relation to an application C.
	applicationB := s.Factory.MakeApplication(c, &factory.ApplicationParams{
		Charm: s.Factory.MakeCharm(c, &factory.CharmParams{
			Name: s.appB,
		}),
	})
	endpoint1, err := applicationB.Endpoint("juju-info")
	c.Assert(err, jc.ErrorIsNil)

	s.Factory.MakeUnit(c, &factory.UnitParams{
		Application: applicationA,
	})
	appBUnit := s.Factory.MakeUnit(c, &factory.UnitParams{
		Application: applicationB,
	})

	// Application C has a relation to application B but has no touch points with
	// an application A.
	applicationC := s.Factory.MakeApplication(c, &factory.ApplicationParams{
		Charm: s.Factory.MakeCharm(c, &factory.CharmParams{Name: s.subB}),
	})
	endpoint2, err := applicationC.Endpoint("info")
	c.Assert(err, jc.ErrorIsNil)
	rel := s.Factory.MakeRelation(c, &factory.RelationParams{
		Endpoints: []state.Endpoint{endpoint2, endpoint1},
	})
	// Trigger the creation of the subordinate unit by entering scope
	// on the principal unit.
	ru, err := rel.Unit(appBUnit)
	c.Assert(err, jc.ErrorIsNil)
	err = ru.EnterScope(nil)
	c.Assert(err, jc.ErrorIsNil)
	s.WaitForModelWatchersIdle(c, s.State.ModelUUID())
}

func (s *filteringBranchesSuite) TestFullStatusBranchNoFilter(c *gc.C) {
	err := s.State.AddBranch("apple", "test-user")
	c.Assert(err, jc.ErrorIsNil)

	client := s.clientForTest(c)

	status, err := client.FullStatus(params.StatusParams{})
	c.Assert(err, jc.ErrorIsNil)
	c.Logf("%#v", status.Branches)
	b, ok := status.Branches["apple"]
	c.Assert(ok, jc.IsTrue)
	c.Assert(b.AssignedUnits, jc.DeepEquals, map[string][]string{})
	c.Assert(status.Applications, gc.HasLen, 3)
}

func (s *filteringBranchesSuite) TestFullStatusBranchFilterUnit(c *gc.C) {
	s.assertBranchAssignUnit(c, "apple", s.appA+"/0")
	err := s.State.AddBranch("banana", "test-user")
	c.Assert(err, jc.ErrorIsNil)

	client := s.clientForTest(c)

	status, err := client.FullStatus(params.StatusParams{
		Patterns: []string{s.appA + "/0"},
	})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(status.Branches, gc.HasLen, 1)
	b, ok := status.Branches["apple"]
	c.Assert(ok, jc.IsTrue)
	c.Assert(b.AssignedUnits, jc.DeepEquals, map[string][]string{s.appA: {s.appA + "/0"}})
	c.Assert(status.Applications, gc.HasLen, 1)
}

func (s *filteringBranchesSuite) TestFullStatusBranchFilterApplication(c *gc.C) {
	err := s.State.AddBranch("apple", "test-user")
	c.Assert(err, jc.ErrorIsNil)
	s.assertBranchAssignApplication(c, "banana", s.appB)

	client := s.clientForTest(c)

	status, err := client.FullStatus(params.StatusParams{
		Patterns: []string{s.appB},
	})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(status.Branches, gc.HasLen, 1)
	b, ok := status.Branches["banana"]
	c.Assert(ok, jc.IsTrue)
	c.Assert(b.AssignedUnits, jc.DeepEquals, map[string][]string{s.appB: {}})
	c.Assert(status.Applications, gc.HasLen, 2)
}

func (s *filteringBranchesSuite) TestFullStatusBranchFilterSubordinateUnit(c *gc.C) {
	s.assertBranchAssignUnit(c, "apple", s.subB+"/0")
	s.assertBranchAssignUnit(c, "banana", s.appA+"/0")
	err := s.State.AddBranch("cucumber", "test-user")
	c.Assert(err, jc.ErrorIsNil)

	client := s.clientForTest(c)

	status, err := client.FullStatus(params.StatusParams{
		Patterns: []string{s.subB + "/0"},
	})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(status.Branches, gc.HasLen, 1)
	b, ok := status.Branches["apple"]
	c.Assert(ok, jc.IsTrue)
	c.Assert(b.AssignedUnits, jc.DeepEquals, map[string][]string{s.subB: {s.subB + "/0"}})
	c.Assert(status.Applications, gc.HasLen, 2)

}

func (s *filteringBranchesSuite) TestFullStatusBranchFilterTwoBranchesSubordinateUnit(c *gc.C) {
	s.assertBranchAssignUnit(c, "apple", s.subB+"/0")
	s.assertBranchAssignUnit(c, "banana", s.appA+"/0")
	s.assertBranchAssignUnit(c, "cucumber", s.appB+"/0")

	client := s.clientForTest(c)

	status, err := client.FullStatus(params.StatusParams{
		Patterns: []string{s.appB + "/0"},
	})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(status.Branches, gc.HasLen, 2)
	b, ok := status.Branches["apple"]
	c.Assert(ok, jc.IsTrue)
	c.Assert(b.AssignedUnits, jc.DeepEquals, map[string][]string{s.subB: {s.subB + "/0"}})
	b, ok = status.Branches["cucumber"]
	c.Assert(ok, jc.IsTrue)
	c.Assert(b.AssignedUnits, jc.DeepEquals, map[string][]string{s.appB: {s.appB + "/0"}})
	c.Assert(status.Applications, gc.HasLen, 2)
}

func (s *filteringBranchesSuite) clientForTest(c *gc.C) *client.Client {
	s.State.StartSync()
	s.WaitForModelWatchersIdle(c, s.State.ModelUUID())

	ctx := &facadetest.Context{
		Controller_: s.Controller,
		State_:      s.State,
		StatePool_:  s.StatePool,
		Auth_: apiservertesting.FakeAuthorizer{
			Tag:        s.AdminUserTag(c),
			Controller: true,
		},
		Resources_:        common.NewResources(),
		LeadershipReader_: mockLeadershipReader{},
	}
	client, err := client.NewFacade(ctx)
	c.Assert(err, jc.ErrorIsNil)
	return client
}

func (s *filteringBranchesSuite) assertBranchAssignUnit(c *gc.C, bName, uName string) {
	err := s.State.AddBranch(bName, "test-user")
	c.Assert(err, jc.ErrorIsNil)
	gen, err := s.State.Branch(bName)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(gen, gc.NotNil)
	err = gen.AssignUnit(uName)
	c.Assert(err, jc.ErrorIsNil)
}

func (s *filteringBranchesSuite) assertBranchAssignApplication(c *gc.C, bName, aName string) {
	err := s.State.AddBranch(bName, "test-user")
	c.Assert(err, jc.ErrorIsNil)
	gen, err := s.State.Branch(bName)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(gen, gc.NotNil)
	err = gen.AssignApplication(aName)
	c.Assert(err, jc.ErrorIsNil)
}

type mockLeadershipReader struct{}

func (m mockLeadershipReader) Leaders() (map[string]string, error) {
	return make(map[string]string), nil
}

func setGenerationsControllerConfig(c *gc.C, st *state.State) {
	err := st.UpdateControllerConfig(map[string]interface{}{
		"features": []interface{}{feature.Branches},
	}, nil)
	c.Assert(err, jc.ErrorIsNil)
}
