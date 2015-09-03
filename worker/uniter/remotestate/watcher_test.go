// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package remotestate_test

import (
	"time"

	"github.com/juju/names"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"
	"gopkg.in/juju/charm.v5"

	"github.com/juju/juju/apiserver/params"
	"github.com/juju/juju/state/multiwatcher"
	"github.com/juju/juju/testing"
	"github.com/juju/juju/worker/uniter/remotestate"
)

type WatcherSuite struct {
	testing.BaseSuite

	st         mockState
	leadership mockLeadershipTracker
	watcher    *remotestate.RemoteStateWatcher
	clock      *testing.Clock
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
			actionWatcher:         mockStringsWatcher{changes: make(chan []string, 1)},
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

	s.clock = testing.NewClock(time.Now())
	statusTicker := func() <-chan time.Time {
		// Duration is arbitrary, we'll trigger the ticker
		// by advancing the clock past the duration.
		return s.clock.After(10 * time.Second)
	}

	w, err := remotestate.NewWatcher(remotestate.WatcherConfig{
		State:               &s.st,
		LeadershipTracker:   &s.leadership,
		UnitTag:             s.st.unit.tag,
		UpdateStatusChannel: statusTicker,
	})
	c.Assert(err, jc.ErrorIsNil)
	s.watcher = w
}

func (s *WatcherSuite) TearDownTest(c *gc.C) {
	err := s.watcher.Stop()
	c.Assert(err, jc.ErrorIsNil)
}

func (s *WatcherSuite) TestInitialSnapshot(c *gc.C) {
	snap := s.watcher.Snapshot()
	c.Assert(snap, jc.DeepEquals, remotestate.Snapshot{
		Relations: map[int]remotestate.RelationSnapshot{},
		Storage:   map[names.StorageTag]remotestate.StorageSnapshot{},
	})
}

func (s *WatcherSuite) TestInitialSignal(c *gc.C) {
	// There should not be a remote state change until
	// we've seen all of the top-level notifications.
	s.st.unit.unitWatcher.changes <- struct{}{}
	assertNoNotifyEvent(c, s.watcher.RemoteStateChanged(), "remote state change")

	s.st.unit.addressesWatcher.changes <- struct{}{}
	s.st.unit.configSettingsWatcher.changes <- struct{}{}
	s.st.unit.storageWatcher.changes <- []string{}
	s.st.unit.actionWatcher.changes <- []string{}
	s.st.unit.service.serviceWatcher.changes <- struct{}{}
	s.st.unit.service.leaderSettingsWatcher.changes <- struct{}{}
	s.st.unit.service.relationsWatcher.changes <- []string{}
	s.leadership.claimTicket.ch <- struct{}{}
	assertNotifyEvent(c, s.watcher.RemoteStateChanged(), "waiting for remote state change")
}

func signalAll(st *mockState, l *mockLeadershipTracker) {
	st.unit.unitWatcher.changes <- struct{}{}
	st.unit.addressesWatcher.changes <- struct{}{}
	st.unit.configSettingsWatcher.changes <- struct{}{}
	st.unit.storageWatcher.changes <- []string{}
	st.unit.actionWatcher.changes <- []string{}
	st.unit.service.serviceWatcher.changes <- struct{}{}
	st.unit.service.leaderSettingsWatcher.changes <- struct{}{}
	st.unit.service.relationsWatcher.changes <- []string{}
	l.claimTicket.ch <- struct{}{}
}

func (s *WatcherSuite) TestSnapshot(c *gc.C) {
	signalAll(&s.st, &s.leadership)
	assertNotifyEvent(c, s.watcher.RemoteStateChanged(), "waiting for remote state change")

	snap := s.watcher.Snapshot()
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
	assertOneChange := func() {
		assertNotifyEvent(c, s.watcher.RemoteStateChanged(), "waiting for remote state change")
		assertNoNotifyEvent(c, s.watcher.RemoteStateChanged(), "remote state change")
	}

	signalAll(&s.st, &s.leadership)
	assertOneChange()
	initial := s.watcher.Snapshot()

	s.st.unit.life = params.Dying
	s.st.unit.unitWatcher.changes <- struct{}{}
	assertOneChange()
	c.Assert(s.watcher.Snapshot().Life, gc.Equals, params.Dying)

	s.st.unit.addressesWatcher.changes <- struct{}{}
	assertOneChange()
	c.Assert(s.watcher.Snapshot().ConfigVersion, gc.Equals, initial.ConfigVersion+1)

	s.st.unit.configSettingsWatcher.changes <- struct{}{}
	assertOneChange()
	c.Assert(s.watcher.Snapshot().ConfigVersion, gc.Equals, initial.ConfigVersion+2)

	s.st.unit.storageWatcher.changes <- []string{}
	assertOneChange()

	s.st.unit.service.forceUpgrade = true
	s.st.unit.service.serviceWatcher.changes <- struct{}{}
	assertOneChange()
	c.Assert(s.watcher.Snapshot().ForceCharmUpgrade, jc.IsTrue)

	s.st.unit.service.leaderSettingsWatcher.changes <- struct{}{}
	assertOneChange()
	c.Assert(s.watcher.Snapshot().LeaderSettingsVersion, gc.Equals, initial.LeaderSettingsVersion+1)

	s.st.unit.service.relationsWatcher.changes <- []string{}
	assertOneChange()

	s.clock.Advance(15 * time.Second)
	assertOneChange()
}

func (s *WatcherSuite) TestActionsReceived(c *gc.C) {
	statusTicker := func() <-chan time.Time {
		// Duration is arbitrary, we'll trigger the ticker
		// by advancing the clock past the duration.
		return s.clock.After(10 * time.Second)
	}
	config := remotestate.WatcherConfig{
		State:               &s.st,
		LeadershipTracker:   &s.leadership,
		UnitTag:             s.st.unit.tag,
		UpdateStatusChannel: statusTicker,
	}
	w, err := remotestate.NewWatcher(config)
	c.Assert(err, jc.ErrorIsNil)
	defer func() { c.Assert(w.Stop(), jc.ErrorIsNil) }()
	signalAll(&s.st, &s.leadership)
	assertNotifyEvent(c, w.RemoteStateChanged(), "waiting for remote state change")
	s.st.unit.actionWatcher.changes <- []string{"an-action"}
	assertNotifyEvent(c, w.RemoteStateChanged(), "waiting for remote state change")
	c.Assert(w.Snapshot().Actions, gc.DeepEquals, []string{"an-action"})
}

func (s *WatcherSuite) TestClearResolvedMode(c *gc.C) {
	s.st.unit.resolved = params.ResolvedRetryHooks
	signalAll(&s.st, &s.leadership)
	assertNotifyEvent(c, s.watcher.RemoteStateChanged(), "waiting for remote state change")

	snap := s.watcher.Snapshot()
	c.Assert(snap.ResolvedMode, gc.Equals, params.ResolvedRetryHooks)

	s.watcher.ClearResolvedMode()
	snap = s.watcher.Snapshot()
	c.Assert(snap.ResolvedMode, gc.Equals, params.ResolvedNone)
}

func (s *WatcherSuite) TestLeadershipChanged(c *gc.C) {
	s.leadership.claimTicket.result = false
	signalAll(&s.st, &s.leadership)
	assertNotifyEvent(c, s.watcher.RemoteStateChanged(), "waiting for remote state change")
	c.Assert(s.watcher.Snapshot().Leader, jc.IsFalse)

	s.leadership.leaderTicket.ch <- struct{}{}
	assertNotifyEvent(c, s.watcher.RemoteStateChanged(), "waiting for remote state change")
	c.Assert(s.watcher.Snapshot().Leader, jc.IsTrue)

	s.leadership.minionTicket.ch <- struct{}{}
	assertNotifyEvent(c, s.watcher.RemoteStateChanged(), "waiting for remote state change")
	c.Assert(s.watcher.Snapshot().Leader, jc.IsFalse)
}

func (s *WatcherSuite) TestLeadershipMinionUnchanged(c *gc.C) {
	s.leadership.claimTicket.result = false
	signalAll(&s.st, &s.leadership)
	assertNotifyEvent(c, s.watcher.RemoteStateChanged(), "waiting for remote state change")

	// Initially minion, so triggering minion should have no effect.
	s.leadership.minionTicket.ch <- struct{}{}
	assertNoNotifyEvent(c, s.watcher.RemoteStateChanged(), "remote state change")
}

func (s *WatcherSuite) TestLeadershipLeaderUnchanged(c *gc.C) {
	signalAll(&s.st, &s.leadership)
	assertNotifyEvent(c, s.watcher.RemoteStateChanged(), "waiting for remote state change")

	// Initially leader, so triggering leader should have no effect.
	s.leadership.leaderTicket.ch <- struct{}{}
	assertNoNotifyEvent(c, s.watcher.RemoteStateChanged(), "remote state change")
}

func (s *WatcherSuite) TestStorageChanged(c *gc.C) {
	signalAll(&s.st, &s.leadership)
	assertNotifyEvent(c, s.watcher.RemoteStateChanged(), "waiting for remote state change")

	storageTag0 := names.NewStorageTag("blob/0")
	storageAttachmentId0 := params.StorageAttachmentId{
		UnitTag:    s.st.unit.tag.String(),
		StorageTag: storageTag0.String(),
	}
	storageTag0Watcher := &mockStorageAttachmentWatcher{
		changes: make(chan struct{}, 1),
	}
	s.st.storageAttachmentWatchers[storageTag0] = storageTag0Watcher
	s.st.storageAttachment[storageAttachmentId0] = params.StorageAttachment{
		UnitTag:    storageAttachmentId0.UnitTag,
		StorageTag: storageAttachmentId0.StorageTag,
		Life:       params.Alive,
		Kind:       params.StorageKindUnknown, // unprovisioned
		Location:   "nowhere",
	}

	storageTag1 := names.NewStorageTag("blob/1")
	storageAttachmentId1 := params.StorageAttachmentId{
		UnitTag:    s.st.unit.tag.String(),
		StorageTag: storageTag1.String(),
	}
	storageTag1Watcher := &mockStorageAttachmentWatcher{
		changes: make(chan struct{}, 1),
	}
	s.st.storageAttachmentWatchers[storageTag1] = storageTag1Watcher
	s.st.storageAttachment[storageAttachmentId1] = params.StorageAttachment{
		UnitTag:    storageAttachmentId1.UnitTag,
		StorageTag: storageAttachmentId1.StorageTag,
		Life:       params.Dying,
		Kind:       params.StorageKindBlock,
		Location:   "malta",
	}

	// We should not see any event until the storage attachment watchers
	// return their initial events.
	s.st.unit.storageWatcher.changes <- []string{"blob/0", "blob/1"}
	assertNoNotifyEvent(c, s.watcher.RemoteStateChanged(), "remote state change")
	storageTag0Watcher.changes <- struct{}{}
	storageTag1Watcher.changes <- struct{}{}
	assertNotifyEvent(c, s.watcher.RemoteStateChanged(), "waiting for remote state change")

	c.Assert(s.watcher.Snapshot().Storage, jc.DeepEquals, map[names.StorageTag]remotestate.StorageSnapshot{
		storageTag0: remotestate.StorageSnapshot{
			Life: params.Alive,
		},
		storageTag1: remotestate.StorageSnapshot{
			Life:     params.Dying,
			Kind:     params.StorageKindBlock,
			Attached: true,
			Location: "malta",
		},
	})

	s.st.storageAttachment[storageAttachmentId0] = params.StorageAttachment{
		UnitTag:    storageAttachmentId0.UnitTag,
		StorageTag: storageAttachmentId0.StorageTag,
		Life:       params.Dying,
		Kind:       params.StorageKindFilesystem,
		Location:   "somewhere",
	}
	delete(s.st.storageAttachment, storageAttachmentId1)
	storageTag0Watcher.changes <- struct{}{}
	assertNotifyEvent(c, s.watcher.RemoteStateChanged(), "waiting for remote state change")
	s.st.unit.storageWatcher.changes <- []string{"blob/1"}
	assertNotifyEvent(c, s.watcher.RemoteStateChanged(), "waiting for remote state change")
	c.Assert(s.watcher.Snapshot().Storage, jc.DeepEquals, map[names.StorageTag]remotestate.StorageSnapshot{
		storageTag0: remotestate.StorageSnapshot{
			Life:     params.Dying,
			Attached: true,
			Kind:     params.StorageKindFilesystem,
			Location: "somewhere",
		},
	})
}

func (s *WatcherSuite) TestStorageUnattachedChanged(c *gc.C) {
	signalAll(&s.st, &s.leadership)
	assertNotifyEvent(c, s.watcher.RemoteStateChanged(), "waiting for remote state change")

	storageTag0 := names.NewStorageTag("blob/0")
	storageAttachmentId0 := params.StorageAttachmentId{
		UnitTag:    s.st.unit.tag.String(),
		StorageTag: storageTag0.String(),
	}
	storageTag0Watcher := &mockStorageAttachmentWatcher{
		changes: make(chan struct{}, 1),
	}
	s.st.storageAttachmentWatchers[storageTag0] = storageTag0Watcher
	s.st.storageAttachment[storageAttachmentId0] = params.StorageAttachment{
		UnitTag:    storageAttachmentId0.UnitTag,
		StorageTag: storageAttachmentId0.StorageTag,
		Life:       params.Alive,
		Kind:       params.StorageKindUnknown, // unprovisioned
	}

	s.st.unit.storageWatcher.changes <- []string{"blob/0"}
	storageTag0Watcher.changes <- struct{}{}
	assertNotifyEvent(c, s.watcher.RemoteStateChanged(), "waiting for remote state change")

	c.Assert(s.watcher.Snapshot().Storage, jc.DeepEquals, map[names.StorageTag]remotestate.StorageSnapshot{
		storageTag0: remotestate.StorageSnapshot{
			Life: params.Alive,
		},
	})

	s.st.storageAttachment[storageAttachmentId0] = params.StorageAttachment{
		UnitTag:    storageAttachmentId0.UnitTag,
		StorageTag: storageAttachmentId0.StorageTag,
		Life:       params.Dying,
	}
	// The storage is still unattached; triggering the storage-specific
	// watcher should not cause any event to be emitted.
	storageTag0Watcher.changes <- struct{}{}
	assertNoNotifyEvent(c, s.watcher.RemoteStateChanged(), "remote state change")
	s.st.unit.storageWatcher.changes <- []string{"blob/0"}
	assertNotifyEvent(c, s.watcher.RemoteStateChanged(), "waiting for remote state change")
	c.Assert(s.watcher.Snapshot().Storage, jc.DeepEquals, map[names.StorageTag]remotestate.StorageSnapshot{
		storageTag0: remotestate.StorageSnapshot{
			Life: params.Dying,
		},
	})

	// Removing the storage attachment and then triggering the storage-
	// specific watcher should not cause an event to be emitted, but it
	// will cause that watcher to stop running. Triggering the top-level
	// storage watcher will remove it and update the snapshot.
	delete(s.st.storageAttachment, storageAttachmentId0)
	storageTag0Watcher.changes <- struct{}{}
	assertNoNotifyEvent(c, s.watcher.RemoteStateChanged(), "remote state change")
	c.Assert(storageTag0Watcher.stopped, jc.IsTrue)
	s.st.unit.storageWatcher.changes <- []string{"blob/0"}
	assertNotifyEvent(c, s.watcher.RemoteStateChanged(), "waiting for remote state change")
	c.Assert(s.watcher.Snapshot().Storage, gc.HasLen, 0)
}

func (s *WatcherSuite) TestStorageChangedNotFoundInitially(c *gc.C) {
	signalAll(&s.st, &s.leadership)
	assertNotifyEvent(c, s.watcher.RemoteStateChanged(), "waiting for remote state change")

	// blob/0 is initially in state, but is removed between the
	// watcher signal and the uniter querying it. This should
	// not cause the watcher to raise an error.
	s.st.unit.storageWatcher.changes <- []string{"blob/0"}
	assertNotifyEvent(c, s.watcher.RemoteStateChanged(), "waiting for remote state change")
	c.Assert(s.watcher.Snapshot().Storage, gc.HasLen, 0)
}

func (s *WatcherSuite) TestRelationsChanged(c *gc.C) {
	signalAll(&s.st, &s.leadership)
	assertNotifyEvent(c, s.watcher.RemoteStateChanged(), "waiting for remote state change")

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
	assertNoNotifyEvent(c, s.watcher.RemoteStateChanged(), "remote state change")
	s.st.relationUnitsWatchers[relationTag].changes <- multiwatcher.RelationUnitsChange{
		Changed: map[string]multiwatcher.UnitSettings{"mysql/1": {1}, "mysql/2": {2}},
	}
	assertNotifyEvent(c, s.watcher.RemoteStateChanged(), "waiting for remote state change")
	c.Assert(
		s.watcher.Snapshot().Relations,
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
	assertNotifyEvent(c, s.watcher.RemoteStateChanged(), "waiting for remote state change")
	c.Assert(s.watcher.Snapshot().Relations[123].Life, gc.Equals, params.Dying)

	// If a relation is not found, then it should be removed from the
	// snapshot and its relation units watcher stopped.
	delete(s.st.relations, relationTag)
	s.st.unit.service.relationsWatcher.changes <- []string{relationTag.Id()}
	assertNotifyEvent(c, s.watcher.RemoteStateChanged(), "waiting for remote state change")
	c.Assert(s.watcher.Snapshot().Relations, gc.HasLen, 0)
	c.Assert(s.st.relationUnitsWatchers[relationTag].stopped, jc.IsTrue)
}

func (s *WatcherSuite) TestRelationUnitsChanged(c *gc.C) {
	signalAll(&s.st, &s.leadership)
	assertNotifyEvent(c, s.watcher.RemoteStateChanged(), "waiting for remote state change")

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
	assertNotifyEvent(c, s.watcher.RemoteStateChanged(), "waiting for remote state change")

	s.st.relationUnitsWatchers[relationTag].changes <- multiwatcher.RelationUnitsChange{
		Changed: map[string]multiwatcher.UnitSettings{"mysql/1": {2}, "mysql/2": {1}},
	}
	assertNotifyEvent(c, s.watcher.RemoteStateChanged(), "waiting for remote state change")
	c.Assert(
		s.watcher.Snapshot().Relations[123].Members,
		jc.DeepEquals,
		map[string]int64{"mysql/1": 2, "mysql/2": 1},
	)

	s.st.relationUnitsWatchers[relationTag].changes <- multiwatcher.RelationUnitsChange{
		Departed: []string{"mysql/1", "mysql/42"},
	}
	assertNotifyEvent(c, s.watcher.RemoteStateChanged(), "waiting for remote state change")
	c.Assert(
		s.watcher.Snapshot().Relations[123].Members,
		jc.DeepEquals,
		map[string]int64{"mysql/2": 1},
	)
}

func (s *WatcherSuite) TestUpdateStatusTicker(c *gc.C) {
	signalAll(&s.st, &s.leadership)
	assertNotifyEvent(c, s.watcher.RemoteStateChanged(), "waiting for remote state change")
	c.Assert(s.watcher.Snapshot().UpdateStatusRequired, jc.IsFalse)

	// Advance the clock past the triiger time.
	s.clock.Advance(11 * time.Second)
	assertNotifyEvent(c, s.watcher.RemoteStateChanged(), "waiting for remote state change")
	c.Assert(s.watcher.Snapshot().UpdateStatusRequired, jc.IsTrue)
	// Flag is reset after snapshot is first read.
	c.Assert(s.watcher.Snapshot().UpdateStatusRequired, jc.IsFalse)

	// Advance again but not past the trigger time.
	s.clock.Advance(6 * time.Second)
	assertNoNotifyEvent(c, s.watcher.RemoteStateChanged(), "unexpected remote state change")
	c.Assert(s.watcher.Snapshot().UpdateStatusRequired, jc.IsFalse)

	// And we ht the trigger time.
	s.clock.Advance(5 * time.Second)
	assertNotifyEvent(c, s.watcher.RemoteStateChanged(), "waiting for remote state change")
	c.Assert(s.watcher.Snapshot().UpdateStatusRequired, jc.IsTrue)
	c.Assert(s.watcher.Snapshot().UpdateStatusRequired, jc.IsFalse)
}
