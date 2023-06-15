// Copyright 2012, 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package watcher_test

import (
	"time"

	"github.com/golang/mock/gomock"
	"github.com/juju/names/v4"
	"github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	"github.com/juju/worker/v3"
	"github.com/juju/worker/v3/workertest"
	gc "gopkg.in/check.v1"
	"gopkg.in/macaroon.v2"

	"github.com/juju/juju/api/agent/deployer"
	"github.com/juju/juju/api/agent/machiner"
	"github.com/juju/juju/api/agent/migrationminion"
	"github.com/juju/juju/api/agent/secretsmanager"
	"github.com/juju/juju/api/agent/storageprovisioner"
	apimocks "github.com/juju/juju/api/base/mocks"
	"github.com/juju/juju/api/controller/crossmodelrelations"
	apitesting "github.com/juju/juju/api/testing"
	"github.com/juju/juju/core/life"
	"github.com/juju/juju/core/migration"
	"github.com/juju/juju/core/secrets"
	"github.com/juju/juju/core/status"
	corewatcher "github.com/juju/juju/core/watcher"
	"github.com/juju/juju/core/watcher/watchertest"
	"github.com/juju/juju/rpc/params"
	coretesting "github.com/juju/juju/testing"
)

type watcherSuite struct {
	testing.IsolationSuite
}

var _ = gc.Suite(&watcherSuite{})

func setupWatcher[T any](c *gc.C, caller *apimocks.MockAPICaller, facadeName string) (string, chan T) {
	caller.EXPECT().BestFacadeVersion(facadeName).Return(666).AnyTimes()
	// Initial event.
	eventCh := make(chan T)

	stopped := make(chan bool)
	caller.EXPECT().APICall(facadeName, 666, "id-666", "Stop", nil, gomock.Any()).DoAndReturn(func(_ string, _ int, _ string, _ string, _ any, _ any) {
		select {
		case stopped <- true:
		default:
		}
	}).Return(nil).AnyTimes()

	caller.EXPECT().APICall(facadeName, 666, "id-666", "Next", nil, gomock.Any()).DoAndReturn(func(_ string, _ int, _ string, _ string, _ any, r *any) error {
		select {
		case ev, ok := <-eventCh:
			if !ok {
				c.FailNow()
			}
			*(*r).(*any) = ev
			return nil
		case <-stopped:
		}
		return &params.Error{Code: params.CodeStopped}
	}).AnyTimes()
	return "id-666", eventCh
}

func (s *watcherSuite) TestWatchMachine(c *gc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	caller := apimocks.NewMockAPICaller(ctrl)
	caller.EXPECT().BestFacadeVersion("Machiner").Return(666)

	watcherID, _ := setupWatcher[any](c, caller, "NotifyWatcher")

	args := params.Entities{Entities: []params.Entity{{Tag: "machine-666"}}}
	lifeResults := params.LifeResults{
		Results: []params.LifeResult{{Life: life.Alive}},
	}
	caller.EXPECT().APICall("Machiner", 666, "", "Life", args, gomock.Any()).SetArg(5, lifeResults).Return(nil)
	initialResults := params.NotifyWatchResults{
		Results: []params.NotifyWatchResult{{
			NotifyWatcherId: watcherID,
		}},
	}
	caller.EXPECT().APICall("Machiner", 666, "", "Watch", args, gomock.Any()).SetArg(5, initialResults).Return(nil)

	client := machiner.NewClient(caller)
	m, err := client.Machine(names.NewMachineTag("666"))
	c.Assert(err, jc.ErrorIsNil)

	w, err := m.Watch()
	c.Assert(err, jc.ErrorIsNil)
	defer workertest.CleanKill(c, w)

	wc := watchertest.NewNotifyWatcherC(c, w)
	defer wc.AssertStops()

	wc.AssertOneChange()
}

func (s *watcherSuite) TestNotifyWatcherStopsWithPendingSend(c *gc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	caller := apimocks.NewMockAPICaller(ctrl)
	caller.EXPECT().BestFacadeVersion("Machiner").Return(666)

	watcherID, _ := setupWatcher[any](c, caller, "NotifyWatcher")

	args := params.Entities{Entities: []params.Entity{{Tag: "machine-666"}}}
	lifeResults := params.LifeResults{
		Results: []params.LifeResult{{Life: life.Alive}},
	}
	caller.EXPECT().APICall("Machiner", 666, "", "Life", args, gomock.Any()).SetArg(5, lifeResults).Return(nil)
	initialResults := params.NotifyWatchResults{
		Results: []params.NotifyWatchResult{{
			NotifyWatcherId: watcherID,
		}},
	}
	caller.EXPECT().APICall("Machiner", 666, "", "Watch", args, gomock.Any()).SetArg(5, initialResults).Return(nil)

	client := machiner.NewClient(caller)
	m, err := client.Machine(names.NewMachineTag("666"))
	c.Assert(err, jc.ErrorIsNil)

	w, err := m.Watch()
	c.Assert(err, jc.ErrorIsNil)
	defer workertest.CleanKill(c, w)

	wc := watchertest.NewNotifyWatcherC(c, w)
	defer wc.AssertStops()
}

func (s *watcherSuite) TestWatchUnits(c *gc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	caller := apimocks.NewMockAPICaller(ctrl)
	caller.EXPECT().BestFacadeVersion("Deployer").Return(666)

	watcherID, eventCh := setupWatcher[*params.StringsWatchResult](c, caller, "StringsWatcher")

	args := params.Entities{Entities: []params.Entity{{Tag: "machine-666"}}}
	initialResults := params.StringsWatchResults{
		Results: []params.StringsWatchResult{{
			StringsWatcherId: watcherID,
			Changes:          []string{"unit-1", "unit-3"},
		}},
	}
	caller.EXPECT().APICall("Deployer", 666, "", "WatchUnits", args, gomock.Any()).SetArg(5, initialResults).Return(nil)

	client := deployer.NewClient(caller)
	m, err := client.Machine(names.NewMachineTag("666"))
	c.Assert(err, jc.ErrorIsNil)

	w, err := m.WatchUnits()
	c.Assert(err, jc.ErrorIsNil)
	defer workertest.CleanKill(c, w)

	wc := watchertest.NewStringsWatcherC(c, w)
	defer wc.AssertStops()

	go func() {
		eventCh <- &params.StringsWatchResult{
			StringsWatcherId: watcherID,
			Changes:          []string{"unit-1", "unit-2"},
		}
	}()

	// Initial event.
	wc.AssertChange("unit-1", "unit-3")
	wc.AssertChange("unit-1", "unit-2")
}

func (s *watcherSuite) TestStringsWatcherStopsWithPendingSend(c *gc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	caller := apimocks.NewMockAPICaller(ctrl)
	caller.EXPECT().BestFacadeVersion("Deployer").Return(666)

	watcherID, _ := setupWatcher[*params.StringsWatchResult](c, caller, "StringsWatcher")

	args := params.Entities{Entities: []params.Entity{{Tag: "machine-666"}}}
	initialResults := params.StringsWatchResults{
		Results: []params.StringsWatchResult{{
			StringsWatcherId: watcherID,
			Changes:          []string{"unit-1", "unit-3"},
		}},
	}
	caller.EXPECT().APICall("Deployer", 666, "", "WatchUnits", args, gomock.Any()).SetArg(5, initialResults).Return(nil)

	client := deployer.NewClient(caller)
	m, err := client.Machine(names.NewMachineTag("666"))
	c.Assert(err, jc.ErrorIsNil)

	w, err := m.WatchUnits()
	c.Assert(err, jc.ErrorIsNil)
	defer workertest.CleanKill(c, w)

	wc := watchertest.NewStringsWatcherC(c, w)
	defer wc.AssertStops()
}

func (s *watcherSuite) TestWatchMachineStorage(c *gc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	caller := apimocks.NewMockAPICaller(ctrl)
	caller.EXPECT().BestFacadeVersion("StorageProvisioner").Return(666)

	watcherID, eventCh := setupWatcher[*params.MachineStorageIdsWatchResult](c, caller, "VolumeAttachmentsWatcher")

	args := params.Entities{Entities: []params.Entity{{
		Tag: "machine-666",
	}}}
	initialResults := params.MachineStorageIdsWatchResults{
		Results: []params.MachineStorageIdsWatchResult{{
			MachineStorageIdsWatcherId: watcherID,
			Changes: []params.MachineStorageId{{
				MachineTag:    "machine-666",
				AttachmentTag: "volume-0",
			}},
		}},
	}
	caller.EXPECT().APICall("StorageProvisioner", 666, "", "WatchVolumeAttachments", args, gomock.Any()).SetArg(5, initialResults).Return(nil)

	client, err := storageprovisioner.NewClient(caller)
	c.Assert(err, jc.ErrorIsNil)
	w, err := client.WatchVolumeAttachments(names.NewMachineTag("666"))
	c.Assert(err, jc.ErrorIsNil)
	defer workertest.CleanKill(c, w)

	assertNoChange := func() {
		select {
		case _, ok := <-w.Changes():
			c.Fatalf("watcher sent unexpected change: (_, %v)", ok)
		case <-time.After(coretesting.ShortWait):
		}
	}

	assertChange := func(machine, attachment string) {
		select {
		case changes, ok := <-w.Changes():
			c.Assert(ok, jc.IsTrue)
			c.Assert(changes, jc.SameContents, []corewatcher.MachineStorageID{{
				MachineTag:    machine,
				AttachmentTag: attachment,
			}})
		case <-time.After(coretesting.LongWait):
			c.Fatalf("timed out waiting for change")
		}
		assertNoChange()
	}

	// Initial event.
	assertChange("machine-666", "volume-0")

	go func() {
		eventCh <- &params.MachineStorageIdsWatchResult{
			MachineStorageIdsWatcherId: watcherID,
			Changes: []params.MachineStorageId{{
				MachineTag:    "machine-666",
				AttachmentTag: "volume-1",
			}},
		}
	}()
	assertChange("machine-666", "volume-1")
}

type apicloser struct {
	*apimocks.MockAPICaller
}

func (*apicloser) Close() error {
	return nil
}

func (s *watcherSuite) TestRelationStatusWatcher(c *gc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	caller := apimocks.NewMockAPICaller(ctrl)
	caller.EXPECT().BestFacadeVersion("CrossModelRelations").Return(666)

	watcherID, eventCh := setupWatcher[*params.RelationLifeSuspendedStatusWatchResult](c, caller, "RelationStatusWatcher")

	mac, err := apitesting.NewMacaroon("apimac")
	c.Assert(err, jc.ErrorIsNil)
	arg := params.RemoteEntityArg{
		Token:     "token",
		Macaroons: macaroon.Slice{mac},
	}
	args := params.RemoteEntityArgs{
		Args: []params.RemoteEntityArg{arg},
	}
	initialResults := params.RelationStatusWatchResults{
		Results: []params.RelationLifeSuspendedStatusWatchResult{{
			RelationStatusWatcherId: watcherID,
			Changes: []params.RelationLifeSuspendedStatusChange{{
				Key:  "relation-wordpress:database mysql:server",
				Life: "alive",
			}},
		}},
	}
	caller.EXPECT().APICall("CrossModelRelations", 666, "", "WatchRelationsSuspendedStatus", args, gomock.Any()).SetArg(5, initialResults).Return(nil)

	client := crossmodelrelations.NewClient(&apicloser{caller})
	w, err := client.WatchRelationSuspendedStatus(arg)
	c.Assert(err, jc.ErrorIsNil)
	defer workertest.CleanKill(c, w)

	assertNoChange := func() {
		select {
		case _, ok := <-w.Changes():
			c.Fatalf("watcher sent unexpected change: (_, %v)", ok)
		case <-time.After(coretesting.ShortWait):
		}
	}

	assertChange := func(life life.Value, suspended bool, reason string) {
		select {
		case changes, ok := <-w.Changes():
			c.Check(ok, jc.IsTrue)
			c.Check(changes, gc.HasLen, 1)
			c.Check(changes[0].Key, gc.Equals, "relation-wordpress:database mysql:server")
			c.Check(changes[0].Life, gc.Equals, life)
			c.Check(changes[0].Suspended, gc.Equals, suspended)
			c.Check(changes[0].SuspendedReason, gc.Equals, reason)
		case <-time.After(coretesting.LongWait):
			c.Fatalf("watcher didn't emit an event")
		}
		assertNoChange()
	}

	// Initial event.
	assertChange(life.Alive, false, "")

	go func() {
		eventCh <- &params.RelationLifeSuspendedStatusWatchResult{
			RelationStatusWatcherId: watcherID,
			Changes: []params.RelationLifeSuspendedStatusChange{{
				Key:             "relation-wordpress:database mysql:server",
				Life:            "dying",
				Suspended:       true,
				SuspendedReason: "suspended",
			}},
		}
	}()
	assertChange(life.Dying, true, "suspended")
}

func (s *watcherSuite) TestOfferStatusWatcher(c *gc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	caller := apimocks.NewMockAPICaller(ctrl)
	caller.EXPECT().BestFacadeVersion("CrossModelRelations").Return(666)

	watcherID, eventCh := setupWatcher[*params.OfferStatusWatchResult](c, caller, "OfferStatusWatcher")

	mac, err := apitesting.NewMacaroon("apimac")
	c.Assert(err, jc.ErrorIsNil)
	arg := params.OfferArg{
		OfferUUID:     "offer-uuid",
		Macaroons:     macaroon.Slice{mac},
		BakeryVersion: 3,
	}
	args := params.OfferArgs{
		Args: []params.OfferArg{arg},
	}
	now := time.Now()
	initialResults := params.OfferStatusWatchResults{
		Results: []params.OfferStatusWatchResult{{
			OfferStatusWatcherId: watcherID,
			Changes: []params.OfferStatusChange{{
				OfferName: "my offer",
				Status: params.EntityStatus{
					Status: "maintenance",
					Info:   "working",
					Data:   map[string]interface{}{"foo": "bar"},
					Since:  &now,
				},
			}},
		}},
	}
	caller.EXPECT().APICall("CrossModelRelations", 666, "", "WatchOfferStatus", args, gomock.Any()).SetArg(5, initialResults).Return(nil)

	client := crossmodelrelations.NewClient(&apicloser{caller})
	w, err := client.WatchOfferStatus(arg)
	c.Assert(err, jc.ErrorIsNil)
	defer workertest.CleanKill(c, w)

	assertNoChange := func() {
		select {
		case _, ok := <-w.Changes():
			c.Fatalf("watcher sent unexpected change: (_, %v)", ok)
		case <-time.After(coretesting.ShortWait):
		}
	}

	assertChange := func(s status.Status, info string) {
		select {
		case changes, ok := <-w.Changes():
			c.Check(ok, jc.IsTrue)
			c.Check(changes, gc.HasLen, 1)
			c.Check(changes[0].Name, gc.Equals, "my offer")
			c.Check(changes[0].Status, jc.DeepEquals, status.StatusInfo{
				Status:  s,
				Message: info,
				Data:    map[string]interface{}{"foo": "bar"},
				Since:   &now,
			})
		case <-time.After(coretesting.LongWait):
			c.Fatalf("watcher didn't emit an event")
		}
		assertNoChange()
	}

	// Initial event.
	assertChange(status.Maintenance, "working")

	go func() {
		eventCh <- &params.OfferStatusWatchResult{
			OfferStatusWatcherId: watcherID,
			Changes: []params.OfferStatusChange{{
				OfferName: "my offer",
				Status: params.EntityStatus{
					Status: "active",
					Info:   "finished",
					Data:   map[string]interface{}{"foo": "bar"},
					Since:  &now,
				},
			}},
		}
	}()
	assertChange(status.Active, "finished")
}

func (s *watcherSuite) assertSecretsTriggerWatcher(c *gc.C, caller *apimocks.MockAPICaller, apiName string, watchFunc func(ownerTags ...names.Tag) (corewatcher.SecretTriggerWatcher, error)) {
	watcherID, eventCh := setupWatcher[*params.SecretTriggerWatchResult](c, caller, "SecretsTriggerWatcher")

	args := params.Entities{
		Entities: []params.Entity{{Tag: "application-mysql"}},
	}
	next := time.Now()
	later := next.Add(time.Hour)
	initialResults := params.SecretTriggerWatchResult{
		WatcherId: watcherID,
		Changes: []params.SecretTriggerChange{{
			URI:             "secret:9m4e2mr0ui3e8a215n4g",
			Revision:        666,
			NextTriggerTime: next,
		}},
	}
	caller.EXPECT().APICall("SecretsManager", 666, "", apiName, args, gomock.Any()).SetArg(5, initialResults).Return(nil)

	w, err := watchFunc(names.NewApplicationTag("mysql"))
	c.Assert(err, jc.ErrorIsNil)
	defer workertest.CleanKill(c, w)

	assertNoChange := func() {
		select {
		case _, ok := <-w.Changes():
			c.Fatalf("watcher sent unexpected change: (_, %v)", ok)
		case <-time.After(coretesting.ShortWait):
		}
	}

	assertChange := func(when time.Time) {
		select {
		case changes, ok := <-w.Changes():
			c.Check(ok, jc.IsTrue)
			c.Check(changes, gc.HasLen, 1)
			c.Check(changes[0].URI, jc.DeepEquals, &secrets.URI{ID: "9m4e2mr0ui3e8a215n4g"})
			c.Check(changes[0].Revision, gc.Equals, 666)
			c.Check(changes[0].NextTriggerTime, jc.DeepEquals, when)
		case <-time.After(coretesting.LongWait):
			c.Fatalf("watcher didn't emit an event")
		}
		assertNoChange()
	}

	// Initial event.
	assertChange(next)

	go func() {
		eventCh <- &params.SecretTriggerWatchResult{
			WatcherId: "id-666",
			Changes: []params.SecretTriggerChange{{
				URI:             "secret:9m4e2mr0ui3e8a215n4g",
				Revision:        666,
				NextTriggerTime: later,
			}},
		}
	}()
	assertChange(later)
}

func (s *watcherSuite) TestSecretsRotationWatcher(c *gc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	caller := apimocks.NewMockAPICaller(ctrl)
	caller.EXPECT().BestFacadeVersion("SecretsManager").Return(666).AnyTimes()

	client := secretsmanager.NewClient(caller)
	s.assertSecretsTriggerWatcher(c, caller, "WatchSecretsRotationChanges", client.WatchSecretsRotationChanges)
}

func (s *watcherSuite) TestSecretsRevisionsExpiryWatcher(c *gc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	caller := apimocks.NewMockAPICaller(ctrl)
	caller.EXPECT().BestFacadeVersion("SecretsManager").Return(666).AnyTimes()

	client := secretsmanager.NewClient(caller)
	s.assertSecretsTriggerWatcher(c, caller, "WatchSecretRevisionsExpiryChanges", client.WatchSecretRevisionsExpiryChanges)
}

func (s *watcherSuite) TestCrossModelSecretsRevisionWatcher(c *gc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	caller := apimocks.NewMockAPICaller(ctrl)
	caller.EXPECT().BestFacadeVersion("CrossModelRelations").Return(666).AnyTimes()
	watcherID, eventCh := setupWatcher[*params.SecretRevisionWatchResult](c, caller, "SecretsRevisionWatcher")

	mac, err := apitesting.NewMacaroon("apimac")
	c.Assert(err, jc.ErrorIsNil)
	args := params.WatchRemoteSecretChangesArgs{Args: []params.WatchRemoteSecretChangesArg{{
		ApplicationToken: "app-token",
		RelationToken:    "rel-token",
		Macaroons:        macaroon.Slice{mac},
		BakeryVersion:    3,
	}}}
	initialResults := params.SecretRevisionWatchResults{
		Results: []params.SecretRevisionWatchResult{{
			WatcherId: watcherID,
			Changes: []params.SecretRevisionChange{{
				URI:      "secret:9m4e2mr0ui3e8a215n4g",
				Revision: 666,
			}},
		}},
	}
	caller.EXPECT().APICall("CrossModelRelations", 666, "", "WatchConsumedSecretsChanges", args, gomock.Any()).SetArg(5, initialResults).Return(nil)

	client := crossmodelrelations.NewClient(&apicloser{caller})
	w, err := client.WatchConsumedSecretsChanges("app-token", "rel-token", mac)
	c.Assert(err, jc.ErrorIsNil)
	defer workertest.CleanKill(c, w)

	assertNoChange := func() {
		select {
		case _, ok := <-w.Changes():
			c.Fatalf("watcher sent unexpected change: (_, %v)", ok)
		case <-time.After(coretesting.ShortWait):
		}
	}

	assertChange := func(rev int) {
		select {
		case changes, ok := <-w.Changes():
			c.Check(ok, jc.IsTrue)
			c.Check(changes, gc.HasLen, 1)
			c.Check(changes[0].URI, jc.DeepEquals, &secrets.URI{ID: "9m4e2mr0ui3e8a215n4g"})
			c.Check(changes[0].Revision, gc.Equals, rev)
		case <-time.After(coretesting.LongWait):
			c.Fatalf("watcher didn't emit an event")
		}
		assertNoChange()
	}

	// Initial event.
	assertChange(666)

	go func() {
		eventCh <- &params.SecretRevisionWatchResult{
			WatcherId: watcherID,
			Changes: []params.SecretRevisionChange{{
				URI:      "secret:9m4e2mr0ui3e8a215n4g",
				Revision: 667,
			}},
		}
	}()
	assertChange(667)
}

type migrationSuite struct {
	testing.IsolationSuite
}

var _ = gc.Suite(&migrationSuite{})

func (s *migrationSuite) TestMigrationStatusWatcher(c *gc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	caller := apimocks.NewMockAPICaller(ctrl)
	caller.EXPECT().BestFacadeVersion("MigrationMinion").Return(666).AnyTimes()
	watcherID, eventCh := setupWatcher[*params.MigrationStatus](c, caller, "MigrationStatusWatcher")

	initialResult := params.NotifyWatchResult{
		NotifyWatcherId: watcherID,
	}
	caller.EXPECT().APICall("MigrationMinion", 666, "", "Watch", nil, gomock.Any()).SetArg(5, initialResult).Return(nil)

	client := migrationminion.NewClient(caller)
	w, err := client.Watch()
	c.Assert(err, jc.ErrorIsNil)
	defer func() {
		c.Assert(worker.Stop(w), jc.ErrorIsNil)
	}()

	assertNoChange := func() {
		select {
		case _, ok := <-w.Changes():
			c.Fatalf("watcher sent unexpected change: (_, %v)", ok)
		case <-time.After(coretesting.ShortWait):
		}
	}

	assertChange := func(id string, phase migration.Phase) {
		select {
		case status, ok := <-w.Changes():
			c.Assert(ok, jc.IsTrue)
			c.Check(status.MigrationId, gc.Equals, id)
			c.Check(status.Phase, gc.Equals, phase)
		case <-time.After(coretesting.LongWait):
			c.Fatalf("watcher didn't emit an event")
		}
		assertNoChange()
	}

	go func() {
		eventCh <- &params.MigrationStatus{
			MigrationId: "mig-id",
			Phase:       migration.QUIESCE.String(),
		}
	}()

	assertChange("mig-id", migration.QUIESCE)
}
