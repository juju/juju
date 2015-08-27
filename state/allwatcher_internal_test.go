// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state

import (
	"fmt"
	"sort"
	"time"

	"github.com/juju/errors"
	"github.com/juju/loggo"
	"github.com/juju/names"
	jc "github.com/juju/testing/checkers"
	"github.com/juju/utils"
	gc "gopkg.in/check.v1"
	"gopkg.in/juju/charm.v5"

	"github.com/juju/juju/constraints"
	"github.com/juju/juju/instance"
	"github.com/juju/juju/network"
	"github.com/juju/juju/state/multiwatcher"
	"github.com/juju/juju/state/watcher"
	"github.com/juju/juju/testing"
)

var (
	_ backingEntityDoc = (*backingMachine)(nil)
	_ backingEntityDoc = (*backingUnit)(nil)
	_ backingEntityDoc = (*backingService)(nil)
	_ backingEntityDoc = (*backingRelation)(nil)
	_ backingEntityDoc = (*backingAnnotation)(nil)
	_ backingEntityDoc = (*backingStatus)(nil)
	_ backingEntityDoc = (*backingConstraints)(nil)
	_ backingEntityDoc = (*backingSettings)(nil)
	_ backingEntityDoc = (*backingOpenedPorts)(nil)
	_ backingEntityDoc = (*backingAction)(nil)
	_ backingEntityDoc = (*backingBlock)(nil)
)

var dottedConfig = `
options:
  key.dotted: {default: My Key, description: Desc, type: string}
`

type allWatcherBaseSuite struct {
	internalStateSuite
	envCount int
}

func (s *allWatcherBaseSuite) newState(c *gc.C) *State {
	s.envCount++
	cfg := testing.CustomEnvironConfig(c, testing.Attrs{
		"name": fmt.Sprintf("testenv%d", s.envCount),
		"uuid": utils.MustNewUUID().String(),
	})
	_, st, err := s.state.NewEnvironment(cfg, s.owner)
	c.Assert(err, jc.ErrorIsNil)
	s.AddCleanup(func(*gc.C) { st.Close() })
	return st
}

// setUpScenario adds some entities to the state so that
// we can check that they all get pulled in by
// all(Env)WatcherStateBacking.GetAll.
func (s *allWatcherBaseSuite) setUpScenario(c *gc.C, st *State, units int) (entities entityInfoSlice) {
	envUUID := st.EnvironUUID()
	add := func(e multiwatcher.EntityInfo) {
		entities = append(entities, e)
	}
	m, err := st.AddMachine("quantal", JobHostUnits)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(m.Tag(), gc.Equals, names.NewMachineTag("0"))
	err = m.SetHasVote(true)
	c.Assert(err, jc.ErrorIsNil)
	// TODO(dfc) instance.Id should take a TAG!
	err = m.SetProvisioned(instance.Id("i-"+m.Tag().String()), "fake_nonce", nil)
	c.Assert(err, jc.ErrorIsNil)
	hc, err := m.HardwareCharacteristics()
	c.Assert(err, jc.ErrorIsNil)
	err = m.SetProviderAddresses(network.NewAddress("example.com"))
	c.Assert(err, jc.ErrorIsNil)
	add(&multiwatcher.MachineInfo{
		EnvUUID:                 envUUID,
		Id:                      "0",
		InstanceId:              "i-machine-0",
		Status:                  multiwatcher.Status("pending"),
		StatusData:              map[string]interface{}{},
		Life:                    multiwatcher.Life("alive"),
		Series:                  "quantal",
		Jobs:                    []multiwatcher.MachineJob{JobHostUnits.ToParams()},
		Addresses:               m.Addresses(),
		HardwareCharacteristics: hc,
		HasVote:                 true,
		WantsVote:               false,
	})

	wordpress := AddTestingService(c, st, "wordpress", AddTestingCharm(c, st, "wordpress"), s.owner)
	err = wordpress.SetExposed()
	c.Assert(err, jc.ErrorIsNil)
	err = wordpress.SetMinUnits(units)
	c.Assert(err, jc.ErrorIsNil)
	err = wordpress.SetConstraints(constraints.MustParse("mem=100M"))
	c.Assert(err, jc.ErrorIsNil)
	setServiceConfigAttr(c, wordpress, "blog-title", "boring")
	add(&multiwatcher.ServiceInfo{
		EnvUUID:     envUUID,
		Name:        "wordpress",
		Exposed:     true,
		CharmURL:    serviceCharmURL(wordpress).String(),
		OwnerTag:    s.owner.String(),
		Life:        multiwatcher.Life("alive"),
		MinUnits:    units,
		Constraints: constraints.MustParse("mem=100M"),
		Config:      charm.Settings{"blog-title": "boring"},
		Subordinate: false,
		Status: multiwatcher.StatusInfo{
			Current: "unknown",
			Message: "Waiting for agent initialization to finish",
			Data:    map[string]interface{}{},
		},
	})
	pairs := map[string]string{"x": "12", "y": "99"}
	err = st.SetAnnotations(wordpress, pairs)
	c.Assert(err, jc.ErrorIsNil)
	add(&multiwatcher.AnnotationInfo{
		EnvUUID:     envUUID,
		Tag:         "service-wordpress",
		Annotations: pairs,
	})

	logging := AddTestingService(c, st, "logging", AddTestingCharm(c, st, "logging"), s.owner)
	add(&multiwatcher.ServiceInfo{
		EnvUUID:     envUUID,
		Name:        "logging",
		CharmURL:    serviceCharmURL(logging).String(),
		OwnerTag:    s.owner.String(),
		Life:        multiwatcher.Life("alive"),
		Config:      charm.Settings{},
		Subordinate: true,
		Status: multiwatcher.StatusInfo{
			Current: "unknown",
			Message: "Waiting for agent initialization to finish",
			Data:    map[string]interface{}{},
		},
	})

	eps, err := st.InferEndpoints("logging", "wordpress")
	c.Assert(err, jc.ErrorIsNil)
	rel, err := st.AddRelation(eps...)
	c.Assert(err, jc.ErrorIsNil)
	add(&multiwatcher.RelationInfo{
		EnvUUID: envUUID,
		Key:     "logging:logging-directory wordpress:logging-dir",
		Id:      rel.Id(),
		Endpoints: []multiwatcher.Endpoint{
			{ServiceName: "logging", Relation: charm.Relation{Name: "logging-directory", Role: "requirer", Interface: "logging", Optional: false, Limit: 1, Scope: "container"}},
			{ServiceName: "wordpress", Relation: charm.Relation{Name: "logging-dir", Role: "provider", Interface: "logging", Optional: false, Limit: 0, Scope: "container"}}},
	})

	for i := 0; i < units; i++ {
		wu, err := wordpress.AddUnit()
		c.Assert(err, jc.ErrorIsNil)
		c.Assert(wu.Tag().String(), gc.Equals, fmt.Sprintf("unit-wordpress-%d", i))

		m, err := st.AddMachine("quantal", JobHostUnits)
		c.Assert(err, jc.ErrorIsNil)
		c.Assert(m.Tag().String(), gc.Equals, fmt.Sprintf("machine-%d", i+1))

		add(&multiwatcher.UnitInfo{
			EnvUUID:     envUUID,
			Name:        fmt.Sprintf("wordpress/%d", i),
			Service:     wordpress.Name(),
			Series:      m.Series(),
			MachineId:   m.Id(),
			Ports:       []network.Port{},
			Status:      multiwatcher.Status("pending"),
			StatusData:  map[string]interface{}{},
			Subordinate: false,
			WorkloadStatus: multiwatcher.StatusInfo{
				Current: "unknown",
				Message: "Waiting for agent initialization to finish",
				Data:    map[string]interface{}{},
			},
			AgentStatus: multiwatcher.StatusInfo{
				Current: "allocating",
				Message: "",
				Data:    map[string]interface{}{},
			},
		})
		pairs := map[string]string{"name": fmt.Sprintf("bar %d", i)}
		err = st.SetAnnotations(wu, pairs)
		c.Assert(err, jc.ErrorIsNil)
		add(&multiwatcher.AnnotationInfo{
			EnvUUID:     envUUID,
			Tag:         fmt.Sprintf("unit-wordpress-%d", i),
			Annotations: pairs,
		})

		err = m.SetProvisioned(instance.Id("i-"+m.Tag().String()), "fake_nonce", nil)
		c.Assert(err, jc.ErrorIsNil)
		err = m.SetStatus(StatusError, m.Tag().String(), nil)
		c.Assert(err, jc.ErrorIsNil)
		hc, err := m.HardwareCharacteristics()
		c.Assert(err, jc.ErrorIsNil)
		add(&multiwatcher.MachineInfo{
			EnvUUID:                 envUUID,
			Id:                      fmt.Sprint(i + 1),
			InstanceId:              "i-" + m.Tag().String(),
			Status:                  multiwatcher.Status("error"),
			StatusInfo:              m.Tag().String(),
			StatusData:              map[string]interface{}{},
			Life:                    multiwatcher.Life("alive"),
			Series:                  "quantal",
			Jobs:                    []multiwatcher.MachineJob{JobHostUnits.ToParams()},
			Addresses:               []network.Address{},
			HardwareCharacteristics: hc,
			HasVote:                 false,
			WantsVote:               false,
		})
		err = wu.AssignToMachine(m)
		c.Assert(err, jc.ErrorIsNil)

		deployer, ok := wu.DeployerTag()
		c.Assert(ok, jc.IsTrue)
		c.Assert(deployer, gc.Equals, names.NewMachineTag(fmt.Sprintf("%d", i+1)))

		wru, err := rel.Unit(wu)
		c.Assert(err, jc.ErrorIsNil)

		// Create the subordinate unit as a side-effect of entering
		// scope in the principal's relation-unit.
		err = wru.EnterScope(nil)
		c.Assert(err, jc.ErrorIsNil)

		lu, err := st.Unit(fmt.Sprintf("logging/%d", i))
		c.Assert(err, jc.ErrorIsNil)
		c.Assert(lu.IsPrincipal(), jc.IsFalse)
		deployer, ok = lu.DeployerTag()
		c.Assert(ok, jc.IsTrue)
		c.Assert(deployer, gc.Equals, names.NewUnitTag(fmt.Sprintf("wordpress/%d", i)))
		add(&multiwatcher.UnitInfo{
			EnvUUID:     envUUID,
			Name:        fmt.Sprintf("logging/%d", i),
			Service:     "logging",
			Series:      "quantal",
			Ports:       []network.Port{},
			Status:      multiwatcher.Status("pending"),
			StatusData:  map[string]interface{}{},
			Subordinate: true,
			WorkloadStatus: multiwatcher.StatusInfo{
				Current: "unknown",
				Message: "Waiting for agent initialization to finish",
				Data:    map[string]interface{}{},
			},
			AgentStatus: multiwatcher.StatusInfo{
				Current: "allocating",
				Message: "",
				Data:    map[string]interface{}{},
			},
		})
	}
	return
}

var _ = gc.Suite(&allWatcherStateSuite{})

type allWatcherStateSuite struct {
	allWatcherBaseSuite
}

func (s *allWatcherStateSuite) Reset(c *gc.C) {
	s.TearDownTest(c)
	s.SetUpTest(c)
}

func (s *allWatcherStateSuite) TestGetAll(c *gc.C) {
	expectEntities := s.setUpScenario(c, s.state, 2)
	s.checkGetAll(c, expectEntities)
}

func (s *allWatcherStateSuite) TestGetAllMultiEnv(c *gc.C) {
	// Set up 2 environments and ensure that GetAll returns the
	// entities for the first environment with no errors.
	expectEntities := s.setUpScenario(c, s.state, 2)

	// Use more units in the second env to ensure the number of
	// entities will mismatch if environment filtering isn't in place.
	s.setUpScenario(c, s.newState(c), 4)

	s.checkGetAll(c, expectEntities)
}

func (s *allWatcherStateSuite) checkGetAll(c *gc.C, expectEntities entityInfoSlice) {
	b := newAllWatcherStateBacking(s.state)
	all := newStore()
	err := b.GetAll(all)
	c.Assert(err, jc.ErrorIsNil)
	var gotEntities entityInfoSlice = all.All()
	sort.Sort(gotEntities)
	sort.Sort(expectEntities)
	substNilSinceTimeForEntities(c, gotEntities)
	assertEntitiesEqual(c, gotEntities, expectEntities)
}

func serviceCharmURL(svc *Service) *charm.URL {
	url, _ := svc.CharmURL()
	return url
}

func setServiceConfigAttr(c *gc.C, svc *Service, attr string, val interface{}) {
	err := svc.UpdateConfigSettings(charm.Settings{attr: val})
	c.Assert(err, jc.ErrorIsNil)
}

// changeTestCase encapsulates entities to add, a change, and
// the expected contents for a test.
type changeTestCase struct {
	// about describes the test case.
	about string

	// initialContents contains the infos of the
	// watcher before signalling the change.
	initialContents []multiwatcher.EntityInfo

	// change signals the change of the watcher.
	change watcher.Change

	// expectContents contains the expected infos of
	// the watcher before signalling the change.
	expectContents []multiwatcher.EntityInfo
}

func substNilSinceTimeForStatus(c *gc.C, status *multiwatcher.StatusInfo) {
	if status.Current != "" {
		c.Assert(status.Since, gc.NotNil)
	}
	status.Since = nil
}

// substNilSinceTimeForEntities zeros out any updated timestamps for unit
// or service status values so we can easily check the results.
func substNilSinceTimeForEntities(c *gc.C, entities []multiwatcher.EntityInfo) {
	// Zero out any updated timestamps for unit or service status values
	// so we can easily check the results.
	for i, entity := range entities {
		if unitInfo, ok := entity.(*multiwatcher.UnitInfo); ok {
			substNilSinceTimeForStatus(c, &unitInfo.WorkloadStatus)
			substNilSinceTimeForStatus(c, &unitInfo.AgentStatus)
			entities[i] = unitInfo
		}
		if serviceInfo, ok := entity.(*multiwatcher.ServiceInfo); ok {
			substNilSinceTimeForStatus(c, &serviceInfo.Status)
			entities[i] = serviceInfo
		}
	}
}

// changeTestFunc is a function for the preparation of a test and
// the creation of the according case.
type changeTestFunc func(c *gc.C, st *State) changeTestCase

// performChangeTestCases runs a passed number of test cases for changes.
func (s *allWatcherStateSuite) performChangeTestCases(c *gc.C, changeTestFuncs []changeTestFunc) {
	for i, changeTestFunc := range changeTestFuncs {
		test := changeTestFunc(c, s.state)

		c.Logf("test %d. %s", i, test.about)
		b := newAllWatcherStateBacking(s.state)
		all := newStore()
		for _, info := range test.initialContents {
			all.Update(info)
		}
		err := b.Changed(all, test.change)
		c.Assert(err, jc.ErrorIsNil)
		entities := all.All()
		substNilSinceTimeForEntities(c, entities)
		assertEntitiesEqual(c, entities, test.expectContents)
		s.Reset(c)
	}
}

func (s *allWatcherStateSuite) TestChangeAnnotations(c *gc.C) {
	testChangeAnnotations(c, s.performChangeTestCases)
}

func (s *allWatcherStateSuite) TestChangeMachines(c *gc.C) {
	testChangeMachines(c, s.performChangeTestCases)
}

func (s *allWatcherStateSuite) TestChangeRelations(c *gc.C) {
	testChangeRelations(c, s.owner, s.performChangeTestCases)
}

func (s *allWatcherStateSuite) TestChangeServices(c *gc.C) {
	testChangeServices(c, s.owner, s.performChangeTestCases)
}

func (s *allWatcherStateSuite) TestChangeServicesConstraints(c *gc.C) {
	testChangeServicesConstraints(c, s.owner, s.performChangeTestCases)
}

func (s *allWatcherStateSuite) TestChangeUnits(c *gc.C) {
	testChangeUnits(c, s.owner, s.performChangeTestCases)
}

func (s *allWatcherStateSuite) TestChangeUnitsNonNilPorts(c *gc.C) {
	testChangeUnitsNonNilPorts(c, s.owner, s.performChangeTestCases)
}

func (s *allWatcherStateSuite) TestChangeActions(c *gc.C) {
	changeTestFuncs := []changeTestFunc{
		func(c *gc.C, st *State) changeTestCase {
			wordpress := AddTestingService(c, st, "wordpress", AddTestingCharm(c, st, "wordpress"), s.owner)
			u, err := wordpress.AddUnit()
			c.Assert(err, jc.ErrorIsNil)
			action, err := st.EnqueueAction(u.Tag(), "vacuumdb", map[string]interface{}{})
			c.Assert(err, jc.ErrorIsNil)
			enqueued := makeActionInfo(action, st)
			action, err = action.Begin()
			c.Assert(err, jc.ErrorIsNil)
			started := makeActionInfo(action, st)
			return changeTestCase{
				about:           "action change picks up last change",
				initialContents: []multiwatcher.EntityInfo{&enqueued, &started},
				change:          watcher.Change{C: actionsC, Id: st.docID(action.Id())},
				expectContents:  []multiwatcher.EntityInfo{&started},
			}
		},
	}
	s.performChangeTestCases(c, changeTestFuncs)
}

func (s *allWatcherStateSuite) TestChangeBlocks(c *gc.C) {
	changeTestFuncs := []changeTestFunc{
		func(c *gc.C, st *State) changeTestCase {
			return changeTestCase{
				about: "no blocks in state, no blocks in store -> do nothing",
				change: watcher.Change{
					C:  blocksC,
					Id: "1",
				}}
		},
		func(c *gc.C, st *State) changeTestCase {
			blockId := st.docID("0")
			blockType := DestroyBlock.ToParams()
			blockMsg := "woot"
			return changeTestCase{
				about: "no change if block is not in backing",
				initialContents: []multiwatcher.EntityInfo{&multiwatcher.BlockInfo{
					EnvUUID: st.EnvironUUID(),
					Id:      blockId,
					Type:    blockType,
					Message: blockMsg,
					Tag:     st.EnvironTag().String(),
				}},
				change: watcher.Change{
					C:  blocksC,
					Id: st.localID(blockId),
				},
				expectContents: []multiwatcher.EntityInfo{&multiwatcher.BlockInfo{
					EnvUUID: st.EnvironUUID(),
					Id:      blockId,
					Type:    blockType,
					Message: blockMsg,
					Tag:     st.EnvironTag().String(),
				}},
			}
		},
		func(c *gc.C, st *State) changeTestCase {
			err := st.SwitchBlockOn(DestroyBlock, "multiwatcher testing")
			c.Assert(err, jc.ErrorIsNil)
			b, found, err := st.GetBlockForType(DestroyBlock)
			c.Assert(err, jc.ErrorIsNil)
			c.Assert(found, jc.IsTrue)
			blockId := b.Id()

			return changeTestCase{
				about: "block is added if it's in backing but not in Store",
				change: watcher.Change{
					C:  blocksC,
					Id: blockId,
				},
				expectContents: []multiwatcher.EntityInfo{
					&multiwatcher.BlockInfo{
						EnvUUID: st.EnvironUUID(),
						Id:      st.localID(blockId),
						Type:    b.Type().ToParams(),
						Message: b.Message(),
						Tag:     st.EnvironTag().String(),
					}}}
		},
		func(c *gc.C, st *State) changeTestCase {
			err := st.SwitchBlockOn(DestroyBlock, "multiwatcher testing")
			c.Assert(err, jc.ErrorIsNil)
			b, found, err := st.GetBlockForType(DestroyBlock)
			c.Assert(err, jc.ErrorIsNil)
			c.Assert(found, jc.IsTrue)
			err = st.SwitchBlockOff(DestroyBlock)
			c.Assert(err, jc.ErrorIsNil)

			return changeTestCase{
				about: "block is removed if it's in backing and in multiwatcher.Store",
				change: watcher.Change{
					C:  blocksC,
					Id: b.Id(),
				},
			}
		},
	}
	s.performChangeTestCases(c, changeTestFuncs)
}

func (s *allWatcherStateSuite) TestClosingPorts(c *gc.C) {
	defer s.Reset(c)
	// Init the test environment.
	wordpress := AddTestingService(c, s.state, "wordpress", AddTestingCharm(c, s.state, "wordpress"), s.owner)
	u, err := wordpress.AddUnit()
	c.Assert(err, jc.ErrorIsNil)
	m, err := s.state.AddMachine("quantal", JobHostUnits)
	c.Assert(err, jc.ErrorIsNil)
	err = u.AssignToMachine(m)
	c.Assert(err, jc.ErrorIsNil)
	publicAddress := network.NewScopedAddress("1.2.3.4", network.ScopePublic)
	privateAddress := network.NewScopedAddress("4.3.2.1", network.ScopeCloudLocal)
	err = m.SetProviderAddresses(publicAddress)
	c.Assert(err, jc.ErrorIsNil)
	err = m.SetMachineAddresses(privateAddress)
	c.Assert(err, jc.ErrorIsNil)
	err = u.OpenPorts("tcp", 12345, 12345)
	c.Assert(err, jc.ErrorIsNil)
	// Create all watcher state backing.
	b := newAllWatcherStateBacking(s.state)
	all := newStore()
	all.Update(&multiwatcher.MachineInfo{
		EnvUUID: s.state.EnvironUUID(),
		Id:      "0",
	})
	// Check opened ports.
	err = b.Changed(all, watcher.Change{
		C:  "units",
		Id: s.state.docID("wordpress/0"),
	})
	c.Assert(err, jc.ErrorIsNil)
	entities := all.All()
	substNilSinceTimeForEntities(c, entities)
	assertEntitiesEqual(c, entities, []multiwatcher.EntityInfo{
		&multiwatcher.UnitInfo{
			EnvUUID:        s.state.EnvironUUID(),
			Name:           "wordpress/0",
			Service:        "wordpress",
			Series:         "quantal",
			MachineId:      "0",
			PublicAddress:  "1.2.3.4",
			PrivateAddress: "4.3.2.1",
			Ports:          []network.Port{{"tcp", 12345}},
			PortRanges:     []network.PortRange{{12345, 12345, "tcp"}},
			Status:         "pending",
			StatusData:     map[string]interface{}{},
			WorkloadStatus: multiwatcher.StatusInfo{
				Current: "unknown",
				Message: "Waiting for agent initialization to finish",
				Data:    map[string]interface{}{},
			},
			AgentStatus: multiwatcher.StatusInfo{
				Current: "allocating",
				Data:    map[string]interface{}{},
			},
		},
		&multiwatcher.MachineInfo{
			EnvUUID: s.state.EnvironUUID(),
			Id:      "0",
		},
	})
	// Close the ports.
	err = u.ClosePorts("tcp", 12345, 12345)
	c.Assert(err, jc.ErrorIsNil)
	err = b.Changed(all, watcher.Change{
		C:  openedPortsC,
		Id: s.state.docID("m#0#n#juju-public"),
	})
	c.Assert(err, jc.ErrorIsNil)
	entities = all.All()
	assertEntitiesEqual(c, entities, []multiwatcher.EntityInfo{
		&multiwatcher.UnitInfo{
			EnvUUID:        s.state.EnvironUUID(),
			Name:           "wordpress/0",
			Service:        "wordpress",
			Series:         "quantal",
			MachineId:      "0",
			PublicAddress:  "1.2.3.4",
			PrivateAddress: "4.3.2.1",
			Ports:          []network.Port{},
			PortRanges:     []network.PortRange{},
			Status:         "pending",
			StatusData:     map[string]interface{}{},
			WorkloadStatus: multiwatcher.StatusInfo{
				Current: "unknown",
				Message: "Waiting for agent initialization to finish",
				Data:    map[string]interface{}{},
			},
			AgentStatus: multiwatcher.StatusInfo{
				Current: "allocating",
				Data:    map[string]interface{}{},
			},
		},
		&multiwatcher.MachineInfo{
			EnvUUID: s.state.EnvironUUID(),
			Id:      "0",
		},
	})
}

func (s *allWatcherStateSuite) TestSettings(c *gc.C) {
	defer s.Reset(c)
	// Init the test environment.
	svc := AddTestingService(c, s.state, "dummy-service", AddTestingCharm(c, s.state, "dummy"), s.owner)
	b := newAllWatcherStateBacking(s.state)
	all := newStore()
	// 1st scenario part: set settings and signal change.
	setServiceConfigAttr(c, svc, "username", "foo")
	setServiceConfigAttr(c, svc, "outlook", "foo@bar")
	all.Update(&multiwatcher.ServiceInfo{
		EnvUUID:  s.state.EnvironUUID(),
		Name:     "dummy-service",
		CharmURL: "local:quantal/quantal-dummy-1",
	})
	err := b.Changed(all, watcher.Change{
		C:  "settings",
		Id: s.state.docID("s#dummy-service#local:quantal/quantal-dummy-1"),
	})
	c.Assert(err, jc.ErrorIsNil)
	entities := all.All()
	substNilSinceTimeForEntities(c, entities)
	assertEntitiesEqual(c, entities, []multiwatcher.EntityInfo{
		&multiwatcher.ServiceInfo{
			EnvUUID:  s.state.EnvironUUID(),
			Name:     "dummy-service",
			CharmURL: "local:quantal/quantal-dummy-1",
			Config:   charm.Settings{"outlook": "foo@bar", "username": "foo"},
		},
	})
	// 2nd scenario part: destroy the service and signal change.
	err = svc.Destroy()
	c.Assert(err, jc.ErrorIsNil)
	err = b.Changed(all, watcher.Change{
		C:  "settings",
		Id: s.state.docID("s#dummy-service#local:quantal/quantal-dummy-1"),
	})
	c.Assert(err, jc.ErrorIsNil)
	entities = all.All()
	assertEntitiesEqual(c, entities, []multiwatcher.EntityInfo{
		&multiwatcher.ServiceInfo{
			EnvUUID:  s.state.EnvironUUID(),
			Name:     "dummy-service",
			CharmURL: "local:quantal/quantal-dummy-1",
		},
	})
}

// TestStateWatcher tests the integration of the state watcher
// with the state-based backing. Most of the logic is tested elsewhere -
// this just tests end-to-end.
func (s *allWatcherStateSuite) TestStateWatcher(c *gc.C) {
	m0, err := s.state.AddMachine("trusty", JobManageEnviron)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(m0.Id(), gc.Equals, "0")

	m1, err := s.state.AddMachine("saucy", JobHostUnits)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(m1.Id(), gc.Equals, "1")

	tw := newTestAllWatcher(s.state, c)
	defer tw.Stop()

	// Expect to see events for the already created machines first.
	deltas := tw.All(2)
	checkDeltasEqual(c, deltas, []multiwatcher.Delta{{
		Entity: &multiwatcher.MachineInfo{
			EnvUUID:    s.state.EnvironUUID(),
			Id:         "0",
			Status:     multiwatcher.Status("pending"),
			StatusData: map[string]interface{}{},
			Life:       multiwatcher.Life("alive"),
			Series:     "trusty",
			Jobs:       []multiwatcher.MachineJob{JobManageEnviron.ToParams()},
			Addresses:  []network.Address{},
			HasVote:    false,
			WantsVote:  true,
		},
	}, {
		Entity: &multiwatcher.MachineInfo{
			EnvUUID:    s.state.EnvironUUID(),
			Id:         "1",
			Status:     multiwatcher.Status("pending"),
			StatusData: map[string]interface{}{},
			Life:       multiwatcher.Life("alive"),
			Series:     "saucy",
			Jobs:       []multiwatcher.MachineJob{JobHostUnits.ToParams()},
			Addresses:  []network.Address{},
			HasVote:    false,
			WantsVote:  false,
		},
	}})

	// Destroy a machine and make sure that's seen.
	err = m1.Destroy()
	c.Assert(err, jc.ErrorIsNil)

	deltas = tw.All(1)
	zeroOutTimestampsForDeltas(c, deltas)
	checkDeltasEqual(c, deltas, []multiwatcher.Delta{{
		Entity: &multiwatcher.MachineInfo{
			EnvUUID:    s.state.EnvironUUID(),
			Id:         "1",
			Status:     multiwatcher.Status("pending"),
			StatusData: map[string]interface{}{},
			Life:       multiwatcher.Life("dying"),
			Series:     "saucy",
			Jobs:       []multiwatcher.MachineJob{JobHostUnits.ToParams()},
			Addresses:  []network.Address{},
			HasVote:    false,
			WantsVote:  false,
		},
	}})

	err = m1.EnsureDead()
	c.Assert(err, jc.ErrorIsNil)

	deltas = tw.All(1)
	zeroOutTimestampsForDeltas(c, deltas)
	checkDeltasEqual(c, deltas, []multiwatcher.Delta{{
		Entity: &multiwatcher.MachineInfo{
			EnvUUID:    s.state.EnvironUUID(),
			Id:         "1",
			Status:     multiwatcher.Status("pending"),
			StatusData: map[string]interface{}{},
			Life:       multiwatcher.Life("dead"),
			Series:     "saucy",
			Jobs:       []multiwatcher.MachineJob{JobHostUnits.ToParams()},
			Addresses:  []network.Address{},
			HasVote:    false,
			WantsVote:  false,
		},
	}})

	// Make some more changes to the state.
	arch := "amd64"
	mem := uint64(4096)
	hc := &instance.HardwareCharacteristics{
		Arch: &arch,
		Mem:  &mem,
	}
	err = m0.SetProvisioned("i-0", "bootstrap_nonce", hc)
	c.Assert(err, jc.ErrorIsNil)

	err = m1.Remove()
	c.Assert(err, jc.ErrorIsNil)

	m2, err := s.state.AddMachine("quantal", JobHostUnits)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(m2.Id(), gc.Equals, "2")

	wordpress := AddTestingService(c, s.state, "wordpress", AddTestingCharm(c, s.state, "wordpress"), s.owner)
	wu, err := wordpress.AddUnit()
	c.Assert(err, jc.ErrorIsNil)
	err = wu.AssignToMachine(m2)
	c.Assert(err, jc.ErrorIsNil)

	// Look for the state changes from the allwatcher.
	deltas = tw.All(5)

	zeroOutTimestampsForDeltas(c, deltas)

	checkDeltasEqual(c, deltas, []multiwatcher.Delta{{
		Entity: &multiwatcher.MachineInfo{
			EnvUUID:                 s.state.EnvironUUID(),
			Id:                      "0",
			InstanceId:              "i-0",
			Status:                  multiwatcher.Status("pending"),
			StatusData:              map[string]interface{}{},
			Life:                    multiwatcher.Life("alive"),
			Series:                  "trusty",
			Jobs:                    []multiwatcher.MachineJob{JobManageEnviron.ToParams()},
			Addresses:               []network.Address{},
			HardwareCharacteristics: hc,
			HasVote:                 false,
			WantsVote:               true,
		},
	}, {
		Removed: true,
		Entity: &multiwatcher.MachineInfo{
			EnvUUID: s.state.EnvironUUID(),
			Id:      "1",
		},
	}, {
		Entity: &multiwatcher.MachineInfo{
			EnvUUID:    s.state.EnvironUUID(),
			Id:         "2",
			Status:     multiwatcher.Status("pending"),
			StatusData: map[string]interface{}{},
			Life:       multiwatcher.Life("alive"),
			Series:     "quantal",
			Jobs:       []multiwatcher.MachineJob{JobHostUnits.ToParams()},
			Addresses:  []network.Address{},
			HasVote:    false,
			WantsVote:  false,
		},
	}, {
		Entity: &multiwatcher.ServiceInfo{
			EnvUUID:  s.state.EnvironUUID(),
			Name:     "wordpress",
			CharmURL: "local:quantal/quantal-wordpress-3",
			OwnerTag: s.owner.String(),
			Life:     "alive",
			Config:   make(map[string]interface{}),
			Status: multiwatcher.StatusInfo{
				Current: "unknown",
				Message: "Waiting for agent initialization to finish",
				Data:    map[string]interface{}{},
			},
		},
	}, {
		Entity: &multiwatcher.UnitInfo{
			EnvUUID:    s.state.EnvironUUID(),
			Name:       "wordpress/0",
			Service:    "wordpress",
			Series:     "quantal",
			MachineId:  "2",
			Status:     "pending",
			StatusData: map[string]interface{}{},
			WorkloadStatus: multiwatcher.StatusInfo{
				Current: "unknown",
				Message: "Waiting for agent initialization to finish",
				Data:    map[string]interface{}{},
			},
			AgentStatus: multiwatcher.StatusInfo{
				Current: "allocating",
				Message: "",
				Data:    map[string]interface{}{},
			},
		},
	}})
}

func (s *allWatcherStateSuite) TestStateWatcherTwoEnvironments(c *gc.C) {
	loggo.GetLogger("juju.state.watcher").SetLogLevel(loggo.TRACE)
	for i, test := range []struct {
		about        string
		setUpState   func(*State)
		triggerEvent func(*State)
	}{
		{
			about: "machines",
			triggerEvent: func(st *State) {
				m0, err := st.AddMachine("trusty", JobHostUnits)
				c.Assert(err, jc.ErrorIsNil)
				c.Assert(m0.Id(), gc.Equals, "0")
			},
		}, {
			about: "services",
			triggerEvent: func(st *State) {
				AddTestingService(c, st, "wordpress", AddTestingCharm(c, st, "wordpress"), s.owner)
			},
		}, {
			about: "units",
			setUpState: func(st *State) {
				AddTestingService(c, st, "wordpress", AddTestingCharm(c, st, "wordpress"), s.owner)
			},
			triggerEvent: func(st *State) {
				svc, err := st.Service("wordpress")
				c.Assert(err, jc.ErrorIsNil)

				_, err = svc.AddUnit()
				c.Assert(err, jc.ErrorIsNil)
			},
		}, {
			about: "relations",
			setUpState: func(st *State) {
				AddTestingService(c, st, "wordpress", AddTestingCharm(c, st, "wordpress"), s.owner)
				AddTestingService(c, st, "mysql", AddTestingCharm(c, st, "mysql"), s.owner)
			},
			triggerEvent: func(st *State) {
				eps, err := st.InferEndpoints("mysql", "wordpress")
				c.Assert(err, jc.ErrorIsNil)
				_, err = st.AddRelation(eps...)
				c.Assert(err, jc.ErrorIsNil)
			},
		}, {
			about: "annotations",
			setUpState: func(st *State) {
				m, err := st.AddMachine("trusty", JobHostUnits)
				c.Assert(err, jc.ErrorIsNil)
				c.Assert(m.Id(), gc.Equals, "0")
			},
			triggerEvent: func(st *State) {
				m, err := st.Machine("0")
				c.Assert(err, jc.ErrorIsNil)

				err = st.SetAnnotations(m, map[string]string{"foo": "bar"})
				c.Assert(err, jc.ErrorIsNil)
			},
		}, {
			about: "statuses",
			setUpState: func(st *State) {
				m, err := st.AddMachine("trusty", JobHostUnits)
				c.Assert(err, jc.ErrorIsNil)
				c.Assert(m.Id(), gc.Equals, "0")
				err = m.SetProvisioned("inst-id", "fake_nonce", nil)
				c.Assert(err, jc.ErrorIsNil)
			},
			triggerEvent: func(st *State) {
				m, err := st.Machine("0")
				c.Assert(err, jc.ErrorIsNil)

				err = m.SetStatus("error", "pete tong", nil)
				c.Assert(err, jc.ErrorIsNil)
			},
		}, {
			about: "constraints",
			setUpState: func(st *State) {
				AddTestingService(c, st, "wordpress", AddTestingCharm(c, st, "wordpress"), s.owner)
			},
			triggerEvent: func(st *State) {
				svc, err := st.Service("wordpress")
				c.Assert(err, jc.ErrorIsNil)

				cpuCores := uint64(99)
				err = svc.SetConstraints(constraints.Value{CpuCores: &cpuCores})
				c.Assert(err, jc.ErrorIsNil)
			},
		}, {
			about: "settings",
			setUpState: func(st *State) {
				AddTestingService(c, st, "wordpress", AddTestingCharm(c, st, "wordpress"), s.owner)
			},
			triggerEvent: func(st *State) {
				svc, err := st.Service("wordpress")
				c.Assert(err, jc.ErrorIsNil)

				err = svc.UpdateConfigSettings(charm.Settings{"blog-title": "boring"})
				c.Assert(err, jc.ErrorIsNil)
			},
		}, {
			about: "blocks",
			triggerEvent: func(st *State) {
				m, found, err := st.GetBlockForType(DestroyBlock)
				c.Assert(err, jc.ErrorIsNil)
				c.Assert(found, jc.IsFalse)
				c.Assert(m, gc.IsNil)

				err = st.SwitchBlockOn(DestroyBlock, "test block")
				c.Assert(err, jc.ErrorIsNil)
			},
		},
	} {
		c.Logf("Test %d: %s", i, test.about)
		func() {
			checkIsolationForEnv := func(st *State, w, otherW *testWatcher) {
				c.Logf("Making changes to environment %s", st.EnvironUUID())

				if test.setUpState != nil {
					test.setUpState(st)
					// Consume events from setup.
					w.AssertChanges()
					w.AssertNoChange()
					otherW.AssertNoChange()
				}

				test.triggerEvent(st)
				// Check event was isolated to the correct watcher.
				w.AssertChanges()
				w.AssertNoChange()
				otherW.AssertNoChange()
			}
			otherState := s.newState(c)

			w1 := newTestAllWatcher(s.state, c)
			defer w1.Stop()
			w2 := newTestAllWatcher(otherState, c)
			defer w2.Stop()

			checkIsolationForEnv(s.state, w1, w2)
			checkIsolationForEnv(otherState, w2, w1)
		}()
		s.Reset(c)
	}
}

var _ = gc.Suite(&allEnvWatcherStateSuite{})

type allEnvWatcherStateSuite struct {
	allWatcherBaseSuite
	state1 *State
}

func (s *allEnvWatcherStateSuite) SetUpTest(c *gc.C) {
	s.allWatcherBaseSuite.SetUpTest(c)
	s.state1 = s.newState(c)
}

func (s *allEnvWatcherStateSuite) Reset(c *gc.C) {
	s.TearDownTest(c)
	s.SetUpTest(c)
}

// performChangeTestCases runs a passed number of test cases for changes.
func (s *allEnvWatcherStateSuite) performChangeTestCases(c *gc.C, changeTestFuncs []changeTestFunc) {
	for i, changeTestFunc := range changeTestFuncs {
		func() { // in aid of per-loop defers
			defer s.Reset(c)

			test0 := changeTestFunc(c, s.state)

			c.Logf("test %d. %s", i, test0.about)
			b := newAllEnvWatcherStateBacking(s.state)
			defer b.Release()
			all := newStore()

			// Do updates and check for first env.
			for _, info := range test0.initialContents {
				all.Update(info)
			}
			err := b.Changed(all, test0.change)
			c.Assert(err, jc.ErrorIsNil)
			var entities entityInfoSlice = all.All()
			substNilSinceTimeForEntities(c, entities)
			assertEntitiesEqual(c, entities, test0.expectContents)

			// Now do the same updates for a second env.
			test1 := changeTestFunc(c, s.state1)
			for _, info := range test1.initialContents {
				all.Update(info)
			}
			err = b.Changed(all, test1.change)
			c.Assert(err, jc.ErrorIsNil)

			entities = all.All()

			// substNilSinceTimeForEntities gets upset if it sees non-nil
			// times - which the entities for the first env will have - so
			// build a list of the entities for the second env.
			newEntities := make([]multiwatcher.EntityInfo, 0)
			for _, entity := range entities {
				if entity.EntityId().EnvUUID == s.state1.EnvironUUID() {
					newEntities = append(newEntities, entity)
				}
			}
			substNilSinceTimeForEntities(c, newEntities)

			// Expected to see entities for both envs.
			var expectedEntities entityInfoSlice = append(
				test0.expectContents,
				test1.expectContents...)
			sort.Sort(entities)
			sort.Sort(expectedEntities)
			assertEntitiesEqual(c, entities, expectedEntities)
		}()
	}
}

func (s *allEnvWatcherStateSuite) TestChangeAnnotations(c *gc.C) {
	testChangeAnnotations(c, s.performChangeTestCases)
}

func (s *allEnvWatcherStateSuite) TestChangeMachines(c *gc.C) {
	testChangeMachines(c, s.performChangeTestCases)
}

func (s *allEnvWatcherStateSuite) TestChangeRelations(c *gc.C) {
	testChangeRelations(c, s.owner, s.performChangeTestCases)
}

func (s *allEnvWatcherStateSuite) TestChangeServices(c *gc.C) {
	testChangeServices(c, s.owner, s.performChangeTestCases)
}

func (s *allEnvWatcherStateSuite) TestChangeServicesConstraints(c *gc.C) {
	testChangeServicesConstraints(c, s.owner, s.performChangeTestCases)
}

func (s *allEnvWatcherStateSuite) TestChangeUnits(c *gc.C) {
	testChangeUnits(c, s.owner, s.performChangeTestCases)
}

func (s *allEnvWatcherStateSuite) TestChangeUnitsNonNilPorts(c *gc.C) {
	testChangeUnitsNonNilPorts(c, s.owner, s.performChangeTestCases)
}

func (s *allEnvWatcherStateSuite) TestChangeEnvironments(c *gc.C) {
	changeTestFuncs := []changeTestFunc{
		func(c *gc.C, st *State) changeTestCase {
			return changeTestCase{
				about: "no environment in state -> do nothing",
				change: watcher.Change{
					C:  "environments",
					Id: "non-existing-uuid",
				}}
		},
		func(c *gc.C, st *State) changeTestCase {
			return changeTestCase{
				about: "environment is removed if it's not in backing",
				initialContents: []multiwatcher.EntityInfo{&multiwatcher.EnvironmentInfo{
					EnvUUID: "some-uuid",
				}},
				change: watcher.Change{
					C:  "environments",
					Id: "some-uuid",
				}}
		},
		func(c *gc.C, st *State) changeTestCase {
			env, err := st.Environment()
			c.Assert(err, jc.ErrorIsNil)
			return changeTestCase{
				about: "environment is added if it's in backing but not in Store",
				change: watcher.Change{
					C:  "environments",
					Id: st.EnvironUUID(),
				},
				expectContents: []multiwatcher.EntityInfo{
					&multiwatcher.EnvironmentInfo{
						EnvUUID:    env.UUID(),
						Name:       env.Name(),
						Life:       multiwatcher.Life("alive"),
						Owner:      env.Owner().Id(),
						ServerUUID: env.ServerUUID(),
					}}}
		},
		func(c *gc.C, st *State) changeTestCase {
			env, err := st.Environment()
			c.Assert(err, jc.ErrorIsNil)
			return changeTestCase{
				about: "environment is updated if it's in backing and in Store",
				initialContents: []multiwatcher.EntityInfo{
					&multiwatcher.EnvironmentInfo{
						EnvUUID:    env.UUID(),
						Name:       "",
						Life:       multiwatcher.Life("alive"),
						Owner:      env.Owner().Id(),
						ServerUUID: env.ServerUUID(),
					},
				},
				change: watcher.Change{
					C:  "environments",
					Id: env.UUID(),
				},
				expectContents: []multiwatcher.EntityInfo{
					&multiwatcher.EnvironmentInfo{
						EnvUUID:    env.UUID(),
						Name:       env.Name(),
						Life:       multiwatcher.Life("alive"),
						Owner:      env.Owner().Id(),
						ServerUUID: env.ServerUUID(),
					}}}
		},
		func(c *gc.C, st *State) changeTestCase {
			svc := AddTestingService(c, st, "wordpress", AddTestingCharm(c, st, "wordpress"), s.owner)
			err := svc.SetConstraints(constraints.MustParse("mem=4G arch=amd64"))
			c.Assert(err, jc.ErrorIsNil)

			return changeTestCase{
				about: "status is changed if the service exists in the store",
				initialContents: []multiwatcher.EntityInfo{&multiwatcher.ServiceInfo{
					EnvUUID:     st.EnvironUUID(),
					Name:        "wordpress",
					Constraints: constraints.MustParse("mem=99M cpu-cores=2 cpu-power=4"),
				}},
				change: watcher.Change{
					C:  "constraints",
					Id: st.docID("s#wordpress"),
				},
				expectContents: []multiwatcher.EntityInfo{
					&multiwatcher.ServiceInfo{
						EnvUUID:     st.EnvironUUID(),
						Name:        "wordpress",
						Constraints: constraints.MustParse("mem=4G arch=amd64"),
					}}}
		},
	}
	s.performChangeTestCases(c, changeTestFuncs)
}

func (s *allEnvWatcherStateSuite) TestChangeForDeadEnv(c *gc.C) {
	// Ensure an entity is removed when a change is seen but
	// the environment the entity belonged to has already died.

	b := newAllEnvWatcherStateBacking(s.state)
	defer b.Release()
	all := newStore()

	// Insert a machine for an environment that doesn't actually
	// exist (mimics env removal).
	all.Update(&multiwatcher.MachineInfo{
		EnvUUID: "uuid",
		Id:      "0",
	})
	c.Assert(all.All(), gc.HasLen, 1)

	err := b.Changed(all, watcher.Change{
		C:  "machines",
		Id: ensureEnvUUID("uuid", "0"),
	})
	c.Assert(err, jc.ErrorIsNil)

	// Entity info should be gone now.
	c.Assert(all.All(), gc.HasLen, 0)
}

func (s *allEnvWatcherStateSuite) TestGetAll(c *gc.C) {
	// Set up 2 environments and ensure that GetAll returns the
	// entities for both of them.
	entities0 := s.setUpScenario(c, s.state, 2)
	entities1 := s.setUpScenario(c, s.state1, 4)
	expectedEntities := append(entities0, entities1...)

	// allEnvWatcherStateBacking also watches environments so add those in.
	env, err := s.state.Environment()
	c.Assert(err, jc.ErrorIsNil)
	env1, err := s.state1.Environment()
	c.Assert(err, jc.ErrorIsNil)
	expectedEntities = append(expectedEntities,
		&multiwatcher.EnvironmentInfo{
			EnvUUID:    env.UUID(),
			Name:       env.Name(),
			Life:       multiwatcher.Life("alive"),
			Owner:      env.Owner().Id(),
			ServerUUID: env.ServerUUID(),
		},
		&multiwatcher.EnvironmentInfo{
			EnvUUID:    env1.UUID(),
			Name:       env1.Name(),
			Life:       multiwatcher.Life("alive"),
			Owner:      env1.Owner().Id(),
			ServerUUID: env1.ServerUUID(),
		},
	)

	b := newAllEnvWatcherStateBacking(s.state)
	all := newStore()
	err = b.GetAll(all)
	c.Assert(err, jc.ErrorIsNil)
	var gotEntities entityInfoSlice = all.All()
	sort.Sort(gotEntities)
	sort.Sort(expectedEntities)
	substNilSinceTimeForEntities(c, gotEntities)
	assertEntitiesEqual(c, gotEntities, expectedEntities)
}

// TestStateWatcher tests the integration of the state watcher with
// allEnvWatcherStateBacking. Most of the logic is comprehensively
// tested elsewhere - this just tests end-to-end.
func (s *allEnvWatcherStateSuite) TestStateWatcher(c *gc.C) {
	st0 := s.state
	env0, err := st0.Environment()
	c.Assert(err, jc.ErrorIsNil)

	st1 := s.state1
	env1, err := st1.Environment()
	c.Assert(err, jc.ErrorIsNil)

	// Create some initial machines across 2 environments
	m00, err := st0.AddMachine("trusty", JobManageEnviron)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(m00.Id(), gc.Equals, "0")

	m10, err := st1.AddMachine("saucy", JobHostUnits)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(m10.Id(), gc.Equals, "0")

	tw := newTestAllEnvWatcher(st0, c)
	defer tw.Stop()

	// Expect to see events for the already created environments and
	// machines first.
	deltas := tw.All(4)
	checkDeltasEqual(c, deltas, []multiwatcher.Delta{{
		Entity: &multiwatcher.EnvironmentInfo{
			EnvUUID:    env0.UUID(),
			Name:       env0.Name(),
			Life:       "alive",
			Owner:      env0.Owner().Id(),
			ServerUUID: env0.ServerUUID(),
		},
	}, {
		Entity: &multiwatcher.EnvironmentInfo{
			EnvUUID:    env1.UUID(),
			Name:       env1.Name(),
			Life:       "alive",
			Owner:      env1.Owner().Id(),
			ServerUUID: env1.ServerUUID(),
		},
	}, {
		Entity: &multiwatcher.MachineInfo{
			EnvUUID:    st0.EnvironUUID(),
			Id:         "0",
			Status:     multiwatcher.Status("pending"),
			StatusData: map[string]interface{}{},
			Life:       multiwatcher.Life("alive"),
			Series:     "trusty",
			Jobs:       []multiwatcher.MachineJob{JobManageEnviron.ToParams()},
			Addresses:  []network.Address{},
			HasVote:    false,
			WantsVote:  true,
		},
	}, {
		Entity: &multiwatcher.MachineInfo{
			EnvUUID:    st1.EnvironUUID(),
			Id:         "0",
			Status:     multiwatcher.Status("pending"),
			StatusData: map[string]interface{}{},
			Life:       multiwatcher.Life("alive"),
			Series:     "saucy",
			Jobs:       []multiwatcher.MachineJob{JobHostUnits.ToParams()},
			Addresses:  []network.Address{},
			HasVote:    false,
			WantsVote:  false,
		},
	}})

	// Destroy a machine and make sure that's seen.
	err = m10.Destroy()
	c.Assert(err, jc.ErrorIsNil)

	deltas = tw.All(1)
	zeroOutTimestampsForDeltas(c, deltas)
	checkDeltasEqual(c, deltas, []multiwatcher.Delta{{
		Entity: &multiwatcher.MachineInfo{
			EnvUUID:    st1.EnvironUUID(),
			Id:         "0",
			Status:     multiwatcher.Status("pending"),
			StatusData: map[string]interface{}{},
			Life:       multiwatcher.Life("dying"),
			Series:     "saucy",
			Jobs:       []multiwatcher.MachineJob{JobHostUnits.ToParams()},
			Addresses:  []network.Address{},
			HasVote:    false,
			WantsVote:  false,
		},
	}})

	err = m10.EnsureDead()
	c.Assert(err, jc.ErrorIsNil)

	deltas = tw.All(1)
	zeroOutTimestampsForDeltas(c, deltas)
	checkDeltasEqual(c, deltas, []multiwatcher.Delta{{
		Entity: &multiwatcher.MachineInfo{
			EnvUUID:    st1.EnvironUUID(),
			Id:         "0",
			Status:     multiwatcher.Status("pending"),
			StatusData: map[string]interface{}{},
			Life:       multiwatcher.Life("dead"),
			Series:     "saucy",
			Jobs:       []multiwatcher.MachineJob{JobHostUnits.ToParams()},
			Addresses:  []network.Address{},
			HasVote:    false,
			WantsVote:  false,
		},
	}})

	// Make further changes to the state, including the addition of a
	// new environment.
	err = m00.SetProvisioned("i-0", "bootstrap_nonce", nil)
	c.Assert(err, jc.ErrorIsNil)

	err = m10.Remove()
	c.Assert(err, jc.ErrorIsNil)

	m11, err := st1.AddMachine("quantal", JobHostUnits)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(m11.Id(), gc.Equals, "1")

	wordpress := AddTestingService(c, st1, "wordpress", AddTestingCharm(c, st1, "wordpress"), s.owner)
	wu, err := wordpress.AddUnit()
	c.Assert(err, jc.ErrorIsNil)
	err = wu.AssignToMachine(m11)
	c.Assert(err, jc.ErrorIsNil)

	st2 := s.newState(c)
	env2, err := st2.Environment()
	c.Assert(err, jc.ErrorIsNil)

	m20, err := st2.AddMachine("trusty", JobHostUnits)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(m20.Id(), gc.Equals, "0")

	// Look for the state changes from the allwatcher.
	deltas = tw.All(7)
	zeroOutTimestampsForDeltas(c, deltas)

	checkDeltasEqual(c, deltas, []multiwatcher.Delta{{
		Entity: &multiwatcher.MachineInfo{
			EnvUUID:                 st0.EnvironUUID(),
			Id:                      "0",
			InstanceId:              "i-0",
			Status:                  multiwatcher.Status("pending"),
			StatusData:              map[string]interface{}{},
			Life:                    multiwatcher.Life("alive"),
			Series:                  "trusty",
			Jobs:                    []multiwatcher.MachineJob{JobManageEnviron.ToParams()},
			Addresses:               []network.Address{},
			HardwareCharacteristics: &instance.HardwareCharacteristics{},
			HasVote:                 false,
			WantsVote:               true,
		},
	}, {
		Removed: true,
		Entity: &multiwatcher.MachineInfo{
			EnvUUID: st1.EnvironUUID(),
			Id:      "0",
		},
	}, {
		Entity: &multiwatcher.MachineInfo{
			EnvUUID:    st1.EnvironUUID(),
			Id:         "1",
			Status:     multiwatcher.Status("pending"),
			StatusData: map[string]interface{}{},
			Life:       multiwatcher.Life("alive"),
			Series:     "quantal",
			Jobs:       []multiwatcher.MachineJob{JobHostUnits.ToParams()},
			Addresses:  []network.Address{},
			HasVote:    false,
			WantsVote:  false,
		},
	}, {
		Entity: &multiwatcher.ServiceInfo{
			EnvUUID:  st1.EnvironUUID(),
			Name:     "wordpress",
			CharmURL: "local:quantal/quantal-wordpress-3",
			OwnerTag: s.owner.String(),
			Life:     "alive",
			Config:   make(map[string]interface{}),
			Status: multiwatcher.StatusInfo{
				Current: "unknown",
				Message: "Waiting for agent initialization to finish",
				Data:    map[string]interface{}{},
			},
		},
	}, {
		Entity: &multiwatcher.UnitInfo{
			EnvUUID:    st1.EnvironUUID(),
			Name:       "wordpress/0",
			Service:    "wordpress",
			Series:     "quantal",
			MachineId:  "1",
			Status:     "pending",
			StatusData: map[string]interface{}{},
			WorkloadStatus: multiwatcher.StatusInfo{
				Current: "unknown",
				Message: "Waiting for agent initialization to finish",
				Data:    map[string]interface{}{},
			},
			AgentStatus: multiwatcher.StatusInfo{
				Current: "allocating",
				Message: "",
				Data:    map[string]interface{}{},
			},
		},
	}, {
		Entity: &multiwatcher.EnvironmentInfo{
			EnvUUID:    env2.UUID(),
			Name:       env2.Name(),
			Life:       "alive",
			Owner:      env2.Owner().Id(),
			ServerUUID: env2.ServerUUID(),
		},
	}, {
		Entity: &multiwatcher.MachineInfo{
			EnvUUID:    st2.EnvironUUID(),
			Id:         "0",
			Status:     multiwatcher.Status("pending"),
			StatusData: map[string]interface{}{},
			Life:       multiwatcher.Life("alive"),
			Series:     "trusty",
			Jobs:       []multiwatcher.MachineJob{JobHostUnits.ToParams()},
			Addresses:  []network.Address{},
			HasVote:    false,
			WantsVote:  false,
		},
	}})
}

func zeroOutTimestampsForDeltas(c *gc.C, deltas []multiwatcher.Delta) {
	for i, delta := range deltas {
		if unitInfo, ok := delta.Entity.(*multiwatcher.UnitInfo); ok {
			substNilSinceTimeForStatus(c, &unitInfo.WorkloadStatus)
			substNilSinceTimeForStatus(c, &unitInfo.AgentStatus)
			delta.Entity = unitInfo
		} else if serviceInfo, ok := delta.Entity.(*multiwatcher.ServiceInfo); ok {
			substNilSinceTimeForStatus(c, &serviceInfo.Status)
			delta.Entity = serviceInfo
		}
		deltas[i] = delta
	}
}

// The testChange* funcs are extracted so the test cases can be used
// to test both the allWatcher and allEnvWatcher.

func testChangeAnnotations(c *gc.C, runChangeTests func(*gc.C, []changeTestFunc)) {
	changeTestFuncs := []changeTestFunc{
		func(c *gc.C, st *State) changeTestCase {
			return changeTestCase{
				about: "no annotation in state, no annotation in store -> do nothing",
				change: watcher.Change{
					C:  "annotations",
					Id: st.docID("m#0"),
				}}
		},
		func(c *gc.C, st *State) changeTestCase {
			return changeTestCase{
				about: "annotation is removed if it's not in backing",
				initialContents: []multiwatcher.EntityInfo{&multiwatcher.AnnotationInfo{
					EnvUUID: st.EnvironUUID(),
					Tag:     "machine-0",
				}},
				change: watcher.Change{
					C:  "annotations",
					Id: st.docID("m#0"),
				}}
		},
		func(c *gc.C, st *State) changeTestCase {
			m, err := st.AddMachine("quantal", JobHostUnits)
			c.Assert(err, jc.ErrorIsNil)
			err = st.SetAnnotations(m, map[string]string{"foo": "bar", "arble": "baz"})
			c.Assert(err, jc.ErrorIsNil)

			return changeTestCase{
				about: "annotation is added if it's in backing but not in Store",
				change: watcher.Change{
					C:  "annotations",
					Id: st.docID("m#0"),
				},
				expectContents: []multiwatcher.EntityInfo{
					&multiwatcher.AnnotationInfo{
						EnvUUID:     st.EnvironUUID(),
						Tag:         "machine-0",
						Annotations: map[string]string{"foo": "bar", "arble": "baz"},
					}}}
		},
		func(c *gc.C, st *State) changeTestCase {
			m, err := st.AddMachine("quantal", JobHostUnits)
			c.Assert(err, jc.ErrorIsNil)
			err = st.SetAnnotations(m, map[string]string{
				"arble":  "khroomph",
				"pretty": "",
				"new":    "attr",
			})
			c.Assert(err, jc.ErrorIsNil)

			return changeTestCase{
				about: "annotation is updated if it's in backing and in multiwatcher.Store",
				initialContents: []multiwatcher.EntityInfo{&multiwatcher.AnnotationInfo{
					EnvUUID: st.EnvironUUID(),
					Tag:     "machine-0",
					Annotations: map[string]string{
						"arble":  "baz",
						"foo":    "bar",
						"pretty": "polly",
					},
				}},
				change: watcher.Change{
					C:  "annotations",
					Id: st.docID("m#0"),
				},
				expectContents: []multiwatcher.EntityInfo{
					&multiwatcher.AnnotationInfo{
						EnvUUID: st.EnvironUUID(),
						Tag:     "machine-0",
						Annotations: map[string]string{
							"arble": "khroomph",
							"new":   "attr",
						}}}}
		},
	}
	runChangeTests(c, changeTestFuncs)
}

func testChangeMachines(c *gc.C, runChangeTests func(*gc.C, []changeTestFunc)) {
	changeTestFuncs := []changeTestFunc{
		func(c *gc.C, st *State) changeTestCase {
			return changeTestCase{
				about: "no machine in state -> do nothing",
				change: watcher.Change{
					C:  "statuses",
					Id: st.docID("m#0"),
				}}
		},
		func(c *gc.C, st *State) changeTestCase {
			return changeTestCase{
				about: "no machine in state, no machine in store -> do nothing",
				change: watcher.Change{
					C:  "machines",
					Id: st.docID("1"),
				}}
		},
		func(c *gc.C, st *State) changeTestCase {
			return changeTestCase{
				about: "machine is removed if it's not in backing",
				initialContents: []multiwatcher.EntityInfo{&multiwatcher.MachineInfo{
					EnvUUID: st.EnvironUUID(),
					Id:      "1",
				}},
				change: watcher.Change{
					C:  "machines",
					Id: st.docID("1"),
				}}
		},
		func(c *gc.C, st *State) changeTestCase {
			m, err := st.AddMachine("quantal", JobHostUnits)
			c.Assert(err, jc.ErrorIsNil)
			err = m.SetStatus(StatusError, "failure", nil)
			c.Assert(err, jc.ErrorIsNil)

			return changeTestCase{
				about: "machine is added if it's in backing but not in Store",
				change: watcher.Change{
					C:  "machines",
					Id: st.docID("0"),
				},
				expectContents: []multiwatcher.EntityInfo{
					&multiwatcher.MachineInfo{
						EnvUUID:    st.EnvironUUID(),
						Id:         "0",
						Status:     multiwatcher.Status("error"),
						StatusInfo: "failure",
						StatusData: map[string]interface{}{},
						Life:       multiwatcher.Life("alive"),
						Series:     "quantal",
						Jobs:       []multiwatcher.MachineJob{JobHostUnits.ToParams()},
						Addresses:  []network.Address{},
						HasVote:    false,
						WantsVote:  false,
					}}}
		},
		func(c *gc.C, st *State) changeTestCase {
			m, err := st.AddMachine("trusty", JobHostUnits)
			c.Assert(err, jc.ErrorIsNil)
			err = m.SetProvisioned("i-0", "bootstrap_nonce", nil)
			c.Assert(err, jc.ErrorIsNil)
			err = m.SetSupportedContainers([]instance.ContainerType{instance.LXC})
			c.Assert(err, jc.ErrorIsNil)

			return changeTestCase{
				about: "machine is updated if it's in backing and in Store",
				initialContents: []multiwatcher.EntityInfo{
					&multiwatcher.MachineInfo{
						EnvUUID:    st.EnvironUUID(),
						Id:         "0",
						Status:     multiwatcher.Status("error"),
						StatusInfo: "another failure",
						StatusData: map[string]interface{}{},
					},
				},
				change: watcher.Change{
					C:  "machines",
					Id: st.docID("0"),
				},
				expectContents: []multiwatcher.EntityInfo{
					&multiwatcher.MachineInfo{
						EnvUUID:                  st.EnvironUUID(),
						Id:                       "0",
						InstanceId:               "i-0",
						Status:                   multiwatcher.Status("error"),
						StatusInfo:               "another failure",
						StatusData:               map[string]interface{}{},
						Life:                     multiwatcher.Life("alive"),
						Series:                   "trusty",
						Jobs:                     []multiwatcher.MachineJob{JobHostUnits.ToParams()},
						Addresses:                []network.Address{},
						HardwareCharacteristics:  &instance.HardwareCharacteristics{},
						SupportedContainers:      []instance.ContainerType{instance.LXC},
						SupportedContainersKnown: true,
					}}}
		},
		func(c *gc.C, st *State) changeTestCase {
			return changeTestCase{
				about: "no change if status is not in backing",
				initialContents: []multiwatcher.EntityInfo{&multiwatcher.MachineInfo{
					EnvUUID:    st.EnvironUUID(),
					Id:         "0",
					Status:     multiwatcher.Status("error"),
					StatusInfo: "failure",
					StatusData: map[string]interface{}{},
				}},
				change: watcher.Change{
					C:  "statuses",
					Id: st.docID("m#0"),
				},
				expectContents: []multiwatcher.EntityInfo{
					&multiwatcher.MachineInfo{
						EnvUUID:    st.EnvironUUID(),
						Id:         "0",
						Status:     multiwatcher.Status("error"),
						StatusInfo: "failure",
						StatusData: map[string]interface{}{},
					}}}
		},
		func(c *gc.C, st *State) changeTestCase {
			m, err := st.AddMachine("quantal", JobHostUnits)
			c.Assert(err, jc.ErrorIsNil)
			err = m.SetStatus(StatusStarted, "", nil)
			c.Assert(err, jc.ErrorIsNil)

			return changeTestCase{
				about: "status is changed if the machine exists in the store",
				initialContents: []multiwatcher.EntityInfo{&multiwatcher.MachineInfo{
					EnvUUID:    st.EnvironUUID(),
					Id:         "0",
					Status:     multiwatcher.Status("error"),
					StatusInfo: "failure",
					StatusData: map[string]interface{}{},
				}},
				change: watcher.Change{
					C:  "statuses",
					Id: st.docID("m#0"),
				},
				expectContents: []multiwatcher.EntityInfo{
					&multiwatcher.MachineInfo{
						EnvUUID:    st.EnvironUUID(),
						Id:         "0",
						Status:     multiwatcher.Status("started"),
						StatusData: make(map[string]interface{}),
					}}}
		},
	}
	runChangeTests(c, changeTestFuncs)
}

func testChangeRelations(c *gc.C, owner names.UserTag, runChangeTests func(*gc.C, []changeTestFunc)) {
	changeTestFuncs := []changeTestFunc{
		func(c *gc.C, st *State) changeTestCase {
			return changeTestCase{
				about: "no relation in state, no service in store -> do nothing",
				change: watcher.Change{
					C:  "relations",
					Id: st.docID("logging:logging-directory wordpress:logging-dir"),
				}}
		},
		func(c *gc.C, st *State) changeTestCase {
			return changeTestCase{
				about: "relation is removed if it's not in backing",
				initialContents: []multiwatcher.EntityInfo{&multiwatcher.RelationInfo{
					EnvUUID: st.EnvironUUID(),
					Key:     "logging:logging-directory wordpress:logging-dir",
				}},
				change: watcher.Change{
					C:  "relations",
					Id: st.docID("logging:logging-directory wordpress:logging-dir"),
				}}
		},
		func(c *gc.C, st *State) changeTestCase {
			AddTestingService(c, st, "wordpress", AddTestingCharm(c, st, "wordpress"), owner)
			AddTestingService(c, st, "logging", AddTestingCharm(c, st, "logging"), owner)
			eps, err := st.InferEndpoints("logging", "wordpress")
			c.Assert(err, jc.ErrorIsNil)
			_, err = st.AddRelation(eps...)
			c.Assert(err, jc.ErrorIsNil)

			return changeTestCase{
				about: "relation is added if it's in backing but not in Store",
				change: watcher.Change{
					C:  "relations",
					Id: st.docID("logging:logging-directory wordpress:logging-dir"),
				},
				expectContents: []multiwatcher.EntityInfo{
					&multiwatcher.RelationInfo{
						EnvUUID: st.EnvironUUID(),
						Key:     "logging:logging-directory wordpress:logging-dir",
						Endpoints: []multiwatcher.Endpoint{
							{ServiceName: "logging", Relation: charm.Relation{Name: "logging-directory", Role: "requirer", Interface: "logging", Optional: false, Limit: 1, Scope: "container"}},
							{ServiceName: "wordpress", Relation: charm.Relation{Name: "logging-dir", Role: "provider", Interface: "logging", Optional: false, Limit: 0, Scope: "container"}}},
					}}}
		},
	}
	runChangeTests(c, changeTestFuncs)
}

func testChangeServices(c *gc.C, owner names.UserTag, runChangeTests func(*gc.C, []changeTestFunc)) {
	// TODO(wallyworld) - add test for changing service status when that is implemented
	changeTestFuncs := []changeTestFunc{
		// Services.
		func(c *gc.C, st *State) changeTestCase {
			return changeTestCase{
				about: "no service in state, no service in store -> do nothing",
				change: watcher.Change{
					C:  "services",
					Id: st.docID("wordpress"),
				}}
		},
		func(c *gc.C, st *State) changeTestCase {
			return changeTestCase{
				about: "service is removed if it's not in backing",
				initialContents: []multiwatcher.EntityInfo{
					&multiwatcher.ServiceInfo{
						EnvUUID: st.EnvironUUID(),
						Name:    "wordpress",
					},
				},
				change: watcher.Change{
					C:  "services",
					Id: st.docID("wordpress"),
				}}
		},
		func(c *gc.C, st *State) changeTestCase {
			wordpress := AddTestingService(c, st, "wordpress", AddTestingCharm(c, st, "wordpress"), owner)
			err := wordpress.SetExposed()
			c.Assert(err, jc.ErrorIsNil)
			err = wordpress.SetMinUnits(42)
			c.Assert(err, jc.ErrorIsNil)

			return changeTestCase{
				about: "service is added if it's in backing but not in Store",
				change: watcher.Change{
					C:  "services",
					Id: st.docID("wordpress"),
				},
				expectContents: []multiwatcher.EntityInfo{
					&multiwatcher.ServiceInfo{
						EnvUUID:  st.EnvironUUID(),
						Name:     "wordpress",
						Exposed:  true,
						CharmURL: "local:quantal/quantal-wordpress-3",
						OwnerTag: owner.String(),
						Life:     multiwatcher.Life("alive"),
						MinUnits: 42,
						Config:   charm.Settings{},
						Status: multiwatcher.StatusInfo{
							Current: "unknown",
							Message: "Waiting for agent initialization to finish",
							Data:    map[string]interface{}{},
						},
					}}}
		},
		func(c *gc.C, st *State) changeTestCase {
			svc := AddTestingService(c, st, "wordpress", AddTestingCharm(c, st, "wordpress"), owner)
			setServiceConfigAttr(c, svc, "blog-title", "boring")

			return changeTestCase{
				about: "service is updated if it's in backing and in multiwatcher.Store",
				initialContents: []multiwatcher.EntityInfo{&multiwatcher.ServiceInfo{
					EnvUUID:     st.EnvironUUID(),
					Name:        "wordpress",
					Exposed:     true,
					CharmURL:    "local:quantal/quantal-wordpress-3",
					MinUnits:    47,
					Constraints: constraints.MustParse("mem=99M"),
					Config:      charm.Settings{"blog-title": "boring"},
				}},
				change: watcher.Change{
					C:  "services",
					Id: st.docID("wordpress"),
				},
				expectContents: []multiwatcher.EntityInfo{
					&multiwatcher.ServiceInfo{
						EnvUUID:     st.EnvironUUID(),
						Name:        "wordpress",
						CharmURL:    "local:quantal/quantal-wordpress-3",
						OwnerTag:    owner.String(),
						Life:        multiwatcher.Life("alive"),
						Constraints: constraints.MustParse("mem=99M"),
						Config:      charm.Settings{"blog-title": "boring"},
					}}}
		},
		func(c *gc.C, st *State) changeTestCase {
			svc := AddTestingService(c, st, "wordpress", AddTestingCharm(c, st, "wordpress"), owner)
			setServiceConfigAttr(c, svc, "blog-title", "boring")

			return changeTestCase{
				about: "service re-reads config when charm URL changes",
				initialContents: []multiwatcher.EntityInfo{&multiwatcher.ServiceInfo{
					EnvUUID: st.EnvironUUID(),
					Name:    "wordpress",
					// Note: CharmURL has a different revision number from
					// the wordpress revision in the testing repo.
					CharmURL: "local:quantal/quantal-wordpress-2",
					Config:   charm.Settings{"foo": "bar"},
				}},
				change: watcher.Change{
					C:  "services",
					Id: st.docID("wordpress"),
				},
				expectContents: []multiwatcher.EntityInfo{
					&multiwatcher.ServiceInfo{
						EnvUUID:  st.EnvironUUID(),
						Name:     "wordpress",
						CharmURL: "local:quantal/quantal-wordpress-3",
						OwnerTag: owner.String(),
						Life:     multiwatcher.Life("alive"),
						Config:   charm.Settings{"blog-title": "boring"},
					}}}
		},
		// Settings.
		func(c *gc.C, st *State) changeTestCase {
			return changeTestCase{
				about: "no service in state -> do nothing",
				change: watcher.Change{
					C:  "settings",
					Id: st.docID("s#dummy-service#local:quantal/quantal-dummy-1"),
				}}
		},
		func(c *gc.C, st *State) changeTestCase {
			return changeTestCase{
				about: "no change if service is not in backing",
				initialContents: []multiwatcher.EntityInfo{&multiwatcher.ServiceInfo{
					EnvUUID:  st.EnvironUUID(),
					Name:     "dummy-service",
					CharmURL: "local:quantal/quantal-dummy-1",
				}},
				change: watcher.Change{
					C:  "settings",
					Id: st.docID("s#dummy-service#local:quantal/quantal-dummy-1"),
				},
				expectContents: []multiwatcher.EntityInfo{&multiwatcher.ServiceInfo{
					EnvUUID:  st.EnvironUUID(),
					Name:     "dummy-service",
					CharmURL: "local:quantal/quantal-dummy-1",
				}}}
		},
		func(c *gc.C, st *State) changeTestCase {
			svc := AddTestingService(c, st, "dummy-service", AddTestingCharm(c, st, "dummy"), owner)
			setServiceConfigAttr(c, svc, "username", "foo")
			setServiceConfigAttr(c, svc, "outlook", "foo@bar")

			return changeTestCase{
				about: "service config is changed if service exists in the store with the same URL",
				initialContents: []multiwatcher.EntityInfo{&multiwatcher.ServiceInfo{
					EnvUUID:  st.EnvironUUID(),
					Name:     "dummy-service",
					CharmURL: "local:quantal/quantal-dummy-1",
				}},
				change: watcher.Change{
					C:  "settings",
					Id: st.docID("s#dummy-service#local:quantal/quantal-dummy-1"),
				},
				expectContents: []multiwatcher.EntityInfo{
					&multiwatcher.ServiceInfo{
						EnvUUID:  st.EnvironUUID(),
						Name:     "dummy-service",
						CharmURL: "local:quantal/quantal-dummy-1",
						Config:   charm.Settings{"username": "foo", "outlook": "foo@bar"},
					}}}
		},
		func(c *gc.C, st *State) changeTestCase {
			svc := AddTestingService(c, st, "dummy-service", AddTestingCharm(c, st, "dummy"), owner)
			setServiceConfigAttr(c, svc, "username", "foo")
			setServiceConfigAttr(c, svc, "outlook", "foo@bar")
			setServiceConfigAttr(c, svc, "username", nil)

			return changeTestCase{
				about: "service config is changed after removing of a setting",
				initialContents: []multiwatcher.EntityInfo{&multiwatcher.ServiceInfo{
					EnvUUID:  st.EnvironUUID(),
					Name:     "dummy-service",
					CharmURL: "local:quantal/quantal-dummy-1",
					Config:   charm.Settings{"username": "foo", "outlook": "foo@bar"},
				}},
				change: watcher.Change{
					C:  "settings",
					Id: st.docID("s#dummy-service#local:quantal/quantal-dummy-1"),
				},
				expectContents: []multiwatcher.EntityInfo{
					&multiwatcher.ServiceInfo{
						EnvUUID:  st.EnvironUUID(),
						Name:     "dummy-service",
						CharmURL: "local:quantal/quantal-dummy-1",
						Config:   charm.Settings{"outlook": "foo@bar"},
					}}}
		},
		func(c *gc.C, st *State) changeTestCase {
			testCharm := AddCustomCharm(
				c, st, "dummy",
				"config.yaml", dottedConfig,
				"quantal", 1)
			svc := AddTestingService(c, st, "dummy-service", testCharm, owner)
			setServiceConfigAttr(c, svc, "key.dotted", "foo")

			return changeTestCase{
				about: "service config is unescaped when reading from the backing store",
				initialContents: []multiwatcher.EntityInfo{&multiwatcher.ServiceInfo{
					EnvUUID:  st.EnvironUUID(),
					Name:     "dummy-service",
					CharmURL: "local:quantal/quantal-dummy-1",
					Config:   charm.Settings{"key.dotted": "bar"},
				}},
				change: watcher.Change{
					C:  "settings",
					Id: st.docID("s#dummy-service#local:quantal/quantal-dummy-1"),
				},
				expectContents: []multiwatcher.EntityInfo{
					&multiwatcher.ServiceInfo{
						EnvUUID:  st.EnvironUUID(),
						Name:     "dummy-service",
						CharmURL: "local:quantal/quantal-dummy-1",
						Config:   charm.Settings{"key.dotted": "foo"},
					}}}
		},
		func(c *gc.C, st *State) changeTestCase {
			svc := AddTestingService(c, st, "dummy-service", AddTestingCharm(c, st, "dummy"), owner)
			setServiceConfigAttr(c, svc, "username", "foo")

			return changeTestCase{
				about: "service config is unchanged if service exists in the store with a different URL",
				initialContents: []multiwatcher.EntityInfo{&multiwatcher.ServiceInfo{
					EnvUUID:  st.EnvironUUID(),
					Name:     "dummy-service",
					CharmURL: "local:quantal/quantal-dummy-2", // Note different revno.
					Config:   charm.Settings{"username": "bar"},
				}},
				change: watcher.Change{
					C:  "settings",
					Id: st.docID("s#dummy-service#local:quantal/quantal-dummy-1"),
				},
				expectContents: []multiwatcher.EntityInfo{
					&multiwatcher.ServiceInfo{
						EnvUUID:  st.EnvironUUID(),
						Name:     "dummy-service",
						CharmURL: "local:quantal/quantal-dummy-2",
						Config:   charm.Settings{"username": "bar"},
					}}}
		},
		func(c *gc.C, st *State) changeTestCase {
			return changeTestCase{
				about: "non-service config change is ignored",
				change: watcher.Change{
					C:  "settings",
					Id: st.docID("m#0"),
				}}
		},
		func(c *gc.C, st *State) changeTestCase {
			return changeTestCase{
				about: "service config change with no charm url is ignored",
				change: watcher.Change{
					C:  "settings",
					Id: st.docID("s#foo"),
				}}
		},
	}
	runChangeTests(c, changeTestFuncs)
}

func testChangeServicesConstraints(c *gc.C, owner names.UserTag, runChangeTests func(*gc.C, []changeTestFunc)) {
	changeTestFuncs := []changeTestFunc{
		func(c *gc.C, st *State) changeTestCase {
			return changeTestCase{
				about: "no service in state -> do nothing",
				change: watcher.Change{
					C:  "constraints",
					Id: st.docID("s#wordpress"),
				}}
		},
		func(c *gc.C, st *State) changeTestCase {
			return changeTestCase{
				about: "no change if service is not in backing",
				initialContents: []multiwatcher.EntityInfo{&multiwatcher.ServiceInfo{
					EnvUUID:     st.EnvironUUID(),
					Name:        "wordpress",
					Constraints: constraints.MustParse("mem=99M"),
				}},
				change: watcher.Change{
					C:  "constraints",
					Id: st.docID("s#wordpress"),
				},
				expectContents: []multiwatcher.EntityInfo{&multiwatcher.ServiceInfo{
					EnvUUID:     st.EnvironUUID(),
					Name:        "wordpress",
					Constraints: constraints.MustParse("mem=99M"),
				}}}
		},
		func(c *gc.C, st *State) changeTestCase {
			svc := AddTestingService(c, st, "wordpress", AddTestingCharm(c, st, "wordpress"), owner)
			err := svc.SetConstraints(constraints.MustParse("mem=4G arch=amd64"))
			c.Assert(err, jc.ErrorIsNil)

			return changeTestCase{
				about: "status is changed if the service exists in the store",
				initialContents: []multiwatcher.EntityInfo{&multiwatcher.ServiceInfo{
					EnvUUID:     st.EnvironUUID(),
					Name:        "wordpress",
					Constraints: constraints.MustParse("mem=99M cpu-cores=2 cpu-power=4"),
				}},
				change: watcher.Change{
					C:  "constraints",
					Id: st.docID("s#wordpress"),
				},
				expectContents: []multiwatcher.EntityInfo{
					&multiwatcher.ServiceInfo{
						EnvUUID:     st.EnvironUUID(),
						Name:        "wordpress",
						Constraints: constraints.MustParse("mem=4G arch=amd64"),
					}}}
		},
	}
	runChangeTests(c, changeTestFuncs)
}

func testChangeUnits(c *gc.C, owner names.UserTag, runChangeTests func(*gc.C, []changeTestFunc)) {
	now := time.Now()
	changeTestFuncs := []changeTestFunc{
		func(c *gc.C, st *State) changeTestCase {
			return changeTestCase{
				about: "no unit in state, no unit in store -> do nothing",
				change: watcher.Change{
					C:  "units",
					Id: st.docID("1"),
				}}
		},
		func(c *gc.C, st *State) changeTestCase {
			return changeTestCase{
				about: "unit is removed if it's not in backing",
				initialContents: []multiwatcher.EntityInfo{
					&multiwatcher.UnitInfo{
						EnvUUID: st.EnvironUUID(),
						Name:    "wordpress/1",
					},
				},
				change: watcher.Change{
					C:  "units",
					Id: st.docID("wordpress/1"),
				}}
		},
		func(c *gc.C, st *State) changeTestCase {
			wordpress := AddTestingService(c, st, "wordpress", AddTestingCharm(c, st, "wordpress"), owner)
			u, err := wordpress.AddUnit()
			c.Assert(err, jc.ErrorIsNil)
			m, err := st.AddMachine("quantal", JobHostUnits)
			c.Assert(err, jc.ErrorIsNil)
			err = u.AssignToMachine(m)
			c.Assert(err, jc.ErrorIsNil)
			err = u.OpenPort("tcp", 12345)
			c.Assert(err, jc.ErrorIsNil)
			err = u.OpenPort("udp", 54321)
			c.Assert(err, jc.ErrorIsNil)
			err = u.OpenPorts("tcp", 5555, 5558)
			c.Assert(err, jc.ErrorIsNil)
			err = u.SetAgentStatus(StatusError, "failure", nil)
			c.Assert(err, jc.ErrorIsNil)

			return changeTestCase{
				about: "unit is added if it's in backing but not in Store",
				change: watcher.Change{
					C:  "units",
					Id: st.docID("wordpress/0"),
				},
				expectContents: []multiwatcher.EntityInfo{
					&multiwatcher.UnitInfo{
						EnvUUID:   st.EnvironUUID(),
						Name:      "wordpress/0",
						Service:   "wordpress",
						Series:    "quantal",
						MachineId: "0",
						Ports: []network.Port{
							{"tcp", 5555},
							{"tcp", 5556},
							{"tcp", 5557},
							{"tcp", 5558},
							{"tcp", 12345},
							{"udp", 54321},
						},
						PortRanges: []network.PortRange{
							{5555, 5558, "tcp"},
							{12345, 12345, "tcp"},
							{54321, 54321, "udp"},
						},
						Status:     multiwatcher.Status("error"),
						StatusInfo: "failure",
						StatusData: map[string]interface{}{},
						AgentStatus: multiwatcher.StatusInfo{
							Current: "idle",
							Message: "",
							Data:    map[string]interface{}{},
						},
						WorkloadStatus: multiwatcher.StatusInfo{
							Current: "error",
							Message: "failure",
							Data:    map[string]interface{}{},
						},
					}}}
		},
		func(c *gc.C, st *State) changeTestCase {
			wordpress := AddTestingService(c, st, "wordpress", AddTestingCharm(c, st, "wordpress"), owner)
			u, err := wordpress.AddUnit()
			c.Assert(err, jc.ErrorIsNil)
			m, err := st.AddMachine("quantal", JobHostUnits)
			c.Assert(err, jc.ErrorIsNil)
			err = u.AssignToMachine(m)
			c.Assert(err, jc.ErrorIsNil)
			err = u.OpenPort("udp", 17070)
			c.Assert(err, jc.ErrorIsNil)

			return changeTestCase{
				about: "unit is updated if it's in backing and in multiwatcher.Store",
				initialContents: []multiwatcher.EntityInfo{&multiwatcher.UnitInfo{
					EnvUUID:    st.EnvironUUID(),
					Name:       "wordpress/0",
					Status:     multiwatcher.Status("error"),
					StatusInfo: "another failure",
					StatusData: map[string]interface{}{},
					AgentStatus: multiwatcher.StatusInfo{
						Current: "idle",
						Message: "",
						Data:    map[string]interface{}{},
						Since:   &now,
					},
					WorkloadStatus: multiwatcher.StatusInfo{
						Current: "error",
						Message: "another failure",
						Data:    map[string]interface{}{},
						Since:   &now,
					},
					Ports:      []network.Port{{"udp", 17070}},
					PortRanges: []network.PortRange{{17070, 17070, "udp"}},
				}},
				change: watcher.Change{
					C:  "units",
					Id: st.docID("wordpress/0"),
				},
				expectContents: []multiwatcher.EntityInfo{
					&multiwatcher.UnitInfo{
						EnvUUID:    st.EnvironUUID(),
						Name:       "wordpress/0",
						Service:    "wordpress",
						Series:     "quantal",
						MachineId:  "0",
						Ports:      []network.Port{{"udp", 17070}},
						PortRanges: []network.PortRange{{17070, 17070, "udp"}},
						Status:     multiwatcher.Status("error"),
						StatusInfo: "another failure",
						StatusData: map[string]interface{}{},
						AgentStatus: multiwatcher.StatusInfo{
							Current: "idle",
							Message: "",
							Data:    map[string]interface{}{},
						},
						WorkloadStatus: multiwatcher.StatusInfo{
							Current: "error",
							Message: "another failure",
							Data:    map[string]interface{}{},
						},
					}}}
		},
		func(c *gc.C, st *State) changeTestCase {
			wordpress := AddTestingService(c, st, "wordpress", AddTestingCharm(c, st, "wordpress"), owner)
			u, err := wordpress.AddUnit()
			c.Assert(err, jc.ErrorIsNil)
			m, err := st.AddMachine("quantal", JobHostUnits)
			c.Assert(err, jc.ErrorIsNil)
			err = u.AssignToMachine(m)
			c.Assert(err, jc.ErrorIsNil)
			err = u.OpenPort("tcp", 4242)
			c.Assert(err, jc.ErrorIsNil)

			return changeTestCase{
				about: "unit info is updated if a port is opened on the machine it is placed in",
				initialContents: []multiwatcher.EntityInfo{
					&multiwatcher.UnitInfo{
						EnvUUID:    st.EnvironUUID(),
						Name:       "wordpress/0",
						StatusData: map[string]interface{}{},
					},
					&multiwatcher.MachineInfo{
						EnvUUID:    st.EnvironUUID(),
						Id:         "0",
						StatusData: map[string]interface{}{},
					},
				},
				change: watcher.Change{
					C:  openedPortsC,
					Id: st.docID("m#0#n#juju-public"),
				},
				expectContents: []multiwatcher.EntityInfo{
					&multiwatcher.UnitInfo{
						EnvUUID:    st.EnvironUUID(),
						Name:       "wordpress/0",
						Ports:      []network.Port{{"tcp", 4242}},
						PortRanges: []network.PortRange{{4242, 4242, "tcp"}},
						StatusData: map[string]interface{}{},
					},
					&multiwatcher.MachineInfo{
						EnvUUID:    st.EnvironUUID(),
						Id:         "0",
						StatusData: map[string]interface{}{},
					},
				}}
		},
		func(c *gc.C, st *State) changeTestCase {
			wordpress := AddTestingService(c, st, "wordpress", AddTestingCharm(c, st, "wordpress"), owner)
			u, err := wordpress.AddUnit()
			c.Assert(err, jc.ErrorIsNil)
			m, err := st.AddMachine("quantal", JobHostUnits)
			c.Assert(err, jc.ErrorIsNil)
			err = u.AssignToMachine(m)
			c.Assert(err, jc.ErrorIsNil)
			err = u.OpenPorts("tcp", 21, 22)
			c.Assert(err, jc.ErrorIsNil)

			return changeTestCase{
				about: "unit is created if a port is opened on the machine it is placed in",
				initialContents: []multiwatcher.EntityInfo{
					&multiwatcher.MachineInfo{
						EnvUUID: st.EnvironUUID(),
						Id:      "0",
					},
				},
				change: watcher.Change{
					C:  "units",
					Id: st.docID("wordpress/0"),
				},
				expectContents: []multiwatcher.EntityInfo{
					&multiwatcher.UnitInfo{
						EnvUUID:    st.EnvironUUID(),
						Name:       "wordpress/0",
						Service:    "wordpress",
						Series:     "quantal",
						MachineId:  "0",
						Status:     "pending",
						StatusData: map[string]interface{}{},
						WorkloadStatus: multiwatcher.StatusInfo{
							Current: "unknown",
							Message: "Waiting for agent initialization to finish",
							Data:    map[string]interface{}{},
						},
						AgentStatus: multiwatcher.StatusInfo{
							Current: "allocating",
							Data:    map[string]interface{}{},
						},
						Ports:      []network.Port{{"tcp", 21}, {"tcp", 22}},
						PortRanges: []network.PortRange{{21, 22, "tcp"}},
					},
					&multiwatcher.MachineInfo{
						EnvUUID: st.EnvironUUID(),
						Id:      "0",
					},
				}}
		},
		func(c *gc.C, st *State) changeTestCase {
			wordpress := AddTestingService(c, st, "wordpress", AddTestingCharm(c, st, "wordpress"), owner)
			u, err := wordpress.AddUnit()
			c.Assert(err, jc.ErrorIsNil)
			m, err := st.AddMachine("quantal", JobHostUnits)
			c.Assert(err, jc.ErrorIsNil)
			err = u.AssignToMachine(m)
			c.Assert(err, jc.ErrorIsNil)
			err = u.OpenPort("tcp", 12345)
			c.Assert(err, jc.ErrorIsNil)
			publicAddress := network.NewScopedAddress("public", network.ScopePublic)
			privateAddress := network.NewScopedAddress("private", network.ScopeCloudLocal)
			err = m.SetProviderAddresses(publicAddress, privateAddress)
			c.Assert(err, jc.ErrorIsNil)
			err = u.SetAgentStatus(StatusError, "failure", nil)
			c.Assert(err, jc.ErrorIsNil)

			return changeTestCase{
				about: "unit addresses are read from the assigned machine for recent Juju releases",
				change: watcher.Change{
					C:  "units",
					Id: st.docID("wordpress/0"),
				},
				expectContents: []multiwatcher.EntityInfo{
					&multiwatcher.UnitInfo{
						EnvUUID:        st.EnvironUUID(),
						Name:           "wordpress/0",
						Service:        "wordpress",
						Series:         "quantal",
						PublicAddress:  "public",
						PrivateAddress: "private",
						MachineId:      "0",
						Ports:          []network.Port{{"tcp", 12345}},
						PortRanges:     []network.PortRange{{12345, 12345, "tcp"}},
						Status:         multiwatcher.Status("error"),
						StatusInfo:     "failure",
						StatusData:     map[string]interface{}{},
						AgentStatus: multiwatcher.StatusInfo{
							Current: "idle",
							Message: "",
							Data:    map[string]interface{}{},
						},
						WorkloadStatus: multiwatcher.StatusInfo{
							Current: "error",
							Message: "failure",
							Data:    map[string]interface{}{},
						},
					}}}
		},
		func(c *gc.C, st *State) changeTestCase {
			return changeTestCase{
				about: "no unit in state -> do nothing",
				change: watcher.Change{
					C:  "statuses",
					Id: st.docID("u#wordpress/0"),
				}}
		},
		func(c *gc.C, st *State) changeTestCase {
			return changeTestCase{
				about: "no change if status is not in backing",
				initialContents: []multiwatcher.EntityInfo{&multiwatcher.UnitInfo{
					EnvUUID:    st.EnvironUUID(),
					Name:       "wordpress/0",
					Service:    "wordpress",
					Status:     multiwatcher.Status("error"),
					StatusInfo: "failure",
					StatusData: map[string]interface{}{},
					AgentStatus: multiwatcher.StatusInfo{
						Current: "idle",
						Message: "",
						Data:    map[string]interface{}{},
						Since:   &now,
					},
					WorkloadStatus: multiwatcher.StatusInfo{
						Current: "error",
						Message: "failure",
						Data:    map[string]interface{}{},
						Since:   &now,
					},
				}},
				change: watcher.Change{
					C:  "statuses",
					Id: st.docID("u#wordpress/0"),
				},
				expectContents: []multiwatcher.EntityInfo{
					&multiwatcher.UnitInfo{
						EnvUUID:    st.EnvironUUID(),
						Name:       "wordpress/0",
						Service:    "wordpress",
						Status:     multiwatcher.Status("error"),
						StatusInfo: "failure",
						StatusData: map[string]interface{}{},
						AgentStatus: multiwatcher.StatusInfo{
							Current: "idle",
							Message: "",
							Data:    map[string]interface{}{},
						},
						WorkloadStatus: multiwatcher.StatusInfo{
							Current: "error",
							Message: "failure",
							Data:    map[string]interface{}{},
						},
					}}}
		},
		func(c *gc.C, st *State) changeTestCase {
			wordpress := AddTestingService(c, st, "wordpress", AddTestingCharm(c, st, "wordpress"), owner)
			u, err := wordpress.AddUnit()
			c.Assert(err, jc.ErrorIsNil)
			err = u.SetAgentStatus(StatusIdle, "", nil)
			c.Assert(err, jc.ErrorIsNil)

			return changeTestCase{
				about: "status is changed if the unit exists in the store",
				initialContents: []multiwatcher.EntityInfo{&multiwatcher.UnitInfo{
					EnvUUID:    st.EnvironUUID(),
					Name:       "wordpress/0",
					Service:    "wordpress",
					Status:     multiwatcher.Status("started"),
					StatusInfo: "",
					StatusData: map[string]interface{}{},
					AgentStatus: multiwatcher.StatusInfo{
						Current: "idle",
						Message: "",
						Data:    map[string]interface{}{},
						Since:   &now,
					},
					WorkloadStatus: multiwatcher.StatusInfo{
						Current: "maintenance",
						Message: "working",
						Data:    map[string]interface{}{},
						Since:   &now,
					},
				}},
				change: watcher.Change{
					C:  "statuses",
					Id: st.docID("u#wordpress/0"),
				},
				expectContents: []multiwatcher.EntityInfo{
					&multiwatcher.UnitInfo{
						EnvUUID:    st.EnvironUUID(),
						Name:       "wordpress/0",
						Service:    "wordpress",
						Status:     multiwatcher.Status("started"),
						StatusInfo: "",
						StatusData: make(map[string]interface{}),
						WorkloadStatus: multiwatcher.StatusInfo{
							Current: "maintenance",
							Message: "working",
							Data:    map[string]interface{}{},
						},
						AgentStatus: multiwatcher.StatusInfo{
							Current: "idle",
							Message: "",
							Data:    map[string]interface{}{},
						},
					}}}
		},
		func(c *gc.C, st *State) changeTestCase {
			wordpress := AddTestingService(c, st, "wordpress", AddTestingCharm(c, st, "wordpress"), owner)
			u, err := wordpress.AddUnit()
			c.Assert(err, jc.ErrorIsNil)
			err = u.SetAgentStatus(StatusIdle, "", nil)
			c.Assert(err, jc.ErrorIsNil)
			err = u.SetStatus(StatusMaintenance, "doing work", nil)
			c.Assert(err, jc.ErrorIsNil)

			return changeTestCase{
				about: "unit status is changed if the agent comes off error state",
				initialContents: []multiwatcher.EntityInfo{&multiwatcher.UnitInfo{
					EnvUUID:    st.EnvironUUID(),
					Name:       "wordpress/0",
					Service:    "wordpress",
					Status:     multiwatcher.Status("error"),
					StatusInfo: "failure",
					AgentStatus: multiwatcher.StatusInfo{
						Current: "idle",
						Message: "",
						Data:    map[string]interface{}{},
						Since:   &now,
					},
					WorkloadStatus: multiwatcher.StatusInfo{
						Current: "error",
						Message: "failure",
						Data:    map[string]interface{}{},
						Since:   &now,
					},
				}},
				change: watcher.Change{
					C:  "statuses",
					Id: st.docID("u#wordpress/0"),
				},
				expectContents: []multiwatcher.EntityInfo{
					&multiwatcher.UnitInfo{
						EnvUUID:    st.EnvironUUID(),
						Name:       "wordpress/0",
						Service:    "wordpress",
						Status:     multiwatcher.Status("started"),
						StatusInfo: "",
						StatusData: make(map[string]interface{}),
						WorkloadStatus: multiwatcher.StatusInfo{
							Current: "maintenance",
							Message: "doing work",
							Data:    map[string]interface{}{},
						},
						AgentStatus: multiwatcher.StatusInfo{
							Current: "idle",
							Message: "",
							Data:    map[string]interface{}{},
						},
					}}}
		},
		func(c *gc.C, st *State) changeTestCase {
			wordpress := AddTestingService(c, st, "wordpress", AddTestingCharm(c, st, "wordpress"), owner)
			u, err := wordpress.AddUnit()
			c.Assert(err, jc.ErrorIsNil)
			err = u.SetAgentStatus(StatusError, "hook error", map[string]interface{}{
				"1st-key": "one",
				"2nd-key": 2,
				"3rd-key": true,
			})
			c.Assert(err, jc.ErrorIsNil)

			return changeTestCase{
				about: "status is changed with additional status data",
				initialContents: []multiwatcher.EntityInfo{&multiwatcher.UnitInfo{
					EnvUUID: st.EnvironUUID(),
					Name:    "wordpress/0",
					Service: "wordpress",
					Status:  multiwatcher.Status("started"),
					AgentStatus: multiwatcher.StatusInfo{
						Current: "idle",
						Message: "",
						Data:    map[string]interface{}{},
						Since:   &now,
					},
					WorkloadStatus: multiwatcher.StatusInfo{
						Current: "active",
						Since:   &now,
					},
				}},
				change: watcher.Change{
					C:  "statuses",
					Id: st.docID("u#wordpress/0"),
				},
				expectContents: []multiwatcher.EntityInfo{
					&multiwatcher.UnitInfo{
						EnvUUID:    st.EnvironUUID(),
						Name:       "wordpress/0",
						Service:    "wordpress",
						Status:     multiwatcher.Status("error"),
						StatusInfo: "hook error",
						StatusData: map[string]interface{}{
							"1st-key": "one",
							"2nd-key": 2,
							"3rd-key": true,
						},
						WorkloadStatus: multiwatcher.StatusInfo{
							Current: "error",
							Message: "hook error",
							Data: map[string]interface{}{
								"1st-key": "one",
								"2nd-key": 2,
								"3rd-key": true,
							},
						},
						AgentStatus: multiwatcher.StatusInfo{
							Current: "idle",
							Message: "",
							Data:    map[string]interface{}{},
						},
					}}}
		},
		func(c *gc.C, st *State) changeTestCase {
			wordpress := AddTestingService(c, st, "wordpress", AddTestingCharm(c, st, "wordpress"), owner)
			u, err := wordpress.AddUnit()
			c.Assert(err, jc.ErrorIsNil)
			err = u.SetStatus(StatusActive, "", nil)
			c.Assert(err, jc.ErrorIsNil)

			return changeTestCase{
				about: "service status is changed if the unit status changes",
				initialContents: []multiwatcher.EntityInfo{
					&multiwatcher.UnitInfo{
						EnvUUID: st.EnvironUUID(),
						Name:    "wordpress/0",
						Service: "wordpress",
						Status:  multiwatcher.Status("error"),
						AgentStatus: multiwatcher.StatusInfo{
							Current: "idle",
							Message: "",
							Data:    map[string]interface{}{},
							Since:   &now,
						},
						WorkloadStatus: multiwatcher.StatusInfo{
							Current: "error",
							Message: "failure",
							Data:    map[string]interface{}{},
							Since:   &now,
						},
					},
					&multiwatcher.ServiceInfo{
						EnvUUID: st.EnvironUUID(),
						Name:    "wordpress",
						Status: multiwatcher.StatusInfo{
							Current: "error",
							Message: "failure",
							Data:    map[string]interface{}{},
							Since:   &now,
						},
					},
				},
				change: watcher.Change{
					C:  "statuses",
					Id: st.docID("u#wordpress/0#charm"),
				},
				expectContents: []multiwatcher.EntityInfo{
					&multiwatcher.UnitInfo{
						EnvUUID:    st.EnvironUUID(),
						Name:       "wordpress/0",
						Service:    "wordpress",
						Status:     multiwatcher.Status("started"),
						StatusData: map[string]interface{}{},
						WorkloadStatus: multiwatcher.StatusInfo{
							Current: "active",
							Message: "",
							Data:    map[string]interface{}{},
						},
						AgentStatus: multiwatcher.StatusInfo{
							Current: "idle",
							Message: "",
							Data:    map[string]interface{}{},
						},
					},
					&multiwatcher.ServiceInfo{
						EnvUUID: st.EnvironUUID(),
						Name:    "wordpress",
						Status: multiwatcher.StatusInfo{
							Current: "active",
							Message: "",
							Data:    map[string]interface{}{},
						},
					},
				}}
		},
	}
	runChangeTests(c, changeTestFuncs)
}

// initFlag helps to control the different test scenarios.
type initFlag int

const (
	noFlag     initFlag = 0
	assignUnit initFlag = 1
	openPorts  initFlag = 2
	closePorts initFlag = 4
)

func testChangeUnitsNonNilPorts(c *gc.C, owner names.UserTag, runChangeTests func(*gc.C, []changeTestFunc)) {
	initEnv := func(c *gc.C, st *State, flag initFlag) {
		wordpress := AddTestingService(c, st, "wordpress", AddTestingCharm(c, st, "wordpress"), owner)
		u, err := wordpress.AddUnit()
		c.Assert(err, jc.ErrorIsNil)
		m, err := st.AddMachine("quantal", JobHostUnits)
		c.Assert(err, jc.ErrorIsNil)
		if flag&assignUnit != 0 {
			// Assign the unit.
			err = u.AssignToMachine(m)
			c.Assert(err, jc.ErrorIsNil)
		}
		if flag&openPorts != 0 {
			// Add a network to the machine and open a port.
			publicAddress := network.NewScopedAddress("1.2.3.4", network.ScopePublic)
			privateAddress := network.NewScopedAddress("4.3.2.1", network.ScopeCloudLocal)
			err = m.SetProviderAddresses(publicAddress, privateAddress)
			c.Assert(err, jc.ErrorIsNil)
			err = u.OpenPort("tcp", 12345)
			if flag&assignUnit != 0 {
				c.Assert(err, jc.ErrorIsNil)
			} else {
				c.Assert(err, gc.ErrorMatches, `cannot open ports 12345-12345/tcp \("wordpress/0"\) for unit "wordpress/0": .*`)
				c.Assert(err, jc.Satisfies, errors.IsNotAssigned)
			}
		}
		if flag&closePorts != 0 {
			// Close the port again (only if been opened before).
			err = u.ClosePort("tcp", 12345)
			c.Assert(err, jc.ErrorIsNil)
		}
	}
	changeTestFuncs := []changeTestFunc{
		func(c *gc.C, st *State) changeTestCase {
			initEnv(c, st, assignUnit)

			return changeTestCase{
				about: "don't open ports on unit",
				change: watcher.Change{
					C:  "units",
					Id: st.docID("wordpress/0"),
				},
				expectContents: []multiwatcher.EntityInfo{
					&multiwatcher.UnitInfo{
						EnvUUID:    st.EnvironUUID(),
						Name:       "wordpress/0",
						Service:    "wordpress",
						Series:     "quantal",
						MachineId:  "0",
						Ports:      []network.Port{},
						PortRanges: []network.PortRange{},
						Status:     "pending",
						StatusData: map[string]interface{}{},
						WorkloadStatus: multiwatcher.StatusInfo{
							Current: "unknown",
							Message: "Waiting for agent initialization to finish",
							Data:    map[string]interface{}{},
						},
						AgentStatus: multiwatcher.StatusInfo{
							Current: "allocating",
							Message: "",
							Data:    map[string]interface{}{},
						},
					}}}
		},
		func(c *gc.C, st *State) changeTestCase {
			initEnv(c, st, assignUnit|openPorts)

			return changeTestCase{
				about: "open a port on unit",
				change: watcher.Change{
					C:  "units",
					Id: st.docID("wordpress/0"),
				},
				expectContents: []multiwatcher.EntityInfo{
					&multiwatcher.UnitInfo{
						EnvUUID:        st.EnvironUUID(),
						Name:           "wordpress/0",
						Service:        "wordpress",
						Series:         "quantal",
						MachineId:      "0",
						PublicAddress:  "1.2.3.4",
						PrivateAddress: "4.3.2.1",
						Ports:          []network.Port{{"tcp", 12345}},
						PortRanges:     []network.PortRange{{12345, 12345, "tcp"}},
						Status:         "pending",
						StatusData:     map[string]interface{}{},
						WorkloadStatus: multiwatcher.StatusInfo{
							Current: "unknown",
							Message: "Waiting for agent initialization to finish",
							Data:    map[string]interface{}{},
						},
						AgentStatus: multiwatcher.StatusInfo{
							Current: "allocating",
							Message: "",
							Data:    map[string]interface{}{},
						},
					}}}
		},
		func(c *gc.C, st *State) changeTestCase {
			initEnv(c, st, assignUnit|openPorts|closePorts)

			return changeTestCase{
				about: "open a port on unit and close it again",
				change: watcher.Change{
					C:  "units",
					Id: st.docID("wordpress/0"),
				},
				expectContents: []multiwatcher.EntityInfo{
					&multiwatcher.UnitInfo{
						EnvUUID:        st.EnvironUUID(),
						Name:           "wordpress/0",
						Service:        "wordpress",
						Series:         "quantal",
						MachineId:      "0",
						PublicAddress:  "1.2.3.4",
						PrivateAddress: "4.3.2.1",
						Ports:          []network.Port{},
						PortRanges:     []network.PortRange{},
						Status:         "pending",
						StatusData:     map[string]interface{}{},
						WorkloadStatus: multiwatcher.StatusInfo{
							Current: "unknown",
							Message: "Waiting for agent initialization to finish",
							Data:    map[string]interface{}{},
						},
						AgentStatus: multiwatcher.StatusInfo{
							Current: "allocating",
							Message: "",
							Data:    map[string]interface{}{},
						},
					}}}
		},
		func(c *gc.C, st *State) changeTestCase {
			initEnv(c, st, openPorts)

			return changeTestCase{
				about: "open ports on an unassigned unit",
				change: watcher.Change{
					C:  "units",
					Id: st.docID("wordpress/0"),
				},
				expectContents: []multiwatcher.EntityInfo{
					&multiwatcher.UnitInfo{
						EnvUUID:    st.EnvironUUID(),
						Name:       "wordpress/0",
						Service:    "wordpress",
						Series:     "quantal",
						Ports:      []network.Port{},
						PortRanges: []network.PortRange{},
						Status:     "pending",
						StatusData: map[string]interface{}{},
						WorkloadStatus: multiwatcher.StatusInfo{
							Current: "unknown",
							Message: "Waiting for agent initialization to finish",
							Data:    map[string]interface{}{},
						},
						AgentStatus: multiwatcher.StatusInfo{
							Current: "allocating",
							Message: "",
							Data:    map[string]interface{}{},
						},
					}}}
		},
	}
	runChangeTests(c, changeTestFuncs)
}

func newTestAllWatcher(st *State, c *gc.C) *testWatcher {
	return newTestWatcher(newAllWatcherStateBacking(st), st, c)
}

func newTestAllEnvWatcher(st *State, c *gc.C) *testWatcher {
	return newTestWatcher(newAllEnvWatcherStateBacking(st), st, c)
}

type testWatcher struct {
	st     *State
	c      *gc.C
	b      Backing
	sm     *storeManager
	w      *Multiwatcher
	deltas chan []multiwatcher.Delta
}

func newTestWatcher(b Backing, st *State, c *gc.C) *testWatcher {
	sm := newStoreManager(b)
	w := NewMultiwatcher(sm)
	tw := &testWatcher{
		st:     st,
		c:      c,
		b:      b,
		sm:     sm,
		w:      w,
		deltas: make(chan []multiwatcher.Delta),
	}
	go func() {
		for {
			deltas, err := tw.w.Next()
			if err != nil {
				break
			}
			tw.deltas <- deltas
		}
	}()
	return tw
}

func (tw *testWatcher) Next(timeout time.Duration) []multiwatcher.Delta {
	select {
	case d := <-tw.deltas:
		return d
	case <-time.After(timeout):
		return nil
	}
}

func (tw *testWatcher) NumDeltas() int {
	count := 0
	tw.st.StartSync()
	for {
		// TODO(mjs) - this is somewhat fragile. There are no
		// guarentees that the watcher will be able to return deltas
		// in ShortWait time.
		deltas := len(tw.Next(testing.ShortWait))
		if deltas == 0 {
			break
		}
		count += deltas
	}
	return count
}

func (tw *testWatcher) All(expectedCount int) []multiwatcher.Delta {
	var allDeltas []multiwatcher.Delta
	tw.st.StartSync()

	//  Wait up to LongWait for the expected deltas to arrive, unless
	//  we don't expect any (then just wait for ShortWait).
	maxDuration := testing.LongWait
	if expectedCount <= 0 {
		maxDuration = testing.ShortWait
	}

	now := time.Now()
	maxTime := now.Add(maxDuration)
	for {
		remaining := maxTime.Sub(now)
		if remaining < time.Duration(0) {
			break // timed out
		}

		deltas := tw.Next(remaining)
		if len(deltas) > 0 {
			allDeltas = append(allDeltas, deltas...)
			if len(allDeltas) >= expectedCount {
				break
			}
		}

		now = time.Now()
	}
	return allDeltas
}

func (tw *testWatcher) Stop() {
	tw.c.Assert(tw.w.Stop(), jc.ErrorIsNil)
	tw.c.Assert(tw.sm.Stop(), jc.ErrorIsNil)
	tw.c.Assert(tw.b.Release(), jc.ErrorIsNil)
}

func (tw *testWatcher) AssertNoChange() {
	tw.c.Assert(tw.NumDeltas(), gc.Equals, 0)
}

func (tw *testWatcher) AssertChanges() {
	tw.c.Assert(tw.NumDeltas(), jc.GreaterThan, 0)
}

type entityInfoSlice []multiwatcher.EntityInfo

func (s entityInfoSlice) Len() int      { return len(s) }
func (s entityInfoSlice) Swap(i, j int) { s[i], s[j] = s[j], s[i] }
func (s entityInfoSlice) Less(i, j int) bool {
	id0, id1 := s[i].EntityId(), s[j].EntityId()
	if id0.Kind != id1.Kind {
		return id0.Kind < id1.Kind
	}
	if id0.EnvUUID != id1.EnvUUID {
		return id0.EnvUUID < id1.EnvUUID
	}
	return id0.Id < id1.Id
}

func checkDeltasEqual(c *gc.C, d0, d1 []multiwatcher.Delta) {
	// Deltas are returned in arbitrary order, so we compare them as maps.
	c.Check(deltaMap(d0), jc.DeepEquals, deltaMap(d1))
}

func deltaMap(deltas []multiwatcher.Delta) map[interface{}]multiwatcher.EntityInfo {
	m := make(map[interface{}]multiwatcher.EntityInfo)
	for _, d := range deltas {
		id := d.Entity.EntityId()
		if d.Removed {
			m[id] = nil
		} else {
			m[id] = d.Entity
		}
	}
	return m
}

func makeActionInfo(a *Action, st *State) multiwatcher.ActionInfo {
	results, message := a.Results()
	return multiwatcher.ActionInfo{
		EnvUUID:    st.EnvironUUID(),
		Id:         a.Id(),
		Receiver:   a.Receiver(),
		Name:       a.Name(),
		Parameters: a.Parameters(),
		Status:     string(a.Status()),
		Message:    message,
		Results:    results,
		Enqueued:   a.Enqueued(),
		Started:    a.Started(),
		Completed:  a.Completed(),
	}
}

func jcDeepEqualsCheck(c *gc.C, got, want interface{}) bool {
	ok, err := jc.DeepEqual(got, want)
	if ok {
		c.Check(err, jc.ErrorIsNil)
	}
	return ok
}

// assertEntitiesEqual is a specialised version of the typical
// jc.DeepEquals check that provides more informative output when
// comparing EntityInfo slices.
func assertEntitiesEqual(c *gc.C, got, want []multiwatcher.EntityInfo) {
	if jcDeepEqualsCheck(c, got, want) {
		return
	}
	if len(got) != len(want) {
		c.Errorf("entity length mismatch; got %d; want %d", len(got), len(want))
	} else {
		c.Errorf("entity contents mismatch; same length %d", len(got))
	}
	// Lets construct a decent output.
	var errorOutput string
	errorOutput = "\ngot: \n"
	for _, e := range got {
		errorOutput += fmt.Sprintf("  %T %#v\n", e, e)
	}
	errorOutput += "expected: \n"
	for _, e := range want {
		errorOutput += fmt.Sprintf("  %T %#v\n", e, e)
	}

	c.Errorf(errorOutput)

	var firstDiffError string
	if len(got) == len(want) {
		for i := 0; i < len(got); i++ {
			g := got[i]
			w := want[i]
			if !jcDeepEqualsCheck(c, g, w) {
				firstDiffError += "\n"
				firstDiffError += fmt.Sprintf("first difference at position %d\n", i)
				firstDiffError += "got:\n"
				firstDiffError += fmt.Sprintf("  %T %#v\n", g, g)
				firstDiffError += "expected:\n"
				firstDiffError += fmt.Sprintf("  %T %#v\n", w, w)
				break
			}
		}
		c.Errorf(firstDiffError)
	}
	c.FailNow()
}

func deepEqual(c *gc.C, got, want interface{}) bool {
	same, err := jc.DeepEqual(got, want)
	return err == nil && same
}
