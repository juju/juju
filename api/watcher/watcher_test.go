// Copyright 2012, 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package watcher_test

import (
	"context"
	"time"

	"github.com/juju/names/v6"
	"github.com/juju/tc"
	"github.com/juju/testing"
	"github.com/juju/worker/v4"
	"github.com/juju/worker/v4/workertest"
	"go.uber.org/mock/gomock"
	"gopkg.in/macaroon.v2"

	"github.com/juju/juju/api/agent/deployer"
	"github.com/juju/juju/api/agent/machiner"
	"github.com/juju/juju/api/agent/migrationminion"
	"github.com/juju/juju/api/agent/secretsmanager"
	"github.com/juju/juju/api/agent/storageprovisioner"
	apimocks "github.com/juju/juju/api/base/mocks"
	"github.com/juju/juju/api/controller/crossmodelrelations"
	"github.com/juju/juju/api/watcher"
	"github.com/juju/juju/core/life"
	"github.com/juju/juju/core/migration"
	"github.com/juju/juju/core/secrets"
	"github.com/juju/juju/core/status"
	corewatcher "github.com/juju/juju/core/watcher"
	"github.com/juju/juju/core/watcher/watchertest"
	coretesting "github.com/juju/juju/internal/testing"
	jujutesting "github.com/juju/juju/juju/testing"
	"github.com/juju/juju/rpc/params"
)

type watcherSuite struct {
	testing.IsolationSuite
}

var _ = tc.Suite(&watcherSuite{})

func (s *watcherSuite) TestWatcherStopsOnBlockedNext(c *tc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	// This test ensures that if there is a blocking call to next, because of
	// a bad watcher, then we can correctly recover.

	caller := apimocks.NewMockAPICaller(ctrl)

	facadeName := "StringsWatcher"
	watcherID := "id-666"

	done := make(chan struct{})
	called := make(chan struct{})

	caller.EXPECT().BestFacadeVersion(facadeName).Return(666).AnyTimes()
	caller.EXPECT().APICall(gomock.Any(), facadeName, 666, "id-666", "Stop", nil, gomock.Any()).Return(nil).AnyTimes()
	caller.EXPECT().APICall(gomock.Any(), facadeName, 666, "id-666", "Next", nil, gomock.Any()).DoAndReturn(func(ctx context.Context, _ string, _ int, _, _ string, _, _ any) error {
		close(called)

		select {
		case <-done:
			return nil
		case <-ctx.Done():
			return context.Canceled
		}
	})

	result := params.StringsWatchResult{
		StringsWatcherId: watcherID,
		Changes:          []string{"unit-1", "unit-3"},
	}
	w := watcher.NewStringsWatcher(caller, result)

	wc := watchertest.NewStringsWatcherC(c, w)
	defer wc.AssertStops()

	// Ensure we consume the initial event.
	wc.AssertChanges()

	// Wait for the Next call to be made, before killing the worker. We need
	// to ensure that the worker is blocked on the Next call.
	select {
	case <-called:
	case <-time.After(coretesting.LongWait):
		c.Fatalf("timed out waiting for Next call")
	}

	defer close(done)

	workertest.CleanKill(c, w)
}

func setupWatcher[T any](c *tc.C, caller *apimocks.MockAPICaller, facadeName string) (string, chan T) {
	caller.EXPECT().BestFacadeVersion(facadeName).Return(666).AnyTimes()
	// Initial event.
	eventCh := make(chan T)

	stopped := make(chan bool)
	caller.EXPECT().APICall(gomock.Any(), facadeName, 666, "id-666", "Stop", nil, gomock.Any()).DoAndReturn(
		func(context.Context, string, int, string, string, any, any) error {
			select {
			case stopped <- true:
			case <-time.After(testing.LongWait):
				c.Fatalf("timed out waiting for stop call")
			}
			return nil
		},
	).Return(nil).AnyTimes()

	caller.EXPECT().APICall(gomock.Any(), facadeName, 666, "id-666", "Next", nil, gomock.Any()).DoAndReturn(
		func(_ context.Context, _ string, _ int, _ string, _ string, _ any, r any) error {
			select {
			case ev, ok := <-eventCh:
				if !ok {
					c.Fatalf("next channel closed")
				}
				*(*r.(*any)).(*any) = ev
				return nil
			case <-stopped:
			}
			return &params.Error{Code: params.CodeStopped}
		},
	).AnyTimes()
	return "id-666", eventCh
}

func (s *watcherSuite) TestWatchMachine(c *tc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	caller := apimocks.NewMockAPICaller(ctrl)
	caller.EXPECT().BestFacadeVersion("Machiner").Return(666)

	watcherID, _ := setupWatcher[any](c, caller, "NotifyWatcher")

	args := params.Entities{Entities: []params.Entity{{Tag: "machine-666"}}}
	lifeResults := params.LifeResults{
		Results: []params.LifeResult{{Life: life.Alive}},
	}
	caller.EXPECT().APICall(gomock.Any(), "Machiner", 666, "", "Life", args, gomock.Any()).SetArg(6, lifeResults).Return(nil)
	initialResults := params.NotifyWatchResults{
		Results: []params.NotifyWatchResult{{
			NotifyWatcherId: watcherID,
		}},
	}
	caller.EXPECT().APICall(gomock.Any(), "Machiner", 666, "", "Watch", args, gomock.Any()).SetArg(6, initialResults).Return(nil)

	client := machiner.NewClient(caller)
	m, err := client.Machine(context.Background(), names.NewMachineTag("666"))
	c.Assert(err, tc.ErrorIsNil)

	w, err := m.Watch(context.Background())
	c.Assert(err, tc.ErrorIsNil)
	defer workertest.CleanKill(c, w)

	wc := watchertest.NewNotifyWatcherC(c, w)
	defer wc.AssertStops()

	wc.AssertOneChange()
}

func (s *watcherSuite) TestNotifyWatcherStopsWithPendingSend(c *tc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	caller := apimocks.NewMockAPICaller(ctrl)
	caller.EXPECT().BestFacadeVersion("Machiner").Return(666)

	watcherID, _ := setupWatcher[any](c, caller, "NotifyWatcher")

	args := params.Entities{Entities: []params.Entity{{Tag: "machine-666"}}}
	lifeResults := params.LifeResults{
		Results: []params.LifeResult{{Life: life.Alive}},
	}
	caller.EXPECT().APICall(gomock.Any(), "Machiner", 666, "", "Life", args, gomock.Any()).SetArg(6, lifeResults).Return(nil)
	initialResults := params.NotifyWatchResults{
		Results: []params.NotifyWatchResult{{
			NotifyWatcherId: watcherID,
		}},
	}
	caller.EXPECT().APICall(gomock.Any(), "Machiner", 666, "", "Watch", args, gomock.Any()).SetArg(6, initialResults).Return(nil)

	client := machiner.NewClient(caller)
	m, err := client.Machine(context.Background(), names.NewMachineTag("666"))
	c.Assert(err, tc.ErrorIsNil)

	w, err := m.Watch(context.Background())
	c.Assert(err, tc.ErrorIsNil)
	defer workertest.CleanKill(c, w)

	wc := watchertest.NewNotifyWatcherC(c, w)
	defer wc.AssertStops()
}

func (s *watcherSuite) TestWatchUnits(c *tc.C) {
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
	caller.EXPECT().APICall(gomock.Any(), "Deployer", 666, "", "WatchUnits", args, gomock.Any()).SetArg(6, initialResults).Return(nil)

	client := deployer.NewClient(caller)
	m, err := client.Machine(names.NewMachineTag("666"))
	c.Assert(err, tc.ErrorIsNil)

	w, err := m.WatchUnits(context.Background())
	c.Assert(err, tc.ErrorIsNil)
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

func (s *watcherSuite) TestStringsWatcherStopsWithPendingSend(c *tc.C) {
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
	caller.EXPECT().APICall(gomock.Any(), "Deployer", 666, "", "WatchUnits", args, gomock.Any()).SetArg(6, initialResults).Return(nil)

	client := deployer.NewClient(caller)
	m, err := client.Machine(names.NewMachineTag("666"))
	c.Assert(err, tc.ErrorIsNil)

	w, err := m.WatchUnits(context.Background())
	c.Assert(err, tc.ErrorIsNil)
	defer workertest.CleanKill(c, w)

	wc := watchertest.NewStringsWatcherC(c, w)
	defer wc.AssertStops()
}

func (s *watcherSuite) TestWatchMachineStorage(c *tc.C) {
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
	caller.EXPECT().APICall(gomock.Any(), "StorageProvisioner", 666, "", "WatchVolumeAttachments", args, gomock.Any()).SetArg(6, initialResults).Return(nil)

	client, err := storageprovisioner.NewClient(caller)
	c.Assert(err, tc.ErrorIsNil)
	w, err := client.WatchVolumeAttachments(context.Background(), names.NewMachineTag("666"))
	c.Assert(err, tc.ErrorIsNil)
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
			c.Assert(ok, tc.IsTrue)
			c.Assert(changes, tc.SameContents, []corewatcher.MachineStorageID{{
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

func (s *watcherSuite) TestRelationStatusWatcher(c *tc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	caller := apimocks.NewMockAPICaller(ctrl)
	caller.EXPECT().BestFacadeVersion("CrossModelRelations").Return(666)

	watcherID, eventCh := setupWatcher[*params.RelationLifeSuspendedStatusWatchResult](c, caller, "RelationStatusWatcher")

	mac, err := jujutesting.NewMacaroon("apimac")
	c.Assert(err, tc.ErrorIsNil)
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
	caller.EXPECT().APICall(gomock.Any(), "CrossModelRelations", 666, "", "WatchRelationsSuspendedStatus", args, gomock.Any()).SetArg(6, initialResults).Return(nil)

	client := crossmodelrelations.NewClient(&apicloser{caller})
	w, err := client.WatchRelationSuspendedStatus(context.Background(), arg)
	c.Assert(err, tc.ErrorIsNil)
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
			c.Check(ok, tc.IsTrue)
			c.Check(changes, tc.HasLen, 1)
			c.Check(changes[0].Key, tc.Equals, "relation-wordpress:database mysql:server")
			c.Check(changes[0].Life, tc.Equals, life)
			c.Check(changes[0].Suspended, tc.Equals, suspended)
			c.Check(changes[0].SuspendedReason, tc.Equals, reason)
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

func (s *watcherSuite) TestOfferStatusWatcher(c *tc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	caller := apimocks.NewMockAPICaller(ctrl)
	caller.EXPECT().BestFacadeVersion("CrossModelRelations").Return(666)

	watcherID, eventCh := setupWatcher[*params.OfferStatusWatchResult](c, caller, "OfferStatusWatcher")

	mac, err := jujutesting.NewMacaroon("apimac")
	c.Assert(err, tc.ErrorIsNil)
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
	caller.EXPECT().APICall(gomock.Any(), "CrossModelRelations", 666, "", "WatchOfferStatus", args, gomock.Any()).SetArg(6, initialResults).Return(nil)

	client := crossmodelrelations.NewClient(&apicloser{caller})
	w, err := client.WatchOfferStatus(context.Background(), arg)
	c.Assert(err, tc.ErrorIsNil)
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
			c.Check(ok, tc.IsTrue)
			c.Check(changes, tc.HasLen, 1)
			c.Check(changes[0].Name, tc.Equals, "my offer")
			c.Check(changes[0].Status, tc.DeepEquals, status.StatusInfo{
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

func (s *watcherSuite) assertSecretsTriggerWatcher(c *tc.C, caller *apimocks.MockAPICaller, apiName string, watchFunc func(ctx context.Context, ownerTags ...names.Tag) (corewatcher.SecretTriggerWatcher, error)) {
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
	caller.EXPECT().APICall(gomock.Any(), "SecretsManager", 666, "", apiName, args, gomock.Any()).SetArg(6, initialResults).Return(nil)

	w, err := watchFunc(context.Background(), names.NewApplicationTag("mysql"))
	c.Assert(err, tc.ErrorIsNil)
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
			c.Check(ok, tc.IsTrue)
			c.Check(changes, tc.HasLen, 1)
			c.Check(changes[0].URI, tc.DeepEquals, &secrets.URI{ID: "9m4e2mr0ui3e8a215n4g"})
			c.Check(changes[0].Revision, tc.Equals, 666)
			c.Check(changes[0].NextTriggerTime, tc.DeepEquals, when)
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

func (s *watcherSuite) TestSecretsRotationWatcher(c *tc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	caller := apimocks.NewMockAPICaller(ctrl)
	caller.EXPECT().BestFacadeVersion("SecretsManager").Return(666).AnyTimes()

	client := secretsmanager.NewClient(caller)
	s.assertSecretsTriggerWatcher(c, caller, "WatchSecretsRotationChanges", client.WatchSecretsRotationChanges)
}

func (s *watcherSuite) TestSecretsRevisionsExpiryWatcher(c *tc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	caller := apimocks.NewMockAPICaller(ctrl)
	caller.EXPECT().BestFacadeVersion("SecretsManager").Return(666).AnyTimes()

	client := secretsmanager.NewClient(caller)
	s.assertSecretsTriggerWatcher(c, caller, "WatchSecretRevisionsExpiryChanges", client.WatchSecretRevisionsExpiryChanges)
}

func (s *watcherSuite) TestCrossModelSecretsRevisionWatcher(c *tc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	caller := apimocks.NewMockAPICaller(ctrl)
	caller.EXPECT().BestFacadeVersion("CrossModelRelations").Return(666).AnyTimes()
	watcherID, eventCh := setupWatcher[*params.SecretRevisionWatchResult](c, caller, "SecretsRevisionWatcher")

	mac, err := jujutesting.NewMacaroon("apimac")
	c.Assert(err, tc.ErrorIsNil)
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
				URI:            "secret:9m4e2mr0ui3e8a215n4g",
				LatestRevision: 666,
			}},
		}},
	}
	caller.EXPECT().APICall(gomock.Any(), "CrossModelRelations", 666, "", "WatchConsumedSecretsChanges", args, gomock.Any()).SetArg(6, initialResults).Return(nil)

	client := crossmodelrelations.NewClient(&apicloser{caller})
	w, err := client.WatchConsumedSecretsChanges(context.Background(), "app-token", "rel-token", mac)
	c.Assert(err, tc.ErrorIsNil)
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
			c.Check(ok, tc.IsTrue)
			c.Check(changes, tc.HasLen, 1)
			c.Check(changes[0].URI, tc.DeepEquals, &secrets.URI{ID: "9m4e2mr0ui3e8a215n4g"})
			c.Check(changes[0].Revision, tc.Equals, rev)
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
				URI:            "secret:9m4e2mr0ui3e8a215n4g",
				LatestRevision: 667,
			}},
		}
	}()
	assertChange(667)
}

type migrationSuite struct {
	testing.IsolationSuite
}

var _ = tc.Suite(&migrationSuite{})

func (s *migrationSuite) TestMigrationStatusWatcher(c *tc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	caller := apimocks.NewMockAPICaller(ctrl)
	caller.EXPECT().BestFacadeVersion("MigrationMinion").Return(666).AnyTimes()
	watcherID, eventCh := setupWatcher[*params.MigrationStatus](c, caller, "MigrationStatusWatcher")

	initialResult := params.NotifyWatchResult{
		NotifyWatcherId: watcherID,
	}
	caller.EXPECT().APICall(gomock.Any(), "MigrationMinion", 666, "", "Watch", nil, gomock.Any()).SetArg(6, initialResult).Return(nil)

	client := migrationminion.NewClient(caller)
	w, err := client.Watch(context.Background())
	c.Assert(err, tc.ErrorIsNil)
	defer func() {
		c.Assert(worker.Stop(w), tc.ErrorIsNil)
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
			c.Assert(ok, tc.IsTrue)
			c.Check(status.MigrationId, tc.Equals, id)
			c.Check(status.Phase, tc.Equals, phase)
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
