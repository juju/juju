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
		creationRevno: 1,
		revno:         1,
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
		creationRevno: 1,
		revno:         1,
		info: &params.MachineInfo{
			Id:         "0",
			InstanceId: "i-0",
		},
	}, {
		creationRevno: 2,
		revno:         2,
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
		creationRevno: 1,
		revno:         1,
		info:          &params.MachineInfo{Id: "1"},
	}},
}, {
	about: "mark removed on existing entry",
	change: func(all *allInfo) {
		allInfoAdd(all, &params.MachineInfo{Id: "0"})
		allInfoAdd(all, &params.MachineInfo{Id: "1"})
		allInfoIncRef(all, entityId{"machine", "0"})
		all.markRemoved(entityId{"machine", "0"})
	},
	expectRevno: 3,
	expectContents: []entityEntry{{
		creationRevno: 2,
		revno:         2,
		info:          &params.MachineInfo{Id: "1"},
	}, {
		creationRevno: 1,
		revno:         3,
		refCount:      1,
		removed:       true,
		info:          &params.MachineInfo{Id: "0"},
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
		allInfoIncRef(all, entityId{"machine", "0"})
		all.markRemoved(entityId{"machine", "0"})
		all.update(entityId{"machine", "1"}, &params.MachineInfo{
			Id:         "1",
			InstanceId: "i-1",
		})
		all.markRemoved(entityId{"machine", "0"})
	},
	expectRevno: 4,
	expectContents: []entityEntry{{
		creationRevno: 1,
		revno:         3,
		refCount:      1,
		removed:       true,
		info:          &params.MachineInfo{Id: "0"},
	}, {
		creationRevno: 2,
		revno:         4,
		info: &params.MachineInfo{
			Id:         "1",
			InstanceId: "i-1",
		},
	}},
}, {
	about: "mark removed on entry with zero ref count",
	change: func(all *allInfo) {
		allInfoAdd(all, &params.MachineInfo{Id: "0"})
		all.markRemoved(entityId{"machine", "0"})
	},
	expectRevno: 2,
}, {
	about: "delete entry",
	change: func(all *allInfo) {
		allInfoAdd(all, &params.MachineInfo{Id: "0"})
		all.delete(entityId{"machine", "0"})
	},
	expectRevno: 1,
}, {
	about: "decref of non-removed entity",
	change: func(all *allInfo) {
		m := &params.MachineInfo{Id: "0"}
		id := entityIdForInfo(m)
		allInfoAdd(all, m)
		allInfoIncRef(all, id)
		entry := all.entities[id].Value.(*entityEntry)
		all.decRef(entry, id)
	},
	expectRevno: 1,
	expectContents: []entityEntry{{
		creationRevno: 1,
		revno:         1,
		refCount:      0,
		info:          &params.MachineInfo{Id: "0"},
	}},
}, {
	about: "decref of removed entity",
	change: func(all *allInfo) {
		m := &params.MachineInfo{Id: "0"}
		id := entityIdForInfo(m)
		allInfoAdd(all, m)
		entry := all.entities[id].Value.(*entityEntry)
		entry.refCount++
		all.markRemoved(id)
		all.decRef(entry, id)
	},
	expectRevno: 2,
},
}

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
	// Add three entries.
	var deltas []params.Delta
	for i := 0; i < 3; i++ {
		m := &params.MachineInfo{Id: fmt.Sprint(i)}
		allInfoAdd(a, m)
		deltas = append(deltas, params.Delta{Entity: m})
	}
	// Check that the deltas from each revno are as expected.
	for i := 0; i < 3; i++ {
		c.Logf("test %d", i)
		c.Assert(a.changesSince(int64(i)), DeepEquals, deltas[i:])
	}

	// Check boundary cases.
	c.Assert(a.changesSince(-1), DeepEquals, deltas)
	c.Assert(a.changesSince(99), HasLen, 0)

	// Update one machine and check we see the changes.
	rev := a.latestRevno
	m1 := &params.MachineInfo{
		Id:         "1",
		InstanceId: "foo",
	}
	a.update(entityIdForInfo(m1), m1)
	c.Assert(a.changesSince(rev), DeepEquals, []params.Delta{{Entity: m1}})

	// Make sure the machine isn't simply removed from
	// the list when it's marked as removed.
	allInfoIncRef(a, entityId{"machine", "0"})

	// Remove another machine and check we see it's removed.
	m0 := &params.MachineInfo{Id: "0"}
	a.markRemoved(entityIdForInfo(m0))

	// Check that something that never saw m0 does not get
	// informed of its removal (even those the removed entity
	// is still in the list.
	c.Assert(a.changesSince(0), DeepEquals, []params.Delta{{
		Entity: &params.MachineInfo{Id: "2"},
	}, {
		Entity: m1,
	}})

	c.Assert(a.changesSince(rev), DeepEquals, []params.Delta{{
		Entity: m1,
	}, {
		Removed: true,
		Entity:  m0,
	}})

	c.Assert(a.changesSince(rev+1), DeepEquals, []params.Delta{{
		Removed: true,
		Entity:  m0,
	}})

}

func allInfoAdd(a *allInfo, info params.EntityInfo) {
	a.add(entityIdForInfo(info), info)
}

func allInfoIncRef(a *allInfo, id entityId) {
	entry := a.entities[id].Value.(*entityEntry)
	entry.refCount++
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
		creationRevno: 1,
		revno:         2,
		refCount:      1,
		removed:       true,
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
		creationRevno: 1,
		revno:         1,
		info:          &params.MachineInfo{Id: "1"},
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
		creationRevno: 1,
		refCount:      1,
		revno:         2,
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
			allInfoIncRef(aw.all, entityIdForInfo(info))
		}
		err := aw.changed(test.change)
		c.Assert(err, IsNil)
		assertAllInfoContents(c, aw.all, test.expectRevno, test.expectContents)
	}
}

func (*allWatcherSuite) TestHandle(c *C) {
	aw := newAllWatcher(&allWatcherTestBacking{})

	// Add request from first watcher.
	w0 := &StateWatcher{}
	req0 := &allRequest{
		w:     w0,
		reply: make(chan bool, 1),
	}
	aw.handle(req0)
	assertWaitingRequests(c, aw, map[*StateWatcher][]*allRequest{
		w0: {req0},
	})

	// Add second request from first watcher.
	req1 := &allRequest{
		w:     w0,
		reply: make(chan bool, 1),
	}
	aw.handle(req1)
	assertWaitingRequests(c, aw, map[*StateWatcher][]*allRequest{
		w0: {req1, req0},
	})

	// Add request from second watcher.
	w1 := &StateWatcher{}
	req2 := &allRequest{
		w:     w1,
		reply: make(chan bool, 1),
	}
	aw.handle(req2)
	assertWaitingRequests(c, aw, map[*StateWatcher][]*allRequest{
		w0: {req1, req0},
		w1: {req2},
	})

	// Stop first watcher.
	aw.handle(&allRequest{
		w: w0,
	})
	assertWaitingRequests(c, aw, map[*StateWatcher][]*allRequest{
		w1: {req2},
	})
	assertReplied(c, false, req0)
	assertReplied(c, false, req1)

	// Stop second watcher.
	aw.handle(&allRequest{
		w: w1,
	})
	assertWaitingRequests(c, aw, nil)
	assertReplied(c, false, req2)
}

func (*allWatcherSuite) TestHandleStopNoDecRefIfMoreRecentlyCreated(c *C) {
	// If the StateWatcher hasn't seen the item, then we shouldn't
	// decrement its ref count when it is stopped.
	aw := newAllWatcher(&allWatcherTestBacking{})
	allInfoAdd(aw.all, &params.MachineInfo{Id: "0"})
	allInfoIncRef(aw.all, entityId{"machine", "0"})
	w := &StateWatcher{}

	// Stop the watcher.
	aw.handle(&allRequest{w: w})
	assertAllInfoContents(c, aw.all, 1, []entityEntry{{
		creationRevno: 1,
		revno:         1,
		refCount:      1,
		info: &params.MachineInfo{
			Id: "0",
		},
	}})
}

func (*allWatcherSuite) TestHandleStopNoDecRefIfAlreadySeenRemoved(c *C) {
	// If the StateWatcher has already seen the item removed, then
	// we shouldn't decrement its ref count when it is stopped.
	aw := newAllWatcher(&allWatcherTestBacking{})
	allInfoAdd(aw.all, &params.MachineInfo{Id: "0"})
	allInfoIncRef(aw.all, entityId{"machine", "0"})
	aw.all.markRemoved(entityId{"machine", "0"})
	w := &StateWatcher{}
	// Stop the watcher.
	aw.handle(&allRequest{w: w})
	assertAllInfoContents(c, aw.all, 2, []entityEntry{{
		creationRevno: 1,
		revno:         2,
		refCount:      1,
		removed:       true,
		info: &params.MachineInfo{
			Id: "0",
		},
	}})
}

func (*allWatcherSuite) TestHandleStopDecRefIfAlreadySeenAndNotRemoved(c *C) {
	// If the StateWatcher has already seen the item removed, then
	// we should decrement its ref count when it is stopped.
	aw := newAllWatcher(&allWatcherTestBacking{})
	allInfoAdd(aw.all, &params.MachineInfo{Id: "0"})
	allInfoIncRef(aw.all, entityId{"machine", "0"})
	w := &StateWatcher{}
	w.revno = aw.all.latestRevno
	// Stop the watcher.
	aw.handle(&allRequest{w: w})
	assertAllInfoContents(c, aw.all, 1, []entityEntry{{
		creationRevno: 1,
		revno:         1,
		info: &params.MachineInfo{
			Id: "0",
		},
	}})
}

func (*allWatcherSuite) TestHandleStopNoDecRefIfNotSeen(c *C) {
	// If the StateWatcher hasn't seen the item at all, it should
	// leave the ref count untouched.
	aw := newAllWatcher(&allWatcherTestBacking{})
	allInfoAdd(aw.all, &params.MachineInfo{Id: "0"})
	allInfoIncRef(aw.all, entityId{"machine", "0"})
	w := &StateWatcher{}
	// Stop the watcher.
	aw.handle(&allRequest{w: w})
	assertAllInfoContents(c, aw.all, 1, []entityEntry{{
		creationRevno: 1,
		revno:         1,
		refCount:      1,
		info: &params.MachineInfo{
			Id: "0",
		},
	}})
}

//
//func (*allWatcherSuite) TestRespond(c *C) {
//	ws := make([]*StateWatcher, 3)
//	for i := range ws {
//		ws[i] = &StateWatcher{}
//	}
//	
//
//keep a map for each watcher containing the watcher's currently
//known status of item.
//
//
//type allWatcherRespondTest struct {
//	about          string
//	add            []params.EntityInfo
//	inBacking      []params.EntityInfo
//	change         entityId
//	expectRevno    int64
//	expectContents []entityEntry
//}
//
//type allWatcherTestRequest struct {
//	// watcher identifies the StateWatcher making the request.
//	watcher int
//	// watcherRevno is the revno that the StateWatcher is currently at.
//	watcherRevno int64
//}

//var allWatcherRespondTests = []struct {
//	about          string
//	change func(all *allInfo)
//	
//}{
//
//some stuff in backing
//mutate the allwatcher
//call respond
//check that waiting requests 
//
//
//remove(
//func (*allWatcherSuite) TestRespond(c *C) {
//	
//}

var respondTestChanges = []func(all *allInfo){
	func(all *allInfo) {
		allInfoAdd(all, &params.MachineInfo{Id: "0"})
	},
	func(all *allInfo) {
		allInfoAdd(all, &params.MachineInfo{Id: "1"})
	},
	func(all *allInfo) {
		all.markRemoved(entityId{"machine", "0"})
	},
	func(all *allInfo) {
		all.update(entityId{"machine", "1"}, &params.MachineInfo{
			Id:         "1",
			InstanceId: "i-1",
		})
	},
}

var (
	respondTestFinalState = []entityEntry{{
		creationRevno: 2,
		revno:         4,
		info: &params.MachineInfo{
			Id:         "1",
			InstanceId: "i-1",
		},
	}}
	respondTestFinalRevno = respondTestFinalState[0].revno
)

func (*allWatcherSuite) TestRespondResults(c *C) {
	// We test the response results for a single watcher by
	// interleaving notional Next requests in all possible
	// combinations after each change in respondTestChanges and
	// checking that the view of the world as seen by the watcher
	// matches the actual current state.

	// We decide whether if we call respond by inspecting a number n
	// - bit i of n determines whether a request will be responded
	// to after running respondTestChanges[i].

	for n := 0; n < 1<<uint(len(respondTestChanges)); n++ {
		aw := newAllWatcher(&allWatcherTestBacking{})
		c.Logf("test %d. (%0*b)", n, len(respondTestChanges), n)
		w := &StateWatcher{}
		wstate := make(watcherState)
		req := &allRequest{
			w:     w,
			reply: make(chan bool, 1),
		}
		// Add the request, ready to be responded to.
		aw.handle(req)
		assertWaitingRequests(c, aw, map[*StateWatcher][]*allRequest{
			w: {req},
		})
		// Make each change in turn, and respond if n dictates it.
		for i, change := range respondTestChanges {
			c.Logf("change %d", i)
			change(aw.all)
			if n&(1<<uint(i)) == 0 {
				continue
			}
			aw.respond()
			select {
			case ok := <-req.reply:
				c.Assert(ok, Equals, true)
				c.Assert(len(req.changes) > 0, Equals, true)
				wstate.update(req.changes)
				req = &allRequest{
					w:     w,
					reply: make(chan bool, 1),
				}
				aw.handle(req)
				assertWaitingRequests(c, aw, map[*StateWatcher][]*allRequest{
					w: {req},
				})
			default:
			}
			wstate.check(c, aw.all)
		}
		// Stop the watcher and check that all ref counts end up at zero
		// and removed objects are deleted.
		aw.handle(&allRequest{w: w})
		assertReplied(c, false, req)
		assertAllInfoContents(c, aw.all, respondTestFinalRevno, respondTestFinalState)
	}
}

func (*allWatcherSuite) TestRespondMultiple(c *C) {
	aw := newAllWatcher(&allWatcherTestBacking{})
	allInfoAdd(aw.all, &params.MachineInfo{Id: "0"})

	// Add one request and respond.
	// It should see the above change.
	w0 := &StateWatcher{}
	req0 := &allRequest{
		w:     w0,
		reply: make(chan bool, 1),
	}
	aw.handle(req0)
	aw.respond()
	assertReplied(c, true, req0)
	c.Assert(req0.changes, DeepEquals, []params.Delta{{Entity: &params.MachineInfo{Id: "0"}}})
	assertWaitingRequests(c, aw, nil)

	// Add another request from the same watcher and respond.
	// It should have no reply because nothing has changed.
	req0 = &allRequest{
		w:     w0,
		reply: make(chan bool, 1),
	}
	aw.handle(req0)
	aw.respond()
	assertNotReplied(c, req0)

	// Add two requests from another watcher and respond.
	// The request from the first watcher should still not
	// be replied to, but the later of the two requests from
	// the second watcher should get a reply.
	w1 := &StateWatcher{}
	req1 := &allRequest{
		w:     w1,
		reply: make(chan bool, 1),
	}
	aw.handle(req1)
	req2 := &allRequest{
		w:     w1,
		reply: make(chan bool, 1),
	}
	aw.handle(req2)
	assertWaitingRequests(c, aw, map[*StateWatcher][]*allRequest{
		w0: {req0},
		w1: {req2, req1},
	})
	aw.respond()
	assertNotReplied(c, req0)
	assertNotReplied(c, req1)
	assertReplied(c, true, req2)
	c.Assert(req2.changes, DeepEquals, []params.Delta{{Entity: &params.MachineInfo{Id: "0"}}})
	assertWaitingRequests(c, aw, map[*StateWatcher][]*allRequest{
		w0: {req0},
		w1: {req1},
	})

	// Check that nothing more gets responded to if we call respond again.
	aw.respond()
	assertNotReplied(c, req0)
	assertNotReplied(c, req1)

	// Now make a change and check that both waiting requests
	// get serviced.
	allInfoAdd(aw.all, &params.MachineInfo{Id: "1"})
	aw.respond()
	assertReplied(c, true, req0)
	assertReplied(c, true, req1)
	assertWaitingRequests(c, aw, nil)

	deltas := []params.Delta{{Entity: &params.MachineInfo{Id: "1"}}}
	c.Assert(req0.changes, DeepEquals, deltas)
	c.Assert(req1.changes, DeepEquals, deltas)
}

// watcherState represents a StateWatcher client's
// current view of the state. It holds the last delta that a given
// state watcher has seen for each entity.
type watcherState map[entityId]params.Delta

func (s watcherState) update(changes []params.Delta) {
	for _, d := range changes {
		id := entityIdForInfo(d.Entity)
		if d.Removed {
			if _, ok := s[id]; !ok {
				panic(fmt.Errorf("entity id %v removed when it wasn't there", id))
			}
			delete(s, id)
		} else {
			s[id] = d
		}
	}
}

// check checks that the watcher state matches that
// held in current.
func (s watcherState) check(c *C, current *allInfo) {
	currentEntities := make(watcherState)
	for id, elem := range current.entities {
		entry := elem.Value.(*entityEntry)
		if !entry.removed {
			currentEntities[id] = params.Delta{Entity: entry.info}
		}
	}
	c.Assert(s, DeepEquals, currentEntities)
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
	c.Assert(aw.waiting, HasLen, len(waiting))
	for w, reqs := range waiting {
		i := 0
		for req := aw.waiting[w]; ; req = req.next {
			if i >= len(reqs) {
				c.Assert(req, IsNil)
				break
			}
			c.Assert(req, Equals, reqs[i])
			assertNotReplied(c, req)
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
