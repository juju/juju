package state

import (
	"container/list"
	"fmt"
	. "launchpad.net/gocheck"
	"launchpad.net/juju-core/testing"
)

type AllInfoSuite struct {
	testing.LoggingSuite
}

var _ = Suite(&AllInfoSuite{})

// assertContents checks that the given allWatcher
// has the given contents, in oldest-to-newest order.
func (*AllInfoSuite) assertContents(c *C, a *allInfo, latestRevno int64, entries []entityEntry) {
	var gotEntries []entityEntry
	var gotElems []*list.Element
	c.Assert(a.list.Len(), Equals, len(entries))
	for e := a.list.Back(); e != nil; e = e.Prev() {
		gotEntries = append(gotEntries, *e.Value.(*entityEntry))
		gotElems = append(gotElems, e)
	}

	c.Assert(gotEntries, DeepEquals, entries)
	for i, ent := range entries {
		c.Assert(a.entities[entityIdForInfo(ent.info)], Equals, gotElems[i])
	}
	c.Assert(a.entities, HasLen, len(entries))
	c.Assert(a.latestRevno, Equals, latestRevno)
}

func (s *AllInfoSuite) TestAdd(c *C) {
	a := newAllInfo()
	s.assertContents(c, a, 0, nil)

	allInfoAdd(a, &MachineInfo{
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

	allInfoAdd(a, &ServiceInfo{
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

var updateTests = []struct {
	about  string
	add    []EntityInfo
	update EntityInfo
	result []entityEntry
}{{
	about: "update an entity that's not currently there",
	update: &MachineInfo{
		Id:         "0",
		InstanceId: "i-0",
	},
	result: []entityEntry{{
		revno: 1,
		info: &MachineInfo{
			Id:         "0",
			InstanceId: "i-0",
		},
	}},
},
}

func (s *AllInfoSuite) TestUpdate(c *C) {
	for i, test := range updateTests {
		a := newAllInfo()
		c.Logf("test %d. %s", i, test.about)
		for _, info := range test.add {
			allInfoAdd(a, info)
		}
		a.update(entityIdForInfo(test.update), test.update)
		s.assertContents(c, a, test.result[len(test.result)-1].revno, test.result)
	}
}

func entityIdForInfo(info EntityInfo) entityId {
	return entityId{
		collection: info.EntityKind(),
		id:         info.EntityId(),
	}
}

func allInfoAdd(a *allInfo, info EntityInfo) {
	a.add(entityIdForInfo(info), info)
}

func (s *AllInfoSuite) TestMarkRemoved(c *C) {
	a := newAllInfo()
	allInfoAdd(a, &MachineInfo{Id: "0"})
	allInfoAdd(a, &MachineInfo{Id: "1"})
	a.markRemoved(entityId{"machine", "0"})
	s.assertContents(c, a, 3, []entityEntry{{
		revno: 2,
		info:  &MachineInfo{Id: "1"},
	}, {
		revno:   3,
		removed: true,
		info:    &MachineInfo{Id: "0"},
	}})
}

func (s *AllInfoSuite) TestMarkRemovedNonExistent(c *C) {
	a := newAllInfo()
	a.markRemoved(entityId{"machine", "0"})
	s.assertContents(c, a, 0, nil)
}

func (s *AllInfoSuite) TestDelete(c *C) {
	a := newAllInfo()
	allInfoAdd(a, &MachineInfo{Id: "0"})
	a.delete(entityId{"machine", "0"})
	s.assertContents(c, a, 1, nil)
}

func (s *AllInfoSuite) TestChangesSince(c *C) {
	a := newAllInfo()
	var deltas []Delta
	for i := 0; i < 3; i++ {
		m := &MachineInfo{Id: fmt.Sprint(i)}
		allInfoAdd(a, m)
		deltas = append(deltas, Delta{Entity: m})
	}
	for i := 0; i < 3; i++ {
		c.Logf("test %d", i)
		c.Assert(a.changesSince(int64(i)), DeepEquals, deltas[i:])
	}

	c.Assert(a.changesSince(-1), DeepEquals, deltas)
	c.Assert(a.changesSince(99), HasLen, 0)

	rev := a.latestRevno
	m1 := &MachineInfo{
		Id:         "1",
		InstanceId: "foo",
	}
	a.update(entityIdForInfo(m1), m1)
	c.Assert(a.changesSince(rev), DeepEquals, []Delta{{Entity: m1}})

	m0 := &MachineInfo{Id: "0"}
	a.markRemoved(entityIdForInfo(m0))
	c.Assert(a.changesSince(rev), DeepEquals, []Delta{{
		Entity: m1,
	}, {
		Remove: true,
		Entity: m0,
	}})

	c.Assert(a.changesSince(rev+1), DeepEquals, []Delta{{
		Remove: true,
		Entity: m0,
	}})

}
