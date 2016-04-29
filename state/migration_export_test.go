// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state_test

import (
	"math/rand"
	"runtime"
	"time"

	"github.com/juju/names"
	jc "github.com/juju/testing/checkers"
	"github.com/juju/version"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/constraints"
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

type statusSetter interface {
	SetStatus(status.Status, string, map[string]interface{}) error
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
	for i := 0; i < count; i++ {
		c.Logf("setting status for %v", entity)
		err := entity.SetStatus(statusVal, "", map[string]interface{}{"index": count - i})
		c.Assert(err, jc.ErrorIsNil)
		if runtime.GOOS == "windows" {
			// The default clock tick on Windows is 15.6 ms.
			time.Sleep(20 * time.Millisecond)
		}
	}
}

func (s *MigrationSuite) makeServiceWithLeader(c *gc.C, serviceName string, count int, leader int) {
	c.Assert(leader < count, jc.IsTrue)
	units := make([]*state.Unit, count)
	service := s.Factory.MakeService(c, &factory.ServiceParams{
		Name: serviceName,
		Charm: s.Factory.MakeCharm(c, &factory.CharmParams{
			Name: serviceName,
		}),
	})
	for i := 0; i < count; i++ {
		units[i] = s.Factory.MakeUnit(c, &factory.UnitParams{
			Service: service,
		})
	}
	err := s.State.LeadershipClaimer().ClaimLeadership(
		service.Name(),
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
	fooSeq := s.setRandSequenceValue(c, "service-foo")
	s.State.SwitchBlockOn(state.ChangeBlock, "locked down")

	model, err := s.State.Export()
	c.Assert(err, jc.ErrorIsNil)

	dbModel, err := s.State.Model()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(model.Tag(), gc.Equals, dbModel.ModelTag())
	c.Assert(model.Owner(), gc.Equals, dbModel.Owner())
	config, err := dbModel.Config()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(model.Config(), jc.DeepEquals, config.AllAttrs())
	c.Assert(model.LatestToolsVersion(), gc.Equals, latestTools)
	c.Assert(model.Annotations(), jc.DeepEquals, testAnnotations)
	constraints := model.Constraints()
	c.Assert(constraints, gc.NotNil)
	c.Assert(constraints.Architecture(), gc.Equals, "amd64")
	c.Assert(constraints.Memory(), gc.Equals, 8*gig)
	c.Assert(model.Sequences(), jc.DeepEquals, map[string]int{
		"machine":     machineSeq,
		"service-foo": fooSeq,
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
	service := s.Factory.MakeService(c, &factory.ServiceParams{
		Settings: map[string]interface{}{
			"foo": "bar",
		},
		Constraints: constraints.MustParse("arch=amd64 mem=8G"),
	})
	err := service.UpdateLeaderSettings(&goodToken{}, map[string]string{
		"leader": "true",
	})
	c.Assert(err, jc.ErrorIsNil)
	err = service.SetMetricCredentials([]byte("sekrit"))
	c.Assert(err, jc.ErrorIsNil)
	err = s.State.SetAnnotations(service, testAnnotations)
	c.Assert(err, jc.ErrorIsNil)
	s.primeStatusHistory(c, service, status.StatusActive, addedHistoryCount)

	model, err := s.State.Export()
	c.Assert(err, jc.ErrorIsNil)

	services := model.Services()
	c.Assert(services, gc.HasLen, 1)

	exported := services[0]
	c.Assert(exported.Name(), gc.Equals, service.Name())
	c.Assert(exported.Tag(), gc.Equals, service.ServiceTag())
	c.Assert(exported.Series(), gc.Equals, service.Series())
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
	s.Factory.MakeService(c, &factory.ServiceParams{Name: "first"})
	s.Factory.MakeService(c, &factory.ServiceParams{Name: "second"})
	s.Factory.MakeService(c, &factory.ServiceParams{Name: "third"})

	model, err := s.State.Export()
	c.Assert(err, jc.ErrorIsNil)

	services := model.Services()
	c.Assert(services, gc.HasLen, 3)
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

	services := model.Services()
	c.Assert(services, gc.HasLen, 1)

	service := services[0]
	units := service.Units()
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
	s.makeServiceWithLeader(c, "mysql", 2, 1)
	s.makeServiceWithLeader(c, "wordpress", 4, 2)

	model, err := s.State.Export()
	c.Assert(err, jc.ErrorIsNil)

	leaders := make(map[string]string)
	for _, service := range model.Services() {
		leaders[service.Name()] = service.Leader()
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
	// Need to remove owner from service.
	ignored := s.Owner
	wordpress := state.AddTestingService(c, s.State, "wordpress", state.AddTestingCharm(c, s.State, "wordpress"), ignored)
	mysql := state.AddTestingService(c, s.State, "mysql", state.AddTestingCharm(c, s.State, "mysql"), ignored)
	// InferEndpoints will always return provider, requirer
	eps, err := s.State.InferEndpoints("mysql", "wordpress")
	c.Assert(err, jc.ErrorIsNil)
	rel, err := s.State.AddRelation(eps...)
	msEp, wpEp := eps[0], eps[1]
	c.Assert(err, jc.ErrorIsNil)
	wordpress_0 := s.Factory.MakeUnit(c, &factory.UnitParams{Service: wordpress})
	mysql_0 := s.Factory.MakeUnit(c, &factory.UnitParams{Service: mysql})

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
		c.Check(exEndpoint.ServiceName(), gc.Equals, ep.ServiceName)
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
