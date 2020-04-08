// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package remotestate_test

import (
	"time"

	"github.com/juju/clock/testclock"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"
	"gopkg.in/juju/charm.v6"
	"gopkg.in/juju/names.v3"

	"github.com/juju/juju/apiserver/params"
	"github.com/juju/juju/core/life"
	"github.com/juju/juju/core/model"
	"github.com/juju/juju/core/watcher"
	coretesting "github.com/juju/juju/testing"
	"github.com/juju/juju/worker/uniter/remotestate"
)

type WatcherSuite struct {
	coretesting.BaseSuite

	modelType  model.ModelType
	st         *mockState
	leadership *mockLeadershipTracker
	watcher    *remotestate.RemoteStateWatcher
	clock      *testclock.Clock

	applicationWatcher   *mockNotifyWatcher
	runningStatusWatcher *mockNotifyWatcher
	running              *remotestate.ContainerRunningStatus
}

type WatcherSuiteIAAS struct {
	WatcherSuite
}

type WatcherSuiteCAAS struct {
	WatcherSuite
}

var _ = gc.Suite(&WatcherSuiteIAAS{WatcherSuite{modelType: model.IAAS}})
var _ = gc.Suite(&WatcherSuiteCAAS{WatcherSuite{modelType: model.CAAS}})

func (s *WatcherSuite) SetUpTest(c *gc.C) {
	s.BaseSuite.SetUpTest(c)
	s.st = &mockState{
		modelType: s.modelType,
		unit: mockUnit{
			tag:  names.NewUnitTag("mysql/0"),
			life: life.Alive,
			application: mockApplication{
				tag:                   names.NewApplicationTag("mysql"),
				life:                  life.Alive,
				curl:                  charm.MustParseURL("cs:trusty/mysql"),
				charmModifiedVersion:  5,
				leaderSettingsWatcher: newMockNotifyWatcher(),
			},
			unitWatcher:                      newMockNotifyWatcher(),
			addressesWatcher:                 newMockStringsWatcher(),
			configSettingsWatcher:            newMockStringsWatcher(),
			applicationConfigSettingsWatcher: newMockStringsWatcher(),
			storageWatcher:                   newMockStringsWatcher(),
			actionWatcher:                    newMockStringsWatcher(),
			relationsWatcher:                 newMockStringsWatcher(),
		},
		relations:                   make(map[names.RelationTag]*mockRelation),
		storageAttachment:           make(map[params.StorageAttachmentId]params.StorageAttachment),
		relationUnitsWatchers:       make(map[names.RelationTag]*mockRelationUnitsWatcher),
		relationAppWatchers:         make(map[names.RelationTag]map[string]*mockNotifyWatcher),
		storageAttachmentWatchers:   make(map[names.StorageTag]*mockNotifyWatcher),
		updateStatusInterval:        5 * time.Minute,
		updateStatusIntervalWatcher: newMockNotifyWatcher(),
	}

	s.leadership = &mockLeadershipTracker{
		claimTicket:  mockTicket{make(chan struct{}, 1), true},
		leaderTicket: mockTicket{make(chan struct{}, 1), true},
		minionTicket: mockTicket{make(chan struct{}, 1), true},
	}

	s.clock = testclock.NewClock(time.Now())
}

func (s *WatcherSuiteIAAS) SetUpTest(c *gc.C) {
	s.WatcherSuite.SetUpTest(c)
	statusTicker := func(wait time.Duration) remotestate.Waiter {
		return dummyWaiter{s.clock.After(wait)}
	}

	s.st.unit.application.applicationWatcher = newMockNotifyWatcher()
	s.applicationWatcher = s.st.unit.application.applicationWatcher
	s.st.unit.upgradeSeriesWatcher = newMockNotifyWatcher()
	w, err := remotestate.NewWatcher(remotestate.WatcherConfig{
		State:               s.st,
		ModelType:           s.modelType,
		LeadershipTracker:   s.leadership,
		UnitTag:             s.st.unit.tag,
		UpdateStatusChannel: statusTicker,
	})
	c.Assert(err, jc.ErrorIsNil)
	s.watcher = w
}

func (s *WatcherSuiteCAAS) SetUpTest(c *gc.C) {
	s.WatcherSuite.SetUpTest(c)
	statusTicker := func(wait time.Duration) remotestate.Waiter {
		return dummyWaiter{s.clock.After(wait)}
	}

	s.applicationWatcher = newMockNotifyWatcher()
	s.runningStatusWatcher = newMockNotifyWatcher()
	w, err := remotestate.NewWatcher(remotestate.WatcherConfig{
		State:                         s.st,
		ModelType:                     s.modelType,
		LeadershipTracker:             s.leadership,
		UnitTag:                       s.st.unit.tag,
		UpdateStatusChannel:           statusTicker,
		ApplicationChannel:            s.applicationWatcher.Changes(),
		ContainerRunningStatusChannel: s.runningStatusWatcher.Changes(),
		ContainerRunningStatusFunc:    func(providerID string) (*remotestate.ContainerRunningStatus, error) { return s.running, nil },
	})
	c.Assert(err, jc.ErrorIsNil)
	s.watcher = w
}

type dummyWaiter struct {
	c <-chan time.Time
}

func (w dummyWaiter) After() <-chan time.Time {
	return w.c
}

func (s *WatcherSuite) TearDownTest(c *gc.C) {
	if s.watcher != nil {
		s.watcher.Kill()
		err := s.watcher.Wait()
		c.Assert(err, jc.ErrorIsNil)
	}
}

func (s *WatcherSuiteIAAS) TestInitialSnapshot(c *gc.C) {
	snap := s.watcher.Snapshot()
	c.Assert(snap, jc.DeepEquals, remotestate.Snapshot{
		Relations:     map[int]remotestate.RelationSnapshot{},
		Storage:       map[names.StorageTag]remotestate.StorageSnapshot{},
		ActionChanged: map[string]int{},
	})
}

func (s *WatcherSuiteCAAS) TestInitialSnapshot(c *gc.C) {
	snap := s.watcher.Snapshot()
	c.Assert(snap, jc.DeepEquals, remotestate.Snapshot{
		Relations:      map[int]remotestate.RelationSnapshot{},
		Storage:        map[names.StorageTag]remotestate.StorageSnapshot{},
		ActionChanged:  map[string]int{},
		ActionsBlocked: true,
	})
}

func (s *WatcherSuite) TestInitialSignal(c *gc.C) {
	// There should not be a remote state change until
	// we've seen all of the top-level notifications.
	s.st.unit.unitWatcher.changes <- struct{}{}
	assertNoNotifyEvent(c, s.watcher.RemoteStateChanged(), "remote state change")
	s.st.unit.addressesWatcher.changes <- []string{"addresseshash"}
	s.st.unit.configSettingsWatcher.changes <- []string{"confighash"}
	s.st.unit.applicationConfigSettingsWatcher.changes <- []string{"trusthash"}
	if s.st.unit.upgradeSeriesWatcher != nil {
		s.st.unit.upgradeSeriesWatcher.changes <- struct{}{}
	}
	s.st.unit.storageWatcher.changes <- []string{}
	s.st.unit.actionWatcher.changes <- []string{}
	if s.st.unit.application.applicationWatcher != nil {
		s.st.unit.application.applicationWatcher.changes <- struct{}{}
	}
	s.st.unit.application.leaderSettingsWatcher.changes <- struct{}{}
	s.st.unit.relationsWatcher.changes <- []string{}
	s.st.updateStatusIntervalWatcher.changes <- struct{}{}
	s.leadership.claimTicket.ch <- struct{}{}
	assertNotifyEvent(c, s.watcher.RemoteStateChanged(), "waiting for remote state change")
}

func (s *WatcherSuite) signalAll() {
	s.st.unit.unitWatcher.changes <- struct{}{}
	s.st.unit.configSettingsWatcher.changes <- []string{"confighash"}
	s.st.unit.applicationConfigSettingsWatcher.changes <- []string{"trusthash"}
	s.st.unit.actionWatcher.changes <- []string{}
	s.st.unit.application.leaderSettingsWatcher.changes <- struct{}{}
	s.st.unit.relationsWatcher.changes <- []string{}
	s.st.unit.addressesWatcher.changes <- []string{"addresseshash"}
	s.st.updateStatusIntervalWatcher.changes <- struct{}{}
	s.leadership.claimTicket.ch <- struct{}{}
	s.st.unit.storageWatcher.changes <- []string{}
	if s.st.modelType == model.IAAS {
		s.applicationWatcher.changes <- struct{}{}
		s.st.unit.upgradeSeriesWatcher.changes <- struct{}{}
	}
}

func (s *WatcherSuiteIAAS) TestSnapshot(c *gc.C) {
	s.signalAll()
	assertNotifyEvent(c, s.watcher.RemoteStateChanged(), "waiting for remote state change")

	snap := s.watcher.Snapshot()
	c.Assert(snap, jc.DeepEquals, remotestate.Snapshot{
		Life:                  s.st.unit.life,
		Relations:             map[int]remotestate.RelationSnapshot{},
		Storage:               map[names.StorageTag]remotestate.StorageSnapshot{},
		ActionChanged:         map[string]int{},
		CharmModifiedVersion:  s.st.unit.application.charmModifiedVersion,
		CharmURL:              s.st.unit.application.curl,
		ForceCharmUpgrade:     s.st.unit.application.forceUpgrade,
		ResolvedMode:          s.st.unit.resolved,
		ConfigHash:            "confighash",
		TrustHash:             "trusthash",
		AddressesHash:         "addresseshash",
		LeaderSettingsVersion: 1,
		Leader:                true,
		UpgradeSeriesStatus:   model.UpgradeSeriesPrepareStarted,
	})
}

func (s *WatcherSuiteCAAS) TestSnapshot(c *gc.C) {
	s.signalAll()
	assertNotifyEvent(c, s.watcher.RemoteStateChanged(), "waiting for remote state change")

	snap := s.watcher.Snapshot()
	c.Assert(snap, jc.DeepEquals, remotestate.Snapshot{
		Life:                   s.st.unit.life,
		Relations:              map[int]remotestate.RelationSnapshot{},
		Storage:                map[names.StorageTag]remotestate.StorageSnapshot{},
		CharmModifiedVersion:   0,
		CharmURL:               nil,
		ForceCharmUpgrade:      s.st.unit.application.forceUpgrade,
		ResolvedMode:           s.st.unit.resolved,
		ConfigHash:             "confighash",
		TrustHash:              "trusthash",
		AddressesHash:          "addresseshash",
		LeaderSettingsVersion:  1,
		Leader:                 true,
		UpgradeSeriesStatus:    "",
		ActionsBlocked:         true,
		ActionChanged:          map[string]int{},
		ContainerRunningStatus: nil,
	})

	t := time.Now()
	s.st.unit.providerID = "provider-id"
	s.running = &remotestate.ContainerRunningStatus{
		Initialising:     true,
		InitialisingTime: t,
		PodName:          "wow",
		Running:          false,
	}
	select {
	case s.runningStatusWatcher.changes <- struct{}{}:
	case <-time.After(coretesting.LongWait):
		c.Fatal("timeout waiting to post running status change")
	}
	assertNotifyEvent(c, s.watcher.RemoteStateChanged(), "waiting for remote state change")

	snap = s.watcher.Snapshot()
	c.Assert(snap, jc.DeepEquals, remotestate.Snapshot{
		Life:                   s.st.unit.life,
		Relations:              map[int]remotestate.RelationSnapshot{},
		Storage:                map[names.StorageTag]remotestate.StorageSnapshot{},
		CharmModifiedVersion:   0,
		CharmURL:               nil,
		ForceCharmUpgrade:      s.st.unit.application.forceUpgrade,
		ResolvedMode:           s.st.unit.resolved,
		ConfigHash:             "confighash",
		TrustHash:              "trusthash",
		AddressesHash:          "addresseshash",
		LeaderSettingsVersion:  1,
		Leader:                 true,
		UpgradeSeriesStatus:    "",
		ActionsBlocked:         true,
		ActionChanged:          map[string]int{},
		ProviderID:             s.st.unit.providerID,
		ContainerRunningStatus: s.running,
	})

	s.running = &remotestate.ContainerRunningStatus{
		Initialising:     false,
		InitialisingTime: t,
		PodName:          "wow",
		Running:          true,
	}
	select {
	case s.runningStatusWatcher.changes <- struct{}{}:
	case <-time.After(coretesting.LongWait):
		c.Fatal("timeout waiting to post running status change")
	}
	assertNotifyEvent(c, s.watcher.RemoteStateChanged(), "waiting for remote state change")

	snap = s.watcher.Snapshot()
	c.Assert(snap, jc.DeepEquals, remotestate.Snapshot{
		Life:                   s.st.unit.life,
		Relations:              map[int]remotestate.RelationSnapshot{},
		Storage:                map[names.StorageTag]remotestate.StorageSnapshot{},
		CharmModifiedVersion:   0,
		CharmURL:               nil,
		ForceCharmUpgrade:      s.st.unit.application.forceUpgrade,
		ResolvedMode:           s.st.unit.resolved,
		ConfigHash:             "confighash",
		TrustHash:              "trusthash",
		AddressesHash:          "addresseshash",
		LeaderSettingsVersion:  1,
		Leader:                 true,
		UpgradeSeriesStatus:    "",
		ActionsBlocked:         false,
		ActionChanged:          map[string]int{},
		ProviderID:             s.st.unit.providerID,
		ContainerRunningStatus: s.running,
	})
}

func (s *WatcherSuite) TestRemoteStateChanged(c *gc.C) {
	assertOneChange := func() {
		assertNotifyEvent(c, s.watcher.RemoteStateChanged(), "waiting for remote state change")
		assertNoNotifyEvent(c, s.watcher.RemoteStateChanged(), "remote state change")
	}

	s.signalAll()
	assertOneChange()
	initial := s.watcher.Snapshot()

	s.st.unit.life = life.Dying
	s.st.unit.unitWatcher.changes <- struct{}{}
	assertOneChange()
	c.Assert(s.watcher.Snapshot().Life, gc.Equals, life.Dying)

	s.st.unit.resolved = params.ResolvedRetryHooks
	s.st.unit.unitWatcher.changes <- struct{}{}
	assertOneChange()
	c.Assert(s.watcher.Snapshot().ResolvedMode, gc.Equals, params.ResolvedRetryHooks)

	s.st.unit.addressesWatcher.changes <- []string{"addresseshash2"}
	assertOneChange()
	c.Assert(s.watcher.Snapshot().AddressesHash, gc.Equals, "addresseshash2")

	s.st.unit.storageWatcher.changes <- []string{}
	assertOneChange()

	s.st.unit.configSettingsWatcher.changes <- []string{"confighash2"}
	assertOneChange()
	c.Assert(s.watcher.Snapshot().ConfigHash, gc.Equals, "confighash2")

	s.st.unit.applicationConfigSettingsWatcher.changes <- []string{"trusthash2"}
	assertOneChange()
	c.Assert(s.watcher.Snapshot().TrustHash, gc.Equals, "trusthash2")

	s.st.unit.application.leaderSettingsWatcher.changes <- struct{}{}
	assertOneChange()
	c.Assert(s.watcher.Snapshot().LeaderSettingsVersion, gc.Equals, initial.LeaderSettingsVersion+1)

	s.st.unit.relationsWatcher.changes <- []string{}
	assertOneChange()

	if s.modelType == model.IAAS {
		s.st.unit.upgradeSeriesWatcher.changes <- struct{}{}
		assertOneChange()
	}
	s.st.unit.application.forceUpgrade = true
	s.applicationWatcher.changes <- struct{}{}
	assertOneChange()
	c.Assert(s.watcher.Snapshot().ForceCharmUpgrade, jc.IsTrue)

	s.clock.Advance(5 * time.Minute)
	assertOneChange()
}

func (s *WatcherSuite) TestActionsReceived(c *gc.C) {
	s.signalAll()
	assertNotifyEvent(c, s.watcher.RemoteStateChanged(), "waiting for remote state change")

	s.st.unit.actionWatcher.changes <- []string{"an-action"}
	assertNotifyEvent(c, s.watcher.RemoteStateChanged(), "waiting for remote state change")
	snapshot := s.watcher.Snapshot()
	c.Assert(snapshot.ActionsPending, gc.DeepEquals, []string{"an-action"})
	c.Assert(snapshot.ActionChanged["an-action"], gc.NotNil)
}

func (s *WatcherSuite) TestActionsReceivedWithChanges(c *gc.C) {
	s.signalAll()
	assertNotifyEvent(c, s.watcher.RemoteStateChanged(), "waiting for remote state change")

	s.st.unit.actionWatcher.changes <- []string{"an-action"}
	assertNotifyEvent(c, s.watcher.RemoteStateChanged(), "waiting for remote state change")
	snapshot := s.watcher.Snapshot()
	c.Assert(snapshot.ActionsPending, gc.DeepEquals, []string{"an-action"})
	c.Assert(snapshot.ActionChanged["an-action"], gc.Equals, 0)

	s.st.unit.actionWatcher.changes <- []string{"an-action"}
	assertNotifyEvent(c, s.watcher.RemoteStateChanged(), "waiting for remote state change")
	snapshot = s.watcher.Snapshot()
	c.Assert(snapshot.ActionsPending, gc.DeepEquals, []string{"an-action"})
	c.Assert(snapshot.ActionChanged["an-action"], gc.Equals, 1)
}

func (s *WatcherSuite) TestClearResolvedMode(c *gc.C) {
	s.st.unit.resolved = params.ResolvedRetryHooks
	s.signalAll()
	assertNotifyEvent(c, s.watcher.RemoteStateChanged(), "waiting for remote state change")

	snap := s.watcher.Snapshot()
	c.Assert(snap.ResolvedMode, gc.Equals, params.ResolvedRetryHooks)

	s.watcher.ClearResolvedMode()
	snap = s.watcher.Snapshot()
	c.Assert(snap.ResolvedMode, gc.Equals, params.ResolvedNone)
}

func (s *WatcherSuite) TestLeadershipChanged(c *gc.C) {
	s.leadership.claimTicket.result = false
	s.signalAll()
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
	s.signalAll()
	assertNotifyEvent(c, s.watcher.RemoteStateChanged(), "waiting for remote state change")

	// Initially minion, so triggering minion should have no effect.
	s.leadership.minionTicket.ch <- struct{}{}
	assertNoNotifyEvent(c, s.watcher.RemoteStateChanged(), "remote state change")
}

func (s *WatcherSuite) TestLeadershipLeaderUnchanged(c *gc.C) {
	s.signalAll()
	assertNotifyEvent(c, s.watcher.RemoteStateChanged(), "waiting for remote state change")

	// Initially leader, so triggering leader should have no effect.
	s.leadership.leaderTicket.ch <- struct{}{}
	assertNoNotifyEvent(c, s.watcher.RemoteStateChanged(), "remote state change")
}

func (s *WatcherSuite) TestStorageChanged(c *gc.C) {
	s.signalAll()
	assertNotifyEvent(c, s.watcher.RemoteStateChanged(), "waiting for remote state change")

	storageTag0 := names.NewStorageTag("blob/0")
	storageAttachmentId0 := params.StorageAttachmentId{
		UnitTag:    s.st.unit.tag.String(),
		StorageTag: storageTag0.String(),
	}
	storageTag0Watcher := newMockNotifyWatcher()
	s.st.storageAttachmentWatchers[storageTag0] = storageTag0Watcher
	s.st.storageAttachment[storageAttachmentId0] = params.StorageAttachment{
		UnitTag:    storageAttachmentId0.UnitTag,
		StorageTag: storageAttachmentId0.StorageTag,
		Life:       life.Alive,
		Kind:       params.StorageKindUnknown, // unprovisioned
		Location:   "nowhere",
	}

	storageTag1 := names.NewStorageTag("blob/1")
	storageAttachmentId1 := params.StorageAttachmentId{
		UnitTag:    s.st.unit.tag.String(),
		StorageTag: storageTag1.String(),
	}
	storageTag1Watcher := newMockNotifyWatcher()
	s.st.storageAttachmentWatchers[storageTag1] = storageTag1Watcher
	s.st.storageAttachment[storageAttachmentId1] = params.StorageAttachment{
		UnitTag:    storageAttachmentId1.UnitTag,
		StorageTag: storageAttachmentId1.StorageTag,
		Life:       life.Dying,
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
		storageTag0: {
			Life: life.Alive,
		},
		storageTag1: {
			Life:     life.Dying,
			Kind:     params.StorageKindBlock,
			Attached: true,
			Location: "malta",
		},
	})

	s.st.storageAttachment[storageAttachmentId0] = params.StorageAttachment{
		UnitTag:    storageAttachmentId0.UnitTag,
		StorageTag: storageAttachmentId0.StorageTag,
		Life:       life.Dying,
		Kind:       params.StorageKindFilesystem,
		Location:   "somewhere",
	}
	delete(s.st.storageAttachment, storageAttachmentId1)
	storageTag0Watcher.changes <- struct{}{}
	assertNotifyEvent(c, s.watcher.RemoteStateChanged(), "waiting for remote state change")
	s.st.unit.storageWatcher.changes <- []string{"blob/1"}
	assertNotifyEvent(c, s.watcher.RemoteStateChanged(), "waiting for remote state change")
	c.Assert(s.watcher.Snapshot().Storage, jc.DeepEquals, map[names.StorageTag]remotestate.StorageSnapshot{
		storageTag0: {
			Life:     life.Dying,
			Attached: true,
			Kind:     params.StorageKindFilesystem,
			Location: "somewhere",
		},
	})
}

func (s *WatcherSuite) TestStorageUnattachedChanged(c *gc.C) {
	s.signalAll()
	assertNotifyEvent(c, s.watcher.RemoteStateChanged(), "waiting for remote state change")

	storageTag0 := names.NewStorageTag("blob/0")
	storageAttachmentId0 := params.StorageAttachmentId{
		UnitTag:    s.st.unit.tag.String(),
		StorageTag: storageTag0.String(),
	}
	storageTag0Watcher := newMockNotifyWatcher()
	s.st.storageAttachmentWatchers[storageTag0] = storageTag0Watcher
	s.st.storageAttachment[storageAttachmentId0] = params.StorageAttachment{
		UnitTag:    storageAttachmentId0.UnitTag,
		StorageTag: storageAttachmentId0.StorageTag,
		Life:       life.Alive,
		Kind:       params.StorageKindUnknown, // unprovisioned
	}

	s.st.unit.storageWatcher.changes <- []string{"blob/0"}
	storageTag0Watcher.changes <- struct{}{}
	assertNotifyEvent(c, s.watcher.RemoteStateChanged(), "waiting for remote state change")

	c.Assert(s.watcher.Snapshot().Storage, jc.DeepEquals, map[names.StorageTag]remotestate.StorageSnapshot{
		storageTag0: {
			Life: life.Alive,
		},
	})

	s.st.storageAttachment[storageAttachmentId0] = params.StorageAttachment{
		UnitTag:    storageAttachmentId0.UnitTag,
		StorageTag: storageAttachmentId0.StorageTag,
		Life:       life.Dying,
	}
	// The storage is still unattached; triggering the storage-specific
	// watcher should not cause any event to be emitted.
	storageTag0Watcher.changes <- struct{}{}
	assertNoNotifyEvent(c, s.watcher.RemoteStateChanged(), "remote state change")
	s.st.unit.storageWatcher.changes <- []string{"blob/0"}
	assertNotifyEvent(c, s.watcher.RemoteStateChanged(), "waiting for remote state change")
	c.Assert(s.watcher.Snapshot().Storage, jc.DeepEquals, map[names.StorageTag]remotestate.StorageSnapshot{
		storageTag0: {
			Life: life.Dying,
		},
	})
}

func (s *WatcherSuite) TestStorageAttachmentRemoved(c *gc.C) {
	s.signalAll()
	assertNotifyEvent(c, s.watcher.RemoteStateChanged(), "waiting for remote state change")

	storageTag0 := names.NewStorageTag("blob/0")
	storageAttachmentId0 := params.StorageAttachmentId{
		UnitTag:    s.st.unit.tag.String(),
		StorageTag: storageTag0.String(),
	}
	storageTag0Watcher := newMockNotifyWatcher()
	s.st.storageAttachmentWatchers[storageTag0] = storageTag0Watcher
	s.st.storageAttachment[storageAttachmentId0] = params.StorageAttachment{
		UnitTag:    storageAttachmentId0.UnitTag,
		StorageTag: storageAttachmentId0.StorageTag,
		Life:       life.Dying,
		Kind:       params.StorageKindUnknown, // unprovisioned
	}

	s.st.unit.storageWatcher.changes <- []string{"blob/0"}
	storageTag0Watcher.changes <- struct{}{}
	assertNotifyEvent(c, s.watcher.RemoteStateChanged(), "waiting for remote state change")

	c.Assert(s.watcher.Snapshot().Storage, jc.DeepEquals, map[names.StorageTag]remotestate.StorageSnapshot{
		storageTag0: {
			Life: life.Dying,
		},
	})

	// Removing the storage attachment and then triggering the storage-
	// specific watcher should not cause an event to be emitted, but it
	// will cause that watcher to stop running. Triggering the top-level
	// storage watcher will remove it and update the snapshot.
	delete(s.st.storageAttachment, storageAttachmentId0)
	storageTag0Watcher.changes <- struct{}{}
	assertNoNotifyEvent(c, s.watcher.RemoteStateChanged(), "remote state change")
	c.Assert(storageTag0Watcher.Stopped(), jc.IsTrue)
	s.st.unit.storageWatcher.changes <- []string{"blob/0"}
	assertNotifyEvent(c, s.watcher.RemoteStateChanged(), "waiting for remote state change")
	c.Assert(s.watcher.Snapshot().Storage, gc.HasLen, 0)
}

func (s *WatcherSuite) TestStorageChangedNotFoundInitially(c *gc.C) {
	s.signalAll()
	assertNotifyEvent(c, s.watcher.RemoteStateChanged(), "waiting for remote state change")

	// blob/0 is initially in state, but is removed between the
	// watcher signal and the uniter querying it. This should
	// not cause the watcher to raise an error.
	s.st.unit.storageWatcher.changes <- []string{"blob/0"}
	assertNotifyEvent(c, s.watcher.RemoteStateChanged(), "waiting for remote state change")
	c.Assert(s.watcher.Snapshot().Storage, gc.HasLen, 0)
}

func (s *WatcherSuite) TestRelationsChanged(c *gc.C) {
	s.signalAll()
	assertNotifyEvent(c, s.watcher.RemoteStateChanged(), "waiting for remote state change")

	relationTag := names.NewRelationTag("mysql:peer")
	s.st.relations[relationTag] = &mockRelation{
		tag: relationTag, id: 123, life: life.Alive, suspended: false,
	}
	s.st.relationUnitsWatchers[relationTag] = newMockRelationUnitsWatcher()
	s.st.unit.relationsWatcher.changes <- []string{relationTag.Id()}
	s.st.relationAppWatchers[relationTag] = map[string]*mockNotifyWatcher{"mysql": newMockNotifyWatcher()}

	// There should not be any signal until the relation units watcher has
	// returned its initial event also.
	assertNoNotifyEvent(c, s.watcher.RemoteStateChanged(), "remote state change")
	s.st.relationUnitsWatchers[relationTag].changes <- watcher.RelationUnitsChange{
		Changed:    map[string]watcher.UnitSettings{"mysql/1": {1}, "mysql/2": {2}},
		AppChanged: map[string]int64{"mysql": 1},
	}
	assertNotifyEvent(c, s.watcher.RemoteStateChanged(), "waiting for remote state change")
	c.Assert(
		s.watcher.Snapshot().Relations,
		jc.DeepEquals,
		map[int]remotestate.RelationSnapshot{
			123: {
				Life:               life.Alive,
				Suspended:          false,
				Members:            map[string]int64{"mysql/1": 1, "mysql/2": 2},
				ApplicationMembers: map[string]int64{"mysql": 1},
			},
		},
	)

	// If a relation is known, then updating it does not require any input
	// from the relation units watcher.
	s.st.relations[relationTag].life = life.Dying
	s.st.unit.relationsWatcher.changes <- []string{relationTag.Id()}
	assertNotifyEvent(c, s.watcher.RemoteStateChanged(), "waiting for remote state change")
	c.Assert(s.watcher.Snapshot().Relations[123].Life, gc.Equals, life.Dying)

	// If a relation is not found, then it should be removed from the
	// snapshot and its relation units watcher stopped.
	delete(s.st.relations, relationTag)
	s.st.unit.relationsWatcher.changes <- []string{relationTag.Id()}
	assertNotifyEvent(c, s.watcher.RemoteStateChanged(), "waiting for remote state change")
	c.Assert(s.watcher.Snapshot().Relations, gc.HasLen, 0)
	c.Assert(s.st.relationUnitsWatchers[relationTag].Stopped(), jc.IsTrue)
}

func (s *WatcherSuite) TestRelationsSuspended(c *gc.C) {
	s.signalAll()
	assertNotifyEvent(c, s.watcher.RemoteStateChanged(), "waiting for remote state change")

	relationTag := names.NewRelationTag("mysql:db wordpress:db")
	s.st.relations[relationTag] = &mockRelation{
		tag: relationTag, id: 123, life: life.Alive, suspended: false,
	}
	s.st.relationUnitsWatchers[relationTag] = newMockRelationUnitsWatcher()
	s.st.unit.relationsWatcher.changes <- []string{relationTag.Id()}
	assertNoNotifyEvent(c, s.watcher.RemoteStateChanged(), "remote state change")
	s.st.relationUnitsWatchers[relationTag].changes <- watcher.RelationUnitsChange{
		Changed:    map[string]watcher.UnitSettings{"mysql/1": {1}, "mysql/2": {2}},
		AppChanged: map[string]int64{"mysql": 1},
	}
	assertNotifyEvent(c, s.watcher.RemoteStateChanged(), "waiting for remote state change")

	s.st.relations[relationTag].suspended = true
	s.st.unit.relationsWatcher.changes <- []string{relationTag.Id()}
	assertNotifyEvent(c, s.watcher.RemoteStateChanged(), "waiting for remote state change")
	c.Assert(s.watcher.Snapshot().Relations[123].Suspended, jc.IsTrue)
	c.Assert(s.st.relationUnitsWatchers[relationTag].Stopped(), jc.IsTrue)
}

func (s *WatcherSuite) TestRelationUnitsChanged(c *gc.C) {
	s.signalAll()
	assertNotifyEvent(c, s.watcher.RemoteStateChanged(), "waiting for remote state change")

	relationTag := names.NewRelationTag("mysql:peer")
	s.st.relations[relationTag] = &mockRelation{
		tag: relationTag, id: 123, life: life.Alive,
	}
	s.st.relationUnitsWatchers[relationTag] = newMockRelationUnitsWatcher()

	s.st.unit.relationsWatcher.changes <- []string{relationTag.Id()}
	s.st.relationUnitsWatchers[relationTag].changes <- watcher.RelationUnitsChange{
		Changed:    map[string]watcher.UnitSettings{"mysql/1": {1}},
		AppChanged: map[string]int64{"mysql": 1},
	}
	assertNotifyEvent(c, s.watcher.RemoteStateChanged(), "waiting for remote state change")

	s.st.relationUnitsWatchers[relationTag].changes <- watcher.RelationUnitsChange{
		Changed: map[string]watcher.UnitSettings{"mysql/1": {2}, "mysql/2": {1}},
	}
	assertNotifyEvent(c, s.watcher.RemoteStateChanged(), "waiting for remote state change")
	c.Assert( // Members is updated
		s.watcher.Snapshot().Relations[123].Members,
		jc.DeepEquals,
		map[string]int64{"mysql/1": 2, "mysql/2": 1},
	)
	c.Assert( // ApplicationMembers doesn't change
		s.watcher.Snapshot().Relations[123].ApplicationMembers,
		jc.DeepEquals,
		map[string]int64{"mysql": 1},
	)

	s.st.relationUnitsWatchers[relationTag].changes <- watcher.RelationUnitsChange{
		AppChanged: map[string]int64{"mysql": 2},
	}
	assertNotifyEvent(c, s.watcher.RemoteStateChanged(), "waiting for remote state change")
	c.Assert( // Members doesn't change
		s.watcher.Snapshot().Relations[123].Members,
		jc.DeepEquals,
		map[string]int64{"mysql/1": 2, "mysql/2": 1},
	)
	c.Assert( // But ApplicationMembers is updated
		s.watcher.Snapshot().Relations[123].ApplicationMembers,
		jc.DeepEquals,
		map[string]int64{"mysql": 2},
	)

	s.st.relationUnitsWatchers[relationTag].changes <- watcher.RelationUnitsChange{
		Departed: []string{"mysql/1", "mysql/42"},
	}
	assertNotifyEvent(c, s.watcher.RemoteStateChanged(), "waiting for remote state change")
	c.Assert(
		s.watcher.Snapshot().Relations[123].Members,
		jc.DeepEquals,
		map[string]int64{"mysql/2": 1},
	)
}

func (s *WatcherSuite) TestRelationUnitsDontLeakReferences(c *gc.C) {
	s.signalAll()
	assertNotifyEvent(c, s.watcher.RemoteStateChanged(), "waiting for remote state change")

	relationTag := names.NewRelationTag("mysql:peer")
	s.st.relations[relationTag] = &mockRelation{
		tag: relationTag, id: 123, life: life.Alive,
	}
	s.st.relationUnitsWatchers[relationTag] = newMockRelationUnitsWatcher()

	s.st.unit.relationsWatcher.changes <- []string{relationTag.Id()}
	s.st.relationUnitsWatchers[relationTag].changes <- watcher.RelationUnitsChange{
		Changed: map[string]watcher.UnitSettings{"mysql/1": {1}},
	}
	assertNotifyEvent(c, s.watcher.RemoteStateChanged(), "waiting for remote state change")

	snapshot := s.watcher.Snapshot()
	snapshot.Relations[123].Members["pwned"] = 2600
	c.Assert(
		s.watcher.Snapshot().Relations[123].Members,
		jc.DeepEquals,
		map[string]int64{"mysql/1": 1},
	)
}

func (s *WatcherSuite) TestUpdateStatusTicker(c *gc.C) {
	s.signalAll()
	initial := s.watcher.Snapshot()
	assertNotifyEvent(c, s.watcher.RemoteStateChanged(), "waiting for remote state change")

	// Advance the clock past the trigger time.
	s.waitAlarmsStable(c)
	s.clock.Advance(5 * time.Minute)
	assertNotifyEvent(c, s.watcher.RemoteStateChanged(), "waiting for remote state change")
	c.Assert(s.watcher.Snapshot().UpdateStatusVersion, gc.Equals, initial.UpdateStatusVersion+1)

	// Advance again but not past the trigger time.
	s.waitAlarmsStable(c)
	s.clock.Advance(4 * time.Minute)
	assertNoNotifyEvent(c, s.watcher.RemoteStateChanged(), "unexpected remote state change")
	c.Assert(s.watcher.Snapshot().UpdateStatusVersion, gc.Equals, initial.UpdateStatusVersion+1)

	// And we hit the trigger time.
	s.clock.Advance(1 * time.Minute)
	assertNotifyEvent(c, s.watcher.RemoteStateChanged(), "waiting for remote state change")
	c.Assert(s.watcher.Snapshot().UpdateStatusVersion, gc.Equals, initial.UpdateStatusVersion+2)
}

func (s *WatcherSuite) TestUpdateStatusIntervalChanges(c *gc.C) {
	s.signalAll()
	initial := s.watcher.Snapshot()
	assertNotifyEvent(c, s.watcher.RemoteStateChanged(), "waiting for remote state change")

	// Advance the clock past the trigger time.
	s.waitAlarmsStable(c)
	s.clock.Advance(5 * time.Minute)
	assertNotifyEvent(c, s.watcher.RemoteStateChanged(), "waiting for remote state change")
	c.Assert(s.watcher.Snapshot().UpdateStatusVersion, gc.Equals, initial.UpdateStatusVersion+1)

	// Change the update status interval to 10 seconds.
	s.st.updateStatusInterval = 10 * time.Second
	s.st.updateStatusIntervalWatcher.changes <- struct{}{}

	// Advance 10 seconds; the timer should be triggered.
	s.waitAlarmsStable(c)
	s.clock.Advance(10 * time.Second)
	assertNotifyEvent(c, s.watcher.RemoteStateChanged(), "waiting for remote state change")
	c.Assert(s.watcher.Snapshot().UpdateStatusVersion, gc.Equals, initial.UpdateStatusVersion+2)
}

// waitAlarmsStable is used to wait until the remote watcher's loop has
// stopped churning (at least for testing.ShortWait), so that we can
// then Advance the clock with some confidence that the SUT really is
// waiting for it. This seems likely to be more stable than waiting for
// a specific number of loop iterations; it's currently 9, but waiting
// for a specific number is very likely to start failing intermittently
// again, as in lp:1604955, if the SUT undergoes even subtle changes.
func (s *WatcherSuite) waitAlarmsStable(c *gc.C) {
	timeout := time.After(coretesting.LongWait)
	for i := 0; ; i++ {
		c.Logf("waiting for alarm %d", i)
		select {
		case <-s.clock.Alarms():
		case <-time.After(coretesting.ShortWait):
			return
		case <-timeout:
			c.Fatalf("never stopped setting alarms")
		}
	}
}

func (s *WatcherSuiteCAAS) TestWatcherConfig(c *gc.C) {
	_, err := remotestate.NewWatcher(remotestate.WatcherConfig{
		ModelType: model.CAAS,
	})
	c.Assert(err, gc.ErrorMatches, "watcher config for CAAS model with nil application channel not valid")
}
