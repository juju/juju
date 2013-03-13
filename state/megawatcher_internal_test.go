package state

import (
	"container/list"
	"errors"
	"fmt"
	"labix.org/v2/mgo"
	. "launchpad.net/gocheck"
	"launchpad.net/juju-core/state/api/params"
	"launchpad.net/juju-core/testing"
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
	c.Check(a.list.Len(), Equals, len(entries))
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

var allInfoChangeMethodTests = []struct {
	about          string
	change         func(all *allInfo)
	expectRevno    int64
	expectContents []entityEntry
}{{
	about:  "empty at first",
	change: func(*allInfo) {},
}, {
	about: "add single entry",
	change: func(all *allInfo) {
		allInfoAdd(all, &params.MachineInfo{
			Id:         "0",
			InstanceId: "i-0",
		})
	},
	expectRevno: 1,
	expectContents: []entityEntry{{
		revno: 1,
		info: &params.MachineInfo{
			Id:         "0",
			InstanceId: "i-0",
		},
	}},
}, {
	about: "add two entries",
	change: func(all *allInfo) {
		allInfoAdd(all, &params.MachineInfo{
			Id:         "0",
			InstanceId: "i-0",
		})
		allInfoAdd(all, &params.ServiceInfo{
			Name:    "wordpress",
			Exposed: true,
		})
	},
	expectRevno: 2,
	expectContents: []entityEntry{{
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
	}},
}, {
	about: "update an entity that's not currently there",
	change: func(all *allInfo) {
		m := &params.MachineInfo{Id: "1"}
		all.update(entityIdForInfo(m), m)
	},
	expectRevno: 1,
	expectContents: []entityEntry{{
		revno: 1,
		info:  &params.MachineInfo{Id: "1"},
	}},
}, {
	about: "mark removed on existing entry",
	change: func(all *allInfo) {
		allInfoAdd(all, &params.MachineInfo{Id: "0"})
		allInfoAdd(all, &params.MachineInfo{Id: "1"})
		all.markRemoved(entityId{"machine", "0"})
	},
	expectRevno: 3,
	expectContents: []entityEntry{{
		revno: 2,
		info:  &params.MachineInfo{Id: "1"},
	}, {
		revno:   3,
		removed: true,
		info:    &params.MachineInfo{Id: "0"},
	}},
}, {
	about: "mark removed on nonexistent entry",
	change: func(all *allInfo) {
		all.markRemoved(entityId{"machine", "0"})
	},
}, {
	about: "mark removed on already marked entry",
	change: func(all *allInfo) {
		allInfoAdd(all, &params.MachineInfo{Id: "0"})
		allInfoAdd(all, &params.MachineInfo{Id: "1"})
		all.markRemoved(entityId{"machine", "0"})
		all.update(entityId{"machine", "1"}, &params.MachineInfo{
			Id:         "1",
			InstanceId: "i-1",
		})
		all.markRemoved(entityId{"machine", "0"})
	},
	expectRevno: 4,
	expectContents: []entityEntry{{
		revno:   3,
		removed: true,
		info:    &params.MachineInfo{Id: "0"},
	}, {
		revno: 4,
		info: &params.MachineInfo{
			Id:         "1",
			InstanceId: "i-1",
		},
	}},
}, {
	about: "delete entry",
	change: func(all *allInfo) {
		allInfoAdd(all, &params.MachineInfo{Id: "0"})
		all.delete(entityId{"machine", "0"})
	},
	expectRevno: 1,
}}

func (s *allInfoSuite) TestAllInfoChangeMethods(c *C) {
	for i, test := range allInfoChangeMethodTests {
		all := newAllInfo()
		c.Logf("test %d. %s", i, test.about)
		test.change(all)
		assertAllInfoContents(c, all, test.expectRevno, test.expectContents)
	}
}

func entityIdForInfo(info params.EntityInfo) entityId {
	return entityId{
		collection: info.EntityKind(),
		id:         info.EntityId(),
	}
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

func allInfoAdd(a *allInfo, info params.EntityInfo) {
	a.add(entityIdForInfo(info), info)
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
	about          string
	add            []params.EntityInfo
	inBacking      []params.EntityInfo
	change         entityId
	expectRevno    int64
	expectContents []entityEntry
}{{
	about:  "no entity",
	change: entityId{"machine", "1"},
}, {
	about:       "entity is marked as removed if it's not there",
	add:         []params.EntityInfo{&params.MachineInfo{Id: "1"}},
	change:      entityId{"machine", "1"},
	expectRevno: 2,
	expectContents: []entityEntry{{
		revno:   2,
		removed: true,
		info: &params.MachineInfo{
			Id: "1",
		},
	}},
}, {
	about: "entity is added if it's not there",
	inBacking: []params.EntityInfo{
		&params.MachineInfo{Id: "1"},
	},
	change:      entityId{"machine", "1"},
	expectRevno: 1,
	expectContents: []entityEntry{{
		revno: 1,
		info:  &params.MachineInfo{Id: "1"},
	}},
}, {
	about: "entity is updated if it's there",
	add: []params.EntityInfo{
		&params.MachineInfo{Id: "1"},
	},
	inBacking: []params.EntityInfo{
		&params.MachineInfo{
			Id:         "1",
			InstanceId: "i-1",
		},
	},
	change:      entityId{"machine", "1"},
	expectRevno: 2,
	expectContents: []entityEntry{{
		revno: 2,
		info: &params.MachineInfo{
			Id:         "1",
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

func (*allWatcherSuite) TestHandle(c *C) {
	aw := newAllWatcher(&allWatcherTestBacking{})

	// Add request from first watcher.
	w0 := &StateWatcher{aw: aw}
	req0 := &allRequest{
		w: w0,
		reply: make(chan bool, 1),
	}
	aw.handle(req0)
	assertWaitingRequests(c, aw, map[*StateWatcher][]*allRequest) {
		w0: {req0},
	})

	// Add second request from first watcher.
	req1 := &allRequest{
		w: w0,
		reply: make(chan bool, 1),
	}
	aw.handle(req1)
	assertWaitingRequests(c, aw, map[*StateWatcher][]*allRequest) {
		w0: {req1, req0},
	})

	// Add request from second watcher.
	w1 := &StateWatcher{aw: aw}
	req2 := &allRequest{
		w: w1,
		reply: make(chan bool, 1)
	}
	aw.handle(req1)
	assertWaitingRequests(c, aw, map[*StateWatcher][]*allRequest) {
		w0: {req1, req0},
		w1: {req2},
	})

	// Stop first watcher.
	req3 := &allRequest{
		w: w0,
	}
	aw.handle(req3)
	assertWaitingRequests(c, aw, map[*StateWatcher][]*allRequest) {
		w1: {req2},
	})
	assertReplied(c, false, req0)
	assertReplied(c, false, req1)

	// Stop second watcher.
	
}

func (*allWatcherSuite) TestHandleStop(c *C) {
	aw := newAllWatcher(&allWatcherTestBacking{})
	w0 := &StateWatcher{aw: aw}
	req0 := &allRequest{
		w: w0,
		reply: make(chan bool, 1),
	}
	aw.handle(req0)

	req1 := &allRequest{
		w: w0,
		reply: make(chan bool, 1),
	}
	aw.handle(req0)

	w1 := &StateWatcher{aw: aw}
	req2 := &allRequest{
		w: w1,
		reply: make(chan bool, 1)
	}
	aw.handle(req1)
	assertWaitingRequests(c, aw, map[*StateWatcher][]*allRequest) {
		w0: {req1, req0},
		w1: {req2},
	}
}
	

func assertNotReplied(c *C, req *allRequest) {
	select {
	case v := <-req.reply:
		c.Fatalf("request was unexpectedly replied to (got %v)", v)
	default:
	}
}

func assertReplied(c *C, val bool, req *allRequest) {
	select {
	case v := <-req.reply:
		c.Assert(v, Equals, val)
	default:
		c.Fatalf("request was not replied to")
	}
}

func assertWaitingRequests(c *C, aw *allWatcher, waiting map[*StateWatcher][]*allRequest) {
	c.Assert(aw.reqs, HasLen, len(reqs))
	for w, reqs := range waiting {
		i := 0
		for req := aw.waiting[w]; ; req = req.next {
			if i >= len(reqs) {
				c.Assert(req, Equals, nil)
				break
			}
			c.Assert(req, Equals, reqs[i]
			assertNothingInChan(req.reply)
			i++
		}
	}
}


type entityMap map[entityId]params.EntityInfo

func (em entityMap) add(infos []params.EntityInfo) entityMap {
	for _, info := range infos {
		em[entityIdForInfo(info)] = info
	}
	return em
}

func fetchFromMap(em entityMap) func(entityId) (params.EntityInfo, error) {
	return func(id entityId) (params.EntityInfo, error) {
		if info, ok := em[id]; ok {
			return info, nil
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

func (b *allWatcherTestBacking) entityIdForInfo(info params.EntityInfo) entityId {
	return entityIdForInfo(info)
}
