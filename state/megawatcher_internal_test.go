package state

import (
	"errors"
	"fmt"
	"labix.org/v2/mgo"
	. "launchpad.net/gocheck"
	"launchpad.net/juju-core/charm"
	"launchpad.net/juju-core/constraints"
	"launchpad.net/juju-core/state/api/params"
	"launchpad.net/juju-core/state/multiwatcher"
	"launchpad.net/juju-core/state/watcher"
	"launchpad.net/juju-core/testing"
	"reflect"
	"sort"
	"time"
)

type storeManagerStateSuite struct {
	testing.LoggingSuite
	testing.MgoSuite
	State *State
}

func (s *storeManagerStateSuite) SetUpSuite(c *C) {
	s.LoggingSuite.SetUpSuite(c)
	s.MgoSuite.SetUpSuite(c)
}

func (s *storeManagerStateSuite) TearDownSuite(c *C) {
	s.MgoSuite.TearDownSuite(c)
	s.LoggingSuite.TearDownSuite(c)
}

func (s *storeManagerStateSuite) SetUpTest(c *C) {
	s.LoggingSuite.SetUpTest(c)
	s.MgoSuite.SetUpTest(c)
	s.State = TestingInitialize(c, nil)
}

func (s *storeManagerStateSuite) TearDownTest(c *C) {
	s.State.Close()
	s.MgoSuite.TearDownTest(c)
	s.LoggingSuite.TearDownTest(c)
}

func (s *storeManagerStateSuite) Reset(c *C) {
	s.TearDownTest(c)
	s.SetUpTest(c)
}

var _ = Suite(&storeManagerStateSuite{})

// setUpScenario adds some entities to the state so that
// we can check that they all get pulled in by
// allWatcherStateBacking.getAll.
func (s *storeManagerStateSuite) setUpScenario(c *C) (entities entityInfoSlice) {
	add := func(e params.EntityInfo) {
		entities = append(entities, e)
	}
	m, err := s.State.AddMachine("series", JobManageEnviron)
	c.Assert(err, IsNil)
	c.Assert(m.Tag(), Equals, "machine-0")
	err = m.SetInstanceId(InstanceId("i-" + m.Tag()))
	c.Assert(err, IsNil)
	add(&params.MachineInfo{
		Id:         "0",
		InstanceId: "i-machine-0",
		Status:     params.MachinePending,
	})

	wordpress, err := s.State.AddService("wordpress", AddTestingCharm(c, s.State, "wordpress"))
	c.Assert(err, IsNil)
	err = wordpress.SetExposed()
	c.Assert(err, IsNil)
	err = wordpress.SetConstraints(constraints.MustParse("mem=100M"))
	c.Assert(err, IsNil)
	add(&params.ServiceInfo{
		Name:        "wordpress",
		Exposed:     true,
		CharmURL:    serviceCharmURL(wordpress).String(),
		Constraints: constraints.MustParse("mem=100M"),
	})
	pairs := map[string]string{"x": "12", "y": "99"}
	err = wordpress.SetAnnotations(pairs)
	c.Assert(err, IsNil)
	add(&params.AnnotationInfo{
		Tag:         "service-wordpress",
		Annotations: pairs,
	})

	logging, err := s.State.AddService("logging", AddTestingCharm(c, s.State, "logging"))
	c.Assert(err, IsNil)
	add(&params.ServiceInfo{
		Name:     "logging",
		CharmURL: serviceCharmURL(logging).String(),
	})

	eps, err := s.State.InferEndpoints([]string{"logging", "wordpress"})
	c.Assert(err, IsNil)
	rel, err := s.State.AddRelation(eps...)
	c.Assert(err, IsNil)
	add(&params.RelationInfo{
		Key: "logging:logging-directory wordpress:logging-dir",
		Endpoints: []params.Endpoint{
			params.Endpoint{ServiceName: "logging", Relation: charm.Relation{Name: "logging-directory", Role: "requirer", Interface: "logging", Optional: false, Limit: 1, Scope: "container"}},
			params.Endpoint{ServiceName: "wordpress", Relation: charm.Relation{Name: "logging-dir", Role: "provider", Interface: "logging", Optional: false, Limit: 0, Scope: "container"}}},
	})

	for i := 0; i < 2; i++ {
		wu, err := wordpress.AddUnit()
		c.Assert(err, IsNil)
		c.Assert(wu.Tag(), Equals, fmt.Sprintf("unit-wordpress-%d", i))

		m, err := s.State.AddMachine("series", JobHostUnits)
		c.Assert(err, IsNil)
		c.Assert(m.Tag(), Equals, fmt.Sprintf("machine-%d", i+1))

		add(&params.UnitInfo{
			Name:      fmt.Sprintf("wordpress/%d", i),
			Service:   wordpress.Name(),
			Series:    m.Series(),
			MachineId: m.Id(),
			Ports:     []params.Port{},
			Status:    params.UnitPending,
		})
		pairs := map[string]string{"name": fmt.Sprintf("bar %d", i)}
		err = wu.SetAnnotations(pairs)
		c.Assert(err, IsNil)
		add(&params.AnnotationInfo{
			Tag:         fmt.Sprintf("unit-wordpress-%d", i),
			Annotations: pairs,
		})

		err = m.SetInstanceId(InstanceId("i-" + m.Tag()))
		c.Assert(err, IsNil)
		err = m.SetStatus(params.MachineError, m.Tag())
		c.Assert(err, IsNil)
		add(&params.MachineInfo{
			Id:         fmt.Sprint(i + 1),
			InstanceId: "i-" + m.Tag(),
			Status:     params.MachineError,
			StatusInfo: m.Tag(),
		})
		err = wu.AssignToMachine(m)
		c.Assert(err, IsNil)

		deployer, ok := wu.DeployerTag()
		c.Assert(ok, Equals, true)
		c.Assert(deployer, Equals, fmt.Sprintf("machine-%d", i+1))

		wru, err := rel.Unit(wu)
		c.Assert(err, IsNil)

		// Create the subordinate unit as a side-effect of entering
		// scope in the principal's relation-unit.
		err = wru.EnterScope(nil)
		c.Assert(err, IsNil)

		lu, err := s.State.Unit(fmt.Sprintf("logging/%d", i))
		c.Assert(err, IsNil)
		c.Assert(lu.IsPrincipal(), Equals, false)
		deployer, ok = lu.DeployerTag()
		c.Assert(ok, Equals, true)
		c.Assert(deployer, Equals, fmt.Sprintf("unit-wordpress-%d", i))
		add(&params.UnitInfo{
			Name:    fmt.Sprintf("logging/%d", i),
			Service: "logging",
			Series:  "series",
			Ports:   []params.Port{},
			Status:  params.UnitPending,
		})
	}
	return
}

func serviceCharmURL(svc *Service) *charm.URL {
	url, _ := svc.CharmURL()
	return url
}

func assertEntitiesEqual(c *C, got, want []params.EntityInfo) {
	if len(got) == 0 {
		got = nil
	}
	if len(want) == 0 {
		want = nil
	}
	if reflect.DeepEqual(got, want) {
		return
	}
	c.Errorf("entity mismatch; got len %d; want %d", len(got), len(want))
	c.Logf("got:")
	for _, e := range got {
		c.Logf("\t%T %#v", e, e)
	}
	c.Logf("expected:")
	for _, e := range want {
		c.Logf("\t%T %#v", e, e)
	}
	c.FailNow()
}

func (s *storeManagerStateSuite) TestStateBackingGetAll(c *C) {
	expectEntities := s.setUpScenario(c)
	b := newAllWatcherStateBacking(s.State)
	all := multiwatcher.NewStore()
	err := b.GetAll(all)
	c.Assert(err, IsNil)
	var gotEntities entityInfoSlice = all.All()
	sort.Sort(gotEntities)
	sort.Sort(expectEntities)
	assertEntitiesEqual(c, gotEntities, expectEntities)
}

var allWatcherChangedTests = []struct {
	about          string
	add            []params.EntityInfo
	setUp          func(c *C, st *State)
	change         watcher.Change
	expectContents []params.EntityInfo
}{
	// Machine changes
	{
		about: "no machine in state, no machine in store -> do nothing",
		setUp: func(*C, *State) {},
		change: watcher.Change{
			C:  "machines",
			Id: "1",
		},
	}, {
		about: "machine is removed if it's not in backing",
		add:   []params.EntityInfo{&params.MachineInfo{Id: "1"}},
		setUp: func(*C, *State) {},
		change: watcher.Change{
			C:  "machines",
			Id: "1",
		},
	}, {
		about: "machine is added if it's in backing but not in Store",
		setUp: func(c *C, st *State) {
			m, err := st.AddMachine("series", JobHostUnits)
			c.Assert(err, IsNil)
			err = m.SetStatus(params.MachineError, "failure")
			c.Assert(err, IsNil)
		},
		change: watcher.Change{
			C:  "machines",
			Id: "0",
		},
		expectContents: []params.EntityInfo{
			&params.MachineInfo{
				Id:         "0",
				Status:     params.MachineError,
				StatusInfo: "failure",
			},
		},
	}, {
		about: "machine is updated if it's in backing and in Store",
		add: []params.EntityInfo{
			&params.MachineInfo{
				Id:         "0",
				Status:     params.MachineError,
				StatusInfo: "another failure",
			},
		},
		setUp: func(c *C, st *State) {
			m, err := st.AddMachine("series", JobManageEnviron)
			c.Assert(err, IsNil)
			err = m.SetInstanceId("i-0")
			c.Assert(err, IsNil)
		},
		change: watcher.Change{
			C:  "machines",
			Id: "0",
		},
		expectContents: []params.EntityInfo{
			&params.MachineInfo{
				Id:         "0",
				InstanceId: "i-0",
				Status:     params.MachineError,
				StatusInfo: "another failure",
			},
		},
	},
	// Unit changes
	{
		about: "no unit in state, no unit in store -> do nothing",
		setUp: func(c *C, st *State) {},
		change: watcher.Change{
			C:  "units",
			Id: "1",
		},
	}, {
		about: "unit is removed if it's not in backing",
		add:   []params.EntityInfo{&params.UnitInfo{Name: "wordpress/1"}},
		setUp: func(*C, *State) {},
		change: watcher.Change{
			C:  "units",
			Id: "wordpress/1",
		},
	}, {
		about: "unit is added if it's in backing but not in Store",
		setUp: func(c *C, st *State) {
			wordpress, err := st.AddService("wordpress", AddTestingCharm(c, st, "wordpress"))
			c.Assert(err, IsNil)
			u, err := wordpress.AddUnit()
			c.Assert(err, IsNil)
			err = u.SetPublicAddress("public")
			c.Assert(err, IsNil)
			err = u.SetPrivateAddress("private")
			c.Assert(err, IsNil)
			err = u.SetResolved(params.ResolvedRetryHooks)
			c.Assert(err, IsNil)
			err = u.OpenPort("tcp", 12345)
			c.Assert(err, IsNil)
			m, err := st.AddMachine("series", JobHostUnits)
			c.Assert(err, IsNil)
			err = u.AssignToMachine(m)
			c.Assert(err, IsNil)
			err = u.SetStatus(params.UnitError, "failure")
			c.Assert(err, IsNil)
		},
		change: watcher.Change{
			C:  "units",
			Id: "wordpress/0",
		},
		expectContents: []params.EntityInfo{
			&params.UnitInfo{
				Name:           "wordpress/0",
				Service:        "wordpress",
				Series:         "series",
				PublicAddress:  "public",
				PrivateAddress: "private",
				MachineId:      "0",
				Resolved:       params.ResolvedRetryHooks,
				Ports:          []params.Port{{"tcp", 12345}},
				Status:         params.UnitError,
				StatusInfo:     "failure",
			},
		},
	}, {
		about: "unit is updated if it's in backing and in multiwatcher.Store",
		add: []params.EntityInfo{&params.UnitInfo{
			Name:       "wordpress/0",
			Status:     params.UnitError,
			StatusInfo: "another failure",
		}},
		setUp: func(c *C, st *State) {
			wordpress, err := st.AddService("wordpress", AddTestingCharm(c, st, "wordpress"))
			c.Assert(err, IsNil)
			u, err := wordpress.AddUnit()
			c.Assert(err, IsNil)
			err = u.SetPublicAddress("public")
			c.Assert(err, IsNil)
			err = u.OpenPort("udp", 17070)
			c.Assert(err, IsNil)
		},
		change: watcher.Change{
			C:  "units",
			Id: "wordpress/0",
		},
		expectContents: []params.EntityInfo{
			&params.UnitInfo{
				Name:          "wordpress/0",
				Service:       "wordpress",
				Series:        "series",
				PublicAddress: "public",
				Ports:         []params.Port{{"udp", 17070}},
				Status:        params.UnitError,
				StatusInfo:    "another failure",
			},
		},
	},
	// Service changes
	{
		about: "no service in state, no service in store -> do nothing",
		setUp: func(c *C, st *State) {},
		change: watcher.Change{
			C:  "services",
			Id: "wordpress",
		},
	}, {
		about: "service is removed if it's not in backing",
		add:   []params.EntityInfo{&params.ServiceInfo{Name: "wordpress"}},
		setUp: func(*C, *State) {},
		change: watcher.Change{
			C:  "services",
			Id: "wordpress",
		},
	}, {
		about: "service is added if it's in backing but not in Store",
		setUp: func(c *C, st *State) {
			wordpress, err := st.AddService("wordpress", AddTestingCharm(c, st, "wordpress"))
			c.Assert(err, IsNil)
			err = wordpress.SetExposed()
			c.Assert(err, IsNil)
		},
		change: watcher.Change{
			C:  "services",
			Id: "wordpress",
		},
		expectContents: []params.EntityInfo{
			&params.ServiceInfo{
				Name:     "wordpress",
				Exposed:  true,
				CharmURL: "local:series/series-wordpress-3",
			},
		},
	}, {
		about: "service is updated if it's in backing and in multiwatcher.Store",
		add: []params.EntityInfo{&params.ServiceInfo{
			Name:    "wordpress",
			Exposed: true,
		}},
		setUp: func(c *C, st *State) {
			_, err := st.AddService("wordpress", AddTestingCharm(c, st, "wordpress"))
			c.Assert(err, IsNil)
		},
		change: watcher.Change{
			C:  "services",
			Id: "wordpress",
		},
		expectContents: []params.EntityInfo{
			&params.ServiceInfo{
				Name:     "wordpress",
				CharmURL: "local:series/series-wordpress-3",
			},
		},
	},
	// Relation changes
	{
		about: "no relation in state, no service in store -> do nothing",
		setUp: func(c *C, st *State) {},
		change: watcher.Change{
			C:  "relations",
			Id: "logging:logging-directory wordpress:logging-dir",
		},
	}, {
		about: "relation is removed if it's not in backing",
		add:   []params.EntityInfo{&params.RelationInfo{Key: "logging:logging-directory wordpress:logging-dir"}},
		setUp: func(*C, *State) {},
		change: watcher.Change{
			C:  "relations",
			Id: "logging:logging-directory wordpress:logging-dir",
		},
	}, {
		about: "relation is added if it's in backing but not in Store",
		setUp: func(c *C, st *State) {
			_, err := st.AddService("wordpress", AddTestingCharm(c, st, "wordpress"))
			c.Assert(err, IsNil)

			_, err = st.AddService("logging", AddTestingCharm(c, st, "logging"))
			c.Assert(err, IsNil)
			eps, err := st.InferEndpoints([]string{"logging", "wordpress"})
			c.Assert(err, IsNil)
			_, err = st.AddRelation(eps...)
			c.Assert(err, IsNil)
		},
		change: watcher.Change{
			C:  "relations",
			Id: "logging:logging-directory wordpress:logging-dir",
		},
		expectContents: []params.EntityInfo{
			&params.RelationInfo{
				Key: "logging:logging-directory wordpress:logging-dir",
				Endpoints: []params.Endpoint{
					params.Endpoint{ServiceName: "logging", Relation: charm.Relation{Name: "logging-directory", Role: "requirer", Interface: "logging", Optional: false, Limit: 1, Scope: "container"}},
					params.Endpoint{ServiceName: "wordpress", Relation: charm.Relation{Name: "logging-dir", Role: "provider", Interface: "logging", Optional: false, Limit: 0, Scope: "container"}}},
			},
		},
	},
	// Annotation changes
	{
		about: "no annotation in state, no annotation in store -> do nothing",
		setUp: func(c *C, st *State) {},
		change: watcher.Change{
			C:  "relations",
			Id: "m#0",
		},
	}, {
		about: "annotation is removed if it's not in backing",
		add:   []params.EntityInfo{&params.AnnotationInfo{Tag: "machine-0"}},
		setUp: func(*C, *State) {},
		change: watcher.Change{
			C:  "annotations",
			Id: "m#0",
		},
	}, {
		about: "annotation is added if it's in backing but not in Store",
		setUp: func(c *C, st *State) {
			m, err := st.AddMachine("series", JobHostUnits)
			c.Assert(err, IsNil)
			err = m.SetAnnotations(map[string]string{"foo": "bar", "arble": "baz"})
			c.Assert(err, IsNil)
		},
		change: watcher.Change{
			C:  "annotations",
			Id: "m#0",
		},
		expectContents: []params.EntityInfo{
			&params.AnnotationInfo{
				Tag:         "machine-0",
				Annotations: map[string]string{"foo": "bar", "arble": "baz"},
			},
		},
	}, {
		about: "annotation is updated if it's in backing and in multiwatcher.Store",
		add: []params.EntityInfo{&params.AnnotationInfo{
			Tag: "machine-0",
			Annotations: map[string]string{
				"arble":  "baz",
				"foo":    "bar",
				"pretty": "polly",
			},
		}},
		setUp: func(c *C, st *State) {
			m, err := st.AddMachine("series", JobHostUnits)
			c.Assert(err, IsNil)
			err = m.SetAnnotations(map[string]string{
				"arble":  "khroomph",
				"pretty": "",
				"new":    "attr",
			})
			c.Assert(err, IsNil)
		},
		change: watcher.Change{
			C:  "annotations",
			Id: "m#0",
		},
		expectContents: []params.EntityInfo{
			&params.AnnotationInfo{
				Tag: "machine-0",
				Annotations: map[string]string{
					"arble": "khroomph",
					"new":   "attr",
				},
			},
		},
	},
	// Unit status changes
	{
		about: "no unit in state -> do nothing",
		setUp: func(c *C, st *State) {},
		change: watcher.Change{
			C:  "statuses",
			Id: "u#wordpress/0",
		},
	}, {
		about: "no change if status is not in backing",
		add: []params.EntityInfo{&params.UnitInfo{
			Name:       "wordpress/0",
			Status:     params.UnitError,
			StatusInfo: "failure",
		}},
		setUp: func(*C, *State) {},
		change: watcher.Change{
			C:  "statuses",
			Id: "u#wordpress/0",
		},
		expectContents: []params.EntityInfo{
			&params.UnitInfo{
				Name:       "wordpress/0",
				Status:     params.UnitError,
				StatusInfo: "failure",
			},
		},
	}, {
		about: "status is changed if the unit exists in the store",
		add: []params.EntityInfo{&params.UnitInfo{
			Name:       "wordpress/0",
			Status:     params.UnitError,
			StatusInfo: "failure",
		}},
		setUp: func(c *C, st *State) {
			wordpress, err := st.AddService("wordpress", AddTestingCharm(c, st, "wordpress"))
			c.Assert(err, IsNil)
			u, err := wordpress.AddUnit()
			c.Assert(err, IsNil)
			err = u.SetStatus(params.UnitStarted, "")
			c.Assert(err, IsNil)
		},
		change: watcher.Change{
			C:  "statuses",
			Id: "u#wordpress/0",
		},
		expectContents: []params.EntityInfo{
			&params.UnitInfo{
				Name:   "wordpress/0",
				Status: params.UnitStarted,
			},
		},
	},
	// Machine status changes
	{
		about: "no machine in state -> do nothing",
		setUp: func(c *C, st *State) {},
		change: watcher.Change{
			C:  "statuses",
			Id: "m#0",
		},
	}, {
		about: "no change if status is not in backing",
		add: []params.EntityInfo{&params.MachineInfo{
			Id:         "0",
			Status:     params.MachineError,
			StatusInfo: "failure",
		}},
		setUp: func(*C, *State) {},
		change: watcher.Change{
			C:  "statuses",
			Id: "m#0",
		},
		expectContents: []params.EntityInfo{&params.MachineInfo{
			Id:         "0",
			Status:     params.MachineError,
			StatusInfo: "failure",
		}},
	}, {
		about: "status is changed if the machine exists in the store",
		add: []params.EntityInfo{&params.MachineInfo{
			Id:         "0",
			Status:     params.MachineError,
			StatusInfo: "failure",
		}},
		setUp: func(c *C, st *State) {
			m, err := st.AddMachine("series", JobHostUnits)
			c.Assert(err, IsNil)
			err = m.SetStatus(params.MachineStarted, "")
			c.Assert(err, IsNil)
		},
		change: watcher.Change{
			C:  "statuses",
			Id: "m#0",
		},
		expectContents: []params.EntityInfo{
			&params.MachineInfo{
				Id:     "0",
				Status: params.MachineStarted,
			},
		},
	},
	// Service constraints changes
	{
		about: "no service in state -> do nothing",
		setUp: func(c *C, st *State) {},
		change: watcher.Change{
			C:  "constraints",
			Id: "s#wordpress",
		},
	}, {
		about: "no change if service is not in backing",
		add: []params.EntityInfo{&params.ServiceInfo{
			Name:        "wordpress",
			Constraints: constraints.MustParse("mem=99M"),
		}},
		setUp: func(*C, *State) {},
		change: watcher.Change{
			C:  "constraints",
			Id: "s#wordpress",
		},
		expectContents: []params.EntityInfo{&params.ServiceInfo{
			Name:        "wordpress",
			Constraints: constraints.MustParse("mem=99M"),
		}},
	}, {
		about: "status is changed if the service exists in the store",
		add: []params.EntityInfo{&params.ServiceInfo{
			Name:        "wordpress",
			Constraints: constraints.MustParse("mem=99M cpu-cores=2 cpu-power=4"),
		}},
		setUp: func(c *C, st *State) {
			svc, err := st.AddService("wordpress", AddTestingCharm(c, st, "wordpress"))
			c.Assert(err, IsNil)
			err = svc.SetConstraints(constraints.MustParse("mem=4G cpu-cores= arch=amd64"))
			c.Assert(err, IsNil)
		},
		change: watcher.Change{
			C:  "constraints",
			Id: "s#wordpress",
		},
		expectContents: []params.EntityInfo{
			&params.ServiceInfo{
				Name:        "wordpress",
				Constraints: constraints.MustParse("mem=4G cpu-cores= arch=amd64"),
			},
		},
	},
}

func (s *storeManagerStateSuite) TestChanged(c *C) {
	collections := map[string]*mgo.Collection{
		"machines":    s.State.machines,
		"units":       s.State.units,
		"services":    s.State.services,
		"relations":   s.State.relations,
		"annotations": s.State.annotations,
		"statuses":    s.State.statuses,
		"constraints": s.State.constraints,
	}
	for i, test := range allWatcherChangedTests {
		c.Logf("test %d. %s", i, test.about)
		b := newAllWatcherStateBacking(s.State)
		all := multiwatcher.NewStore()
		for _, info := range test.add {
			all.Update(info)
		}
		test.setUp(c, s.State)
		c.Logf("done set up")
		ch := test.change
		ch.C = collections[ch.C].Name
		err := b.Changed(all, test.change)
		c.Assert(err, IsNil)
		assertEntitiesEqual(c, all.All(), test.expectContents)
		s.Reset(c)
	}
}

// StateWatcher tests the integration of the state watcher
// with the state-based backing. Most of the logic is tested elsewhere -
// this just tests end-to-end.
func (s *storeManagerStateSuite) TestStateWatcher(c *C) {
	m0, err := s.State.AddMachine("series", JobManageEnviron)
	c.Assert(err, IsNil)
	c.Assert(m0.Id(), Equals, "0")

	m1, err := s.State.AddMachine("series", JobHostUnits)
	c.Assert(err, IsNil)
	c.Assert(m1.Id(), Equals, "1")

	b := newAllWatcherStateBacking(s.State)
	aw := multiwatcher.NewStoreManager(b)
	defer aw.Stop()
	w := multiwatcher.NewWatcher(aw)
	s.State.StartSync()
	checkNext(c, w, b, []params.Delta{{
		Entity: &params.MachineInfo{
			Id:     "0",
			Status: params.MachinePending,
		},
	}, {
		Entity: &params.MachineInfo{
			Id:     "1",
			Status: params.MachinePending,
		},
	}}, "")

	// Make some changes to the state.
	err = m0.SetInstanceId("i-0")
	c.Assert(err, IsNil)
	err = m1.Destroy()
	c.Assert(err, IsNil)
	err = m1.EnsureDead()
	c.Assert(err, IsNil)
	err = m1.Remove()
	c.Assert(err, IsNil)
	m2, err := s.State.AddMachine("series", JobManageEnviron)
	c.Assert(err, IsNil)
	c.Assert(m2.Id(), Equals, "2")
	s.State.StartSync()

	// Check that we see the changes happen within a
	// reasonable time.
	var deltas []params.Delta
	for {
		d, err := getNext(c, w, 100*time.Millisecond)
		if err == errTimeout {
			break
		}
		c.Assert(err, IsNil)
		deltas = append(deltas, d...)
	}
	checkDeltasEqual(c, b, deltas, []params.Delta{{
		Removed: true,
		Entity: &params.MachineInfo{
			Id:     "1",
			Status: params.MachinePending,
		},
	}, {
		Entity: &params.MachineInfo{
			Id:     "2",
			Status: params.MachinePending,
		},
	}, {
		Entity: &params.MachineInfo{
			Id:         "0",
			InstanceId: "i-0",
			Status:     params.MachinePending,
		},
	}})

	err = w.Stop()
	c.Assert(err, IsNil)

	_, err = w.Next()
	c.Assert(err, ErrorMatches, "state watcher was stopped")
}

type entityInfoSlice []params.EntityInfo

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

var errTimeout = errors.New("no change received in sufficient time")

func getNext(c *C, w *multiwatcher.Watcher, timeout time.Duration) ([]params.Delta, error) {
	var deltas []params.Delta
	var err error
	ch := make(chan struct{}, 1)
	go func() {
		deltas, err = w.Next()
		ch <- struct{}{}
	}()
	select {
	case <-ch:
		return deltas, err
	case <-time.After(1 * time.Second):
	}
	return nil, errTimeout
}

func checkNext(c *C, w *multiwatcher.Watcher, b multiwatcher.Backing, deltas []params.Delta, expectErr string) {
	d, err := getNext(c, w, 1*time.Second)
	if expectErr != "" {
		c.Check(err, ErrorMatches, expectErr)
		return
	}
	checkDeltasEqual(c, b, d, deltas)
}

// deltas are returns in arbitrary order, so we compare
// them as sets.
func checkDeltasEqual(c *C, b multiwatcher.Backing, d0, d1 []params.Delta) {
	c.Check(deltaMap(d0, b), DeepEquals, deltaMap(d1, b))
}

func deltaMap(deltas []params.Delta, b multiwatcher.Backing) map[multiwatcher.InfoId]params.EntityInfo {
	m := make(map[multiwatcher.InfoId]params.EntityInfo)
	for _, d := range deltas {
		id := d.Entity.EntityId()
		if _, ok := m[id]; ok {
			panic(fmt.Errorf("%v mentioned twice in delta set", id))
		}
		if d.Removed {
			m[id] = nil
		} else {
			m[id] = d.Entity
		}
	}
	return m
}
