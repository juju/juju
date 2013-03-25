package state

import (
	"container/list"
	"errors"
	"fmt"
	"io/ioutil"
	"labix.org/v2/mgo"
	. "launchpad.net/gocheck"
	"launchpad.net/juju-core/charm"
	"launchpad.net/juju-core/state/api/params"
	"launchpad.net/juju-core/state/watcher"
	"launchpad.net/juju-core/testing"
	"net/url"
	"path/filepath"
	"sort"
	"sync"
	"time"
)

func (st *State) AddTestingCharm(c *C, name string) *Charm {
	return addCharm(c, st, testing.Charms.Dir(name))
}

func (st *State) AddConfigCharm(c *C, name, configYaml string, revision int) *Charm {
	path := testing.Charms.ClonedDirPath(c.MkDir(), name)
	config := filepath.Join(path, "config.yaml")
	err := ioutil.WriteFile(config, []byte(configYaml), 0644)
	c.Assert(err, IsNil)
	ch, err := charm.ReadDir(path)
	c.Assert(err, IsNil)
	ch.SetRevision(revision)
	return addCharm(c, st, ch)
}

func addCharm(c *C, st *State, ch charm.Charm) *Charm {
	ident := fmt.Sprintf("%s-%d", ch.Meta().Name, ch.Revision())
	curl := charm.MustParseURL("local:series/" + ident)
	bundleURL, err := url.Parse("http://bundles.example.com/" + ident)
	c.Assert(err, IsNil)
	sch, err := st.AddCharm(ch, curl, bundleURL, ident+"-sha256")
	c.Assert(err, IsNil)
	return sch
}

type allInfoSuite struct {
	testing.LoggingSuite
}

var _ = Suite(&allInfoSuite{})

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
	b := newTestBacking(nil)
	b.setFetchError(expectErr)
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
		b := newTestBacking(test.inBacking)
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
	aw := newAllWatcher(newTestBacking(nil))

	// Add request from first watcher.
	w0 := &StateWatcher{all: aw}
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
	w1 := &StateWatcher{all: aw}
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
	aw := newAllWatcher(newTestBacking(nil))
	allInfoAdd(aw.all, &params.MachineInfo{Id: "0"})
	allInfoIncRef(aw.all, entityId{"machine", "0"})
	w := &StateWatcher{all: aw}

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
	aw := newAllWatcher(newTestBacking(nil))
	allInfoAdd(aw.all, &params.MachineInfo{Id: "0"})
	allInfoIncRef(aw.all, entityId{"machine", "0"})
	aw.all.markRemoved(entityId{"machine", "0"})
	w := &StateWatcher{all: aw}
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
	aw := newAllWatcher(newTestBacking(nil))
	allInfoAdd(aw.all, &params.MachineInfo{Id: "0"})
	allInfoIncRef(aw.all, entityId{"machine", "0"})
	w := &StateWatcher{all: aw}
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
	aw := newAllWatcher(newTestBacking(nil))
	allInfoAdd(aw.all, &params.MachineInfo{Id: "0"})
	allInfoIncRef(aw.all, entityId{"machine", "0"})
	w := &StateWatcher{all: aw}
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

var respondTestChanges = [...]func(all *allInfo){
	func(all *allInfo) {
		allInfoAdd(all, &params.MachineInfo{Id: "0"})
	},
	func(all *allInfo) {
		allInfoAdd(all, &params.MachineInfo{Id: "1"})
	},
	func(all *allInfo) {
		allInfoAdd(all, &params.MachineInfo{Id: "2"})
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
	func(all *allInfo) {
		all.markRemoved(entityId{"machine", "1"})
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

func (*allWatcherSuite) TestRespondResults(c *C) {
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
			aw := newAllWatcher(&allWatcherTestBacking{})
			c.Logf("test %0*b", len(respondTestChanges), ns)
			var (
				ws      []*StateWatcher
				wstates []watcherState
				reqs    []*allRequest
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
							reqs[wi] = &allRequest{
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
				expectWaiting := make(map[*StateWatcher][]*allRequest)
				for wi, w := range ws {
					if reqs[wi] != nil {
						expectWaiting[w] = []*allRequest{reqs[wi]}
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
				aw.handle(&allRequest{w: w})
				if reqs[wi] != nil {
					assertReplied(c, false, reqs[wi])
				}
			}
			assertAllInfoContents(c, aw.all, respondTestFinalRevno, respondTestFinalState)
		}
	}
}

func (*allWatcherSuite) TestRespondMultiple(c *C) {
	aw := newAllWatcher(newTestBacking(nil))
	allInfoAdd(aw.all, &params.MachineInfo{Id: "0"})

	// Add one request and respond.
	// It should see the above change.
	w0 := &StateWatcher{all: aw}
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
	w1 := &StateWatcher{all: aw}
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

func (*allWatcherSuite) TestRunStop(c *C) {
	aw := newAllWatcher(newTestBacking(nil))
	go aw.run()
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
	aw := newAllWatcher(b)
	defer func() {
		c.Check(aw.Stop(), IsNil)
	}()
	go aw.run()
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
	b.deleteEntity(entityId{"machine", "0"})
	checkNext(c, w, []params.Delta{
		{Removed: true, Entity: &params.MachineInfo{Id: "0"}},
	}, "")
}

func (*allWatcherSuite) TestStateWatcherStop(c *C) {
	aw := newAllWatcher(newTestBacking(nil))
	defer func() {
		c.Check(aw.Stop(), IsNil)
	}()
	go aw.run()
	w := &StateWatcher{all: aw}
	done := make(chan struct{})
	go func() {
		checkNext(c, w, nil, errWatcherStopped.Error())
		done <- struct{}{}
	}()
	err := w.Stop()
	c.Assert(err, IsNil)
	<-done
}

func (*allWatcherSuite) TestStateWatcherStopBecauseAllWatcherError(c *C) {
	b := newTestBacking([]params.EntityInfo{&params.MachineInfo{Id: "0"}})
	aw := newAllWatcher(b)
	go aw.run()
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
	state, err := Open(TestingStateInfo(), TestingDialTimeout)
	c.Assert(err, IsNil)

	s.State = state
}

func (s *allWatcherStateSuite) TearDownTest(c *C) {
	s.State.Close()
	s.MgoSuite.TearDownTest(c)
	s.LoggingSuite.TearDownTest(c)
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
	c.Assert(m.EntityName(), Equals, "machine-0")
	err = m.SetInstanceId(InstanceId("i-" + m.EntityName()))
	c.Assert(err, IsNil)
	add(&params.MachineInfo{
		Id:         "0",
		InstanceId: "i-machine-0",
	})

	wordpress, err := s.State.AddService("wordpress", s.State.AddTestingCharm(c, "wordpress"))
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
		EntityName:  "service-wordpress",
		Annotations: pairs,
	})

	logging, err := s.State.AddService("logging", s.State.AddTestingCharm(c, "logging"))
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
	})

	for i := 0; i < 2; i++ {
		wu, err := wordpress.AddUnit()
		c.Assert(err, IsNil)
		c.Assert(wu.EntityName(), Equals, fmt.Sprintf("unit-wordpress-%d", i))
		add(&params.UnitInfo{
			Name:    fmt.Sprintf("wordpress/%d", i),
			Service: "wordpress",
		})
		pairs := map[string]string{"name": fmt.Sprintf("bar %d", i)}
		err = wu.SetAnnotations(pairs)
		c.Assert(err, IsNil)
		add(&params.AnnotationInfo{
			GlobalKey:   fmt.Sprintf("u#wordpress/%d", i),
			EntityName:  fmt.Sprintf("unit-wordpress-%d", i),
			Annotations: pairs,
		})

		m, err := s.State.AddMachine("series", JobHostUnits)
		c.Assert(err, IsNil)
		c.Assert(m.EntityName(), Equals, fmt.Sprintf("machine-%d", i+1))
		err = m.SetInstanceId(InstanceId("i-" + m.EntityName()))
		c.Assert(err, IsNil)
		add(&params.MachineInfo{
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
		add(&params.UnitInfo{
			Name:    fmt.Sprintf("logging/%d", i),
			Service: "logging",
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
	all := newAllInfo()
	err := b.getAll(all)
	c.Assert(err, IsNil)

	// Check that all the entities have been placed
	// into the list; we can't use assertAllInfoContents
	// here because we don't know the order that
	// things were placed into the list.
	var gotEntities entityInfoSlice
	c.Check(all.latestRevno, Equals, int64(len(expectEntities)))
	i := int64(0)
	for e := all.list.Front(); e != nil; e = e.Next() {
		entry := e.Value.(*entityEntry)
		gotEntities = append(gotEntities, entry.info)
		c.Check(entry.revno, Equals, all.latestRevno-i)
		c.Check(entry.refCount, Equals, 0)
		c.Check(entry.creationRevno, Equals, entry.revno)
		c.Check(entry.removed, Equals, false)
		i++
		c.Assert(all.entities[b.entityIdForInfo(entry.info)], Equals, e)
	}
	c.Assert(len(all.entities), Equals, int(i))

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
	c.Assert(gotEntities, DeepEquals, expectEntities)
}

func (s *allWatcherStateSuite) TestStateBackingEntityIdForInfo(c *C) {
	tests := []struct {
		info       params.EntityInfo
		collection *mgo.Collection
		id         entityId
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
		id := b.entityIdForInfo(test.info)
		c.Assert(id, Equals, entityId{
			collection: test.collection.Name,
			id:         test.info.EntityId(),
		})
	}
}

func (s *allWatcherStateSuite) TestStateBackingFetch(c *C) {
	m, err := s.State.AddMachine("series", JobManageEnviron)
	c.Assert(err, IsNil)
	c.Assert(m.EntityName(), Equals, "machine-0")
	err = m.SetInstanceId(InstanceId("i-0"))
	c.Assert(err, IsNil)

	b0 := newAllWatcherStateBacking(s.State)
	testBackingFetch(c, b0)

	// Test the test backing in the same way to
	// make sure it agrees.
	b1 := newTestBacking([]params.EntityInfo{
		&params.MachineInfo{
			Id:         "0",
			InstanceId: "i-0",
		},
	})
	testBackingFetch(c, b1)
}

// TestStateWatcher tests the integration of the state watcher
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
	aw := newAllWatcher(b)
	go aw.run()
	defer aw.Stop()
	w := &StateWatcher{all: aw}
	s.State.StartSync()
	checkNext(c, w, []params.Delta{{
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
	checkDeltasEqual(c, deltas, []params.Delta{{
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

func testBackingFetch(c *C, b allWatcherBacking) {
	m := &params.MachineInfo{Id: "0", InstanceId: "i-0"}
	id0 := b.entityIdForInfo(m)
	info, err := b.fetch(id0)
	c.Assert(err, IsNil)
	c.Assert(info, DeepEquals, m)

	info, err = b.fetch(entityId{id0.collection, "99"})
	c.Assert(err, Equals, mgo.ErrNotFound)
	c.Assert(info, IsNil)
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

func deltaMap(deltas []params.Delta) map[entityId]params.EntityInfo {
	m := make(map[entityId]params.EntityInfo)
	for _, d := range deltas {
		id := entityIdForInfo(d.Entity)
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

type allWatcherTestBacking struct {
	mu       sync.Mutex
	fetchErr error
	entities map[entityId]params.EntityInfo
	watchc   chan<- watcher.Change
	txnRevno int64
}

func newTestBacking(initial []params.EntityInfo) *allWatcherTestBacking {
	b := &allWatcherTestBacking{
		entities: make(map[entityId]params.EntityInfo),
	}
	for _, info := range initial {
		b.entities[entityIdForInfo(info)] = info
	}
	return b
}

func (b *allWatcherTestBacking) fetch(id entityId) (params.EntityInfo, error) {
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

func (b *allWatcherTestBacking) entityIdForInfo(info params.EntityInfo) entityId {
	return entityIdForInfo(info)
}

func (b *allWatcherTestBacking) watch(c chan<- watcher.Change) {
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.watchc != nil {
		panic("test backing can only watch once")
	}
	b.watchc = c
}

func (b *allWatcherTestBacking) unwatch(c chan<- watcher.Change) {
	b.mu.Lock()
	defer b.mu.Unlock()
	if c != b.watchc {
		panic("unwatching wrong channel")
	}
	b.watchc = nil
}

func (b *allWatcherTestBacking) getAll(all *allInfo) error {
	b.mu.Lock()
	defer b.mu.Unlock()
	for id, info := range b.entities {
		all.update(id, info)
	}
	return nil
}

func (b *allWatcherTestBacking) updateEntity(info params.EntityInfo) {
	b.mu.Lock()
	defer b.mu.Unlock()
	id := b.entityIdForInfo(info)
	b.entities[id] = info
	b.txnRevno++
	if b.watchc != nil {
		b.watchc <- watcher.Change{
			C:     id.collection,
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

func (b *allWatcherTestBacking) deleteEntity(id entityId) {
	b.mu.Lock()
	defer b.mu.Unlock()
	delete(b.entities, id)
	b.txnRevno++
	if b.watchc != nil {
		b.watchc <- watcher.Change{
			C:     id.collection,
			Id:    id.id,
			Revno: -1,
		}
	}
}
