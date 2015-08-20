// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package remotestate_test

import (
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"
	"gopkg.in/juju/charm.v5"

	"github.com/juju/juju/apiserver/params"
	"github.com/juju/juju/state/multiwatcher"
	"github.com/juju/juju/testing"
	"github.com/juju/juju/worker/uniter/remotestate"
	"github.com/juju/names"
)

type WatcherSuite struct {
	testing.BaseSuite

	st         mockState
	leadership mockLeadershipTracker
}

var _ = gc.Suite(&WatcherSuite{})

func (s *WatcherSuite) SetUpTest(c *gc.C) {
	s.BaseSuite.SetUpTest(c)
	s.st = mockState{
		unit: mockUnit{
			tag:  names.NewUnitTag("mysql/0"),
			life: params.Alive,
			service: mockService{
				tag:            names.NewServiceTag("mysql"),
				life:           params.Alive,
				curl:           charm.MustParseURL("cs:trusty/mysql"),
				serviceWatcher: mockNotifyWatcher{changes: make(chan struct{}, 1)},
				leaderSettingsWatcher: mockNotifyWatcher{
					changes: make(chan struct{}, 1),
				},
				relationsWatcher: mockStringsWatcher{
					changes: make(chan []string, 1),
				},
			},
			unitWatcher:           mockNotifyWatcher{changes: make(chan struct{}, 1)},
			addressesWatcher:      mockNotifyWatcher{changes: make(chan struct{}, 1)},
			configSettingsWatcher: mockNotifyWatcher{changes: make(chan struct{}, 1)},
			storageWatcher:        mockStringsWatcher{changes: make(chan []string, 1)},
		},
		relations:                 make(map[names.RelationTag]*mockRelation),
		storageAttachment:         make(map[params.StorageAttachmentId]params.StorageAttachment),
		relationUnitsWatchers:     make(map[names.RelationTag]*mockRelationUnitsWatcher),
		storageAttachmentWatchers: make(map[names.StorageTag]*mockStorageAttachmentWatcher),
	}

	s.leadership = mockLeadershipTracker{
		claimTicket:  mockTicket{make(chan struct{}, 1), true},
		leaderTicket: mockTicket{make(chan struct{}, 1), true},
		minionTicket: mockTicket{make(chan struct{}, 1), true},
	}
}

func (s *WatcherSuite) TestInitialSnapshot(c *gc.C) {
	w, err := remotestate.NewWatcher(&s.st, &s.leadership, s.st.unit.tag)
	c.Assert(err, jc.ErrorIsNil)
	snap := w.Snapshot()
	c.Assert(w.Stop(), jc.ErrorIsNil)
	c.Assert(snap, jc.DeepEquals, remotestate.Snapshot{
		Relations: map[int]remotestate.RelationSnapshot{},
		Storage:   map[names.StorageTag]remotestate.StorageSnapshot{},
	})
}

func (s *WatcherSuite) TestInitialSignal(c *gc.C) {
	w, err := remotestate.NewWatcher(&s.st, &s.leadership, s.st.unit.tag)
	c.Assert(err, jc.ErrorIsNil)
	defer func() { c.Assert(w.Stop(), jc.ErrorIsNil) }()

	// There should not be a remote state change until
	// we've seen all of the top-level notifications.
	s.st.unit.unitWatcher.changes <- struct{}{}
	assertNoNotifyEvent(c, w.RemoteStateChanged(), "remote state change")

	s.st.unit.addressesWatcher.changes <- struct{}{}
	s.st.unit.configSettingsWatcher.changes <- struct{}{}
	s.st.unit.storageWatcher.changes <- []string{}
	s.st.unit.service.serviceWatcher.changes <- struct{}{}
	s.st.unit.service.leaderSettingsWatcher.changes <- struct{}{}
	s.st.unit.service.relationsWatcher.changes <- []string{}
	s.leadership.claimTicket.ch <- struct{}{}
	assertNotifyEvent(c, w.RemoteStateChanged(), "waiting for remote state change")
}

func signalAll(st *mockState, l *mockLeadershipTracker) {
	st.unit.unitWatcher.changes <- struct{}{}
	st.unit.addressesWatcher.changes <- struct{}{}
	st.unit.configSettingsWatcher.changes <- struct{}{}
	st.unit.storageWatcher.changes <- []string{}
	st.unit.service.serviceWatcher.changes <- struct{}{}
	st.unit.service.leaderSettingsWatcher.changes <- struct{}{}
	st.unit.service.relationsWatcher.changes <- []string{}
	l.claimTicket.ch <- struct{}{}
}

func (s *WatcherSuite) TestSnapshot(c *gc.C) {
	w, err := remotestate.NewWatcher(&s.st, &s.leadership, s.st.unit.tag)
	c.Assert(err, jc.ErrorIsNil)
	defer func() { c.Assert(w.Stop(), jc.ErrorIsNil) }()
	signalAll(&s.st, &s.leadership)
	assertNotifyEvent(c, w.RemoteStateChanged(), "waiting for remote state change")

	snap := w.Snapshot()
	c.Assert(snap, jc.DeepEquals, remotestate.Snapshot{
		Life:                  s.st.unit.life,
		Relations:             map[int]remotestate.RelationSnapshot{},
		Storage:               map[names.StorageTag]remotestate.StorageSnapshot{},
		CharmURL:              s.st.unit.service.curl,
		ForceCharmUpgrade:     s.st.unit.service.forceUpgrade,
		ResolvedMode:          s.st.unit.resolved,
		ConfigVersion:         2, // config settings and addresses
		LeaderSettingsVersion: 1,
		Leader:                true,
	})
}

func (s *WatcherSuite) TestRemoteStateChanged(c *gc.C) {
	w, err := remotestate.NewWatcher(&s.st, &s.leadership, s.st.unit.tag)
	c.Assert(err, jc.ErrorIsNil)
	defer func() { c.Assert(w.Stop(), jc.ErrorIsNil) }()

	assertOneChange := func() {
		assertNotifyEvent(c, w.RemoteStateChanged(), "waiting for remote state change")
		assertNoNotifyEvent(c, w.RemoteStateChanged(), "remote state change")
	}

	signalAll(&s.st, &s.leadership)
	assertOneChange()
	initial := w.Snapshot()

	s.st.unit.life = params.Dying
	s.st.unit.unitWatcher.changes <- struct{}{}
	assertOneChange()
	c.Assert(w.Snapshot().Life, gc.Equals, params.Dying)

	s.st.unit.addressesWatcher.changes <- struct{}{}
	assertOneChange()
	c.Assert(w.Snapshot().ConfigVersion, gc.Equals, initial.ConfigVersion+1)

	s.st.unit.configSettingsWatcher.changes <- struct{}{}
	assertOneChange()
	c.Assert(w.Snapshot().ConfigVersion, gc.Equals, initial.ConfigVersion+2)

	s.st.unit.storageWatcher.changes <- []string{}
	assertOneChange()

	s.st.unit.service.forceUpgrade = true
	s.st.unit.service.serviceWatcher.changes <- struct{}{}
	assertOneChange()
	c.Assert(w.Snapshot().ForceCharmUpgrade, jc.IsTrue)

	s.st.unit.service.leaderSettingsWatcher.changes <- struct{}{}
	assertOneChange()
	c.Assert(w.Snapshot().LeaderSettingsVersion, gc.Equals, initial.LeaderSettingsVersion+1)

	s.st.unit.service.relationsWatcher.changes <- []string{}
	assertOneChange()
}

func (s *WatcherSuite) TestClearResolvedMode(c *gc.C) {
	w, err := remotestate.NewWatcher(&s.st, &s.leadership, s.st.unit.tag)
	c.Assert(err, jc.ErrorIsNil)
	defer func() { c.Assert(w.Stop(), jc.ErrorIsNil) }()
	s.st.unit.resolved = params.ResolvedRetryHooks
	signalAll(&s.st, &s.leadership)
	assertNotifyEvent(c, w.RemoteStateChanged(), "waiting for remote state change")

	snap := w.Snapshot()
	c.Assert(snap.ResolvedMode, gc.Equals, params.ResolvedRetryHooks)

	w.ClearResolvedMode()
	snap = w.Snapshot()
	c.Assert(snap.ResolvedMode, gc.Equals, params.ResolvedNone)
}

func (s *WatcherSuite) TestLeadershipChanged(c *gc.C) {
	w, err := remotestate.NewWatcher(&s.st, &s.leadership, s.st.unit.tag)
	c.Assert(err, jc.ErrorIsNil)
	defer func() { c.Assert(w.Stop(), jc.ErrorIsNil) }()

	s.leadership.claimTicket.result = false
	signalAll(&s.st, &s.leadership)
	assertNotifyEvent(c, w.RemoteStateChanged(), "waiting for remote state change")
	c.Assert(w.Snapshot().Leader, jc.IsFalse)

	s.leadership.leaderTicket.ch <- struct{}{}
	assertNotifyEvent(c, w.RemoteStateChanged(), "waiting for remote state change")
	c.Assert(w.Snapshot().Leader, jc.IsTrue)

	s.leadership.minionTicket.ch <- struct{}{}
	assertNotifyEvent(c, w.RemoteStateChanged(), "waiting for remote state change")
	c.Assert(w.Snapshot().Leader, jc.IsFalse)
}

func (s *WatcherSuite) TestLeadershipMinionUnchanged(c *gc.C) {
	w, err := remotestate.NewWatcher(&s.st, &s.leadership, s.st.unit.tag)
	c.Assert(err, jc.ErrorIsNil)
	defer func() { c.Assert(w.Stop(), jc.ErrorIsNil) }()

	s.leadership.claimTicket.result = false
	signalAll(&s.st, &s.leadership)
	assertNotifyEvent(c, w.RemoteStateChanged(), "waiting for remote state change")

	// Initially minion, so triggering minion should have no effect.
	s.leadership.minionTicket.ch <- struct{}{}
	assertNoNotifyEvent(c, w.RemoteStateChanged(), "remote state change")
}

func (s *WatcherSuite) TestLeadershipLeaderUnchanged(c *gc.C) {
	w, err := remotestate.NewWatcher(&s.st, &s.leadership, s.st.unit.tag)
	c.Assert(err, jc.ErrorIsNil)
	defer func() { c.Assert(w.Stop(), jc.ErrorIsNil) }()

	signalAll(&s.st, &s.leadership)
	assertNotifyEvent(c, w.RemoteStateChanged(), "waiting for remote state change")

	// Initially leader, so triggering leader should have no effect.
	s.leadership.leaderTicket.ch <- struct{}{}
	assertNoNotifyEvent(c, w.RemoteStateChanged(), "remote state change")
}

func (s *WatcherSuite) TestStorageChanged(c *gc.C) {
	w, err := remotestate.NewWatcher(&s.st, &s.leadership, s.st.unit.tag)
	c.Assert(err, jc.ErrorIsNil)
	defer func() { c.Assert(w.Stop(), jc.ErrorIsNil) }()

	// These watchers are not used in this test but need to be there
	// when the storage changes are processed.
	storageTag0 := names.NewStorageTag("blob/0")
	s.st.storageAttachmentWatchers[storageTag0] = &mockStorageAttachmentWatcher{
		changes: make(chan struct{}, 1),
	}
	storageTag1 := names.NewStorageTag("blob/1")
	s.st.storageAttachmentWatchers[storageTag1] = &mockStorageAttachmentWatcher{
		changes: make(chan struct{}, 1),
	}
	signalAll(&s.st, &s.leadership)
	assertNotifyEvent(c, w.RemoteStateChanged(), "waiting for remote state change")

	storageAttachmentId0 := params.StorageAttachmentId{
		UnitTag:    s.st.unit.tag.String(),
		StorageTag: storageTag0.String(),
	}
	storageAttachmentId1 := params.StorageAttachmentId{
		UnitTag:    s.st.unit.tag.String(),
		StorageTag: storageTag1.String(),
	}
	s.st.storageAttachment[storageAttachmentId0] = params.StorageAttachment{
		UnitTag:    storageAttachmentId0.UnitTag,
		StorageTag: storageAttachmentId0.StorageTag,
		Life:       params.Alive,
	}
	s.st.storageAttachment[storageAttachmentId1] = params.StorageAttachment{
		UnitTag:    storageAttachmentId1.UnitTag,
		StorageTag: storageAttachmentId1.StorageTag,
		Life:       params.Dying,
	}
	s.st.unit.storageWatcher.changes <- []string{"blob/0", "blob/1"}
	assertNotifyEvent(c, w.RemoteStateChanged(), "waiting for remote state change")

	c.Assert(w.Snapshot().Storage, jc.DeepEquals, map[names.StorageTag]remotestate.StorageSnapshot{
		storageTag0: remotestate.StorageSnapshot{
			Tag:  storageTag0,
			Life: params.Alive,
		},
		storageTag1: remotestate.StorageSnapshot{
			Tag:  storageTag1,
			Life: params.Dying,
		},
	})

	s.st.storageAttachment[storageAttachmentId0] = params.StorageAttachment{
		UnitTag:    storageAttachmentId0.UnitTag,
		StorageTag: storageAttachmentId0.StorageTag,
		Life:       params.Dying,
	}
	delete(s.st.storageAttachment, storageAttachmentId1)
	s.st.unit.storageWatcher.changes <- []string{"blob/0", "blob/1"}
	assertNotifyEvent(c, w.RemoteStateChanged(), "waiting for remote state change")
	c.Assert(w.Snapshot().Storage, jc.DeepEquals, map[names.StorageTag]remotestate.StorageSnapshot{
		storageTag0: remotestate.StorageSnapshot{
			Tag:  storageTag0,
			Life: params.Dying,
		},
	})
}

func (s *WatcherSuite) TestStorageChangedNotFoundInitially(c *gc.C) {
	w, err := remotestate.NewWatcher(&s.st, &s.leadership, s.st.unit.tag)
	c.Assert(err, jc.ErrorIsNil)
	defer func() { c.Assert(w.Stop(), jc.ErrorIsNil) }()
	signalAll(&s.st, &s.leadership)
	assertNotifyEvent(c, w.RemoteStateChanged(), "waiting for remote state change")

	// blob/0 is initially in state, but is removed between the
	// watcher signal and the uniter querying it. This should
	// not cause the watcher to raise an error.
	s.st.unit.storageWatcher.changes <- []string{"blob/0"}
	assertNotifyEvent(c, w.RemoteStateChanged(), "waiting for remote state change")
	c.Assert(w.Snapshot().Storage, gc.HasLen, 0)
}

func (s *WatcherSuite) TestRelationsChanged(c *gc.C) {
	w, err := remotestate.NewWatcher(&s.st, &s.leadership, s.st.unit.tag)
	c.Assert(err, jc.ErrorIsNil)
	defer func() { c.Assert(w.Stop(), jc.ErrorIsNil) }()
	signalAll(&s.st, &s.leadership)
	assertNotifyEvent(c, w.RemoteStateChanged(), "waiting for remote state change")

	relationTag := names.NewRelationTag("mysql:peer")
	s.st.relations[relationTag] = &mockRelation{
		id: 123, life: params.Alive,
	}
	s.st.relationUnitsWatchers[relationTag] = &mockRelationUnitsWatcher{
		changes: make(chan multiwatcher.RelationUnitsChange, 1),
	}
	s.st.unit.service.relationsWatcher.changes <- []string{relationTag.Id()}

	// There should not be any signal until the relation units watcher has
	// returned its initial event also.
	assertNoNotifyEvent(c, w.RemoteStateChanged(), "remote state change")
	s.st.relationUnitsWatchers[relationTag].changes <- multiwatcher.RelationUnitsChange{
		Changed: map[string]multiwatcher.UnitSettings{"mysql/1": {1}, "mysql/2": {2}},
	}
	assertNotifyEvent(c, w.RemoteStateChanged(), "waiting for remote state change")
	c.Assert(
		w.Snapshot().Relations,
		jc.DeepEquals,
		map[int]remotestate.RelationSnapshot{
			123: remotestate.RelationSnapshot{
				Life:    params.Alive,
				Members: map[string]int64{"mysql/1": 1, "mysql/2": 2},
			},
		},
	)

	// If a relation is known, then updating it does not require any input
	// from the relation units watcher.
	s.st.relations[relationTag].life = params.Dying
	s.st.unit.service.relationsWatcher.changes <- []string{relationTag.Id()}
	assertNotifyEvent(c, w.RemoteStateChanged(), "waiting for remote state change")
	c.Assert(w.Snapshot().Relations[123].Life, gc.Equals, params.Dying)

	// If a relation is not found, then it should be removed from the
	// snapshot and its relation units watcher stopped.
	delete(s.st.relations, relationTag)
	s.st.unit.service.relationsWatcher.changes <- []string{relationTag.Id()}
	assertNotifyEvent(c, w.RemoteStateChanged(), "waiting for remote state change")
	c.Assert(w.Snapshot().Relations, gc.HasLen, 0)
	c.Assert(s.st.relationUnitsWatchers[relationTag].stopped, jc.IsTrue)
}

func (s *WatcherSuite) TestRelationUnitsChanged(c *gc.C) {
	w, err := remotestate.NewWatcher(&s.st, &s.leadership, s.st.unit.tag)
	c.Assert(err, jc.ErrorIsNil)
	defer func() { c.Assert(w.Stop(), jc.ErrorIsNil) }()
	signalAll(&s.st, &s.leadership)
	assertNotifyEvent(c, w.RemoteStateChanged(), "waiting for remote state change")

	relationTag := names.NewRelationTag("mysql:peer")
	s.st.relations[relationTag] = &mockRelation{
		id: 123, life: params.Alive,
	}
	s.st.relationUnitsWatchers[relationTag] = &mockRelationUnitsWatcher{
		changes: make(chan multiwatcher.RelationUnitsChange, 1),
	}

	s.st.unit.service.relationsWatcher.changes <- []string{relationTag.Id()}
	s.st.relationUnitsWatchers[relationTag].changes <- multiwatcher.RelationUnitsChange{
		Changed: map[string]multiwatcher.UnitSettings{"mysql/1": {1}},
	}
	assertNotifyEvent(c, w.RemoteStateChanged(), "waiting for remote state change")

	s.st.relationUnitsWatchers[relationTag].changes <- multiwatcher.RelationUnitsChange{
		Changed: map[string]multiwatcher.UnitSettings{"mysql/1": {2}, "mysql/2": {1}},
	}
	assertNotifyEvent(c, w.RemoteStateChanged(), "waiting for remote state change")
	c.Assert(
		w.Snapshot().Relations[123].Members,
		jc.DeepEquals,
		map[string]int64{"mysql/1": 2, "mysql/2": 1},
	)

	s.st.relationUnitsWatchers[relationTag].changes <- multiwatcher.RelationUnitsChange{
		Departed: []string{"mysql/1", "mysql/42"},
	}
	assertNotifyEvent(c, w.RemoteStateChanged(), "waiting for remote state change")
	c.Assert(
		w.Snapshot().Relations[123].Members,
		jc.DeepEquals,
		map[string]int64{"mysql/2": 1},
	)
}
