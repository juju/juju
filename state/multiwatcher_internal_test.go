// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state

import (
	"fmt"
	"sync"
	"time"

	"github.com/juju/errors"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"
	"gopkg.in/mgo.v2"

	"github.com/juju/juju/core/multiwatcher"
	"github.com/juju/juju/state/watcher"
	"github.com/juju/juju/testing"
)

type storeManagerSuite struct {
	testing.BaseSuite
}

var _ = gc.Suite(&storeManagerSuite{})

func (*storeManagerSuite) TestHandle(c *gc.C) {
	sm := newStoreManagerNoRun(newTestBacking(nil))

	// Add request from first watcher.
	w0 := &Multiwatcher{all: sm}
	req0 := &request{
		w:     w0,
		reply: make(chan bool, 1),
	}
	sm.handle(req0)
	assertWaitingRequests(c, sm, map[*Multiwatcher][]*request{
		w0: {req0},
	})

	// Add second request from first watcher.
	req1 := &request{
		w:     w0,
		reply: make(chan bool, 1),
	}
	sm.handle(req1)
	assertWaitingRequests(c, sm, map[*Multiwatcher][]*request{
		w0: {req1, req0},
	})

	// Add request from second watcher.
	w1 := &Multiwatcher{all: sm}
	req2 := &request{
		w:     w1,
		reply: make(chan bool, 1),
	}
	sm.handle(req2)
	assertWaitingRequests(c, sm, map[*Multiwatcher][]*request{
		w0: {req1, req0},
		w1: {req2},
	})

	// Stop first watcher.
	sm.handle(&request{
		w: w0,
	})
	assertWaitingRequests(c, sm, map[*Multiwatcher][]*request{
		w1: {req2},
	})
	assertReplied(c, false, req0)
	assertReplied(c, false, req1)

	// Stop second watcher.
	sm.handle(&request{
		w: w1,
	})
	assertWaitingRequests(c, sm, nil)
	assertReplied(c, false, req2)
}

var respondTestChanges = [...]func(all multiwatcher.Store){
	func(all multiwatcher.Store) {
		all.Update(&multiwatcher.MachineInfo{ModelUUID: "uuid", ID: "0"})
	},
	func(all multiwatcher.Store) {
		all.Update(&multiwatcher.MachineInfo{ModelUUID: "uuid", ID: "1"})
	},
	func(all multiwatcher.Store) {
		all.Update(&multiwatcher.MachineInfo{ModelUUID: "uuid", ID: "2"})
	},
	func(all multiwatcher.Store) {
		all.Remove(multiwatcher.EntityID{"machine", "uuid", "0"})
	},
	func(all multiwatcher.Store) {
		all.Update(&multiwatcher.MachineInfo{
			ModelUUID:  "uuid",
			ID:         "1",
			InstanceID: "i-1",
		})
	},
	func(all multiwatcher.Store) {
		all.Remove(multiwatcher.EntityID{"machine", "uuid", "1"})
	},
}

func (s *storeManagerSuite) TestRespondResults(c *gc.C) {
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
			sm := newStoreManagerNoRun(&storeManagerTestBacking{})
			c.Logf("test %0*b", len(respondTestChanges), ns)
			var (
				ws      []*Multiwatcher
				wstates []watcherState
				reqs    []*request
			)
			for i := 0; i < wcount; i++ {
				ws = append(ws, &Multiwatcher{})
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
								w:     ws[wi],
								reply: make(chan bool, 1),
							}
							sm.handle(reqs[wi])
						}
					}
				}
				if !needRespond {
					continue
				}
				// Check that the expected requests are pending.
				expectWaiting := make(map[*Multiwatcher][]*request)
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
				sm.handle(&request{w: w})
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

func (*storeManagerSuite) TestRespondMultiple(c *gc.C) {
	sm := newStoreManager(newTestBacking(nil))
	sm.store.Update(&multiwatcher.MachineInfo{ID: "0"})

	// Add one request and respond.
	// It should see the above change.
	w0 := &Multiwatcher{all: sm}
	req0 := &request{
		w:     w0,
		reply: make(chan bool, 1),
	}
	sm.handle(req0)
	sm.respond()
	assertReplied(c, true, req0)
	c.Assert(req0.changes, gc.DeepEquals, []multiwatcher.Delta{{Entity: &multiwatcher.MachineInfo{ID: "0"}}})
	assertWaitingRequests(c, sm, nil)

	// Add another request from the same watcher and respond.
	// It should have no reply because nothing has changed.
	req0 = &request{
		w:     w0,
		reply: make(chan bool, 1),
	}
	sm.handle(req0)
	sm.respond()
	assertNotReplied(c, req0)

	// Add two requests from another watcher and respond.
	// The request from the first watcher should still not
	// be replied to, but the later of the two requests from
	// the second watcher should get a reply.
	w1 := &Multiwatcher{all: sm}
	req1 := &request{
		w:     w1,
		reply: make(chan bool, 1),
	}
	sm.handle(req1)
	req2 := &request{
		w:     w1,
		reply: make(chan bool, 1),
	}
	sm.handle(req2)
	assertWaitingRequests(c, sm, map[*Multiwatcher][]*request{
		w0: {req0},
		w1: {req2, req1},
	})
	sm.respond()
	assertNotReplied(c, req0)
	assertNotReplied(c, req1)
	assertReplied(c, true, req2)
	c.Assert(req2.changes, gc.DeepEquals, []multiwatcher.Delta{{Entity: &multiwatcher.MachineInfo{ID: "0"}}})
	assertWaitingRequests(c, sm, map[*Multiwatcher][]*request{
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

func (*storeManagerSuite) TestRunStop(c *gc.C) {
	sm := newStoreManager(newTestBacking(nil))
	w := &Multiwatcher{all: sm}
	err := sm.Stop()
	c.Assert(err, jc.ErrorIsNil)
	d, err := w.Next()
	c.Assert(err, gc.ErrorMatches, "shared state watcher was stopped")
	c.Assert(err, jc.Satisfies, multiwatcher.IsErrStopped)
	c.Assert(d, gc.HasLen, 0)
}

func (*storeManagerSuite) TestRun(c *gc.C) {
	b := newTestBacking([]multiwatcher.EntityInfo{
		&multiwatcher.MachineInfo{ModelUUID: "uuid", ID: "0"},
		&multiwatcher.ApplicationInfo{ModelUUID: "uuid", Name: "logging"},
		&multiwatcher.ApplicationInfo{ModelUUID: "uuid", Name: "wordpress"},
	})
	sm := newStoreManager(b)
	defer func() {
		c.Check(sm.Stop(), gc.IsNil)
	}()
	w := &Multiwatcher{all: sm}
	checkNext(c, w, []multiwatcher.Delta{
		{Entity: &multiwatcher.MachineInfo{ModelUUID: "uuid", ID: "0"}},
		{Entity: &multiwatcher.ApplicationInfo{ModelUUID: "uuid", Name: "logging"}},
		{Entity: &multiwatcher.ApplicationInfo{ModelUUID: "uuid", Name: "wordpress"}},
	}, "")
	b.updateEntity(&multiwatcher.MachineInfo{ModelUUID: "uuid", ID: "0", InstanceID: "i-0"})
	checkNext(c, w, []multiwatcher.Delta{
		{Entity: &multiwatcher.MachineInfo{ModelUUID: "uuid", ID: "0", InstanceID: "i-0"}},
	}, "")
	b.deleteEntity(multiwatcher.EntityID{"machine", "uuid", "0"})
	checkNext(c, w, []multiwatcher.Delta{
		{Removed: true, Entity: &multiwatcher.MachineInfo{ModelUUID: "uuid", ID: "0"}},
	}, "")
}

func (*storeManagerSuite) TestEmptyModel(c *gc.C) {
	b := newTestBacking(nil)
	sm := newStoreManager(b)
	defer func() {
		c.Check(sm.Stop(), gc.IsNil)
	}()
	w := &Multiwatcher{all: sm}
	checkNext(c, w, nil, "")
}

func (*storeManagerSuite) TestMultiplemodels(c *gc.C) {
	b := newTestBacking([]multiwatcher.EntityInfo{
		&multiwatcher.MachineInfo{ModelUUID: "uuid0", ID: "0"},
		&multiwatcher.ApplicationInfo{ModelUUID: "uuid0", Name: "logging"},
		&multiwatcher.ApplicationInfo{ModelUUID: "uuid0", Name: "wordpress"},
		&multiwatcher.MachineInfo{ModelUUID: "uuid1", ID: "0"},
		&multiwatcher.ApplicationInfo{ModelUUID: "uuid1", Name: "logging"},
		&multiwatcher.ApplicationInfo{ModelUUID: "uuid1", Name: "wordpress"},
		&multiwatcher.MachineInfo{ModelUUID: "uuid2", ID: "0"},
	})
	sm := newStoreManager(b)
	defer func() {
		c.Check(sm.Stop(), gc.IsNil)
	}()
	w := &Multiwatcher{all: sm}
	checkNext(c, w, []multiwatcher.Delta{
		{Entity: &multiwatcher.MachineInfo{ModelUUID: "uuid0", ID: "0"}},
		{Entity: &multiwatcher.ApplicationInfo{ModelUUID: "uuid0", Name: "logging"}},
		{Entity: &multiwatcher.ApplicationInfo{ModelUUID: "uuid0", Name: "wordpress"}},
		{Entity: &multiwatcher.MachineInfo{ModelUUID: "uuid1", ID: "0"}},
		{Entity: &multiwatcher.ApplicationInfo{ModelUUID: "uuid1", Name: "logging"}},
		{Entity: &multiwatcher.ApplicationInfo{ModelUUID: "uuid1", Name: "wordpress"}},
		{Entity: &multiwatcher.MachineInfo{ModelUUID: "uuid2", ID: "0"}},
	}, "")
	b.updateEntity(&multiwatcher.MachineInfo{ModelUUID: "uuid1", ID: "0", InstanceID: "i-0"})
	checkNext(c, w, []multiwatcher.Delta{
		{Entity: &multiwatcher.MachineInfo{ModelUUID: "uuid1", ID: "0", InstanceID: "i-0"}},
	}, "")
	b.deleteEntity(multiwatcher.EntityID{"machine", "uuid2", "0"})
	checkNext(c, w, []multiwatcher.Delta{
		{Removed: true, Entity: &multiwatcher.MachineInfo{ModelUUID: "uuid2", ID: "0"}},
	}, "")
	b.updateEntity(&multiwatcher.ApplicationInfo{ModelUUID: "uuid0", Name: "logging", Exposed: true})
	checkNext(c, w, []multiwatcher.Delta{
		{Entity: &multiwatcher.ApplicationInfo{ModelUUID: "uuid0", Name: "logging", Exposed: true}},
	}, "")
}

func (*storeManagerSuite) TestMultiwatcherStop(c *gc.C) {
	sm := newStoreManager(newTestBacking(nil))
	defer func() {
		c.Check(sm.Stop(), gc.IsNil)
	}()
	w := &Multiwatcher{all: sm}
	err := w.Stop()
	c.Assert(err, jc.ErrorIsNil)
	checkNext(c, w, nil, multiwatcher.NewErrStopped().Error())
}

func (*storeManagerSuite) TestMultiwatcherStopBecauseStoreManagerError(c *gc.C) {
	b := newTestBacking([]multiwatcher.EntityInfo{&multiwatcher.MachineInfo{ID: "0"}})
	sm := newStoreManager(b)
	defer func() {
		c.Check(sm.Stop(), gc.ErrorMatches, "some error")
	}()
	w := &Multiwatcher{all: sm}

	// Receive one delta to make sure that the storeManager
	// has seen the initial state.
	checkNext(c, w, []multiwatcher.Delta{{Entity: &multiwatcher.MachineInfo{ID: "0"}}}, "")
	c.Logf("setting fetch error")
	b.setFetchError(errors.New("some error"))

	c.Logf("updating entity")
	b.updateEntity(&multiwatcher.MachineInfo{ID: "1"})
	checkNext(c, w, nil, "some error")
}

func (*storeManagerSuite) TestMultiwatcherStopBecauseStoreManagerStop(c *gc.C) {
	b := newTestBacking([]multiwatcher.EntityInfo{&multiwatcher.MachineInfo{ID: "0"}})
	sm := newStoreManager(b)
	w := &Multiwatcher{all: sm}

	// Receive one delta to make sure that the storeManager
	// has seen the initial state.
	checkNext(c, w, []multiwatcher.Delta{{Entity: &multiwatcher.MachineInfo{ID: "0"}}}, "")

	// Stop the the store manager cleanly and check that
	// the next update returns ErrStopped.
	c.Check(sm.Stop(), jc.ErrorIsNil)
	b.updateEntity(&multiwatcher.MachineInfo{ID: "1"})
	checkNext(c, w, nil, multiwatcher.ErrStoppedf("shared state watcher").Error())
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

func assertWaitingRequests(c *gc.C, sm *storeManager, waiting map[*Multiwatcher][]*request) {
	c.Assert(sm.waiting, gc.HasLen, len(waiting))
	for w, reqs := range waiting {
		i := 0
		for req := sm.waiting[w]; ; req = req.next {
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

type storeManagerTestBacking struct {
	mu       sync.Mutex
	fetchErr error
	entities map[multiwatcher.EntityID]multiwatcher.EntityInfo
	watchc   chan<- watcher.Change
	txnRevno int64
}

func newTestBacking(initial []multiwatcher.EntityInfo) *storeManagerTestBacking {
	b := &storeManagerTestBacking{
		entities: make(map[multiwatcher.EntityID]multiwatcher.EntityInfo),
	}
	for _, info := range initial {
		b.entities[info.EntityID()] = info
	}
	return b
}

func (b *storeManagerTestBacking) Changed(all multiwatcher.Store, change watcher.Change) error {
	modelUUID, changeId, ok := splitDocID(change.Id.(string))
	if !ok {
		return errors.Errorf("unexpected id format: %v", change.Id)
	}
	id := multiwatcher.EntityID{
		Kind:      change.C,
		ModelUUID: modelUUID,
		ID:        changeId,
	}
	info, err := b.fetch(id)
	if err == mgo.ErrNotFound {
		all.Remove(id)
		return nil
	}
	if err != nil {
		return err
	}
	all.Update(info)
	return nil
}

func (b *storeManagerTestBacking) fetch(id multiwatcher.EntityID) (multiwatcher.EntityInfo, error) {
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

func (b *storeManagerTestBacking) Watch(c chan<- watcher.Change) {
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.watchc != nil {
		panic("test backing can only watch once")
	}
	b.watchc = c
}

func (b *storeManagerTestBacking) Unwatch(c chan<- watcher.Change) {
	b.mu.Lock()
	defer b.mu.Unlock()
	if c != b.watchc {
		panic("unwatching wrong channel")
	}
	b.watchc = nil
}

func (b *storeManagerTestBacking) GetAll(all multiwatcher.Store) error {
	b.mu.Lock()
	defer b.mu.Unlock()
	for _, info := range b.entities {
		all.Update(info)
	}
	return nil
}

func (b *storeManagerTestBacking) Release() error {
	return nil
}

func (b *storeManagerTestBacking) updateEntity(info multiwatcher.EntityInfo) {
	b.mu.Lock()
	defer b.mu.Unlock()
	id := info.EntityID()
	b.entities[id] = info
	b.txnRevno++
	if b.watchc != nil {
		b.watchc <- watcher.Change{
			C:     id.Kind,
			Id:    ensureModelUUID(id.ModelUUID, id.ID),
			Revno: b.txnRevno, // This is actually ignored, but fill it in anyway.
		}
	}
}

func (b *storeManagerTestBacking) setFetchError(err error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.fetchErr = err
}

func (b *storeManagerTestBacking) deleteEntity(id multiwatcher.EntityID) {
	b.mu.Lock()
	defer b.mu.Unlock()
	delete(b.entities, id)
	b.txnRevno++
	if b.watchc != nil {
		b.watchc <- watcher.Change{
			C:     id.Kind,
			Id:    ensureModelUUID(id.ModelUUID, id.ID),
			Revno: -1,
		}
	}
}

var errTimeout = errors.New("no change received in sufficient time")

func getNext(c *gc.C, w *Multiwatcher, timeout time.Duration) ([]multiwatcher.Delta, error) {
	var deltas []multiwatcher.Delta
	var err error
	ch := make(chan struct{}, 1)
	go func() {
		deltas, err = w.Next()
		ch <- struct{}{}
	}()
	select {
	case <-ch:
		return deltas, err
	case <-time.After(timeout):
	}
	return nil, errTimeout
}

func checkNext(c *gc.C, w *Multiwatcher, deltas []multiwatcher.Delta, expectErr string) {
	d, err := getNext(c, w, 1*time.Second)
	if expectErr != "" {
		c.Check(err, gc.ErrorMatches, expectErr)
		return
	}
	c.Assert(err, jc.ErrorIsNil)
	checkDeltasEqual(c, d, deltas)
}
