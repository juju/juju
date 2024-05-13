// Copyright 2019 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package multiwatcher

import (
	"fmt"

	"github.com/juju/clock"
	"github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/core/multiwatcher"
	loggertesting "github.com/juju/juju/internal/logger/testing"
	"github.com/juju/juju/internal/worker/multiwatcher/testbacking"
)

type workerSuite struct {
	testing.IsolationSuite
}

var _ = gc.Suite(&workerSuite{})

func (*workerSuite) TestHandle(c *gc.C) {
	sm := &Worker{
		config: Config{
			Clock:   clock.WallClock,
			Logger:  loggertesting.WrapCheckLog(c),
			Backing: testbacking.New(nil),
		},
		request: make(chan *request),
		waiting: make(map[*Watcher]*request),
		store:   multiwatcher.NewStore(loggertesting.WrapCheckLog(c)),
	}

	// Add request from first watcher.
	w0 := sm.newWatcher(nil)
	req0 := &request{
		watcher: w0,
		reply:   make(chan bool, 1),
	}
	sm.handle(req0)
	assertWaitingRequests(c, sm, map[*Watcher][]*request{
		w0: {req0},
	})

	// Add second request from first watcher.
	req1 := &request{
		watcher: w0,
		reply:   make(chan bool, 1),
	}
	sm.handle(req1)
	assertWaitingRequests(c, sm, map[*Watcher][]*request{
		w0: {req1, req0},
	})

	// Add request from second watcher.
	w1 := sm.newWatcher(nil)
	req2 := &request{
		watcher: w1,
		reply:   make(chan bool, 1),
	}
	sm.handle(req2)
	assertWaitingRequests(c, sm, map[*Watcher][]*request{
		w0: {req1, req0},
		w1: {req2},
	})

	// Stop first watcher.
	sm.handle(&request{
		watcher: w0,
	})
	assertWaitingRequests(c, sm, map[*Watcher][]*request{
		w1: {req2},
	})
	assertReplied(c, false, req0)
	assertReplied(c, false, req1)

	// Stop second watcher.
	sm.handle(&request{
		watcher: w1,
	})
	assertWaitingRequests(c, sm, nil)
	assertReplied(c, false, req2)
}

func (*workerSuite) TestRespondMultiple(c *gc.C) {
	sm := &Worker{
		config: Config{
			Clock:   clock.WallClock,
			Logger:  loggertesting.WrapCheckLog(c),
			Backing: testbacking.New(nil),
		},
		request: make(chan *request),
		waiting: make(map[*Watcher]*request),
		store:   multiwatcher.NewStore(loggertesting.WrapCheckLog(c)),
	}

	sm.store.Update(&multiwatcher.MachineInfo{ID: "0"})

	// Add one request and respond.
	// It should see the above change.
	w0 := sm.newWatcher(nil)
	req0 := &request{
		watcher: w0,
		reply:   make(chan bool, 1),
	}
	sm.handle(req0)
	sm.respond()
	assertReplied(c, true, req0)
	c.Assert(req0.changes, gc.DeepEquals, []multiwatcher.Delta{{Entity: &multiwatcher.MachineInfo{ID: "0"}}})
	assertWaitingRequests(c, sm, nil)

	// Add another request from the same watcher and respond.
	// It should have no reply because nothing has changed.
	req0 = &request{
		watcher: w0,
		reply:   make(chan bool, 1),
	}
	sm.handle(req0)
	sm.respond()
	assertNotReplied(c, req0)

	// Add two requests from another watcher and respond.
	// The request from the first watcher should still not
	// be replied to, but the later of the two requests from
	// the second watcher should get a reply.
	w1 := sm.newWatcher(nil)
	req1 := &request{
		watcher: w1,
		reply:   make(chan bool, 1),
	}
	sm.handle(req1)
	req2 := &request{
		watcher: w1,
		reply:   make(chan bool, 1),
	}
	sm.handle(req2)
	assertWaitingRequests(c, sm, map[*Watcher][]*request{
		w0: {req0},
		w1: {req2, req1},
	})
	sm.respond()
	assertNotReplied(c, req0)
	assertNotReplied(c, req1)
	assertReplied(c, true, req2)
	c.Assert(req2.changes, gc.DeepEquals, []multiwatcher.Delta{{Entity: &multiwatcher.MachineInfo{ID: "0"}}})
	assertWaitingRequests(c, sm, map[*Watcher][]*request{
		w0: {req0},
		w1: {req1},
	})

	// Check that nothing more gets responded to if we call respond again.
	sm.respond()
	assertNotReplied(c, req0)
	assertNotReplied(c, req1)

	// Now make a change and check that both waiting requests
	// get serviced.
	sm.store.Update(&multiwatcher.MachineInfo{ID: "1"})
	sm.respond()
	assertReplied(c, true, req0)
	assertReplied(c, true, req1)
	assertWaitingRequests(c, sm, nil)

	deltas := []multiwatcher.Delta{{Entity: &multiwatcher.MachineInfo{ID: "1"}}}
	c.Assert(req0.changes, gc.DeepEquals, deltas)
	c.Assert(req1.changes, gc.DeepEquals, deltas)
}

var respondTestChanges = [...]func(store multiwatcher.Store){
	func(store multiwatcher.Store) {
		store.Update(&multiwatcher.MachineInfo{ModelUUID: "uuid", ID: "0"})
	},
	func(store multiwatcher.Store) {
		store.Update(&multiwatcher.MachineInfo{ModelUUID: "uuid", ID: "1"})
	},
	func(store multiwatcher.Store) {
		store.Update(&multiwatcher.MachineInfo{ModelUUID: "uuid", ID: "2"})
	},
	func(store multiwatcher.Store) {
		store.Remove(multiwatcher.EntityID{"machine", "uuid", "0"})
	},
	func(store multiwatcher.Store) {
		store.Update(&multiwatcher.MachineInfo{
			ModelUUID:  "uuid",
			ID:         "1",
			InstanceID: "i-1",
		})
	},
	func(store multiwatcher.Store) {
		store.Remove(multiwatcher.EntityID{"machine", "uuid", "1"})
	},
}

func (s *workerSuite) TestRespondResults(c *gc.C) {
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

			sm := &Worker{
				config: Config{
					Clock:   clock.WallClock,
					Logger:  loggertesting.WrapCheckLog(c),
					Backing: testbacking.New(nil),
				},
				request: make(chan *request),
				waiting: make(map[*Watcher]*request),
				store:   multiwatcher.NewStore(loggertesting.WrapCheckLog(c)),
			}

			c.Logf("test %0*b", len(respondTestChanges), ns)
			var (
				ws      []*Watcher
				wstates []watcherState
				reqs    []*request
			)
			for i := 0; i < wcount; i++ {
				ws = append(ws, sm.newWatcher(nil))
				wstates = append(wstates, make(watcherState))
				reqs = append(reqs, nil)
			}
			// Make each change in turn, and make a request for each
			// watcher if n and respond
			for i, change := range respondTestChanges {
				c.Logf("change %d", i)
				change(sm.store)
				needRespond := false
				for wi, n := range ns {
					if n&(1<<uint(i)) != 0 {
						needRespond = true
						if reqs[wi] == nil {
							reqs[wi] = &request{
								watcher: ws[wi],
								reply:   make(chan bool, 1),
							}
							sm.handle(reqs[wi])
						}
					}
				}
				if !needRespond {
					continue
				}
				// Check that the expected requests are pending.
				expectWaiting := make(map[*Watcher][]*request)
				for wi, w := range ws {
					if reqs[wi] != nil {
						expectWaiting[w] = []*request{reqs[wi]}
					}
				}
				assertWaitingRequests(c, sm, expectWaiting)
				// Actually respond; then check that each watcher with
				// an outstanding request now has an up to date view
				// of the world.
				sm.respond()
				for wi, req := range reqs {
					if req == nil {
						continue
					}
					select {
					case ok := <-req.reply:
						c.Assert(ok, jc.IsTrue)
						c.Assert(len(req.changes) > 0, jc.IsTrue)
						wstates[wi].update(req.changes)
						reqs[wi] = nil
					default:
					}
					c.Logf("check %d", wi)
					wstates[wi].check(c, sm.store)
				}
			}
			// Stop the watcher and check that all ref counts end up at zero
			// and removed objects are deleted.
			for wi, w := range ws {
				sm.handle(&request{watcher: w})
				if reqs[wi] != nil {
					assertReplied(c, false, reqs[wi])
				}
			}

			c.Assert(sm.store.All(), jc.DeepEquals, []multiwatcher.EntityInfo{
				&multiwatcher.MachineInfo{
					ModelUUID: "uuid",
					ID:        "2",
				},
			})
			c.Assert(sm.store.Size(), gc.Equals, 1)
		}
	}
}

func assertNotReplied(c *gc.C, req *request) {
	select {
	case v := <-req.reply:
		c.Fatalf("request was unexpectedly replied to (got %v)", v)
	default:
	}
}

func assertReplied(c *gc.C, val bool, req *request) {
	select {
	case v := <-req.reply:
		c.Assert(v, gc.Equals, val)
	default:
		c.Fatalf("request was not replied to")
	}
}

func assertWaitingRequests(c *gc.C, worker *Worker, waiting map[*Watcher][]*request) {
	c.Assert(worker.waiting, gc.HasLen, len(waiting))
	for w, reqs := range waiting {
		i := 0
		for req := worker.waiting[w]; ; req = req.next {
			if i >= len(reqs) {
				c.Assert(req, gc.IsNil)
				break
			}
			c.Assert(req, gc.Equals, reqs[i])
			assertNotReplied(c, req)
			i++
		}
	}
}

// watcherState represents a Multiwatcher client's
// current view of the state. It holds the last delta that a given
// state watcher has seen for each entity.
type watcherState map[interface{}]multiwatcher.Delta

func (s watcherState) update(changes []multiwatcher.Delta) {
	for _, d := range changes {
		id := d.Entity.EntityID()
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
func (s watcherState) check(c *gc.C, store multiwatcher.Store) {
	currentEntities := make(watcherState)
	for _, info := range store.All() {
		currentEntities[info.EntityID()] = multiwatcher.Delta{Entity: info}
	}
	c.Assert(s, gc.DeepEquals, currentEntities)
}
