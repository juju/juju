package state

import (
	"fmt"
	"github.com/davecgh/go-spew/spew"
	. "launchpad.net/gocheck"
	"launchpad.net/juju-core/charm"
	"launchpad.net/juju-core/testing"
	"net/url"
	"sort"
)

type AllInfoSuite struct {
	testing.LoggingSuite
	testing.MgoSuite
	State *State
}

func (s *AllInfoSuite) SetUpSuite(c *C) {
	s.LoggingSuite.SetUpSuite(c)
	s.MgoSuite.SetUpSuite(c)
}

func (s *AllInfoSuite) TearDownSuite(c *C) {
	s.MgoSuite.TearDownSuite(c)
	s.LoggingSuite.TearDownSuite(c)
}

func (s *AllInfoSuite) SetUpTest(c *C) {
	s.LoggingSuite.SetUpTest(c)
	s.MgoSuite.SetUpTest(c)
	state, err := Open(TestingStateInfo())
	c.Assert(err, IsNil)

	s.State = state
}

type entityInfoSlice []EntityInfo

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

var _ = Suite(&AllInfoSuite{})

func (s *AllInfoSuite) setUpScenario(c *C) (entities entityInfoSlice) {
	add := func(e EntityInfo) {
		entities = append(entities, e)
	}
	m, err := s.State.AddMachine("series", JobManageEnviron)
	c.Assert(err, IsNil)
	c.Assert(m.EntityName(), Equals, "machine-0")
	err = m.SetInstanceId(InstanceId("i-" + m.EntityName()))
	c.Assert(err, IsNil)
	add(&MachineInfo{
		Id:         "0",
		InstanceId: "i-machine-0",
	})

	wordpress, err := s.State.AddService("wordpress", AddTestingCharm(c, s.State, "wordpress"))
	c.Assert(err, IsNil)
	err = wordpress.SetExposed()
	c.Assert(err, IsNil)
	add(&ServiceInfo{
		Name:    "wordpress",
		Exposed: true,
	})

	_, err = s.State.AddService("logging", AddTestingCharm(c, s.State, "logging"))
	c.Assert(err, IsNil)
	add(&ServiceInfo{
		Name: "logging",
	})

	eps, err := s.State.InferEndpoints([]string{"logging", "wordpress"})
	c.Assert(err, IsNil)
	rel, err := s.State.AddRelation(eps...)
	c.Assert(err, IsNil)
	add(&RelationInfo{
		Key: "logging:logging-directory wordpress:logging-dir",
	})

	for i := 0; i < 2; i++ {
		wu, err := wordpress.AddUnit()
		c.Assert(err, IsNil)
		c.Assert(wu.EntityName(), Equals, fmt.Sprintf("unit-wordpress-%d", i))
		add(&UnitInfo{
			Name:    fmt.Sprintf("wordpress/%d", i),
			Service: "wordpress",
		})

		m, err := s.State.AddMachine("series", JobHostUnits)
		c.Assert(err, IsNil)
		c.Assert(m.EntityName(), Equals, fmt.Sprintf("machine-%d", i+1))
		err = m.SetInstanceId(InstanceId("i-" + m.EntityName()))
		c.Assert(err, IsNil)
		add(&MachineInfo{
			Id:         fmt.Sprint(i + 1),
			InstanceId: "i-" + m.EntityName(),
		})
		err = wu.AssignToMachine(m)
		c.Assert(err, IsNil)

		deployer, ok := wu.DeployerName()
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
		deployer, ok = lu.DeployerName()
		c.Assert(ok, Equals, true)
		c.Assert(deployer, Equals, fmt.Sprintf("unit-wordpress-%d", i))
		add(&UnitInfo{
			Name:    fmt.Sprintf("logging/%d", i),
			Service: "logging",
		})
	}
	return
}

func (s *AllInfoSuite) TestNewAllInfo(c *C) {
	expectEntities := s.setUpScenario(c)
	sort.Sort(expectEntities)
	a, err := newAllInfo(s.State)
	c.Assert(err, IsNil)

	// Check that all the entities have been placed
	// into the list.
	var gotEntities entityInfoSlice
	c.Check(a.latestRevno, Equals, int64(len(expectEntities)))
	i := int64(0)
	for e := a.list.Front(); e != nil; e = e.Next() {
		entry := e.Value.(*entityEntry)
		gotEntities = append(gotEntities, entry.info)
		c.Check(entry.revno, Equals, a.latestRevno-i)
		i++
		c.Assert(a.entities[infoEntityId(s.State, entry.info)], Equals, e)
	}
	c.Assert(len(a.entities), Equals, int(i))

	sort.Sort(gotEntities)
	for _, e := range gotEntities {
		c.Logf("%#v %#v\n", e.EntityKind(), e.EntityId())
	}
	c.Logf("--------------------------- got")
	spew.Dump(gotEntities)
	c.Logf("--------------------------- expect")
	spew.Dump(expectEntities)
	c.Logf("---------------------------")
	c.Assert(gotEntities, DeepEquals, expectEntities)
}

func AddTestingCharm(c *C, st *State, name string) *Charm {
	ch := testing.Charms.Dir(name)
	ident := fmt.Sprintf("%s-%d", name, ch.Revision())
	curl := charm.MustParseURL("local:series/" + ident)
	bundleURL, err := url.Parse("http://bundles.example.com/" + ident)
	c.Assert(err, IsNil)
	sch, err := st.AddCharm(ch, curl, bundleURL, ident+"-sha256")
	c.Assert(err, IsNil)
	return sch
}
