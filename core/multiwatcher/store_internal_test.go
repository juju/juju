// Copyright 2019 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package multiwatcher

import (
	"container/list"
	"fmt"

	"github.com/juju/loggo/v2"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/testing"
)

var _ = gc.Suite(&storeSuite{})

type storeSuite struct {
	testing.BaseSuite
}

var StoreChangeMethodTests = []struct {
	about          string
	change         func(*store)
	expectRevno    int64
	expectContents []entityEntry
}{{
	about:  "empty at first",
	change: func(*store) {},
}, {
	about: "add single entry",
	change: func(all *store) {
		all.Update(&MachineInfo{
			ID:         "0",
			InstanceID: "i-0",
		})
	},
	expectRevno: 1,
	expectContents: []entityEntry{{
		creationRevno: 1,
		revno:         1,
		info: &MachineInfo{
			ID:         "0",
			InstanceID: "i-0",
		},
	}},
}, {
	about: "add two entries",
	change: func(all *store) {
		all.Update(&MachineInfo{
			ID:         "0",
			InstanceID: "i-0",
		})
		all.Update(&ApplicationInfo{
			Name:    "wordpress",
			Exposed: true,
		})
	},
	expectRevno: 2,
	expectContents: []entityEntry{{
		creationRevno: 1,
		revno:         1,
		info: &MachineInfo{
			ID:         "0",
			InstanceID: "i-0",
		},
	}, {
		creationRevno: 2,
		revno:         2,
		info: &ApplicationInfo{
			Name:    "wordpress",
			Exposed: true,
		},
	}},
}, {
	about: "update an entity that's not currently there",
	change: func(all *store) {
		m := &MachineInfo{ID: "1"}
		all.Update(m)
	},
	expectRevno: 1,
	expectContents: []entityEntry{{
		creationRevno: 1,
		revno:         1,
		info:          &MachineInfo{ID: "1"},
	}},
}, {
	about: "mark application removed then update",
	change: func(all *store) {
		all.Update(&ApplicationInfo{ModelUUID: "uuid0", Name: "logging"})
		all.Update(&ApplicationInfo{ModelUUID: "uuid0", Name: "wordpress"})
		StoreIncRef(all, EntityID{"application", "uuid0", "logging"})
		all.Remove(EntityID{"application", "uuid0", "logging"})
		all.Update(&ApplicationInfo{
			ModelUUID: "uuid0",
			Name:      "wordpress",
			Exposed:   true,
		})
		all.Update(&ApplicationInfo{
			ModelUUID: "uuid0",
			Name:      "logging",
			Exposed:   true,
		})
	},
	expectRevno: 5,
	expectContents: []entityEntry{{
		revno:         4,
		creationRevno: 2,
		removed:       false,
		refCount:      0,
		info: &ApplicationInfo{
			ModelUUID: "uuid0",
			Name:      "wordpress",
			Exposed:   true,
		}}, {
		revno:         5,
		creationRevno: 1,
		removed:       false,
		refCount:      1,
		info: &ApplicationInfo{
			ModelUUID: "uuid0",
			Name:      "logging",
			Exposed:   true,
		},
	}},
}, {
	about: "mark removed on existing entry",
	change: func(all *store) {
		all.Update(&MachineInfo{ModelUUID: "uuid", ID: "0"})
		all.Update(&MachineInfo{ModelUUID: "uuid", ID: "1"})
		StoreIncRef(all, EntityID{"machine", "uuid", "0"})
		all.Remove(EntityID{"machine", "uuid", "0"})
	},
	expectRevno: 3,
	expectContents: []entityEntry{{
		creationRevno: 2,
		revno:         2,
		info:          &MachineInfo{ModelUUID: "uuid", ID: "1"},
	}, {
		creationRevno: 1,
		revno:         3,
		refCount:      1,
		removed:       true,
		info:          &MachineInfo{ModelUUID: "uuid", ID: "0"},
	}},
}, {
	about: "mark removed on nonexistent entry",
	change: func(all *store) {
		all.Remove(EntityID{"machine", "uuid", "0"})
	},
}, {
	about: "mark removed on already marked entry",
	change: func(all *store) {
		all.Update(&MachineInfo{ModelUUID: "uuid", ID: "0"})
		all.Update(&MachineInfo{ModelUUID: "uuid", ID: "1"})
		StoreIncRef(all, EntityID{"machine", "uuid", "0"})
		all.Remove(EntityID{"machine", "uuid", "0"})
		all.Update(&MachineInfo{
			ModelUUID:  "uuid",
			ID:         "1",
			InstanceID: "i-1",
		})
		all.Remove(EntityID{"machine", "uuid", "0"})
	},
	expectRevno: 4,
	expectContents: []entityEntry{{
		creationRevno: 1,
		revno:         3,
		refCount:      1,
		removed:       true,
		info:          &MachineInfo{ModelUUID: "uuid", ID: "0"},
	}, {
		creationRevno: 2,
		revno:         4,
		info: &MachineInfo{
			ModelUUID:  "uuid",
			ID:         "1",
			InstanceID: "i-1",
		},
	}},
}, {
	about: "mark removed on entry with zero ref count",
	change: func(all *store) {
		all.Update(&MachineInfo{ModelUUID: "uuid", ID: "0"})
		all.Remove(EntityID{"machine", "uuid", "0"})
	},
	expectRevno: 2,
}, {
	about: "delete entry",
	change: func(all *store) {
		all.Update(&MachineInfo{ModelUUID: "uuid", ID: "0"})
		all.delete(EntityID{"machine", "uuid", "0"})
	},
	expectRevno: 1,
}, {
	about: "decref of non-removed entity",
	change: func(all *store) {
		m := &MachineInfo{ID: "0"}
		all.Update(m)
		id := m.EntityID()
		StoreIncRef(all, id)
		entry := all.entities[id].Value.(*entityEntry)
		all.decRef(entry)
	},
	expectRevno: 1,
	expectContents: []entityEntry{{
		creationRevno: 1,
		revno:         1,
		refCount:      0,
		info:          &MachineInfo{ID: "0"},
	}},
}, {
	about: "decref of removed entity",
	change: func(all *store) {
		m := &MachineInfo{ID: "0"}
		all.Update(m)
		id := m.EntityID()
		entry := all.entities[id].Value.(*entityEntry)
		entry.refCount++
		all.Remove(id)
		all.decRef(entry)
	},
	expectRevno: 2,
},
}

func (s *storeSuite) TestStoreChangeMethods(c *gc.C) {
	for i, test := range StoreChangeMethodTests {
		all := newStore(loggo.GetLogger("test"))
		c.Logf("test %d. %s", i, test.about)
		test.change(all)
		assertStoreContents(c, all, test.expectRevno, test.expectContents)
	}
}

func (s *storeSuite) TestChangesSince(c *gc.C) {
	a := newStore(loggo.GetLogger("test"))
	// Add three entries.
	var deltas []Delta
	for i := 0; i < 3; i++ {
		m := &MachineInfo{
			ModelUUID: "uuid",
			ID:        fmt.Sprint(i),
		}
		a.Update(m)
		deltas = append(deltas, Delta{Entity: m})
	}
	// Check that the deltas from each revno are as expected.
	for i := 0; i < 3; i++ {
		c.Logf("test %d", i)
		changes, _ := a.ChangesSince(int64(i))
		c.Assert(len(changes), gc.Equals, len(deltas)-i)
		c.Assert(changes, jc.DeepEquals, deltas[i:])
	}

	// Check boundary cases.
	changes, _ := a.ChangesSince(-1)
	c.Assert(changes, jc.DeepEquals, deltas)
	changes, rev := a.ChangesSince(99)
	c.Assert(changes, gc.HasLen, 0)

	// Update one machine and check we see the changes.
	m1 := &MachineInfo{
		ModelUUID:  "uuid",
		ID:         "1",
		InstanceID: "foo",
	}
	a.Update(m1)
	changes, latest := a.ChangesSince(rev)
	c.Assert(changes, jc.DeepEquals, []Delta{{Entity: m1}})
	c.Assert(latest, gc.Equals, a.latestRevno)

	// Make sure the machine isn't simply removed from
	// the list when it's marked as removed.
	StoreIncRef(a, EntityID{"machine", "uuid", "0"})

	// Remove another machine and check we see it's removed.
	m0 := &MachineInfo{ModelUUID: "uuid", ID: "0"}
	a.Remove(m0.EntityID())

	// Check that something that never saw m0 does not get
	// informed of its removal (even those the removed entity
	// is still in the list.
	changes, _ = a.ChangesSince(0)
	c.Assert(changes, jc.DeepEquals, []Delta{{
		Entity: &MachineInfo{ModelUUID: "uuid", ID: "2"},
	}, {
		Entity: m1,
	}})

	changes, _ = a.ChangesSince(rev)
	c.Assert(changes, jc.DeepEquals, []Delta{{
		Entity: m1,
	}, {
		Removed: true,
		Entity:  m0,
	}})

	changes, _ = a.ChangesSince(rev + 1)
	c.Assert(changes, jc.DeepEquals, []Delta{{
		Removed: true,
		Entity:  m0,
	}})
}

func (s *storeSuite) TestGet(c *gc.C) {
	a := newStore(loggo.GetLogger("test"))
	m := &MachineInfo{ModelUUID: "uuid", ID: "0"}
	a.Update(m)

	c.Assert(a.Get(m.EntityID()), gc.DeepEquals, m)
	c.Assert(a.Get(EntityID{"machine", "uuid", "1"}), gc.IsNil)
}

func (s *storeSuite) TestDecReferenceWithZero(c *gc.C) {
	// If a watcher is stopped before it had looked at any items, then we shouldn't
	// decrement its ref count when it is stopped.
	store := newStore(loggo.GetLogger("test"))
	m := &MachineInfo{ModelUUID: "uuid", ID: "0"}
	store.Update(m)

	StoreIncRef(store, EntityID{"machine", "uuid", "0"})
	store.DecReference(0)

	assertStoreContents(c, store, 1, []entityEntry{{
		creationRevno: 1,
		revno:         1,
		refCount:      1,
		info:          m,
	}})
}

func (s *storeSuite) TestDecReferenceIfAlreadySeenRemoved(c *gc.C) {
	// If the Multiwatcher has already seen the item removed, then
	// we shouldn't decrement its ref count when it is stopped.

	store := newStore(loggo.GetLogger("test"))
	m := &MachineInfo{ModelUUID: "uuid", ID: "0"}
	store.Update(m)

	id := EntityID{"machine", "uuid", "0"}
	StoreIncRef(store, id)
	store.Remove(id)
	store.DecReference(0)

	assertStoreContents(c, store, 2, []entityEntry{{
		creationRevno: 1,
		revno:         2,
		refCount:      1,
		removed:       true,
		info:          m,
	}})
}

func (s *storeSuite) TestHandleStopDecRefIfAlreadySeenAndNotRemoved(c *gc.C) {
	// If the Multiwatcher has already seen the item removed, then
	// we should decrement its ref count when it is stopped.
	store := newStore(loggo.GetLogger("test"))
	info := &MachineInfo{ModelUUID: "uuid", ID: "0"}
	store.Update(info)

	StoreIncRef(store, EntityID{"machine", "uuid", "0"})
	store.DecReference(store.latestRevno)

	assertStoreContents(c, store, 1, []entityEntry{{
		creationRevno: 1,
		revno:         1,
		info:          info,
	}})
}

func StoreIncRef(a *store, id interface{}) {
	entry := a.entities[id].Value.(*entityEntry)
	entry.refCount++
}

func assertStoreContents(c *gc.C, a *store, latestRevno int64, entries []entityEntry) {
	var gotEntries []entityEntry
	var gotElems []*list.Element
	c.Check(a.list.Len(), gc.Equals, len(entries))
	for e := a.list.Back(); e != nil; e = e.Prev() {
		gotEntries = append(gotEntries, *e.Value.(*entityEntry))
		gotElems = append(gotElems, e)
	}
	c.Assert(gotEntries, jc.DeepEquals, entries)
	for i, ent := range entries {
		c.Assert(a.entities[ent.info.EntityID()], gc.Equals, gotElems[i])
	}
	c.Assert(a.entities, gc.HasLen, len(entries))
	c.Assert(a.latestRevno, gc.Equals, latestRevno)
}
