package state

import (
	"fmt"
	. "launchpad.net/gocheck"
	"launchpad.net/juju-core/charm"
	"launchpad.net/juju-core/testing"
	"net/url"
	"sort"
)

type AllInfoSuite struct {
	testing.LoggingSuite
}

var _ = Suite(&AllInfoSuite{})

// assertContents checks that the given allWatcher
// has the given contents, in oldest-to-newest order.
func (*AllInfoSuite) assertContents(c *C, a *allInfo, latestRevno int64, entries []entityEntry) {
	i := 0
	for e := a.list.Back(); e != nil; e = e.Next() {
		c.Assert(i, Not(Equals), len(entries))
		entry := e.Value.(*entityEntry)
		c.Assert(entry, DeepEquals, &entries[i])
		c.Assert(a.entities[infoEntityId(a.st, entry.info)], Equals, e)
		i++
	}
	c.Assert(a.entities, HasLen, len(entries))
	c.Assert(a.latestRevno, Equals, latestRevno)
}

func (s *AllInfoSuite) TestAdd(c *C) {
	a, err := newAllInfo()
	c.Assert(err, IsNil)
	s.assertContents(c, a, 0, nil)

	a.add(&MachineInfo{
		Id:         "0",
		InstanceId: "i-0",
	})
	s.assertContents(c, a, 1, []entityEntry{{
		revno: 1,
		info: &MachineInfo{
			Id:         "0",
			InstanceId: "i-0",
		},
	}})

	a.add(&ServiceInfo{
		Name:    "wordpress",
		Exposed: true,
	})
	s.assertContents(c, a, 2, []entityEntry{{
		revno: 1,
		info: &MachineInfo{
			Id:         "0",
			InstanceId: "i-0",
		},
	}, {
		revno: 2,
		info: &ServiceInfo{
			Name:    "wordpress",
			Exposed: true,
		},
	}})
}


var updateTests []struct {
	about string
	add []EntityInfo
	update EntityInfo
	result []entityEntry
} {{
	about: "update an entity that's not currently there",
	update: &MachineInfo{
		Id: "0",
		InstanceId: "i-0",
	},
	result: []entityEntry{
		revno: 1,
		info: &MachineInfo{
			Id: "0",
			InstanceId: "i-0",
		},
	},
},
}

func idForInfo(info EntityInfo) (id entityId) {
	id.id = info.EntityId
	id.collection = info.EntityKind
}

func (s *AllInfoSuite) TestUpdate(c *C) {
	for i, test := range updateTests {
		a := newAllInfo()
		c.Logf("test %d. %s", i, test.about)
		for _, info := range test.add {
			allInfoAdd(info)
		}
		a.update(test.update)
		s.assertContents(c, a, test.result[len(test.result)-1].revno, test.result)
	}
}

func allInfoAdd(a *allInfo, info EntityInfo) {
	a.add(entityId{
		collection: info.EntityKind(),
		id: info.EntityId(),
	}, info)
}

func (s *AllInfoSuite) TestMarkRemoved(c *C) {
	a := newAllInfo()
	allInfoAdd(&MachineInfo{
		Id: "0",
		InstanceId: "i-0",
	})
	a.markRemoved(entityId{"machine", "0"})
	s.assertContents(c, a, 1, []entityEntry{&MachineInfo{
		revno: 1,
		removed: true,
		info: &MachineInfo{
		Id: "0",
		InstanceId: "i-0",
	}})
}

func (s *AllInfoSuite) TestMarkRemovedNonExistent(c *C) {
	a := newAllInfo()
	a.markRemoved(entityId{"machine", "0"})
	s.assertContents(c, a, 0, nil)
}

func (s *AllInfoSuite)TestDelete(c *C) {
	a := newAllInfo()
	allInfoAdd(&MachineInfo{
		Id: "0",
		InstanceId: "i-0",
	})
	a.delete(entityId{"machine", "0"})
	s.assertContents(c, a, 1, nil)
}

type changesSinceTests []struct {
	revno int64
	deltas []Delta
} {{
}}



func (s *AllInfoSuite) TestChangesSince(c *C) {
	for i := 0; i < 3; i++ {
		allInfo.Add(&MachineInfo{Id: "0"})
		
	allInfoAdd(&MachineInfo{
		Id: "0",
		InstanceId: "i-0",
	})
	allInfoAdd(&MachineInfo{
		Id: "1",
		InstanceId: "i-1",
	})
	allInfoAdd(&ServiceInfo{
		Name: "wordpress",
		Exposed: true,
	})
	allInfoAdd(&ServiceInfo{
		Name: "logging",
		Exposed: true,
	})
	allInfoAdd(&UnitInfo{
		Name: "wordpress/0",
		Service: "wordpress",
	})
	allInfoAdd(&UnitInfo{

	
	machine 0
	machine 1
	service wordpress
	service logging
	unit wordpress/0
	unit logging/0
	relation
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
