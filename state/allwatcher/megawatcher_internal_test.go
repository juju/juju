package allwatcher

import (
	"container/list"
	"errors"
	"fmt"
	"labix.org/v2/mgo"
	. "launchpad.net/gocheck"
	"launchpad.net/juju-core/state/api/params"
	"launchpad.net/juju-core/state/watcher"
	"launchpad.net/juju-core/testing"
	"sync"
	stdtesting "testing"
	"time"
)

func Test(t *stdtesting.T) {
	TestingT(t)
}

type allInfoSuite struct {
	testing.LoggingSuite
}

func (*allInfoSuite) assertAllInfoContents(c *C, a *AllInfo, latestRevno int64, entries []entityEntry) {
	assertAllInfoContents(c, a, idForInfo, latestRevno, entries)
}

var _ = Suite(&allInfoSuite{})

var AllInfoChangeMethodTests = []struct {
	about          string
	change         func(all *AllInfo)
	expectRevno    int64
	expectContents []entityEntry
}{{
	about:  "empty at first",
	change: func(*AllInfo) {},
}, {
	about: "add single entry",
	change: func(all *AllInfo) {
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
	change: func(all *AllInfo) {
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
	change: func(all *AllInfo) {
		m := &params.MachineInfo{Id: "1"}
		all.Update(idForInfo(m), m)
	},
	expectRevno: 1,
	expectContents: []entityEntry{{
		creationRevno: 1,
		revno:         1,
		info:          &params.MachineInfo{Id: "1"},
	}},
}, {
	about: "mark removed on existing entry",
	change: func(all *AllInfo) {
		allInfoAdd(all, &params.MachineInfo{Id: "0"})
		allInfoAdd(all, &params.MachineInfo{Id: "1"})
		AllInfoIncRef(all, testEntityId{"machine", "0"})
		all.Update(testEntityId{"machine", "0"}, nil)
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
	change: func(all *AllInfo) {
		all.Update(testEntityId{"machine", "0"}, nil)
	},
}, {
	about: "mark removed on already marked entry",
	change: func(all *AllInfo) {
		allInfoAdd(all, &params.MachineInfo{Id: "0"})
		allInfoAdd(all, &params.MachineInfo{Id: "1"})
		AllInfoIncRef(all, testEntityId{"machine", "0"})
		all.Update(testEntityId{"machine", "0"}, nil)
		all.Update(testEntityId{"machine", "1"}, &params.MachineInfo{
			Id:         "1",
			InstanceId: "i-1",
		})
		all.Update(testEntityId{"machine", "0"}, nil)
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
	change: func(all *AllInfo) {
		allInfoAdd(all, &params.MachineInfo{Id: "0"})
		all.Update(testEntityId{"machine", "0"}, nil)
	},
	expectRevno: 2,
}, {
	about: "delete entry",
	change: func(all *AllInfo) {
		allInfoAdd(all, &params.MachineInfo{Id: "0"})
		all.delete(testEntityId{"machine", "0"})
	},
	expectRevno: 1,
}, {
	about: "decref of non-removed entity",
	change: func(all *AllInfo) {
		m := &params.MachineInfo{Id: "0"}
		id := idForInfo(m)
		allInfoAdd(all, m)
		AllInfoIncRef(all, id)
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
	change: func(all *AllInfo) {
		m := &params.MachineInfo{Id: "0"}
		id := idForInfo(m)
		allInfoAdd(all, m)
		entry := all.entities[id].Value.(*entityEntry)
		entry.refCount++
		all.Update(id, nil)
		all.decRef(entry, id)
	},
	expectRevno: 2,
},
}

func (s *allInfoSuite) TestAllInfoChangeMethods(c *C) {
	for i, test := range AllInfoChangeMethodTests {
		all := newAllInfo()
		c.Logf("test %d. %s", i, test.about)
		test.change(all)
		s.assertAllInfoContents(c, all, test.expectRevno, test.expectContents)
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
		c.Assert(a.ChangesSince(int64(i)), DeepEquals, deltas[i:])
	}

	// Check boundary cases.
	c.Assert(a.ChangesSince(-1), DeepEquals, deltas)
	c.Assert(a.ChangesSince(99), HasLen, 0)

	// Update one machine and check we see the changes.
	rev := a.latestRevno
	m1 := &params.MachineInfo{
		Id:         "1",
		InstanceId: "foo",
	}
	a.Update(idForInfo(m1), m1)
	c.Assert(a.ChangesSince(rev), DeepEquals, []params.Delta{{Entity: m1}})

	// Make sure the machine isn't simply removed from
	// the list when it's marked as removed.
	AllInfoIncRef(a, testEntityId{"machine", "0"})

	// Remove another machine and check we see it's removed.
	m0 := &params.MachineInfo{Id: "0"}
	a.Update(idForInfo(m0), nil)

	// Check that something that never saw m0 does not get
	// informed of its removal (even those the removed entity
	// is still in the list.
	c.Assert(a.ChangesSince(0), DeepEquals, []params.Delta{{
		Entity: &params.MachineInfo{Id: "2"},
	}, {
		Entity: m1,
	}})

	c.Assert(a.ChangesSince(rev), DeepEquals, []params.Delta{{
		Entity: m1,
	}, {
		Removed: true,
		Entity:  m0,
	}})

	c.Assert(a.ChangesSince(rev+1), DeepEquals, []params.Delta{{
		Removed: true,
		Entity:  m0,
	}})

}

type allWatcherSuite struct {
	testing.LoggingSuite
}

var _ = Suite(&allWatcherSuite{})

func (*allWatcherSuite) assertAllInfoContents(c *C, a *AllInfo, latestRevno int64, entries []entityEntry) {
	assertAllInfoContents(c, a, idForInfo, latestRevno, entries)
}

func (*allWatcherSuite) TestHandle(c *C) {
	aw := NewAllWatcher(newTestBacking(nil))

	// Add request from first watcher.
	w0 := &StateWatcher{all: aw}
	req0 := &request{
		w:     w0,
		reply: make(chan bool, 1),
	}
	aw.handle(req0)
	assertWaitingRequests(c, aw, map[*StateWatcher][]*request{
		w0: {req0},
	})

	// Add second request from first watcher.
	req1 := &request{
		w:     w0,
		reply: make(chan bool, 1),
	}
	aw.handle(req1)
	assertWaitingRequests(c, aw, map[*StateWatcher][]*request{
		w0: {req1, req0},
	})

	// Add request from second watcher.
	w1 := &StateWatcher{all: aw}
	req2 := &request{
		w:     w1,
		reply: make(chan bool, 1),
	}
	aw.handle(req2)
	assertWaitingRequests(c, aw, map[*StateWatcher][]*request{
		w0: {req1, req0},
		w1: {req2},
	})

	// Stop first watcher.
	aw.handle(&request{
		w: w0,
	})
	assertWaitingRequests(c, aw, map[*StateWatcher][]*request{
		w1: {req2},
	})
	assertReplied(c, false, req0)
	assertReplied(c, false, req1)

	// Stop second watcher.
	aw.handle(&request{
		w: w1,
	})
	assertWaitingRequests(c, aw, nil)
	assertReplied(c, false, req2)
}

func (s *allWatcherSuite) TestHandleStopNoDecRefIfMoreRecentlyCreated(c *C) {
	// If the StateWatcher hasn't seen the item, then we shouldn't
	// decrement its ref count when it is stopped.
	aw := NewAllWatcher(newTestBacking(nil))
	allInfoAdd(aw.all, &params.MachineInfo{Id: "0"})
	AllInfoIncRef(aw.all, testEntityId{"machine", "0"})
	w := &StateWatcher{all: aw}

	// Stop the watcher.
	aw.handle(&request{w: w})
	s.assertAllInfoContents(c, aw.all, 1, []entityEntry{{
		creationRevno: 1,
		revno:         1,
		refCount:      1,
		info: &params.MachineInfo{
			Id: "0",
		},
	}})
}

func (s *allWatcherSuite) TestHandleStopNoDecRefIfAlreadySeenRemoved(c *C) {
	// If the StateWatcher has already seen the item removed, then
	// we shouldn't decrement its ref count when it is stopped.
	aw := NewAllWatcher(newTestBacking(nil))
	allInfoAdd(aw.all, &params.MachineInfo{Id: "0"})
	AllInfoIncRef(aw.all, testEntityId{"machine", "0"})
	aw.all.Update(testEntityId{"machine", "0"}, nil)
	w := &StateWatcher{all: aw}
	// Stop the watcher.
	aw.handle(&request{w: w})
	s.assertAllInfoContents(c, aw.all, 2, []entityEntry{{
		creationRevno: 1,
		revno:         2,
		refCount:      1,
		removed:       true,
		info: &params.MachineInfo{
			Id: "0",
		},
	}})
}

func (s *allWatcherSuite) TestHandleStopDecRefIfAlreadySeenAndNotRemoved(c *C) {
	// If the StateWatcher has already seen the item removed, then
	// we should decrement its ref count when it is stopped.
	aw := NewAllWatcher(newTestBacking(nil))
	allInfoAdd(aw.all, &params.MachineInfo{Id: "0"})
	AllInfoIncRef(aw.all, testEntityId{"machine", "0"})
	w := &StateWatcher{all: aw}
	w.revno = aw.all.latestRevno
	// Stop the watcher.
	aw.handle(&request{w: w})
	s.assertAllInfoContents(c, aw.all, 1, []entityEntry{{
		creationRevno: 1,
		revno:         1,
		info: &params.MachineInfo{
			Id: "0",
		},
	}})
}

func (s *allWatcherSuite) TestHandleStopNoDecRefIfNotSeen(c *C) {
	// If the StateWatcher hasn't seen the item at all, it should
	// leave the ref count untouched.
	aw := NewAllWatcher(newTestBacking(nil))
	allInfoAdd(aw.all, &params.MachineInfo{Id: "0"})
	AllInfoIncRef(aw.all, testEntityId{"machine", "0"})
	w := &StateWatcher{all: aw}
	// Stop the watcher.
	aw.handle(&request{w: w})
	s.assertAllInfoContents(c, aw.all, 1, []entityEntry{{
		creationRevno: 1,
		revno:         1,
		refCount:      1,
		info: &params.MachineInfo{
			Id: "0",
		},
	}})
}

var respondTestChanges = [...]func(all *AllInfo){
	func(all *AllInfo) {
		allInfoAdd(all, &params.MachineInfo{Id: "0"})
	},
	func(all *AllInfo) {
		allInfoAdd(all, &params.MachineInfo{Id: "1"})
	},
	func(all *AllInfo) {
		allInfoAdd(all, &params.MachineInfo{Id: "2"})
	},
	func(all *AllInfo) {
		all.Update(testEntityId{"machine", "0"}, nil)
	},
	func(all *AllInfo) {
		all.Update(testEntityId{"machine", "1"}, &params.MachineInfo{
			Id:         "1",
			InstanceId: "i-1",
		})
	},
	func(all *AllInfo) {
		all.Update(testEntityId{"machine", "1"}, nil)
	},
}

var (
	respondTestFinalState = []entityEntry{{
		creationRevno: 3,
		revno:         3,
		info: &params.MachineInfo{
			Id: "2",
		},
	}}
	respondTestFinalRevno = int64(len(respondTestChanges))
)

func (s *allWatcherSuite) TestRespondResults(c *C) {
	// We test the response results for a pair of watchers by
	// interleaving notional Next requests in all possible
	// combinations after each change in respondTestChanges and
	// checking that the view of the world as seen by the watchers
	// matches the actual current state.

	// We decide whether if we make a request for a given
	// watcher by inspecting a number n - bit i of n determines whether
	// a request will be responded to after running respondTestChanges[i].

	numCombinations := 1 << uint(len(respondTestChanges))
	const wcount = 2
	ns := make([]int, wcount)
	for ns[0] = 0; ns[0] < numCombinations; ns[0]++ {
		for ns[1] = 0; ns[1] < numCombinations; ns[1]++ {
			aw := NewAllWatcher(&allWatcherTestBacking{})
			c.Logf("test %0*b", len(respondTestChanges), ns)
			var (
				ws      []*StateWatcher
				wstates []watcherState
				reqs    []*request
			)
			for i := 0; i < wcount; i++ {
				ws = append(ws, &StateWatcher{})
				wstates = append(wstates, make(watcherState))
				reqs = append(reqs, nil)
			}
			// Make each change in turn, and make a request for each
			// watcher if n and respond
			for i, change := range respondTestChanges {
				c.Logf("change %d", i)
				change(aw.all)
				needRespond := false
				for wi, n := range ns {
					if n&(1<<uint(i)) != 0 {
						needRespond = true
						if reqs[wi] == nil {
							reqs[wi] = &request{
								w:     ws[wi],
								reply: make(chan bool, 1),
							}
							aw.handle(reqs[wi])
						}
					}
				}
				if !needRespond {
					continue
				}
				// Check that the expected requests are pending.
				expectWaiting := make(map[*StateWatcher][]*request)
				for wi, w := range ws {
					if reqs[wi] != nil {
						expectWaiting[w] = []*request{reqs[wi]}
					}
				}
				assertWaitingRequests(c, aw, expectWaiting)
				// Actually respond; then check that each watcher with
				// an outstanding request now has an up to date view
				// of the world.
				aw.respond()
				for wi, req := range reqs {
					if req == nil {
						continue
					}
					select {
					case ok := <-req.reply:
						c.Assert(ok, Equals, true)
						c.Assert(len(req.changes) > 0, Equals, true)
						wstates[wi].update(req.changes)
						reqs[wi] = nil
					default:
					}
					c.Logf("check %d", wi)
					wstates[wi].check(c, aw.all)
				}
			}
			// Stop the watcher and check that all ref counts end up at zero
			// and removed objects are deleted.
			for wi, w := range ws {
				aw.handle(&request{w: w})
				if reqs[wi] != nil {
					assertReplied(c, false, reqs[wi])
				}
			}
			s.assertAllInfoContents(c, aw.all, respondTestFinalRevno, respondTestFinalState)
		}
	}
}

func (*allWatcherSuite) TestRespondMultiple(c *C) {
	aw := NewAllWatcher(newTestBacking(nil))
	allInfoAdd(aw.all, &params.MachineInfo{Id: "0"})

	// Add one request and respond.
	// It should see the above change.
	w0 := &StateWatcher{all: aw}
	req0 := &request{
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
	req0 = &request{
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
	w1 := &StateWatcher{all: aw}
	req1 := &request{
		w:     w1,
		reply: make(chan bool, 1),
	}
	aw.handle(req1)
	req2 := &request{
		w:     w1,
		reply: make(chan bool, 1),
	}
	aw.handle(req2)
	assertWaitingRequests(c, aw, map[*StateWatcher][]*request{
		w0: {req0},
		w1: {req2, req1},
	})
	aw.respond()
	assertNotReplied(c, req0)
	assertNotReplied(c, req1)
	assertReplied(c, true, req2)
	c.Assert(req2.changes, DeepEquals, []params.Delta{{Entity: &params.MachineInfo{Id: "0"}}})
	assertWaitingRequests(c, aw, map[*StateWatcher][]*request{
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

func (*allWatcherSuite) TestRunStop(c *C) {
	aw := NewAllWatcher(newTestBacking(nil))
	go aw.Run()
	w := &StateWatcher{all: aw}
	err := aw.Stop()
	c.Assert(err, IsNil)
	d, err := w.Next()
	c.Assert(err, ErrorMatches, "state watcher was stopped")
	c.Assert(d, HasLen, 0)
}

func (*allWatcherSuite) TestRun(c *C) {
	b := newTestBacking([]params.EntityInfo{
		&params.MachineInfo{Id: "0"},
		&params.UnitInfo{Name: "wordpress/0"},
		&params.ServiceInfo{Name: "wordpress"},
	})
	aw := NewAllWatcher(b)
	defer func() {
		c.Check(aw.Stop(), IsNil)
	}()
	go aw.Run()
	w := &StateWatcher{all: aw}
	checkNext(c, w, []params.Delta{
		{Entity: &params.MachineInfo{Id: "0"}},
		{Entity: &params.UnitInfo{Name: "wordpress/0"}},
		{Entity: &params.ServiceInfo{Name: "wordpress"}},
	}, "")
	b.updateEntity(&params.MachineInfo{Id: "0", InstanceId: "i-0"})
	checkNext(c, w, []params.Delta{
		{Entity: &params.MachineInfo{Id: "0", InstanceId: "i-0"}},
	}, "")
	b.deleteEntity(testEntityId{"machine", "0"})
	checkNext(c, w, []params.Delta{
		{Removed: true, Entity: &params.MachineInfo{Id: "0"}},
	}, "")
}

func (*allWatcherSuite) TestStateWatcherStop(c *C) {
	aw := NewAllWatcher(newTestBacking(nil))
	defer func() {
		c.Check(aw.Stop(), IsNil)
	}()
	go aw.Run()
	w := &StateWatcher{all: aw}
	done := make(chan struct{})
	go func() {
		checkNext(c, w, nil, ErrWatcherStopped.Error())
		done <- struct{}{}
	}()
	err := w.Stop()
	c.Assert(err, IsNil)
	<-done
}

func (*allWatcherSuite) TestStateWatcherStopBecauseAllWatcherError(c *C) {
	b := newTestBacking([]params.EntityInfo{&params.MachineInfo{Id: "0"}})
	aw := NewAllWatcher(b)
	go aw.Run()
	defer func() {
		c.Check(aw.Stop(), ErrorMatches, "some error")
	}()
	w := &StateWatcher{all: aw}
	// Receive one delta to make sure that the allWatcher
	// has seen the initial state.
	checkNext(c, w, []params.Delta{{Entity: &params.MachineInfo{Id: "0"}}}, "")
	c.Logf("setting fetch error")
	b.setFetchError(errors.New("some error"))
	c.Logf("updating entity")
	b.updateEntity(&params.MachineInfo{Id: "1"})
	checkNext(c, w, nil, "some error")
}

func idForInfo(info params.EntityInfo) InfoId {
	return testEntityId{
		kind: info.EntityKind(),
		id:   info.EntityId(),
	}
}

func allInfoAdd(a *AllInfo, info params.EntityInfo) {
	a.add(idForInfo(info), info)
}

func AllInfoIncRef(a *AllInfo, id InfoId) {
	entry := a.entities[id].Value.(*entityEntry)
	entry.refCount++
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

func assertAllInfoContents(c *C, a *AllInfo, idOf func(params.EntityInfo) InfoId, latestRevno int64, entries []entityEntry) {
	var gotEntries []entityEntry
	var gotElems []*list.Element
	c.Check(a.list.Len(), Equals, len(entries))
	for e := a.list.Back(); e != nil; e = e.Prev() {
		gotEntries = append(gotEntries, *e.Value.(*entityEntry))
		gotElems = append(gotElems, e)
	}
	c.Assert(gotEntries, DeepEquals, entries)
	for i, ent := range entries {
		c.Assert(a.entities[idOf(ent.info)], Equals, gotElems[i])
	}
	c.Assert(a.entities, HasLen, len(entries))
	c.Assert(a.latestRevno, Equals, latestRevno)
}

var errTimeout = errors.New("no change received in sufficient time")

func getNext(c *C, w *StateWatcher, timeout time.Duration) ([]params.Delta, error) {
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

func checkNext(c *C, w *StateWatcher, deltas []params.Delta, expectErr string) {
	d, err := getNext(c, w, 1*time.Second)
	if expectErr != "" {
		c.Check(err, ErrorMatches, expectErr)
		return
	}
	checkDeltasEqual(c, d, deltas)
}

// deltas are returns in arbitrary order, so we compare
// them as sets.
func checkDeltasEqual(c *C, d0, d1 []params.Delta) {
	c.Check(deltaMap(d0), DeepEquals, deltaMap(d1))
}

func deltaMap(deltas []params.Delta) map[InfoId]params.EntityInfo {
	m := make(map[InfoId]params.EntityInfo)
	for _, d := range deltas {
		id := idForInfo(d.Entity)
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

// watcherState represents a StateWatcher client's
// current view of the state. It holds the last delta that a given
// state watcher has seen for each entity.
type watcherState map[InfoId]params.Delta

func (s watcherState) update(changes []params.Delta) {
	for _, d := range changes {
		id := idForInfo(d.Entity)
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
func (s watcherState) check(c *C, current *AllInfo) {
	currentEntities := make(watcherState)
	for id, elem := range current.entities {
		entry := elem.Value.(*entityEntry)
		if !entry.removed {
			currentEntities[id] = params.Delta{Entity: entry.info}
		}
	}
	c.Assert(s, DeepEquals, currentEntities)
}

func assertNotReplied(c *C, req *request) {
	select {
	case v := <-req.reply:
		c.Fatalf("request was unexpectedly replied to (got %v)", v)
	default:
	}
}

func assertReplied(c *C, val bool, req *request) {
	select {
	case v := <-req.reply:
		c.Assert(v, Equals, val)
	default:
		c.Fatalf("request was not replied to")
	}
}

func assertWaitingRequests(c *C, aw *AllWatcher, waiting map[*StateWatcher][]*request) {
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

type allWatcherTestBacking struct {
	mu       sync.Mutex
	fetchErr error
	entities map[InfoId]params.EntityInfo
	watchc   chan<- watcher.Change
	txnRevno int64
}

func newTestBacking(initial []params.EntityInfo) *allWatcherTestBacking {
	b := &allWatcherTestBacking{
		entities: make(map[InfoId]params.EntityInfo),
	}
	for _, info := range initial {
		b.entities[idForInfo(info)] = info
	}
	return b
}

type testEntityId struct {
	kind string
	id   interface{}
}

func (b *allWatcherTestBacking) Changed(all *AllInfo, change watcher.Change) error {
	id := testEntityId{
		kind: change.C,
		id:   change.Id,
	}
	info, err := b.fetch(id)
	if err == mgo.ErrNotFound {
		all.Update(id, nil)
		return nil
	}
	if err != nil {
		return err
	}
	all.Update(id, info)
	return nil
}

func (b *allWatcherTestBacking) fetch(id InfoId) (params.EntityInfo, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.fetchErr != nil {
		return nil, b.fetchErr
	}
	if info, ok := b.entities[id]; ok {
		return info, nil
	}
	return nil, mgo.ErrNotFound
}

func (b *allWatcherTestBacking) IdForInfo(info params.EntityInfo) InfoId {
	return idForInfo(info)
}

func (b *allWatcherTestBacking) Watch(c chan<- watcher.Change) {
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.watchc != nil {
		panic("test backing can only watch once")
	}
	b.watchc = c
}

func (b *allWatcherTestBacking) Unwatch(c chan<- watcher.Change) {
	b.mu.Lock()
	defer b.mu.Unlock()
	if c != b.watchc {
		panic("unwatching wrong channel")
	}
	b.watchc = nil
}

func (b *allWatcherTestBacking) GetAll(all *AllInfo) error {
	b.mu.Lock()
	defer b.mu.Unlock()
	for id, info := range b.entities {
		all.Update(id, info)
	}
	return nil
}

func (b *allWatcherTestBacking) updateEntity(info params.EntityInfo) {
	b.mu.Lock()
	defer b.mu.Unlock()
	id := b.IdForInfo(info).(testEntityId)
	b.entities[id] = info
	b.txnRevno++
	if b.watchc != nil {
		b.watchc <- watcher.Change{
			C:     id.kind,
			Id:    id.id,
			Revno: b.txnRevno, // This is actually ignored, but fill it in anyway.
		}
	}
}

func (b *allWatcherTestBacking) setFetchError(err error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.fetchErr = err
}

func (b *allWatcherTestBacking) deleteEntity(id0 InfoId) {
	id := id0.(testEntityId)
	b.mu.Lock()
	defer b.mu.Unlock()
	delete(b.entities, id)
	b.txnRevno++
	if b.watchc != nil {
		b.watchc <- watcher.Change{
			C:     id.kind,
			Id:    id.id,
			Revno: -1,
		}
	}
}
