// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package client_test

import (
	"time"

	jc "github.com/juju/testing/checkers"
	"github.com/juju/utils"
	gc "gopkg.in/check.v1"
	"gopkg.in/juju/names.v2"

	"github.com/juju/juju/api"
	"github.com/juju/juju/apiserver/charmrevisionupdater"
	"github.com/juju/juju/apiserver/charmrevisionupdater/testing"
	"github.com/juju/juju/apiserver/client"
	"github.com/juju/juju/apiserver/common"
	"github.com/juju/juju/apiserver/params"
	apiservertesting "github.com/juju/juju/apiserver/testing"
	"github.com/juju/juju/core/migration"
	"github.com/juju/juju/instance"
	jujutesting "github.com/juju/juju/juju/testing"
	"github.com/juju/juju/state"
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
	client := s.APIState.Client()
	status, err := client.Status(nil)
	c.Assert(err, jc.ErrorIsNil)
	c.Check(status.Model.Name, gc.Equals, "controller")
	c.Check(status.Model.CloudTag, gc.Equals, "cloud-dummy")
	c.Check(status.Applications, gc.HasLen, 0)
	c.Check(status.Machines, gc.HasLen, 1)
	resultMachine, ok := status.Machines[machine.Id()]
	if !ok {
		c.Fatalf("Missing machine with id %q", machine.Id())
	}
	c.Check(resultMachine.Id, gc.Equals, machine.Id())
	c.Check(resultMachine.Series, gc.Equals, machine.Series())
}

var _ = gc.Suite(&statusUnitTestSuite{})

type statusUnitTestSuite struct {
	baseSuite
}

func (s *statusUnitTestSuite) TestProcessMachinesWithOneMachineAndOneContainer(c *gc.C) {
	host := s.Factory.MakeMachine(c, &factory.MachineParams{InstanceId: instance.Id("0")})
	container := s.Factory.MakeMachineNested(c, host.Id(), nil)
	machines := map[string][]*state.Machine{
		host.Id(): {host, container},
	}

	statuses := client.ProcessMachines(machines)
	c.Assert(statuses, gc.Not(gc.IsNil))

	containerStatus := client.MakeMachineStatus(container)
	c.Check(statuses[host.Id()].Containers[container.Id()].Id, gc.Equals, containerStatus.Id)
}

func (s *statusUnitTestSuite) TestProcessMachinesWithEmbeddedContainers(c *gc.C) {
	host := s.Factory.MakeMachine(c, &factory.MachineParams{InstanceId: instance.Id("1")})
	lxdHost := s.Factory.MakeMachineNested(c, host.Id(), nil)
	machines := map[string][]*state.Machine{
		host.Id(): {
			host,
			lxdHost,
			s.Factory.MakeMachineNested(c, lxdHost.Id(), nil),
			s.Factory.MakeMachineNested(c, host.Id(), nil),
		},
	}

	statuses := client.ProcessMachines(machines)
	c.Assert(statuses, gc.Not(gc.IsNil))

	hostContainer := statuses[host.Id()].Containers
	c.Check(hostContainer, gc.HasLen, 2)
	c.Check(hostContainer[lxdHost.Id()].Containers, gc.HasLen, 1)
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

func (s *statusUnitTestSuite) TestMeterStatus(c *gc.C) {
	meteredCharm := s.Factory.MakeCharm(c, &factory.CharmParams{Name: "metered", URL: "cs:quantal/metered"})
	service := s.Factory.MakeApplication(c, &factory.ApplicationParams{Charm: meteredCharm})

	units, err := service.AllUnits()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(units, gc.HasLen, 0)

	for i, unit := range testUnits {
		u, err := service.AddUnit()
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
		u, err := service.AddUnit()
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
		u, err := service.AddUnit()
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
	unit, err := application.AddUnit()
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
	unit, err := application.AddUnit()
	c.Assert(err, jc.ErrorIsNil)
	appStatus := s.checkAppVersion(c, application, "")
	checkUnitVersion(c, appStatus, unit, "")
}

func (s *statusUnitTestSuite) TestMigrationInProgress(c *gc.C) {

	// Create a host model because controller models can't be migrated.
	state2 := s.Factory.MakeModel(c, nil)
	defer state2.Close()

	// Get API connection to hosted model.
	apiInfo := s.APIInfo(c)
	apiInfo.ModelTag = state2.ModelTag()
	conn, err := api.Open(apiInfo, api.DialOpts{})
	c.Assert(err, jc.ErrorIsNil)
	client := conn.Client()

	checkMigStatus := func(expected string) {
		status, err := client.Status(nil)
		c.Assert(err, jc.ErrorIsNil)
		c.Check(status.Model.Migration, gc.Equals, expected)
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

func (s *statusUpgradeUnitSuite) TearDownSuite(c *gc.C) {
	s.CharmSuite.TearDownSuite(c)
	s.JujuConnSuite.TearDownSuite(c)
}

func (s *statusUpgradeUnitSuite) SetUpTest(c *gc.C) {
	s.JujuConnSuite.SetUpTest(c)
	s.CharmSuite.SetUpTest(c)
	s.resources = common.NewResources()
	s.AddCleanup(func(_ *gc.C) { s.resources.StopAll() })
	s.authoriser = apiservertesting.FakeAuthorizer{
		EnvironManager: true,
	}
	var err error
	s.charmrevisionupdater, err = charmrevisionupdater.NewCharmRevisionUpdaterAPI(s.State, s.resources, s.authoriser)
	c.Assert(err, jc.ErrorIsNil)
}

func (s *statusUpgradeUnitSuite) TearDownTest(c *gc.C) {
	s.CharmSuite.TearDownTest(c)
	s.JujuConnSuite.TearDownTest(c)
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
