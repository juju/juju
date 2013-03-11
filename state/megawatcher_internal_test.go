package state

import (
	"container/list"
	"fmt"
	. "launchpad.net/gocheck"
	"launchpad.net/juju-core/state/api/params"
	"launchpad.net/juju-core/testing"
	"labix.org/v2/mgo"
	"errors"
)

type allInfoSuite struct {
	testing.LoggingSuite
}

var _ = Suite(&allInfoSuite{})

// assertAllInfoContents checks that the given allWatcher
// has the given contents, in oldest-to-newest order.
func assertAllInfoContents(c *C, a *allInfo, latestRevno int64, entries []entityEntry) {
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

func (s *allInfoSuite) TestAdd(c *C) {
	a := newAllInfo()
	assertAllInfoContents(c, a, 0, nil)

	allInfoAdd(a, &params.MachineInfo{
		Id:         "0",
		InstanceId: "i-0",
	})
	assertAllInfoContents(c, a, 1, []entityEntry{{
		revno: 1,
		info: &params.MachineInfo{
			Id:         "0",
			InstanceId: "i-0",
		},
	}})

	allInfoAdd(a, &params.ServiceInfo{
		Name:    "wordpress",
		Exposed: true,
	})
	assertAllInfoContents(c, a, 2, []entityEntry{{
		revno: 1,
		info: &params.MachineInfo{
			Id:         "0",
			InstanceId: "i-0",
		},
	}, {
		revno: 2,
		info: &params.ServiceInfo{
			Name:    "wordpress",
			Exposed: true,
		},
	}})
}

var updateTests = []struct {
	about  string
	add    []params.EntityInfo
	update params.EntityInfo
	result []entityEntry
}{{
	about: "update an entity that's not currently there",
	update: &params.MachineInfo{
		Id:         "0",
		InstanceId: "i-0",
	},
	result: []entityEntry{{
		revno: 1,
		info: &params.MachineInfo{
			Id:         "0",
			InstanceId: "i-0",
		},
	}},
},
}

func (s *allInfoSuite) TestUpdate(c *C) {
	for i, test := range updateTests {
		a := newAllInfo()
		c.Logf("test %d. %s", i, test.about)
		for _, info := range test.add {
			allInfoAdd(a, info)
		}
		a.update(entityIdForInfo(test.update), test.update)
		assertAllInfoContents(c, a, test.result[len(test.result)-1].revno, test.result)
	}
}

func entityIdForInfo(info params.EntityInfo) entityId {
	return entityId{
		collection: info.EntityKind(),
		id:         info.EntityId(),
	}
}

func allInfoAdd(a *allInfo, info params.EntityInfo) {
	a.add(entityIdForInfo(info), info)
}

func (s *allInfoSuite) TestMarkRemoved(c *C) {
	a := newAllInfo()
	allInfoAdd(a, &params.MachineInfo{Id: "0"})
	allInfoAdd(a, &params.MachineInfo{Id: "1"})
	a.markRemoved(entityId{"machine", "0"})
	assertAllInfoContents(c, a, 3, []entityEntry{{
		revno: 2,
		info:  &params.MachineInfo{Id: "1"},
	}, {
		revno:   3,
		removed: true,
		info:    &params.MachineInfo{Id: "0"},
	}})
}

func (s *allInfoSuite) TestMarkRemovedNonExistent(c *C) {
	a := newAllInfo()
	a.markRemoved(entityId{"machine", "0"})
	assertAllInfoContents(c, a, 0, nil)
}

func (s *allInfoSuite) TestDelete(c *C) {
	a := newAllInfo()
	allInfoAdd(a, &params.MachineInfo{Id: "0"})
	a.delete(entityId{"machine", "0"})
	assertAllInfoContents(c, a, 1, nil)
}

func (s *allInfoSuite) TestChangesSince(c *C) {
	a := newAllInfo()
	var deltas []Delta
	for i := 0; i < 3; i++ {
		m := &params.MachineInfo{Id: fmt.Sprint(i)}
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
	m1 := &params.MachineInfo{
		Id:         "1",
		InstanceId: "foo",
	}
	a.update(entityIdForInfo(m1), m1)
	c.Assert(a.changesSince(rev), DeepEquals, []Delta{{Entity: m1}})

	m0 := &params.MachineInfo{Id: "0"}
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

type allWatcherSuite struct {
	testing.LoggingSuite
}

var _ = Suite(&allWatcherSuite{})

func (*allWatcherSuite) TestChangedFetchErrorReturn(c *C) {
	expectErr := errors.New("some error")
	b := &allWatcherTestBacking{
		fetchFunc: func(id entityId) (params.EntityInfo, error) {
			return nil, expectErr
		},
	}
	aw := newAllWatcher(b)
	err := aw.changed(entityId{})
	c.Assert(err, Equals, expectErr)
}

var allWatcherChangedTests = []struct {
	about string
	add []params.EntityInfo
	inBacking []params.EntityInfo
	change entityId
	expectRevno int64
	expectContents []entityEntry
} {{
	about: "no entity",
	change: entityId{"machine", "1"},
}, {
	about: "entity is marked as removed if it's not there",
	add: []params.EntityInfo{&params.MachineInfo{Id: "1"}},
	change: entityId{"machine", "1"},
	expectRevno: 2,
	expectContents: []entityEntry{{
		revno: 2,
		removed: true,
		info: &params.MachineInfo{
			Id:         "1",
		},
	}},
}, {
	about: "entity is updated if it's there",
	add: []params.EntityInfo{
		&params.MachineInfo{
			Id: "1",
		},
	},
	inBacking: []params.EntityInfo{
		&params.MachineInfo{
			Id: "1",
			InstanceId: "i-1",
		},
	},
	change: entityId{"machine", "1"},
	expectRevno: 2,
	expectContents: []entityEntry{{
		revno: 2,
		info:  &params.MachineInfo{
			Id: "1",
			InstanceId: "i-1",
		},
	}},
}}

func (*allWatcherSuite) TestChanged(c *C) {
	for i, test := range allWatcherChangedTests {
		c.Logf("test %d. %s", i, test.about)
		b := &allWatcherTestBacking{
			fetchFunc: fetchFromMap(entityMap{}.add(test.inBacking)),
		}
		aw := newAllWatcher(b)
		for _, info := range test.add {
			allInfoAdd(aw.all, info)
		}
		err := aw.changed(test.change)
		c.Assert(err, IsNil)
		assertAllInfoContents(c, aw.all, test.expectRevno, test.expectContents)
	}
}

type entityMap map[entityId] params.EntityInfo

func (em entityMap) add(infos []params.EntityInfo) entityMap {
	for _, info := range infos {
		em[entityIdForInfo(info)] = info
	}
	return em
}

func fetchFromMap(em entityMap) func(entityId) (params.EntityInfo, error) {
	return func(id entityId) (params.EntityInfo, error) {
		if info, ok := em[id]; ok {
			return info, nil;
		}
		return nil, mgo.ErrNotFound
	}
}

type allWatcherTestBacking struct {
	fetchFunc func(id entityId) (params.EntityInfo, error)
}

func (b *allWatcherTestBacking) fetch(id entityId) (params.EntityInfo, error) {
	return b.fetchFunc(id)
}
