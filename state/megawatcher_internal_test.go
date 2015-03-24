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
	"gopkg.in/juju/charm.v4"

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
var _ = gc.Suite(&storeManagerStateSuite{})

type storeManagerStateSuite struct {
	internalStateSuite
	OtherState *State
}

func (s *storeManagerStateSuite) SetUpTest(c *gc.C) {
	s.internalStateSuite.SetUpTest(c)

	s.OtherState = s.newState(c)
	s.AddCleanup(func(*gc.C) { s.OtherState.Close() })
}

func (s *storeManagerStateSuite) newState(c *gc.C) *State {
	uuid, err := utils.NewUUID()
	c.Assert(err, jc.ErrorIsNil)
	cfg := testing.CustomEnvironConfig(c, testing.Attrs{
		"name": "newtestenv",
		"uuid": uuid.String(),
	})
	_, st, err := s.state.NewEnvironment(cfg, s.owner)
	c.Assert(err, jc.ErrorIsNil)
	return st
}

func (s *storeManagerStateSuite) Reset(c *gc.C) {
	s.TearDownTest(c)
	s.SetUpTest(c)
}

func jcDeepEqualsCheck(c *gc.C, got, want interface{}) bool {
	ok, message := jc.DeepEquals.Check([]interface{}{got, want}, []string{"got", "want"})
	if !ok {
		c.Logf(message)
	}
	return ok
}

func assertEntitiesEqual(c *gc.C, got, want []multiwatcher.EntityInfo) {
	if len(got) == 0 {
		got = nil
	}
	if len(want) == 0 {
		want = nil
	}
	if jcDeepEqualsCheck(c, got, want) {
		return
	}
	c.Errorf("entity mismatch; got len %d; want %d", len(got), len(want))
	// Lets construct a decent output.
	var errorOutput string
	errorOutput = "\ngot: \n"
	for _, e := range got {
		errorOutput += fmt.Sprintf("\t%T %#v\n", e, e)
	}
	errorOutput += "expected: \n"
	for _, e := range want {
		errorOutput += fmt.Sprintf("\t%T %#v\n", e, e)
	}

	c.Errorf(errorOutput)

	var firstDiffError string
	if len(got) == len(want) {
		for i := 0; i < len(got); i++ {
			g := got[i]
			w := want[i]
			if !jcDeepEqualsCheck(c, g, w) {
				firstDiffError += "\n"
				firstDiffError += fmt.Sprintf("first difference at position %d", i)
				firstDiffError += "got: \n"
				firstDiffError += fmt.Sprintf("\t%T %#v", g, g)
				firstDiffError += "expected: \n"
				firstDiffError += fmt.Sprintf("\t%T %#v", w, w)
				break
			}
		}
		c.Errorf(firstDiffError)
	}
	c.FailNow()
}

func (s *storeManagerStateSuite) TestStateBackingGetAll(c *gc.C) {
	expectEntities := s.setUpScenario(c, s.state, 2)
	s.checkGetAll(c, expectEntities)
}

func (s *storeManagerStateSuite) TestStateBackingGetAllMultiEnv(c *gc.C) {
	// Set up 2 environments and ensure that GetAll returns the
	// entities for the first environment with no errors.
	expectEntities := s.setUpScenario(c, s.state, 2)

	// Use more units in the second env to ensure the number of
	// entities will mismatch if environment filtering isn't in place.
	s.setUpScenario(c, s.OtherState, 4)

	s.checkGetAll(c, expectEntities)
}

func (s *storeManagerStateSuite) checkGetAll(c *gc.C, expectEntities entityInfoSlice) {
	b := newAllWatcherStateBacking(s.state)
	all := newStore()
	err := b.GetAll(all)
	c.Assert(err, jc.ErrorIsNil)
	var gotEntities entityInfoSlice = all.All()
	sort.Sort(gotEntities)
	sort.Sort(expectEntities)
	assertEntitiesEqual(c, gotEntities, expectEntities)
}

// setUpScenario adds some entities to the state so that
// we can check that they all get pulled in by
// allWatcherStateBacking.GetAll.
func (s *storeManagerStateSuite) setUpScenario(c *gc.C, st *State, units int) (entities entityInfoSlice) {
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
	err = m.SetAddresses(network.NewAddress("example.com"))
	c.Assert(err, jc.ErrorIsNil)
	add(&multiwatcher.MachineInfo{
		Id:                      "0",
		InstanceId:              "i-machine-0",
		Status:                  multiwatcher.Status("pending"),
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
		Name:        "wordpress",
		Exposed:     true,
		CharmURL:    serviceCharmURL(wordpress).String(),
		OwnerTag:    s.owner.String(),
		Life:        multiwatcher.Life("alive"),
		MinUnits:    units,
		Constraints: constraints.MustParse("mem=100M"),
		Config:      charm.Settings{"blog-title": "boring"},
		Subordinate: false,
	})
	pairs := map[string]string{"x": "12", "y": "99"}
	err = st.SetAnnotations(wordpress, pairs)
	c.Assert(err, jc.ErrorIsNil)
	add(&multiwatcher.AnnotationInfo{
		Tag:         "service-wordpress",
		Annotations: pairs,
	})

	logging := AddTestingService(c, st, "logging", AddTestingCharm(c, st, "logging"), s.owner)
	add(&multiwatcher.ServiceInfo{
		Name:        "logging",
		CharmURL:    serviceCharmURL(logging).String(),
		OwnerTag:    s.owner.String(),
		Life:        multiwatcher.Life("alive"),
		Config:      charm.Settings{},
		Subordinate: true,
	})

	eps, err := st.InferEndpoints("logging", "wordpress")
	c.Assert(err, jc.ErrorIsNil)
	rel, err := st.AddRelation(eps...)
	c.Assert(err, jc.ErrorIsNil)
	add(&multiwatcher.RelationInfo{
		Key: "logging:logging-directory wordpress:logging-dir",
		Id:  rel.Id(),
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
			Name:        fmt.Sprintf("wordpress/%d", i),
			Service:     wordpress.Name(),
			Series:      m.Series(),
			MachineId:   m.Id(),
			Ports:       []network.Port{},
			Status:      multiwatcher.Status("allocating"),
			Subordinate: false,
		})
		pairs := map[string]string{"name": fmt.Sprintf("bar %d", i)}
		err = st.SetAnnotations(wu, pairs)
		c.Assert(err, jc.ErrorIsNil)
		add(&multiwatcher.AnnotationInfo{
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
			Id:                      fmt.Sprint(i + 1),
			InstanceId:              "i-" + m.Tag().String(),
			Status:                  multiwatcher.Status("error"),
			StatusInfo:              m.Tag().String(),
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
			Name:        fmt.Sprintf("logging/%d", i),
			Service:     "logging",
			Series:      "quantal",
			Ports:       []network.Port{},
			Status:      multiwatcher.Status("allocating"),
			Subordinate: true,
		})
	}
	return
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

// changeTestFunc is a function for the preparation of a test and
// the creation of the according case.
type changeTestFunc func(c *gc.C, st *State) changeTestCase

// performChangeTestCases runs a passed number of test cases for changes.
func (s *storeManagerStateSuite) performChangeTestCases(c *gc.C, changeTestFuncs []changeTestFunc) {
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
		assertEntitiesEqual(c, all.All(), test.expectContents)
		s.Reset(c)
	}
}

// TestChangeActions tests the changing of actions.
func (s *storeManagerStateSuite) TestChangeActions(c *gc.C) {
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

// TestChangeAnnotations tests the changing of annotations.
func (s *storeManagerStateSuite) TestChangeAnnotations(c *gc.C) {
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
				about:           "annotation is removed if it's not in backing",
				initialContents: []multiwatcher.EntityInfo{&multiwatcher.AnnotationInfo{Tag: "machine-0"}},
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
					Tag: "machine-0",
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
						Tag: "machine-0",
						Annotations: map[string]string{
							"arble": "khroomph",
							"new":   "attr",
						}}}}
		},
	}
	s.performChangeTestCases(c, changeTestFuncs)
}

// TestChangeBlocks tests the changing of blocks.
func (s *storeManagerStateSuite) TestChangeBlocks(c *gc.C) {
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

// TestChangeMachines tests the changing of machines.
func (s *storeManagerStateSuite) TestChangeMachines(c *gc.C) {
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
				about:           "machine is removed if it's not in backing",
				initialContents: []multiwatcher.EntityInfo{&multiwatcher.MachineInfo{Id: "1"}},
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
						Id:         "0",
						Status:     multiwatcher.Status("error"),
						StatusInfo: "failure",
						Life:       multiwatcher.Life("alive"),
						Series:     "quantal",
						Jobs:       []multiwatcher.MachineJob{JobHostUnits.ToParams()},
						Addresses:  []network.Address{},
						HasVote:    false,
						WantsVote:  false,
					}}}
		},
		func(c *gc.C, st *State) changeTestCase {
			m, err := st.AddMachine("trusty", JobManageEnviron)
			c.Assert(err, jc.ErrorIsNil)
			err = m.SetProvisioned("i-0", "bootstrap_nonce", nil)
			c.Assert(err, jc.ErrorIsNil)
			err = m.SetSupportedContainers([]instance.ContainerType{instance.LXC})
			c.Assert(err, jc.ErrorIsNil)

			return changeTestCase{
				about: "machine is updated if it's in backing and in Store",
				initialContents: []multiwatcher.EntityInfo{
					&multiwatcher.MachineInfo{
						Id:         "0",
						Status:     multiwatcher.Status("error"),
						StatusInfo: "another failure",
					},
				},
				change: watcher.Change{
					C:  "machines",
					Id: st.docID("0"),
				},
				expectContents: []multiwatcher.EntityInfo{
					&multiwatcher.MachineInfo{
						Id:                       "0",
						InstanceId:               "i-0",
						Status:                   multiwatcher.Status("error"),
						StatusInfo:               "another failure",
						Life:                     multiwatcher.Life("alive"),
						Series:                   "trusty",
						Jobs:                     []multiwatcher.MachineJob{JobManageEnviron.ToParams()},
						Addresses:                []network.Address{},
						HardwareCharacteristics:  &instance.HardwareCharacteristics{},
						SupportedContainers:      []instance.ContainerType{instance.LXC},
						SupportedContainersKnown: true,
						HasVote:                  false,
						WantsVote:                true,
					}}}
		},
		func(c *gc.C, st *State) changeTestCase {
			return changeTestCase{
				about: "no change if status is not in backing",
				initialContents: []multiwatcher.EntityInfo{&multiwatcher.MachineInfo{
					Id:         "0",
					Status:     multiwatcher.Status("error"),
					StatusInfo: "failure",
				}},
				change: watcher.Change{
					C:  "statuses",
					Id: st.docID("m#0"),
				},
				expectContents: []multiwatcher.EntityInfo{
					&multiwatcher.MachineInfo{
						Id:         "0",
						Status:     multiwatcher.Status("error"),
						StatusInfo: "failure",
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
					Id:         "0",
					Status:     multiwatcher.Status("error"),
					StatusInfo: "failure",
				}},
				change: watcher.Change{
					C:  "statuses",
					Id: st.docID("m#0"),
				},
				expectContents: []multiwatcher.EntityInfo{
					&multiwatcher.MachineInfo{
						Id:         "0",
						Status:     multiwatcher.Status("started"),
						StatusData: make(map[string]interface{}),
					}}}
		},
	}
	s.performChangeTestCases(c, changeTestFuncs)
}

// TestChangeRelations tests the changing of relations.
func (s *storeManagerStateSuite) TestChangeRelations(c *gc.C) {
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
				about:           "relation is removed if it's not in backing",
				initialContents: []multiwatcher.EntityInfo{&multiwatcher.RelationInfo{Key: "logging:logging-directory wordpress:logging-dir"}},
				change: watcher.Change{
					C:  "relations",
					Id: st.docID("logging:logging-directory wordpress:logging-dir"),
				}}
		},
		func(c *gc.C, st *State) changeTestCase {
			AddTestingService(c, st, "wordpress", AddTestingCharm(c, st, "wordpress"), s.owner)
			AddTestingService(c, st, "logging", AddTestingCharm(c, st, "logging"), s.owner)
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
						Key: "logging:logging-directory wordpress:logging-dir",
						Endpoints: []multiwatcher.Endpoint{
							{ServiceName: "logging", Relation: charm.Relation{Name: "logging-directory", Role: "requirer", Interface: "logging", Optional: false, Limit: 1, Scope: "container"}},
							{ServiceName: "wordpress", Relation: charm.Relation{Name: "logging-dir", Role: "provider", Interface: "logging", Optional: false, Limit: 0, Scope: "container"}}},
					}}}
		},
	}
	s.performChangeTestCases(c, changeTestFuncs)
}

// TestChangeServices tests the changing of services.
func (s *storeManagerStateSuite) TestChangeServices(c *gc.C) {
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
				about:           "service is removed if it's not in backing",
				initialContents: []multiwatcher.EntityInfo{&multiwatcher.ServiceInfo{Name: "wordpress"}},
				change: watcher.Change{
					C:  "services",
					Id: st.docID("wordpress"),
				}}
		},
		func(c *gc.C, st *State) changeTestCase {
			wordpress := AddTestingService(c, st, "wordpress", AddTestingCharm(c, st, "wordpress"), s.owner)
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
						Name:     "wordpress",
						Exposed:  true,
						CharmURL: "local:quantal/quantal-wordpress-3",
						OwnerTag: s.owner.String(),
						Life:     multiwatcher.Life("alive"),
						MinUnits: 42,
						Config:   charm.Settings{},
					}}}
		},
		func(c *gc.C, st *State) changeTestCase {
			svc := AddTestingService(c, st, "wordpress", AddTestingCharm(c, st, "wordpress"), s.owner)
			setServiceConfigAttr(c, svc, "blog-title", "boring")

			return changeTestCase{
				about: "service is updated if it's in backing and in multiwatcher.Store",
				initialContents: []multiwatcher.EntityInfo{&multiwatcher.ServiceInfo{
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
						Name:        "wordpress",
						CharmURL:    "local:quantal/quantal-wordpress-3",
						OwnerTag:    s.owner.String(),
						Life:        multiwatcher.Life("alive"),
						Constraints: constraints.MustParse("mem=99M"),
						Config:      charm.Settings{"blog-title": "boring"},
					}}}
		},
		func(c *gc.C, st *State) changeTestCase {
			svc := AddTestingService(c, st, "wordpress", AddTestingCharm(c, st, "wordpress"), s.owner)
			setServiceConfigAttr(c, svc, "blog-title", "boring")

			return changeTestCase{
				about: "service re-reads config when charm URL changes",
				initialContents: []multiwatcher.EntityInfo{&multiwatcher.ServiceInfo{
					Name: "wordpress",
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
						Name:     "wordpress",
						CharmURL: "local:quantal/quantal-wordpress-3",
						OwnerTag: s.owner.String(),
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
					Name:     "dummy-service",
					CharmURL: "local:quantal/quantal-dummy-1",
				}},
				change: watcher.Change{
					C:  "settings",
					Id: st.docID("s#dummy-service#local:quantal/quantal-dummy-1"),
				},
				expectContents: []multiwatcher.EntityInfo{&multiwatcher.ServiceInfo{
					Name:     "dummy-service",
					CharmURL: "local:quantal/quantal-dummy-1",
				}}}
		},
		func(c *gc.C, st *State) changeTestCase {
			svc := AddTestingService(c, st, "dummy-service", AddTestingCharm(c, st, "dummy"), s.owner)
			setServiceConfigAttr(c, svc, "username", "foo")
			setServiceConfigAttr(c, svc, "outlook", "foo@bar")

			return changeTestCase{
				about: "service config is changed if service exists in the store with the same URL",
				initialContents: []multiwatcher.EntityInfo{&multiwatcher.ServiceInfo{
					Name:     "dummy-service",
					CharmURL: "local:quantal/quantal-dummy-1",
				}},
				change: watcher.Change{
					C:  "settings",
					Id: st.docID("s#dummy-service#local:quantal/quantal-dummy-1"),
				},
				expectContents: []multiwatcher.EntityInfo{
					&multiwatcher.ServiceInfo{
						Name:     "dummy-service",
						CharmURL: "local:quantal/quantal-dummy-1",
						Config:   charm.Settings{"username": "foo", "outlook": "foo@bar"},
					}}}
		},
		func(c *gc.C, st *State) changeTestCase {
			svc := AddTestingService(c, st, "dummy-service", AddTestingCharm(c, st, "dummy"), s.owner)
			setServiceConfigAttr(c, svc, "username", "foo")
			setServiceConfigAttr(c, svc, "outlook", "foo@bar")
			setServiceConfigAttr(c, svc, "username", nil)

			return changeTestCase{
				about: "service config is changed after removing of a setting",
				initialContents: []multiwatcher.EntityInfo{&multiwatcher.ServiceInfo{
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
			svc := AddTestingService(c, st, "dummy-service", testCharm, s.owner)
			setServiceConfigAttr(c, svc, "key.dotted", "foo")

			return changeTestCase{
				about: "service config is unescaped when reading from the backing store",
				initialContents: []multiwatcher.EntityInfo{&multiwatcher.ServiceInfo{
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
						Name:     "dummy-service",
						CharmURL: "local:quantal/quantal-dummy-1",
						Config:   charm.Settings{"key.dotted": "foo"},
					}}}
		},
		func(c *gc.C, st *State) changeTestCase {
			svc := AddTestingService(c, st, "dummy-service", AddTestingCharm(c, st, "dummy"), s.owner)
			setServiceConfigAttr(c, svc, "username", "foo")

			return changeTestCase{
				about: "service config is unchanged if service exists in the store with a different URL",
				initialContents: []multiwatcher.EntityInfo{&multiwatcher.ServiceInfo{
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
	s.performChangeTestCases(c, changeTestFuncs)
}

// TestChangeServicesConstraints tests the changing of the constraints of services.
func (s *storeManagerStateSuite) TestChangeServicesConstraints(c *gc.C) {
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
					Name:        "wordpress",
					Constraints: constraints.MustParse("mem=99M"),
				}},
				change: watcher.Change{
					C:  "constraints",
					Id: st.docID("s#wordpress"),
				},
				expectContents: []multiwatcher.EntityInfo{&multiwatcher.ServiceInfo{
					Name:        "wordpress",
					Constraints: constraints.MustParse("mem=99M"),
				}}}
		},
		func(c *gc.C, st *State) changeTestCase {
			svc := AddTestingService(c, st, "wordpress", AddTestingCharm(c, st, "wordpress"), s.owner)
			err := svc.SetConstraints(constraints.MustParse("mem=4G cpu-cores= arch=amd64"))
			c.Assert(err, jc.ErrorIsNil)

			return changeTestCase{
				about: "status is changed if the service exists in the store",
				initialContents: []multiwatcher.EntityInfo{&multiwatcher.ServiceInfo{
					Name:        "wordpress",
					Constraints: constraints.MustParse("mem=99M cpu-cores=2 cpu-power=4"),
				}},
				change: watcher.Change{
					C:  "constraints",
					Id: st.docID("s#wordpress"),
				},
				expectContents: []multiwatcher.EntityInfo{
					&multiwatcher.ServiceInfo{
						Name:        "wordpress",
						Constraints: constraints.MustParse("mem=4G cpu-cores= arch=amd64"),
					}}}
		},
	}
	s.performChangeTestCases(c, changeTestFuncs)
}

// TestChangeUnits tests the changing of units.
func (s *storeManagerStateSuite) TestChangeUnits(c *gc.C) {
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
				about:           "unit is removed if it's not in backing",
				initialContents: []multiwatcher.EntityInfo{&multiwatcher.UnitInfo{Name: "wordpress/1"}},
				change: watcher.Change{
					C:  "units",
					Id: st.docID("wordpress/1"),
				}}
		},
		func(c *gc.C, st *State) changeTestCase {
			wordpress := AddTestingService(c, st, "wordpress", AddTestingCharm(c, st, "wordpress"), s.owner)
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
					}}}
		},
		func(c *gc.C, st *State) changeTestCase {
			wordpress := AddTestingService(c, st, "wordpress", AddTestingCharm(c, st, "wordpress"), s.owner)
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
					Name:       "wordpress/0",
					Status:     multiwatcher.Status("error"),
					StatusInfo: "another failure",
					Ports:      []network.Port{{"udp", 17070}},
					PortRanges: []network.PortRange{{17070, 17070, "udp"}},
				}},
				change: watcher.Change{
					C:  "units",
					Id: st.docID("wordpress/0"),
				},
				expectContents: []multiwatcher.EntityInfo{
					&multiwatcher.UnitInfo{
						Name:       "wordpress/0",
						Service:    "wordpress",
						Series:     "quantal",
						MachineId:  "0",
						Ports:      []network.Port{{"udp", 17070}},
						PortRanges: []network.PortRange{{17070, 17070, "udp"}},
						Status:     multiwatcher.Status("error"),
						StatusInfo: "another failure",
					}}}
		},
		func(c *gc.C, st *State) changeTestCase {
			wordpress := AddTestingService(c, st, "wordpress", AddTestingCharm(c, st, "wordpress"), s.owner)
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
						Name: "wordpress/0",
					},
					&multiwatcher.MachineInfo{
						Id: "0",
					},
				},
				change: watcher.Change{
					C:  openedPortsC,
					Id: st.docID("m#0#n#juju-public"),
				},
				expectContents: []multiwatcher.EntityInfo{
					&multiwatcher.UnitInfo{
						Name:       "wordpress/0",
						Ports:      []network.Port{{"tcp", 4242}},
						PortRanges: []network.PortRange{{4242, 4242, "tcp"}},
					},
					&multiwatcher.MachineInfo{
						Id: "0",
					},
				}}
		},
		func(c *gc.C, st *State) changeTestCase {
			wordpress := AddTestingService(c, st, "wordpress", AddTestingCharm(c, st, "wordpress"), s.owner)
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
						Id: "0",
					},
				},
				change: watcher.Change{
					C:  "units",
					Id: st.docID("wordpress/0"),
				},
				expectContents: []multiwatcher.EntityInfo{
					&multiwatcher.UnitInfo{
						Name:       "wordpress/0",
						Service:    "wordpress",
						Series:     "quantal",
						MachineId:  "0",
						Status:     "allocating",
						Ports:      []network.Port{{"tcp", 21}, {"tcp", 22}},
						PortRanges: []network.PortRange{{21, 22, "tcp"}},
					},
					&multiwatcher.MachineInfo{
						Id: "0",
					},
				}}
		},
		func(c *gc.C, st *State) changeTestCase {
			wordpress := AddTestingService(c, st, "wordpress", AddTestingCharm(c, st, "wordpress"), s.owner)
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
			err = m.SetAddresses(publicAddress, privateAddress)
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
					Name:       "wordpress/0",
					Status:     multiwatcher.Status("error"),
					StatusInfo: "failure",
				}},
				change: watcher.Change{
					C:  "statuses",
					Id: st.docID("u#wordpress/0"),
				},
				expectContents: []multiwatcher.EntityInfo{
					&multiwatcher.UnitInfo{
						Name:       "wordpress/0",
						Status:     multiwatcher.Status("error"),
						StatusInfo: "failure",
					}}}
		},
		func(c *gc.C, st *State) changeTestCase {
			wordpress := AddTestingService(c, st, "wordpress", AddTestingCharm(c, st, "wordpress"), s.owner)
			u, err := wordpress.AddUnit()
			c.Assert(err, jc.ErrorIsNil)
			err = u.SetAgentStatus(StatusIdle, "", nil)
			c.Assert(err, jc.ErrorIsNil)

			return changeTestCase{
				about: "status is changed if the unit exists in the store",
				initialContents: []multiwatcher.EntityInfo{&multiwatcher.UnitInfo{
					Name:       "wordpress/0",
					Status:     multiwatcher.Status("error"),
					StatusInfo: "failure",
				}},
				change: watcher.Change{
					C:  "statuses",
					Id: st.docID("u#wordpress/0"),
				},
				expectContents: []multiwatcher.EntityInfo{
					&multiwatcher.UnitInfo{
						Name:       "wordpress/0",
						Status:     multiwatcher.Status("idle"),
						StatusData: make(map[string]interface{}),
					}}}
		},
		func(c *gc.C, st *State) changeTestCase {
			wordpress := AddTestingService(c, st, "wordpress", AddTestingCharm(c, st, "wordpress"), s.owner)
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
					Name:   "wordpress/0",
					Status: multiwatcher.Status("idle"),
				}},
				change: watcher.Change{
					C:  "statuses",
					Id: st.docID("u#wordpress/0"),
				},
				expectContents: []multiwatcher.EntityInfo{
					&multiwatcher.UnitInfo{
						Name:       "wordpress/0",
						Status:     multiwatcher.Status("error"),
						StatusInfo: "hook error",
						StatusData: map[string]interface{}{
							"1st-key": "one",
							"2nd-key": 2,
							"3rd-key": true,
						}}}}
		},
	}
	s.performChangeTestCases(c, changeTestFuncs)
}

// initFlag helps to control the different test scenarios.
type initFlag int

const (
	noFlag     initFlag = 0
	assignUnit initFlag = 1
	openPorts  initFlag = 2
	closePorts initFlag = 4
)

// TestChangeUnitsNonNilPorts tests the changing of unit parts returning
// no nil values.
func (s *storeManagerStateSuite) TestChangeUnitsNonNilPorts(c *gc.C) {
	initEnv := func(c *gc.C, st *State, flag initFlag) {
		wordpress := AddTestingService(c, st, "wordpress", AddTestingCharm(c, st, "wordpress"), s.owner)
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
			err = m.SetAddresses(publicAddress, privateAddress)
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
						Name:       "wordpress/0",
						Service:    "wordpress",
						Series:     "quantal",
						MachineId:  "0",
						Ports:      []network.Port{},
						PortRanges: []network.PortRange{},
						Status:     "allocating",
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
						Name:           "wordpress/0",
						Service:        "wordpress",
						Series:         "quantal",
						MachineId:      "0",
						PublicAddress:  "1.2.3.4",
						PrivateAddress: "4.3.2.1",
						Ports:          []network.Port{{"tcp", 12345}},
						PortRanges:     []network.PortRange{{12345, 12345, "tcp"}},
						Status:         "allocating",
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
						Name:           "wordpress/0",
						Service:        "wordpress",
						Series:         "quantal",
						MachineId:      "0",
						PublicAddress:  "1.2.3.4",
						PrivateAddress: "4.3.2.1",
						Ports:          []network.Port{},
						PortRanges:     []network.PortRange{},
						Status:         "allocating",
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
						Name:       "wordpress/0",
						Service:    "wordpress",
						Series:     "quantal",
						Ports:      []network.Port{},
						PortRanges: []network.PortRange{},
						Status:     "allocating",
					}}}
		},
	}
	s.performChangeTestCases(c, changeTestFuncs)
}

// TestClosingPorts tests the correct reporting of closing ports.
func (s *storeManagerStateSuite) TestClosingPorts(c *gc.C) {
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
	err = m.SetAddresses(publicAddress, privateAddress)
	c.Assert(err, jc.ErrorIsNil)
	err = u.OpenPorts("tcp", 12345, 12345)
	c.Assert(err, jc.ErrorIsNil)
	// Create all watcher state backing.
	b := newAllWatcherStateBacking(s.state)
	all := newStore()
	// Check opened ports.
	err = b.Changed(all, watcher.Change{
		C:  "units",
		Id: s.state.docID("wordpress/0"),
	})
	c.Assert(err, jc.ErrorIsNil)
	assertEntitiesEqual(c, all.All(), []multiwatcher.EntityInfo{
		&multiwatcher.UnitInfo{
			Name:           "wordpress/0",
			Service:        "wordpress",
			Series:         "quantal",
			MachineId:      "0",
			PublicAddress:  "1.2.3.4",
			PrivateAddress: "4.3.2.1",
			Ports:          []network.Port{{"tcp", 12345}},
			PortRanges:     []network.PortRange{{12345, 12345, "tcp"}},
			Status:         "allocating",
		},
	})
	// Close the ports.
	err = u.ClosePorts("tcp", 12345, 12345)
	c.Assert(err, jc.ErrorIsNil)
	err = b.Changed(all, watcher.Change{
		C:  openedPortsC,
		Id: s.state.docID("wordpress/0"),
	})
	c.Assert(err, jc.ErrorIsNil)
	assertEntitiesEqual(c, all.All(), []multiwatcher.EntityInfo{
		&multiwatcher.UnitInfo{
			Name:           "wordpress/0",
			Service:        "wordpress",
			Series:         "quantal",
			MachineId:      "0",
			PublicAddress:  "1.2.3.4",
			PrivateAddress: "4.3.2.1",
			Ports:          []network.Port{},
			PortRanges:     []network.PortRange{},
			Status:         "allocating",
		},
	})
	// Try closing and updating with an invalid unit.
	c.Assert(func() {
		b.Changed(all, watcher.Change{
			C:  openedPortsC,
			Id: s.state.docID("unknown/42"),
		})
	}, gc.PanicMatches, `cannot retrieve unit "unknown/42": unit "unknown/42" not found`)
}

// TestSettings tests the correct reporting of unset service settings.
func (s *storeManagerStateSuite) TestSettings(c *gc.C) {
	defer s.Reset(c)
	// Init the test environment.
	svc := AddTestingService(c, s.state, "dummy-service", AddTestingCharm(c, s.state, "dummy"), s.owner)
	b := newAllWatcherStateBacking(s.state)
	all := newStore()
	// 1st scenario part: set settings and signal change.
	setServiceConfigAttr(c, svc, "username", "foo")
	setServiceConfigAttr(c, svc, "outlook", "foo@bar")
	all.Update(&multiwatcher.ServiceInfo{
		Name:     "dummy-service",
		CharmURL: "local:quantal/quantal-dummy-1",
	})
	err := b.Changed(all, watcher.Change{
		C:  "settings",
		Id: s.state.docID("s#dummy-service#local:quantal/quantal-dummy-1"),
	})
	c.Assert(err, jc.ErrorIsNil)
	assertEntitiesEqual(c, all.All(), []multiwatcher.EntityInfo{
		&multiwatcher.ServiceInfo{
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
	assertEntitiesEqual(c, all.All(), []multiwatcher.EntityInfo{
		&multiwatcher.ServiceInfo{
			Name:     "dummy-service",
			CharmURL: "local:quantal/quantal-dummy-1",
		},
	})
}

// TestStateWatcher tests the integration of the state watcher
// with the state-based backing. Most of the logic is tested elsewhere -
// this just tests end-to-end.
func (s *storeManagerStateSuite) TestStateWatcher(c *gc.C) {
	m0, err := s.state.AddMachine("trusty", JobManageEnviron)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(m0.Id(), gc.Equals, "0")

	m1, err := s.state.AddMachine("saucy", JobHostUnits)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(m1.Id(), gc.Equals, "1")

	tw := newTestWatcher(s.state, c)
	defer tw.Stop()

	// Expect to see events for the already created machines first.
	deltas := tw.All()
	checkDeltasEqual(c, deltas, []multiwatcher.Delta{{
		Entity: &multiwatcher.MachineInfo{
			Id:        "0",
			Status:    multiwatcher.Status("pending"),
			Life:      multiwatcher.Life("alive"),
			Series:    "trusty",
			Jobs:      []multiwatcher.MachineJob{JobManageEnviron.ToParams()},
			Addresses: []network.Address{},
			HasVote:   false,
			WantsVote: true,
		},
	}, {
		Entity: &multiwatcher.MachineInfo{
			Id:        "1",
			Status:    multiwatcher.Status("pending"),
			Life:      multiwatcher.Life("alive"),
			Series:    "saucy",
			Jobs:      []multiwatcher.MachineJob{JobHostUnits.ToParams()},
			Addresses: []network.Address{},
			HasVote:   false,
			WantsVote: false,
		},
	}})

	// Make some changes to the state.
	arch := "amd64"
	mem := uint64(4096)
	hc := &instance.HardwareCharacteristics{
		Arch: &arch,
		Mem:  &mem,
	}
	err = m0.SetProvisioned("i-0", "bootstrap_nonce", hc)
	c.Assert(err, jc.ErrorIsNil)

	err = m1.Destroy()
	c.Assert(err, jc.ErrorIsNil)
	err = m1.EnsureDead()
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
	deltas = tw.All()
	checkDeltasEqual(c, deltas, []multiwatcher.Delta{{
		Entity: &multiwatcher.MachineInfo{
			Id:                      "0",
			InstanceId:              "i-0",
			Status:                  multiwatcher.Status("pending"),
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
			Id:        "1",
			Status:    multiwatcher.Status("pending"),
			Life:      multiwatcher.Life("alive"),
			Series:    "saucy",
			Jobs:      []multiwatcher.MachineJob{JobHostUnits.ToParams()},
			Addresses: []network.Address{},
		},
	}, {
		Entity: &multiwatcher.MachineInfo{
			Id:        "2",
			Status:    multiwatcher.Status("pending"),
			Life:      multiwatcher.Life("alive"),
			Series:    "quantal",
			Jobs:      []multiwatcher.MachineJob{JobHostUnits.ToParams()},
			Addresses: []network.Address{},
			HasVote:   false,
			WantsVote: false,
		},
	}, {
		Entity: &multiwatcher.ServiceInfo{
			Name:     "wordpress",
			CharmURL: "local:quantal/quantal-wordpress-3",
			OwnerTag: s.owner.String(),
			Life:     "alive",
			Config:   make(map[string]interface{}),
		},
	}, {
		Entity: &multiwatcher.UnitInfo{
			Name:      "wordpress/0",
			Service:   "wordpress",
			Series:    "quantal",
			MachineId: "2",
			Status:    "allocating",
		},
	}})
}

func (s *storeManagerStateSuite) TestStateWatcherTwoEnvironments(c *gc.C) {
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

			w1 := newTestWatcher(s.state, c)
			defer w1.Stop()
			w2 := newTestWatcher(s.OtherState, c)
			defer w2.Stop()

			checkIsolationForEnv(s.state, w1, w2)
			checkIsolationForEnv(s.OtherState, w2, w1)
		}()
		s.Reset(c)
	}
}

type testWatcher struct {
	st     *State
	c      *gc.C
	w      *Multiwatcher
	deltas chan []multiwatcher.Delta
}

func newTestWatcher(st *State, c *gc.C) *testWatcher {
	b := newAllWatcherStateBacking(st)
	sm := newStoreManager(b)
	w := NewMultiwatcher(sm)
	tw := &testWatcher{
		st:     st,
		c:      c,
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

func (tw *testWatcher) All() []multiwatcher.Delta {
	var allDeltas []multiwatcher.Delta
	tw.st.StartSync()
	for {
		deltas := tw.Next(testing.ShortWait)
		if len(deltas) == 0 {
			break
		}
		allDeltas = append(allDeltas, deltas...)
	}
	return allDeltas
}

func (tw *testWatcher) Stop() {
	tw.c.Assert(tw.w.Stop(), jc.ErrorIsNil)
}

func (tw *testWatcher) AssertNoChange() {
	tw.c.Assert(tw.All(), gc.HasLen, 0)
}

func (tw *testWatcher) AssertChanges() {
	tw.c.Assert(len(tw.All()), jc.GreaterThan, 0)
}

type entityInfoSlice []multiwatcher.EntityInfo

func (s entityInfoSlice) Len() int      { return len(s) }
func (s entityInfoSlice) Swap(i, j int) { s[i], s[j] = s[j], s[i] }
func (s entityInfoSlice) Less(i, j int) bool {
	id0, id1 := s[i].EntityId(), s[j].EntityId()
	if id0.Kind != id1.Kind {
		return id0.Kind < id1.Kind
	}
	switch id := id0.Id.(type) {
	case string:
		return id < id1.Id.(string)
	default:
	}
	panic("unexpected entity id type")
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
