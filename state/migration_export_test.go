// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state_test

import (
	"math/rand"
	"time"

	jc "github.com/juju/testing/checkers"
	"github.com/juju/version"
	gc "gopkg.in/check.v1"
	"gopkg.in/juju/names.v2"

	"github.com/juju/juju/constraints"
	"github.com/juju/juju/controller"
	"github.com/juju/juju/core/description"
	"github.com/juju/juju/state"
	"github.com/juju/juju/status"
	"github.com/juju/juju/testing/factory"
)

// Constraints stores megabytes by default for memory and root disk.
const (
	gig uint64 = 1024

	addedHistoryCount = 5
	// 6 for the one initial + 5 added.
	expectedHistoryCount = addedHistoryCount + 1
)

var testAnnotations = map[string]string{
	"string":  "value",
	"another": "one",
}

type MigrationSuite struct {
	ConnSuite
}

func (s *MigrationSuite) setLatestTools(c *gc.C, latestTools version.Number) {
	dbModel, err := s.State.Model()
	c.Assert(err, jc.ErrorIsNil)
	err = dbModel.UpdateLatestToolsVersion(latestTools)
	c.Assert(err, jc.ErrorIsNil)
}

func (s *MigrationSuite) setRandSequenceValue(c *gc.C, name string) int {
	var value int
	var err error
	count := rand.Intn(5) + 1
	for i := 0; i < count; i++ {
		value, err = state.Sequence(s.State, name)
		c.Assert(err, jc.ErrorIsNil)
	}
	// The value stored in the doc is one higher than what it returns.
	return value + 1
}

func (s *MigrationSuite) primeStatusHistory(c *gc.C, entity statusSetter, statusVal status.Status, count int) {
	primeStatusHistory(c, entity, statusVal, count, func(i int) map[string]interface{} {
		return map[string]interface{}{"index": count - i}
	}, 0)
}

func (s *MigrationSuite) makeApplicationWithLeader(c *gc.C, applicationname string, count int, leader int) {
	c.Assert(leader < count, jc.IsTrue)
	units := make([]*state.Unit, count)
	application := s.Factory.MakeApplication(c, &factory.ApplicationParams{
		Name: applicationname,
		Charm: s.Factory.MakeCharm(c, &factory.CharmParams{
			Name: applicationname,
		}),
	})
	for i := 0; i < count; i++ {
		units[i] = s.Factory.MakeUnit(c, &factory.UnitParams{
			Application: application,
		})
	}
	err := s.State.LeadershipClaimer().ClaimLeadership(
		application.Name(),
		units[leader].Name(),
		time.Minute)
	c.Assert(err, jc.ErrorIsNil)
}

type MigrationExportSuite struct {
	MigrationSuite
}

var _ = gc.Suite(&MigrationExportSuite{})

func (s *MigrationExportSuite) checkStatusHistory(c *gc.C, history []description.Status, statusVal status.Status) {
	for i, st := range history {
		c.Logf("status history #%d: %s", i, st.Updated())
		c.Check(st.Value(), gc.Equals, string(statusVal))
		c.Check(st.Message(), gc.Equals, "")
		c.Check(st.Data(), jc.DeepEquals, map[string]interface{}{"index": i + 1})
	}
}

func (s *MigrationExportSuite) TestModelInfo(c *gc.C) {
	stModel, err := s.State.Model()
	c.Assert(err, jc.ErrorIsNil)
	err = s.State.SetAnnotations(stModel, testAnnotations)
	c.Assert(err, jc.ErrorIsNil)
	latestTools := version.MustParse("2.0.1")
	s.setLatestTools(c, latestTools)
	err = s.State.SetModelConstraints(constraints.MustParse("arch=amd64 mem=8G"))
	c.Assert(err, jc.ErrorIsNil)
	machineSeq := s.setRandSequenceValue(c, "machine")
	fooSeq := s.setRandSequenceValue(c, "application-foo")
	s.State.SwitchBlockOn(state.ChangeBlock, "locked down")
	settings, err := state.ReadSettings(s.State, state.ControllersC, state.DefaultModelSettingsGlobalKey)
	c.Assert(err, jc.ErrorIsNil)
	settings.Set("apt-mirror", "http://mirror")
	_, err = settings.Write()
	c.Assert(err, jc.ErrorIsNil)

	model, err := s.State.Export()
	c.Assert(err, jc.ErrorIsNil)

	dbModel, err := s.State.Model()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(model.Tag(), gc.Equals, dbModel.ModelTag())
	c.Assert(model.Owner(), gc.Equals, dbModel.Owner())
	dbModelCfg, err := dbModel.Config()
	c.Assert(err, jc.ErrorIsNil)
	modelAttrs := dbModelCfg.AllAttrs()
	c.Assert(modelAttrs["apt-mirror"], gc.Equals, "http://mirror")

	// Remove all controller and cloud config before comparison.
	for _, attr := range controller.ControllerOnlyConfigAttributes {
		delete(modelAttrs, attr)
	}
	delete(modelAttrs, "apt-mirror")
	c.Assert(model.Config(), jc.DeepEquals, modelAttrs)
	c.Assert(model.LatestToolsVersion(), gc.Equals, latestTools)
	c.Assert(model.Annotations(), jc.DeepEquals, testAnnotations)
	constraints := model.Constraints()
	c.Assert(constraints, gc.NotNil)
	c.Assert(constraints.Architecture(), gc.Equals, "amd64")
	c.Assert(constraints.Memory(), gc.Equals, 8*gig)
	c.Assert(model.Sequences(), jc.DeepEquals, map[string]int{
		"machine":         machineSeq,
		"application-foo": fooSeq,
		// blocks is added by the switch block on call above.
		"block": 1,
	})
	c.Assert(model.Blocks(), jc.DeepEquals, map[string]string{
		"all-changes": "locked down",
	})
}

func (s *MigrationExportSuite) TestModelUsers(c *gc.C) {
	// Make sure we have some last connection times for the admin user,
	// and create a few other users.
	lastConnection := state.NowToTheSecond()
	owner, err := s.State.ModelUser(s.Owner)
	c.Assert(err, jc.ErrorIsNil)
	err = state.UpdateModelUserLastConnection(owner, lastConnection)
	c.Assert(err, jc.ErrorIsNil)

	bobTag := names.NewUserTag("bob@external")
	bob, err := s.State.AddModelUser(state.ModelUserSpec{
		User:      bobTag,
		CreatedBy: s.Owner,
		Access:    state.ModelReadAccess,
	})
	c.Assert(err, jc.ErrorIsNil)
	err = state.UpdateModelUserLastConnection(bob, lastConnection)
	c.Assert(err, jc.ErrorIsNil)

	model, err := s.State.Export()
	c.Assert(err, jc.ErrorIsNil)

	users := model.Users()
	c.Assert(users, gc.HasLen, 2)

	exportedBob := users[0]
	// admin is "test-admin", and results are sorted
	exportedAdmin := users[1]

	c.Assert(exportedAdmin.Name(), gc.Equals, s.Owner)
	c.Assert(exportedAdmin.DisplayName(), gc.Equals, owner.DisplayName())
	c.Assert(exportedAdmin.CreatedBy(), gc.Equals, s.Owner)
	c.Assert(exportedAdmin.DateCreated(), gc.Equals, owner.DateCreated())
	c.Assert(exportedAdmin.LastConnection(), gc.Equals, lastConnection)
	c.Assert(exportedAdmin.ReadOnly(), jc.IsFalse)

	c.Assert(exportedBob.Name(), gc.Equals, bobTag)
	c.Assert(exportedBob.DisplayName(), gc.Equals, "")
	c.Assert(exportedBob.CreatedBy(), gc.Equals, s.Owner)
	c.Assert(exportedBob.DateCreated(), gc.Equals, bob.DateCreated())
	c.Assert(exportedBob.LastConnection(), gc.Equals, lastConnection)
	c.Assert(exportedBob.ReadOnly(), jc.IsTrue)
}

func (s *MigrationExportSuite) TestMachines(c *gc.C) {
	// Add a machine with an LXC container.
	machine1 := s.Factory.MakeMachine(c, &factory.MachineParams{
		Constraints: constraints.MustParse("arch=amd64 mem=8G"),
	})
	nested := s.Factory.MakeMachineNested(c, machine1.Id(), nil)
	err := s.State.SetAnnotations(machine1, testAnnotations)
	c.Assert(err, jc.ErrorIsNil)
	s.primeStatusHistory(c, machine1, status.StatusStarted, addedHistoryCount)

	model, err := s.State.Export()
	c.Assert(err, jc.ErrorIsNil)

	machines := model.Machines()
	c.Assert(machines, gc.HasLen, 1)

	exported := machines[0]
	c.Assert(exported.Tag(), gc.Equals, machine1.MachineTag())
	c.Assert(exported.Series(), gc.Equals, machine1.Series())
	c.Assert(exported.Annotations(), jc.DeepEquals, testAnnotations)
	constraints := exported.Constraints()
	c.Assert(constraints, gc.NotNil)
	c.Assert(constraints.Architecture(), gc.Equals, "amd64")
	c.Assert(constraints.Memory(), gc.Equals, 8*gig)

	tools, err := machine1.AgentTools()
	c.Assert(err, jc.ErrorIsNil)
	exTools := exported.Tools()
	c.Assert(exTools, gc.NotNil)
	c.Assert(exTools.Version(), jc.DeepEquals, tools.Version)

	history := exported.StatusHistory()
	c.Assert(history, gc.HasLen, expectedHistoryCount)
	s.checkStatusHistory(c, history[:addedHistoryCount], status.StatusStarted)

	containers := exported.Containers()
	c.Assert(containers, gc.HasLen, 1)
	container := containers[0]
	c.Assert(container.Tag(), gc.Equals, nested.MachineTag())
}

func (s *MigrationExportSuite) TestServices(c *gc.C) {
	application := s.Factory.MakeApplication(c, &factory.ApplicationParams{
		Settings: map[string]interface{}{
			"foo": "bar",
		},
		Constraints: constraints.MustParse("arch=amd64 mem=8G"),
	})
	err := application.UpdateLeaderSettings(&goodToken{}, map[string]string{
		"leader": "true",
	})
	c.Assert(err, jc.ErrorIsNil)
	err = application.SetMetricCredentials([]byte("sekrit"))
	c.Assert(err, jc.ErrorIsNil)
	err = s.State.SetAnnotations(application, testAnnotations)
	c.Assert(err, jc.ErrorIsNil)
	s.primeStatusHistory(c, application, status.StatusActive, addedHistoryCount)

	model, err := s.State.Export()
	c.Assert(err, jc.ErrorIsNil)

	applications := model.Applications()
	c.Assert(applications, gc.HasLen, 1)

	exported := applications[0]
	c.Assert(exported.Name(), gc.Equals, application.Name())
	c.Assert(exported.Tag(), gc.Equals, application.ApplicationTag())
	c.Assert(exported.Series(), gc.Equals, application.Series())
	c.Assert(exported.Annotations(), jc.DeepEquals, testAnnotations)

	c.Assert(exported.Settings(), jc.DeepEquals, map[string]interface{}{
		"foo": "bar",
	})
	c.Assert(exported.SettingsRefCount(), gc.Equals, 1)
	c.Assert(exported.LeadershipSettings(), jc.DeepEquals, map[string]interface{}{
		"leader": "true",
	})
	c.Assert(exported.MetricsCredentials(), jc.DeepEquals, []byte("sekrit"))

	constraints := exported.Constraints()
	c.Assert(constraints, gc.NotNil)
	c.Assert(constraints.Architecture(), gc.Equals, "amd64")
	c.Assert(constraints.Memory(), gc.Equals, 8*gig)

	history := exported.StatusHistory()
	c.Assert(history, gc.HasLen, expectedHistoryCount)
	s.checkStatusHistory(c, history[:addedHistoryCount], status.StatusActive)
}

func (s *MigrationExportSuite) TestMultipleServices(c *gc.C) {
	s.Factory.MakeApplication(c, &factory.ApplicationParams{Name: "first"})
	s.Factory.MakeApplication(c, &factory.ApplicationParams{Name: "second"})
	s.Factory.MakeApplication(c, &factory.ApplicationParams{Name: "third"})

	model, err := s.State.Export()
	c.Assert(err, jc.ErrorIsNil)

	applications := model.Applications()
	c.Assert(applications, gc.HasLen, 3)
}

func (s *MigrationExportSuite) TestUnits(c *gc.C) {
	unit := s.Factory.MakeUnit(c, &factory.UnitParams{
		Constraints: constraints.MustParse("arch=amd64 mem=8G"),
	})
	err := unit.SetMeterStatus("GREEN", "some info")
	c.Assert(err, jc.ErrorIsNil)
	err = s.State.SetAnnotations(unit, testAnnotations)
	c.Assert(err, jc.ErrorIsNil)
	s.primeStatusHistory(c, unit, status.StatusActive, addedHistoryCount)
	s.primeStatusHistory(c, unit.Agent(), status.StatusIdle, addedHistoryCount)

	model, err := s.State.Export()
	c.Assert(err, jc.ErrorIsNil)

	applications := model.Applications()
	c.Assert(applications, gc.HasLen, 1)

	application := applications[0]
	units := application.Units()
	c.Assert(units, gc.HasLen, 1)

	exported := units[0]

	c.Assert(exported.Name(), gc.Equals, unit.Name())
	c.Assert(exported.Tag(), gc.Equals, unit.UnitTag())
	c.Assert(exported.Validate(), jc.ErrorIsNil)
	c.Assert(exported.MeterStatusCode(), gc.Equals, "GREEN")
	c.Assert(exported.MeterStatusInfo(), gc.Equals, "some info")
	c.Assert(exported.Annotations(), jc.DeepEquals, testAnnotations)
	constraints := exported.Constraints()
	c.Assert(constraints, gc.NotNil)
	c.Assert(constraints.Architecture(), gc.Equals, "amd64")
	c.Assert(constraints.Memory(), gc.Equals, 8*gig)

	workloadHistory := exported.WorkloadStatusHistory()
	c.Assert(workloadHistory, gc.HasLen, expectedHistoryCount)
	s.checkStatusHistory(c, workloadHistory[:addedHistoryCount], status.StatusActive)

	agentHistory := exported.AgentStatusHistory()
	c.Assert(agentHistory, gc.HasLen, expectedHistoryCount)
	s.checkStatusHistory(c, agentHistory[:addedHistoryCount], status.StatusIdle)
}

func (s *MigrationExportSuite) TestServiceLeadership(c *gc.C) {
	s.makeApplicationWithLeader(c, "mysql", 2, 1)
	s.makeApplicationWithLeader(c, "wordpress", 4, 2)

	model, err := s.State.Export()
	c.Assert(err, jc.ErrorIsNil)

	leaders := make(map[string]string)
	for _, application := range model.Applications() {
		leaders[application.Name()] = application.Leader()
	}
	c.Assert(leaders, jc.DeepEquals, map[string]string{
		"mysql":     "mysql/1",
		"wordpress": "wordpress/2",
	})
}

func (s *MigrationExportSuite) TestUnitsOpenPorts(c *gc.C) {
	unit := s.Factory.MakeUnit(c, nil)
	err := unit.OpenPorts("tcp", 1234, 2345)
	c.Assert(err, jc.ErrorIsNil)

	model, err := s.State.Export()
	c.Assert(err, jc.ErrorIsNil)

	machines := model.Machines()
	c.Assert(machines, gc.HasLen, 1)

	ports := machines[0].OpenedPorts()
	c.Assert(ports, gc.HasLen, 1)

	port := ports[0]
	c.Assert(port.SubnetID(), gc.Equals, "")
	opened := port.OpenPorts()
	c.Assert(opened, gc.HasLen, 1)
	c.Assert(opened[0].UnitName(), gc.Equals, unit.Name())
}

func (s *MigrationExportSuite) TestRelations(c *gc.C) {
	// Need to remove owner from application.
	ignored := s.Owner
	wordpress := state.AddTestingService(c, s.State, "wordpress", state.AddTestingCharm(c, s.State, "wordpress"), ignored)
	mysql := state.AddTestingService(c, s.State, "mysql", state.AddTestingCharm(c, s.State, "mysql"), ignored)
	// InferEndpoints will always return provider, requirer
	eps, err := s.State.InferEndpoints("mysql", "wordpress")
	c.Assert(err, jc.ErrorIsNil)
	rel, err := s.State.AddRelation(eps...)
	msEp, wpEp := eps[0], eps[1]
	c.Assert(err, jc.ErrorIsNil)
	wordpress_0 := s.Factory.MakeUnit(c, &factory.UnitParams{Application: wordpress})
	mysql_0 := s.Factory.MakeUnit(c, &factory.UnitParams{Application: mysql})

	ru, err := rel.Unit(wordpress_0)
	c.Assert(err, jc.ErrorIsNil)
	wordpressSettings := map[string]interface{}{
		"name": "wordpress/0",
	}
	err = ru.EnterScope(wordpressSettings)
	c.Assert(err, jc.ErrorIsNil)

	ru, err = rel.Unit(mysql_0)
	c.Assert(err, jc.ErrorIsNil)
	mysqlSettings := map[string]interface{}{
		"name": "mysql/0",
	}
	err = ru.EnterScope(mysqlSettings)
	c.Assert(err, jc.ErrorIsNil)

	model, err := s.State.Export()
	c.Assert(err, jc.ErrorIsNil)

	rels := model.Relations()
	c.Assert(rels, gc.HasLen, 1)

	exRel := rels[0]
	c.Assert(exRel.Id(), gc.Equals, rel.Id())
	c.Assert(exRel.Key(), gc.Equals, rel.String())

	exEps := exRel.Endpoints()
	c.Assert(exEps, gc.HasLen, 2)

	checkEndpoint := func(
		exEndpoint description.Endpoint,
		unitName string,
		ep state.Endpoint,
		settings map[string]interface{},
	) {
		c.Logf("%#v", exEndpoint)
		c.Check(exEndpoint.ApplicationName(), gc.Equals, ep.ApplicationName)
		c.Check(exEndpoint.Name(), gc.Equals, ep.Name)
		c.Check(exEndpoint.UnitCount(), gc.Equals, 1)
		c.Check(exEndpoint.Settings(unitName), jc.DeepEquals, settings)
		c.Check(exEndpoint.Role(), gc.Equals, string(ep.Role))
		c.Check(exEndpoint.Interface(), gc.Equals, ep.Interface)
		c.Check(exEndpoint.Optional(), gc.Equals, ep.Optional)
		c.Check(exEndpoint.Limit(), gc.Equals, ep.Limit)
		c.Check(exEndpoint.Scope(), gc.Equals, string(ep.Scope))
	}
	checkEndpoint(exEps[0], mysql_0.Name(), msEp, mysqlSettings)
	checkEndpoint(exEps[1], wordpress_0.Name(), wpEp, wordpressSettings)
}

type goodToken struct{}

// Check implements leadership.Token
func (*goodToken) Check(interface{}) error {
	return nil
}
