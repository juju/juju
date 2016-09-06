// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state_test

import (
	"math/rand"
	"time"

	jc "github.com/juju/testing/checkers"
	"github.com/juju/version"
	gc "gopkg.in/check.v1"
	"gopkg.in/juju/charm.v6-unstable"
	"gopkg.in/juju/names.v2"

	"github.com/juju/juju/constraints"
	"github.com/juju/juju/core/description"
	"github.com/juju/juju/network"
	"github.com/juju/juju/payload"
	"github.com/juju/juju/provider/dummy"
	"github.com/juju/juju/state"
	"github.com/juju/juju/status"
	"github.com/juju/juju/storage/poolmanager"
	"github.com/juju/juju/storage/provider"
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

type MigrationBaseSuite struct {
	ConnSuite
}

func (s *MigrationBaseSuite) setLatestTools(c *gc.C, latestTools version.Number) {
	dbModel, err := s.State.Model()
	c.Assert(err, jc.ErrorIsNil)
	err = dbModel.UpdateLatestToolsVersion(latestTools)
	c.Assert(err, jc.ErrorIsNil)
}

func (s *MigrationBaseSuite) setRandSequenceValue(c *gc.C, name string) int {
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

func (s *MigrationBaseSuite) primeStatusHistory(c *gc.C, entity statusSetter, statusVal status.Status, count int) {
	primeStatusHistory(c, entity, statusVal, count, func(i int) map[string]interface{} {
		return map[string]interface{}{"index": count - i}
	}, 0)
}

func (s *MigrationBaseSuite) makeApplicationWithLeader(c *gc.C, applicationname string, count int, leader int) {
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

func (s *MigrationBaseSuite) makeUnitWithStorage(c *gc.C) (*state.Application, *state.Unit, names.StorageTag) {
	pool := "loop-pool"
	kind := "block"
	// Create a default pool for block devices.
	pm := poolmanager.New(state.NewStateSettings(s.State), dummy.StorageProviders())
	_, err := pm.Create(pool, provider.LoopProviderType, map[string]interface{}{})
	c.Assert(err, jc.ErrorIsNil)

	// There are test charms called "storage-block" and
	// "storage-filesystem" which are what you'd expect.
	ch := s.AddTestingCharm(c, "storage-"+kind)
	storage := map[string]state.StorageConstraints{
		"data": makeStorageCons(pool, 1024, 1),
	}
	service := s.AddTestingServiceWithStorage(c, "storage-"+kind, ch, storage)
	unit, err := service.AddUnit()

	machine := s.Factory.MakeMachine(c, nil)
	err = unit.AssignToMachine(machine)
	c.Assert(err, jc.ErrorIsNil)

	c.Assert(err, jc.ErrorIsNil)
	storageTag := names.NewStorageTag("data/0")
	agentVersion := version.MustParseBinary("2.0.1-quantal-and64")
	err = unit.SetAgentVersion(agentVersion)
	c.Assert(err, jc.ErrorIsNil)
	return service, unit, storageTag
}

type MigrationExportSuite struct {
	MigrationBaseSuite
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

	model, err := s.State.Export()
	c.Assert(err, jc.ErrorIsNil)

	dbModel, err := s.State.Model()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(model.Tag(), gc.Equals, dbModel.ModelTag())
	c.Assert(model.Owner(), gc.Equals, dbModel.Owner())
	dbModelCfg, err := dbModel.Config()
	c.Assert(err, jc.ErrorIsNil)
	modelAttrs := dbModelCfg.AllAttrs()
	modelCfg := model.Config()
	// Config as read from state has resources tags coerced to a map.
	modelCfg["resource-tags"] = map[string]string{}
	c.Assert(modelCfg, jc.DeepEquals, modelAttrs)
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
	owner, err := s.State.UserAccess(s.Owner, s.State.ModelTag())
	c.Assert(err, jc.ErrorIsNil)
	err = state.UpdateModelUserLastConnection(s.State, owner, lastConnection)
	c.Assert(err, jc.ErrorIsNil)

	bobTag := names.NewUserTag("bob@external")
	bob, err := s.State.AddModelUser(s.State.ModelUUID(), state.UserAccessSpec{
		User:      bobTag,
		CreatedBy: s.Owner,
		Access:    description.ReadAccess,
	})
	c.Assert(err, jc.ErrorIsNil)
	err = state.UpdateModelUserLastConnection(s.State, bob, lastConnection)
	c.Assert(err, jc.ErrorIsNil)

	model, err := s.State.Export()
	c.Assert(err, jc.ErrorIsNil)

	users := model.Users()
	c.Assert(users, gc.HasLen, 2)

	exportedBob := users[0]
	// admin is "test-admin", and results are sorted
	exportedAdmin := users[1]

	c.Assert(exportedAdmin.Name(), gc.Equals, s.Owner)
	c.Assert(exportedAdmin.DisplayName(), gc.Equals, owner.DisplayName)
	c.Assert(exportedAdmin.CreatedBy(), gc.Equals, s.Owner)
	c.Assert(exportedAdmin.DateCreated(), gc.Equals, owner.DateCreated)
	c.Assert(exportedAdmin.LastConnection(), gc.Equals, lastConnection)
	c.Assert(exportedAdmin.Access(), gc.Equals, description.AdminAccess)

	c.Assert(exportedBob.Name(), gc.Equals, bobTag)
	c.Assert(exportedBob.DisplayName(), gc.Equals, "")
	c.Assert(exportedBob.CreatedBy(), gc.Equals, s.Owner)
	c.Assert(exportedBob.DateCreated(), gc.Equals, bob.DateCreated)
	c.Assert(exportedBob.LastConnection(), gc.Equals, lastConnection)
	c.Assert(exportedBob.Access(), gc.Equals, description.ReadAccess)
}

func (s *MigrationExportSuite) TestMachines(c *gc.C) {
	s.assertMachinesMigrated(c, constraints.MustParse("arch=amd64 mem=8G"))
}

func (s *MigrationExportSuite) TestMachinesWithVirtConstraint(c *gc.C) {
	s.assertMachinesMigrated(c, constraints.MustParse("arch=amd64 mem=8G virt-type=kvm"))
}

func (s *MigrationExportSuite) assertMachinesMigrated(c *gc.C, cons constraints.Value) {
	// Add a machine with an LXC container.
	machine1 := s.Factory.MakeMachine(c, &factory.MachineParams{
		Constraints: cons,
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
	c.Assert(constraints.Architecture(), gc.Equals, *cons.Arch)
	c.Assert(constraints.Memory(), gc.Equals, *cons.Mem)
	if cons.HasVirtType() {
		c.Assert(constraints.VirtType(), gc.Equals, *cons.VirtType)
	}

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

func (s *MigrationExportSuite) TestMachineDevices(c *gc.C) {
	machine := s.Factory.MakeMachine(c, nil)
	// Create two devices, first with all fields set, second just to show that
	// we do both.
	sda := state.BlockDeviceInfo{
		DeviceName:     "sda",
		DeviceLinks:    []string{"some", "data"},
		Label:          "sda-label",
		UUID:           "some-uuid",
		HardwareId:     "magic",
		BusAddress:     "bus stop",
		Size:           16 * 1024 * 1024 * 1024,
		FilesystemType: "ext4",
		InUse:          true,
		MountPoint:     "/",
	}
	sdb := state.BlockDeviceInfo{DeviceName: "sdb", MountPoint: "/var/lib/lxd"}
	err := machine.SetMachineBlockDevices(sda, sdb)
	c.Assert(err, jc.ErrorIsNil)

	model, err := s.State.Export()
	c.Assert(err, jc.ErrorIsNil)
	machines := model.Machines()
	c.Assert(machines, gc.HasLen, 1)
	exported := machines[0]

	devices := exported.BlockDevices()
	c.Assert(devices, gc.HasLen, 2)
	ex1, ex2 := devices[0], devices[1]

	c.Check(ex1.Name(), gc.Equals, "sda")
	c.Check(ex1.Links(), jc.DeepEquals, []string{"some", "data"})
	c.Check(ex1.Label(), gc.Equals, "sda-label")
	c.Check(ex1.UUID(), gc.Equals, "some-uuid")
	c.Check(ex1.HardwareID(), gc.Equals, "magic")
	c.Check(ex1.BusAddress(), gc.Equals, "bus stop")
	c.Check(ex1.Size(), gc.Equals, uint64(16*1024*1024*1024))
	c.Check(ex1.FilesystemType(), gc.Equals, "ext4")
	c.Check(ex1.InUse(), jc.IsTrue)
	c.Check(ex1.MountPoint(), gc.Equals, "/")

	c.Check(ex2.Name(), gc.Equals, "sdb")
	c.Check(ex2.MountPoint(), gc.Equals, "/var/lib/lxd")
}

func (s *MigrationExportSuite) TestApplications(c *gc.C) {
	s.assertMigrateApplications(c, constraints.MustParse("arch=amd64 mem=8G"))
}

func (s *MigrationExportSuite) TestApplicationsWithVirtConstraint(c *gc.C) {
	s.assertMigrateApplications(c, constraints.MustParse("arch=amd64 mem=8G virt-type=kvm"))
}

func (s *MigrationExportSuite) assertMigrateApplications(c *gc.C, cons constraints.Value) {
	application := s.Factory.MakeApplication(c, &factory.ApplicationParams{
		Settings: map[string]interface{}{
			"foo": "bar",
		},
		Constraints: cons,
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
	c.Assert(exported.LeadershipSettings(), jc.DeepEquals, map[string]interface{}{
		"leader": "true",
	})
	c.Assert(exported.MetricsCredentials(), jc.DeepEquals, []byte("sekrit"))

	constraints := exported.Constraints()
	c.Assert(constraints, gc.NotNil)
	c.Assert(constraints.Architecture(), gc.Equals, *cons.Arch)
	c.Assert(constraints.Memory(), gc.Equals, *cons.Mem)
	if cons.HasVirtType() {
		c.Assert(constraints.VirtType(), gc.Equals, *cons.VirtType)
	}

	history := exported.StatusHistory()
	c.Assert(history, gc.HasLen, expectedHistoryCount)
	s.checkStatusHistory(c, history[:addedHistoryCount], status.StatusActive)
}

func (s *MigrationExportSuite) TestMultipleApplications(c *gc.C) {
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
	for _, version := range []string{"garnet", "amethyst", "pearl", "steven"} {
		err = unit.SetWorkloadVersion(version)
		c.Assert(err, jc.ErrorIsNil)
	}
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
	c.Assert(exported.WorkloadVersion(), gc.Equals, "steven")
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

	versionHistory := exported.WorkloadVersionHistory()
	// There are extra entries at the start that we don't care about.
	c.Assert(len(versionHistory) >= 4, jc.IsTrue)
	versions := make([]string, 4)
	for i, status := range versionHistory[:4] {
		versions[i] = status.Message()
	}
	// The exporter reads history in reverse time order.
	c.Assert(versions, gc.DeepEquals, []string{"steven", "pearl", "amethyst", "garnet"})
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
	wordpress := state.AddTestingService(c, s.State, "wordpress", state.AddTestingCharm(c, s.State, "wordpress"))
	mysql := state.AddTestingService(c, s.State, "mysql", state.AddTestingCharm(c, s.State, "mysql"))
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

func (s *MigrationExportSuite) TestSpaces(c *gc.C) {
	s.Factory.MakeSpace(c, &factory.SpaceParams{
		Name: "one", ProviderID: network.Id("provider"), IsPublic: true})

	model, err := s.State.Export()
	c.Assert(err, jc.ErrorIsNil)

	spaces := model.Spaces()
	c.Assert(spaces, gc.HasLen, 1)
	space := spaces[0]
	c.Assert(space.Name(), gc.Equals, "one")
	c.Assert(space.ProviderID(), gc.Equals, "provider")
	c.Assert(space.Public(), jc.IsTrue)
}

func (s *MigrationExportSuite) TestMultipleSpaces(c *gc.C) {
	s.Factory.MakeSpace(c, &factory.SpaceParams{Name: "one"})
	s.Factory.MakeSpace(c, &factory.SpaceParams{Name: "two"})
	s.Factory.MakeSpace(c, &factory.SpaceParams{Name: "three"})

	model, err := s.State.Export()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(model.Spaces(), gc.HasLen, 3)
}

func (s *MigrationExportSuite) TestLinkLayerDevices(c *gc.C) {
	machine := s.Factory.MakeMachine(c, &factory.MachineParams{
		Constraints: constraints.MustParse("arch=amd64 mem=8G"),
	})
	deviceArgs := state.LinkLayerDeviceArgs{
		Name: "foo",
		Type: state.EthernetDevice,
	}
	err := machine.SetLinkLayerDevices(deviceArgs)
	c.Assert(err, jc.ErrorIsNil)

	model, err := s.State.Export()
	c.Assert(err, jc.ErrorIsNil)

	devices := model.LinkLayerDevices()
	c.Assert(devices, gc.HasLen, 1)
	device := devices[0]
	c.Assert(device.Name(), gc.Equals, "foo")
	c.Assert(device.Type(), gc.Equals, string(state.EthernetDevice))
}

func (s *MigrationExportSuite) TestSubnets(c *gc.C) {
	_, err := s.State.AddSubnet(state.SubnetInfo{
		CIDR:             "10.0.0.0/24",
		ProviderId:       network.Id("foo"),
		VLANTag:          64,
		AvailabilityZone: "bar",
		SpaceName:        "bam",
	})
	c.Assert(err, jc.ErrorIsNil)
	_, err = s.State.AddSpace("bam", "", nil, true)
	c.Assert(err, jc.ErrorIsNil)

	model, err := s.State.Export()
	c.Assert(err, jc.ErrorIsNil)

	subnets := model.Subnets()
	c.Assert(subnets, gc.HasLen, 1)
	subnet := subnets[0]
	c.Assert(subnet.CIDR(), gc.Equals, "10.0.0.0/24")
	c.Assert(subnet.ProviderId(), gc.Equals, "foo")
	c.Assert(subnet.VLANTag(), gc.Equals, 64)
	c.Assert(subnet.AvailabilityZone(), gc.Equals, "bar")
	c.Assert(subnet.SpaceName(), gc.Equals, "bam")
}

func (s *MigrationExportSuite) TestIPAddresses(c *gc.C) {
	machine := s.Factory.MakeMachine(c, &factory.MachineParams{
		Constraints: constraints.MustParse("arch=amd64 mem=8G"),
	})
	_, err := s.State.AddSubnet(state.SubnetInfo{CIDR: "0.1.2.0/24"})
	c.Assert(err, jc.ErrorIsNil)
	deviceArgs := state.LinkLayerDeviceArgs{
		Name: "foo",
		Type: state.EthernetDevice,
	}
	err = machine.SetLinkLayerDevices(deviceArgs)
	c.Assert(err, jc.ErrorIsNil)
	args := state.LinkLayerDeviceAddress{
		DeviceName:       "foo",
		ConfigMethod:     state.StaticAddress,
		CIDRAddress:      "0.1.2.3/24",
		ProviderID:       "bar",
		DNSServers:       []string{"bam", "mam"},
		DNSSearchDomains: []string{"weeee"},
		GatewayAddress:   "0.1.2.1",
	}
	err = machine.SetDevicesAddresses(args)
	c.Assert(err, jc.ErrorIsNil)

	model, err := s.State.Export()
	c.Assert(err, jc.ErrorIsNil)

	addresses := model.IPAddresses()
	c.Assert(addresses, gc.HasLen, 1)
	addr := addresses[0]
	c.Assert(addr.Value(), gc.Equals, "0.1.2.3")
	c.Assert(addr.MachineID(), gc.Equals, machine.Id())
	c.Assert(addr.DeviceName(), gc.Equals, "foo")
	c.Assert(addr.ConfigMethod(), gc.Equals, string(state.StaticAddress))
	c.Assert(addr.SubnetCIDR(), gc.Equals, "0.1.2.0/24")
	c.Assert(addr.ProviderID(), gc.Equals, "bar")
	c.Assert(addr.DNSServers(), jc.DeepEquals, []string{"bam", "mam"})
	c.Assert(addr.DNSSearchDomains(), jc.DeepEquals, []string{"weeee"})
	c.Assert(addr.GatewayAddress(), gc.Equals, "0.1.2.1")
}

func (s *MigrationExportSuite) TestSSHHostKeys(c *gc.C) {
	machine := s.Factory.MakeMachine(c, &factory.MachineParams{
		Constraints: constraints.MustParse("arch=amd64 mem=8G"),
	})
	err := s.State.SetSSHHostKeys(machine.MachineTag(), []string{"bam", "mam"})
	c.Assert(err, jc.ErrorIsNil)

	model, err := s.State.Export()
	c.Assert(err, jc.ErrorIsNil)

	keys := model.SSHHostKeys()
	c.Assert(keys, gc.HasLen, 1)
	key := keys[0]
	c.Assert(key.MachineID(), gc.Equals, machine.Id())
	c.Assert(key.Keys(), jc.DeepEquals, []string{"bam", "mam"})
}

func (s *MigrationExportSuite) TestActions(c *gc.C) {
	machine := s.Factory.MakeMachine(c, &factory.MachineParams{
		Constraints: constraints.MustParse("arch=amd64 mem=8G"),
	})
	_, err := s.State.EnqueueAction(machine.MachineTag(), "foo", nil)
	c.Assert(err, jc.ErrorIsNil)

	model, err := s.State.Export()
	c.Assert(err, jc.ErrorIsNil)

	actions := model.Actions()
	c.Assert(actions, gc.HasLen, 1)
	action := actions[0]
	c.Check(action.Receiver(), gc.Equals, machine.Id())
	c.Check(action.Name(), gc.Equals, "foo")
	c.Check(action.Status(), gc.Equals, "pending")
	c.Check(action.Message(), gc.Equals, "")
}

type goodToken struct{}

// Check implements leadership.Token
func (*goodToken) Check(interface{}) error {
	return nil
}

func (s *MigrationExportSuite) TestVolumes(c *gc.C) {
	machine := s.Factory.MakeMachine(c, &factory.MachineParams{
		Volumes: []state.MachineVolumeParams{{
			Volume:     state.VolumeParams{Size: 1234},
			Attachment: state.VolumeAttachmentParams{ReadOnly: true},
		}, {
			Volume: state.VolumeParams{Size: 4000},
		}},
	})
	machineTag := machine.MachineTag()

	// We know that the first volume is called "0/0" as it is the first volume
	// (volumes use sequences), and it is bound to machine 0.
	volTag := names.NewVolumeTag("0/0")
	err := s.State.SetVolumeInfo(volTag, state.VolumeInfo{
		HardwareId: "magic",
		Size:       1500,
		VolumeId:   "volume id",
		Persistent: true,
	})
	c.Assert(err, jc.ErrorIsNil)
	err = s.State.SetVolumeAttachmentInfo(machineTag, volTag, state.VolumeAttachmentInfo{
		DeviceName: "device name",
		DeviceLink: "device link",
		BusAddress: "bus address",
		ReadOnly:   true,
	})
	c.Assert(err, jc.ErrorIsNil)

	model, err := s.State.Export()
	c.Assert(err, jc.ErrorIsNil)

	volumes := model.Volumes()
	c.Assert(volumes, gc.HasLen, 2)
	provisioned, notProvisioned := volumes[0], volumes[1]

	c.Check(provisioned.Tag(), gc.Equals, volTag)
	binding, err := provisioned.Binding()
	c.Check(err, jc.ErrorIsNil)
	c.Check(binding, gc.Equals, machineTag)
	c.Check(provisioned.Provisioned(), jc.IsTrue)
	c.Check(provisioned.Size(), gc.Equals, uint64(1500))
	c.Check(provisioned.Pool(), gc.Equals, "loop")
	c.Check(provisioned.HardwareID(), gc.Equals, "magic")
	c.Check(provisioned.VolumeID(), gc.Equals, "volume id")
	c.Check(provisioned.Persistent(), jc.IsTrue)
	attachments := provisioned.Attachments()
	c.Assert(attachments, gc.HasLen, 1)
	attachment := attachments[0]
	c.Check(attachment.Machine(), gc.Equals, machineTag)
	c.Check(attachment.Provisioned(), jc.IsTrue)
	c.Check(attachment.ReadOnly(), jc.IsTrue)
	c.Check(attachment.DeviceName(), gc.Equals, "device name")
	c.Check(attachment.DeviceLink(), gc.Equals, "device link")
	c.Check(attachment.BusAddress(), gc.Equals, "bus address")

	c.Check(notProvisioned.Tag(), gc.Equals, names.NewVolumeTag("0/1"))
	binding, err = notProvisioned.Binding()
	c.Check(err, jc.ErrorIsNil)
	c.Check(binding, gc.Equals, machineTag)
	c.Check(notProvisioned.Provisioned(), jc.IsFalse)
	c.Check(notProvisioned.Size(), gc.Equals, uint64(4000))
	c.Check(notProvisioned.Pool(), gc.Equals, "loop")
	c.Check(notProvisioned.HardwareID(), gc.Equals, "")
	c.Check(notProvisioned.VolumeID(), gc.Equals, "")
	c.Check(notProvisioned.Persistent(), jc.IsFalse)
	attachments = notProvisioned.Attachments()
	c.Assert(attachments, gc.HasLen, 1)
	attachment = attachments[0]
	c.Check(attachment.Machine(), gc.Equals, machineTag)
	c.Check(attachment.Provisioned(), jc.IsFalse)
	c.Check(attachment.ReadOnly(), jc.IsFalse)
	c.Check(attachment.DeviceName(), gc.Equals, "")
	c.Check(attachment.DeviceLink(), gc.Equals, "")
	c.Check(attachment.BusAddress(), gc.Equals, "")

	// Make sure there is a status.
	status := provisioned.Status()
	c.Check(status.Value(), gc.Equals, "pending")
}

func (s *MigrationExportSuite) TestFilesystems(c *gc.C) {
	machine := s.Factory.MakeMachine(c, &factory.MachineParams{
		Filesystems: []state.MachineFilesystemParams{{
			Filesystem: state.FilesystemParams{Size: 1234},
			Attachment: state.FilesystemAttachmentParams{
				Location: "location",
				ReadOnly: true},
		}, {
			Filesystem: state.FilesystemParams{Size: 4000},
		}},
	})
	machineTag := machine.MachineTag()

	// We know that the first filesystem is called "0/0" as it is the first
	// filesystem (filesystems use sequences), and it is bound to machine 0.
	fsTag := names.NewFilesystemTag("0/0")
	err := s.State.SetFilesystemInfo(fsTag, state.FilesystemInfo{
		Size:         1500,
		FilesystemId: "filesystem id",
	})
	c.Assert(err, jc.ErrorIsNil)
	err = s.State.SetFilesystemAttachmentInfo(machineTag, fsTag, state.FilesystemAttachmentInfo{
		MountPoint: "/mnt/foo",
		ReadOnly:   true,
	})
	c.Assert(err, jc.ErrorIsNil)

	model, err := s.State.Export()
	c.Assert(err, jc.ErrorIsNil)

	filesystems := model.Filesystems()
	c.Assert(filesystems, gc.HasLen, 2)
	provisioned, notProvisioned := filesystems[0], filesystems[1]

	c.Check(provisioned.Tag(), gc.Equals, fsTag)
	c.Check(provisioned.Volume(), gc.Equals, names.VolumeTag{})
	c.Check(provisioned.Storage(), gc.Equals, names.StorageTag{})
	binding, err := provisioned.Binding()
	c.Check(err, jc.ErrorIsNil)
	c.Check(binding, gc.Equals, machineTag)
	c.Check(provisioned.Provisioned(), jc.IsTrue)
	c.Check(provisioned.Size(), gc.Equals, uint64(1500))
	c.Check(provisioned.Pool(), gc.Equals, "rootfs")
	c.Check(provisioned.FilesystemID(), gc.Equals, "filesystem id")
	attachments := provisioned.Attachments()
	c.Assert(attachments, gc.HasLen, 1)
	attachment := attachments[0]
	c.Check(attachment.Machine(), gc.Equals, machineTag)
	c.Check(attachment.Provisioned(), jc.IsTrue)
	c.Check(attachment.ReadOnly(), jc.IsTrue)
	c.Check(attachment.MountPoint(), gc.Equals, "/mnt/foo")

	c.Check(notProvisioned.Tag(), gc.Equals, names.NewFilesystemTag("0/1"))
	c.Check(notProvisioned.Volume(), gc.Equals, names.VolumeTag{})
	c.Check(notProvisioned.Storage(), gc.Equals, names.StorageTag{})
	binding, err = notProvisioned.Binding()
	c.Check(err, jc.ErrorIsNil)
	c.Check(binding, gc.Equals, machineTag)
	c.Check(notProvisioned.Provisioned(), jc.IsFalse)
	c.Check(notProvisioned.Size(), gc.Equals, uint64(4000))
	c.Check(notProvisioned.Pool(), gc.Equals, "rootfs")
	c.Check(notProvisioned.FilesystemID(), gc.Equals, "")
	attachments = notProvisioned.Attachments()
	c.Assert(attachments, gc.HasLen, 1)
	attachment = attachments[0]
	c.Check(attachment.Machine(), gc.Equals, machineTag)
	c.Check(attachment.Provisioned(), jc.IsFalse)
	c.Check(attachment.ReadOnly(), jc.IsFalse)
	c.Check(attachment.MountPoint(), gc.Equals, "")

	// Make sure there is a status.
	status := provisioned.Status()
	c.Check(status.Value(), gc.Equals, "pending")
}

func (s *MigrationExportSuite) TestStorage(c *gc.C) {
	_, u, storageTag := s.makeUnitWithStorage(c)

	model, err := s.State.Export()
	c.Assert(err, jc.ErrorIsNil)

	apps := model.Applications()
	c.Assert(apps, gc.HasLen, 1)
	constraints := apps[0].StorageConstraints()
	c.Assert(constraints, gc.HasLen, 2)
	cons, found := constraints["data"]
	c.Assert(found, jc.IsTrue)
	c.Check(cons.Pool(), gc.Equals, "loop-pool")
	c.Check(cons.Size(), gc.Equals, uint64(0x400))
	c.Check(cons.Count(), gc.Equals, uint64(1))
	cons, found = constraints["allecto"]
	c.Assert(found, jc.IsTrue)
	c.Check(cons.Pool(), gc.Equals, "loop")
	c.Check(cons.Size(), gc.Equals, uint64(0x400))
	c.Check(cons.Count(), gc.Equals, uint64(0))

	storages := model.Storages()
	c.Assert(storages, gc.HasLen, 1)

	storage := storages[0]

	c.Check(storage.Tag(), gc.Equals, storageTag)
	c.Check(storage.Kind(), gc.Equals, "block")
	owner, err := storage.Owner()
	c.Check(err, jc.ErrorIsNil)
	c.Check(owner, gc.Equals, u.Tag())
	c.Check(storage.Name(), gc.Equals, "data")
	c.Check(storage.Attachments(), jc.DeepEquals, []names.UnitTag{
		u.UnitTag(),
	})
}

func (s *MigrationExportSuite) TestStoragePools(c *gc.C) {
	pm := poolmanager.New(state.NewStateSettings(s.State), provider.CommonStorageProviders())
	_, err := pm.Create("test-pool", provider.LoopProviderType, map[string]interface{}{
		"value": 42,
	})
	c.Assert(err, jc.ErrorIsNil)

	model, err := s.State.Export()
	c.Assert(err, jc.ErrorIsNil)

	pools := model.StoragePools()
	c.Assert(pools, gc.HasLen, 1)
	pool := pools[0]
	c.Assert(pool.Name(), gc.Equals, "test-pool")
	c.Assert(pool.Provider(), gc.Equals, "loop")
	c.Assert(pool.Attributes(), jc.DeepEquals, map[string]interface{}{
		"value": 42,
	})
}

func (s *MigrationExportSuite) TestPayloads(c *gc.C) {
	unit := s.Factory.MakeUnit(c, nil)
	up, err := s.State.UnitPayloads(unit)
	c.Assert(err, jc.ErrorIsNil)
	original := payload.Payload{
		PayloadClass: charm.PayloadClass{
			Name: "something",
			Type: "special",
		},
		ID:     "42",
		Status: "running",
		Labels: []string{"foo", "bar"},
	}
	err = up.Track(original)
	c.Assert(err, jc.ErrorIsNil)

	model, err := s.State.Export()
	c.Assert(err, jc.ErrorIsNil)

	applications := model.Applications()
	c.Assert(applications, gc.HasLen, 1)

	units := applications[0].Units()
	c.Assert(units, gc.HasLen, 1)

	payloads := units[0].Payloads()
	c.Assert(payloads, gc.HasLen, 1)

	payload := payloads[0]
	c.Check(payload.Name(), gc.Equals, original.Name)
	c.Check(payload.Type(), gc.Equals, original.Type)
	c.Check(payload.RawID(), gc.Equals, original.ID)
	c.Check(payload.State(), gc.Equals, original.Status)
	c.Check(payload.Labels(), jc.DeepEquals, original.Labels)
}
