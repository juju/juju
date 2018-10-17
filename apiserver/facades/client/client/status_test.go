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
	"github.com/juju/juju/apiserver/common"
	"github.com/juju/juju/apiserver/facades/controller/charmrevisionupdater"
	"github.com/juju/juju/apiserver/facades/controller/charmrevisionupdater/testing"
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

	// Create a host model because controller models can't be migrated.
	state2 := s.Factory.MakeModel(c, nil)
	defer state2.Close()

	model2, err := state2.Model()
	c.Assert(err, jc.ErrorIsNil)

	// Get API connection to hosted model.
	apiInfo := s.APIInfo(c)
	apiInfo.ModelTag = model2.ModelTag()
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
		Controller: true,
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
