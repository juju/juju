package state

import (
	"errors"
	"fmt"
	"labix.org/v2/mgo"
	. "launchpad.net/gocheck"
	"launchpad.net/juju-core/charm"
	"launchpad.net/juju-core/state/api/params"
	"launchpad.net/juju-core/state/allwatcher"
	"launchpad.net/juju-core/state/watcher"
	"launchpad.net/juju-core/testing"
	"sort"
	"time"
)

type allWatcherStateSuite struct {
	testing.LoggingSuite
	testing.MgoSuite
	State *State
}

func (s *allWatcherStateSuite) SetUpSuite(c *C) {
	s.LoggingSuite.SetUpSuite(c)
	s.MgoSuite.SetUpSuite(c)
}

func (s *allWatcherStateSuite) TearDownSuite(c *C) {
	s.MgoSuite.TearDownSuite(c)
	s.LoggingSuite.TearDownSuite(c)
}

func (s *allWatcherStateSuite) SetUpTest(c *C) {
	s.LoggingSuite.SetUpTest(c)
	s.MgoSuite.SetUpTest(c)
	s.State = TestingInitialize(c, nil)
}

func (s *allWatcherStateSuite) TearDownTest(c *C) {
	s.State.Close()
	s.MgoSuite.TearDownTest(c)
	s.LoggingSuite.TearDownTest(c)
}

func (s *allWatcherStateSuite) Reset(c *C) {
	s.TearDownTest(c)
	s.SetUpTest(c)
}

var _ = Suite(&allWatcherStateSuite{})

// setUpScenario adds some entities to the state so that
// we can check that they all get pulled in by
// allWatcherStateBacking.getAll.
func (s *allWatcherStateSuite) setUpScenario(c *C) (entities entityInfoSlice) {
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
	})

	wordpress, err := s.State.AddService("wordpress", AddTestingCharm(c, s.State, "wordpress"))
	c.Assert(err, IsNil)
	err = wordpress.SetExposed()
	c.Assert(err, IsNil)
	add(&params.ServiceInfo{
		Name:     "wordpress",
		Exposed:  true,
		CharmURL: serviceCharmURL(wordpress).String(),
	})
	pairs := map[string]string{"x": "12", "y": "99"}
	err = wordpress.SetAnnotations(pairs)
	c.Assert(err, IsNil)
	add(&params.AnnotationInfo{
		GlobalKey:   "s#wordpress",
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
		})
		pairs := map[string]string{"name": fmt.Sprintf("bar %d", i)}
		err = wu.SetAnnotations(pairs)
		c.Assert(err, IsNil)
		add(&params.AnnotationInfo{
			GlobalKey:   fmt.Sprintf("u#wordpress/%d", i),
			Tag:         fmt.Sprintf("unit-wordpress-%d", i),
			Annotations: pairs,
		})

		err = m.SetInstanceId(InstanceId("i-" + m.Tag()))
		c.Assert(err, IsNil)
		add(&params.MachineInfo{
			Id:         fmt.Sprint(i + 1),
			InstanceId: "i-" + m.Tag(),
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
		})
	}
	return
}

func serviceCharmURL(svc *Service) *charm.URL {
	url, _ := svc.CharmURL()
	return url
}

func (s *allWatcherStateSuite) TestStateBackingGetAll(c *C) {
	expectEntities := s.setUpScenario(c)
	b := newAllWatcherStateBacking(s.State)
	all := allwatcher.NewAllInfo()
	err := b.GetAll(all)
	c.Assert(err, IsNil)
	var gotEntities entityInfoSlice = all.All()
	sort.Sort(gotEntities)
	sort.Sort(expectEntities)
	c.Logf("got")
	for _, e := range gotEntities {
		c.Logf("\t%#v %#v %#v", e.EntityKind(), e.EntityId(), e)
	}
	c.Logf("expected")
	for _, e := range expectEntities {
		c.Logf("\t%#v %#v %#v", e.EntityKind(), e.EntityId(), e)
	}
	for num, ent := range expectEntities {
		c.Logf("---------------> %d\n", num)
		c.Logf("\n************ EXPECTED:\n%#v", ent)
		c.Logf("************ OBTAINED: \n%#v\n", gotEntities[num])
		c.Assert(gotEntities[num], DeepEquals, ent)
	}

	c.Assert(gotEntities, DeepEquals, expectEntities)
}

func (s *allWatcherStateSuite) TestStateBackingEntityIdForInfo(c *C) {
	tests := []struct {
		info       params.EntityInfo
		collection *mgo.Collection
	}{{
		info:       &params.MachineInfo{Id: "1"},
		collection: s.State.machines,
	}, {
		info:       &params.UnitInfo{Name: "wordpress/1"},
		collection: s.State.units,
	}, {
		info:       &params.ServiceInfo{Name: "wordpress"},
		collection: s.State.services,
	}, {
		info:       &params.RelationInfo{Key: "logging:logging-directory wordpress:logging-dir"},
		collection: s.State.relations,
	}, {
		info:       &params.AnnotationInfo{GlobalKey: "m-0"},
		collection: s.State.annotations,
	}}
	b := newAllWatcherStateBacking(s.State)
	for i, test := range tests {
		c.Logf("test %d: %T", i, test.info)
		id := b.IdForInfo(test.info)
		c.Assert(id, Equals, entityId{
			collection: test.collection.Name,
			id:         test.info.EntityId(),
		})
	}
}

var allWatcherChangedTests = []struct {
	about          string
	add            []params.EntityInfo
	setUp          func(c *C, st *State)
	change         watcher.Change
	expectContents []params.EntityInfo
}{{
	about: "no entity",
	setUp: func(*C, *State) {},
	change: watcher.Change{
		C:  "machines",
		Id: "1",
	},
}, {
	about: "entity is removed if it's not in backing",
	add:   []params.EntityInfo{&params.MachineInfo{Id: "1"}},
	setUp: func(*C, *State) {},
	change: watcher.Change{
		C:  "machines",
		Id: "1",
	},
}, {
	about: "entity is added if it's in backing but not in allwatcher.AllInfo",
	setUp: func(c *C, st *State) {
		_, err := st.AddMachine("series", JobHostUnits)
		c.Assert(err, IsNil)
	},
	change: watcher.Change{
		C:  "machines",
		Id: "0",
	},
	expectContents: []params.EntityInfo{
		&params.MachineInfo{Id: "0"},
	},
}, {
	about: "entity is updated if it's in backing and in allwatcher.AllInfo",
	add:   []params.EntityInfo{&params.MachineInfo{Id: "0"}},
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
		},
	},
}}

func (s *allWatcherStateSuite) TestChanged(c *C) {
	for i, test := range allWatcherChangedTests {
		c.Logf("test %d. %s", i, test.about)
		b := newAllWatcherStateBacking(s.State)
		idOf := func(info params.EntityInfo) allwatcher.InfoId { return b.IdForInfo(info) }
		all := allwatcher.NewAllInfo()
		for _, info := range test.add {
			all.Update(idOf(info), info)
		}
		test.setUp(c, s.State)
		c.Logf("done set up")
		err := b.Changed(all, test.change)
		c.Assert(err, IsNil)
		assertAllInfoContents(c, all, test.expectContents)
		s.Reset(c)
	}
}

// Testallwatcher.StateWatcher tests the integration of the state watcher
// with the state-based backing. Most of the logic is tested elsewhere -
// this just tests end-to-end.
func (s *allWatcherStateSuite) TestStateWatcher(c *C) {
	m0, err := s.State.AddMachine("series", JobManageEnviron)
	c.Assert(err, IsNil)
	c.Assert(m0.Id(), Equals, "0")

	m1, err := s.State.AddMachine("series", JobHostUnits)
	c.Assert(err, IsNil)
	c.Assert(m1.Id(), Equals, "1")

	b := newAllWatcherStateBacking(s.State)
	aw := allwatcher.NewAllWatcher(b)
	go aw.Run()
	defer aw.Stop()
	w := allwatcher.NewStateWatcher(aw)
	s.State.StartSync()
	checkNext(c, w, b, []params.Delta{{
		Entity: &params.MachineInfo{Id: "0"},
	}, {
		Entity: &params.MachineInfo{Id: "1"},
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
		Entity:  &params.MachineInfo{Id: "1"},
	}, {
		Entity: &params.MachineInfo{Id: "2"},
	}, {
		Entity: &params.MachineInfo{
			Id:         "0",
			InstanceId: "i-0",
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
	if s[i].EntityKind() != s[j].EntityKind() {
		return s[i].EntityKind() < s[j].EntityKind()
	}
	switch id := s[i].EntityId().(type) {
	case string:
		return id < s[j].EntityId().(string)
	}
	panic("unknown id type")
}

func assertAllInfoContents(c *C, a *allwatcher.AllInfo, entries []params.EntityInfo) {
	gotEntries := a.All()
	if len(entries) == 0 {
		c.Assert(gotEntries, HasLen, 0)
	} else {
		c.Assert(gotEntries, DeepEquals, entries)
	}
}

var errTimeout = errors.New("no change received in sufficient time")

func getNext(c *C, w *allwatcher.StateWatcher, timeout time.Duration) ([]params.Delta, error) {
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

func checkNext(c *C, w *allwatcher.StateWatcher, b allwatcher.Backing, deltas []params.Delta, expectErr string) {
	d, err := getNext(c, w, 1*time.Second)
	if expectErr != "" {
		c.Check(err, ErrorMatches, expectErr)
		return
	}
	checkDeltasEqual(c, b, d, deltas)
}

// deltas are returns in arbitrary order, so we compare
// them as sets.
func checkDeltasEqual(c *C, b allwatcher.Backing, d0, d1 []params.Delta) {
	c.Check(deltaMap(d0, b), DeepEquals, deltaMap(d1, b))
}

func deltaMap(deltas []params.Delta, b allwatcher.Backing) map[allwatcher.InfoId]params.EntityInfo {
	idOf := func(info params.EntityInfo) allwatcher.InfoId { return b.IdForInfo(info) }
	m := make(map[allwatcher.InfoId]params.EntityInfo)
	for _, d := range deltas {
		id := idOf(d.Entity)
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
