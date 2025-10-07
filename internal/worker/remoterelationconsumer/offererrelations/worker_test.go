// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package offererrelations

import (
	context "context"
	"testing"
	"time"

	"github.com/go-macaroon-bakery/macaroon-bakery/v3/bakery"
	"github.com/juju/clock"
	"github.com/juju/tc"
	"github.com/juju/worker/v4/workertest"
	"go.uber.org/goleak"
	"go.uber.org/mock/gomock"
	"gopkg.in/macaroon.v2"

	coreapplication "github.com/juju/juju/core/application"
	"github.com/juju/juju/core/errors"
	"github.com/juju/juju/core/life"
	corerelation "github.com/juju/juju/core/relation"
	watcher "github.com/juju/juju/core/watcher"
	"github.com/juju/juju/core/watcher/watchertest"
	loggertesting "github.com/juju/juju/internal/logger/testing"
	"github.com/juju/juju/rpc/params"
)

type offererRelationsWorkerSuite struct {
	client *MockRemoteModelRelationsClient

	consumerRelationUUID   corerelation.UUID
	offererApplicationUUID coreapplication.UUID
	macaroon               *macaroon.Macaroon

	changes chan RelationChange
}

func TestOffererRelationsWorker(t *testing.T) {
	defer goleak.VerifyNone(t)
	tc.Run(t, &offererRelationsWorkerSuite{})
}

func (s *offererRelationsWorkerSuite) SetUpTest(c *tc.C) {
	s.consumerRelationUUID = tc.Must(c, corerelation.NewUUID)
	s.offererApplicationUUID = tc.Must(c, coreapplication.NewID)

	s.macaroon = newMacaroon(c, "test")

	s.changes = make(chan RelationChange, 1)
}

func (s *offererRelationsWorkerSuite) TestValidate(c *tc.C) {
	defer s.setupMocks(c).Finish()

	cfg := s.newConfig(c)
	err := cfg.Validate()
	c.Check(err, tc.ErrorIsNil)

	cfg = s.newConfig(c)
	cfg.Client = nil
	err = cfg.Validate()
	c.Check(err, tc.ErrorIs, errors.NotValid)

	cfg = s.newConfig(c)
	cfg.ConsumerRelationUUID = ""
	err = cfg.Validate()
	c.Check(err, tc.ErrorIs, errors.NotValid)

	cfg = s.newConfig(c)
	cfg.OffererApplicationUUID = ""
	err = cfg.Validate()
	c.Check(err, tc.ErrorIs, errors.NotValid)

	cfg = s.newConfig(c)
	cfg.Macaroon = nil
	err = cfg.Validate()
	c.Check(err, tc.ErrorIs, errors.NotValid)

	cfg = s.newConfig(c)
	cfg.Changes = nil
	err = cfg.Validate()
	c.Check(err, tc.ErrorIs, errors.NotValid)

	cfg = s.newConfig(c)
	cfg.Clock = nil
	err = cfg.Validate()
	c.Check(err, tc.ErrorIs, errors.NotValid)

	cfg = s.newConfig(c)
	cfg.Logger = nil
	err = cfg.Validate()
	c.Check(err, tc.ErrorIs, errors.NotValid)
}

func (s *offererRelationsWorkerSuite) TestStart(c *tc.C) {
	defer s.setupMocks(c).Finish()

	done := make(chan struct{})
	s.client.EXPECT().WatchRelationSuspendedStatus(gomock.Any(), params.RemoteEntityArg{
		Token:         s.consumerRelationUUID.String(),
		Macaroons:     macaroon.Slice{s.macaroon},
		BakeryVersion: bakery.LatestVersion,
	}).DoAndReturn(func(context.Context, params.RemoteEntityArg) (watcher.RelationStatusWatcher, error) {
		defer close(done)
		return watchertest.NewMockWatcher(make(<-chan []watcher.RelationStatusChange)), nil
	})

	w := s.newWorker(c, s.newConfig(c))
	defer workertest.DirtyKill(c, w)

	select {
	case <-done:
	case <-c.Context().Done():
		c.Fatalf("timed out waiting for WatchRelationSuspendedStatus to be called")
	}

	workertest.CleanKill(c, w)
}

func (s *offererRelationsWorkerSuite) TestChangeEvent(c *tc.C) {
	defer s.setupMocks(c).Finish()

	ch := make(chan []watcher.RelationStatusChange)

	sync := make(chan struct{})
	s.client.EXPECT().WatchRelationSuspendedStatus(gomock.Any(), params.RemoteEntityArg{
		Token:         s.consumerRelationUUID.String(),
		Macaroons:     macaroon.Slice{s.macaroon},
		BakeryVersion: bakery.LatestVersion,
	}).DoAndReturn(func(context.Context, params.RemoteEntityArg) (watcher.RelationStatusWatcher, error) {
		defer close(sync)
		return watchertest.NewMockWatcher(ch), nil
	})

	w := s.newWorker(c, s.newConfig(c))
	defer workertest.DirtyKill(c, w)

	select {
	case <-sync:
	case <-c.Context().Done():
		c.Fatalf("timed out waiting for WatchRelationSuspendedStatus to be called")
	}

	select {
	case ch <- []watcher.RelationStatusChange{{
		Key:             "key",
		Life:            life.Alive,
		Suspended:       true,
		SuspendedReason: "because I said so",
	}}:
	case <-c.Context().Done():
		c.Fatalf("timed out waiting to send change")
	}

	var change RelationChange
	select {
	case change = <-s.changes:
	case <-c.Context().Done():
		c.Fatalf("timed out waiting for changes to be sent")
	}

	c.Assert(change, tc.DeepEquals, RelationChange{
		ConsumerRelationUUID:   s.consumerRelationUUID,
		OffererApplicationUUID: s.offererApplicationUUID,
		Life:                   life.Alive,
		Suspended:              true,
		SuspendedReason:        "because I said so",
	})

	workertest.CleanKill(c, w)
}

func (s *offererRelationsWorkerSuite) TestChangeEvents(c *tc.C) {
	defer s.setupMocks(c).Finish()

	ch := make(chan []watcher.RelationStatusChange)

	sync := make(chan struct{})
	s.client.EXPECT().WatchRelationSuspendedStatus(gomock.Any(), params.RemoteEntityArg{
		Token:         s.consumerRelationUUID.String(),
		Macaroons:     macaroon.Slice{s.macaroon},
		BakeryVersion: bakery.LatestVersion,
	}).DoAndReturn(func(context.Context, params.RemoteEntityArg) (watcher.RelationStatusWatcher, error) {
		defer close(sync)
		return watchertest.NewMockWatcher(ch), nil
	})

	w := s.newWorker(c, s.newConfig(c))
	defer workertest.DirtyKill(c, w)

	select {
	case <-sync:
	case <-c.Context().Done():
		c.Fatalf("timed out waiting for WatchRelationSuspendedStatus to be called")
	}

	select {
	case ch <- []watcher.RelationStatusChange{{
		Key:             "key1",
		Life:            life.Dead,
		Suspended:       false,
		SuspendedReason: "some reason 1",
	}, {
		Key:             "key2",
		Life:            life.Dying,
		Suspended:       false,
		SuspendedReason: "some reason 2",
	}, {
		Key:             "key3",
		Life:            life.Alive,
		Suspended:       true,
		SuspendedReason: "some reason 3",
	}}:
	case <-c.Context().Done():
		c.Fatalf("timed out waiting to send change")
	}

	var change RelationChange
	select {
	case change = <-s.changes:
	case <-c.Context().Done():
		c.Fatalf("timed out waiting for changes to be sent")
	}

	c.Assert(change, tc.DeepEquals, RelationChange{
		ConsumerRelationUUID:   s.consumerRelationUUID,
		OffererApplicationUUID: s.offererApplicationUUID,
		Life:                   life.Alive,
		Suspended:              true,
		SuspendedReason:        "some reason 3",
	})

	workertest.CleanKill(c, w)
}

func (s *offererRelationsWorkerSuite) TestChangeNoEvents(c *tc.C) {
	defer s.setupMocks(c).Finish()

	ch := make(chan []watcher.RelationStatusChange)

	sync := make(chan struct{})
	s.client.EXPECT().WatchRelationSuspendedStatus(gomock.Any(), params.RemoteEntityArg{
		Token:         s.consumerRelationUUID.String(),
		Macaroons:     macaroon.Slice{s.macaroon},
		BakeryVersion: bakery.LatestVersion,
	}).DoAndReturn(func(context.Context, params.RemoteEntityArg) (watcher.RelationStatusWatcher, error) {
		defer close(sync)
		return watchertest.NewMockWatcher(ch), nil
	})

	w := s.newWorker(c, s.newConfig(c))
	defer workertest.DirtyKill(c, w)

	select {
	case <-sync:
	case <-c.Context().Done():
		c.Fatalf("timed out waiting for WatchRelationSuspendedStatus to be called")
	}

	select {
	case ch <- []watcher.RelationStatusChange{}:
	case <-c.Context().Done():
		c.Fatalf("timed out waiting to send change")
	}

	// We expect to not receive any change events.
	select {
	case <-s.changes:
		c.Fatalf("unexpected change received")
	case <-time.After(500 * time.Millisecond):
	}
}

func (s *offererRelationsWorkerSuite) TestReport(c *tc.C) {
	defer s.setupMocks(c).Finish()

	ch := make(chan []watcher.RelationStatusChange)

	sync := make(chan struct{})
	s.client.EXPECT().WatchRelationSuspendedStatus(gomock.Any(), params.RemoteEntityArg{
		Token:         s.consumerRelationUUID.String(),
		Macaroons:     macaroon.Slice{s.macaroon},
		BakeryVersion: bakery.LatestVersion,
	}).DoAndReturn(func(context.Context, params.RemoteEntityArg) (watcher.RelationStatusWatcher, error) {
		defer close(sync)
		return watchertest.NewMockWatcher(ch), nil
	})

	w := s.newWorker(c, s.newConfig(c))
	defer workertest.DirtyKill(c, w)

	select {
	case <-sync:
	case <-c.Context().Done():
		c.Fatalf("timed out waiting for WatchRelationSuspendedStatus to be called")
	}

	c.Assert(w.Report(), tc.DeepEquals, map[string]any{
		"consumer-relation-uuid":   s.consumerRelationUUID.String(),
		"offerer-application-uuid": s.offererApplicationUUID.String(),
		"life":                     "",
		"suspended":                false,
		"suspended-reason":         "",
	})

	select {
	case ch <- []watcher.RelationStatusChange{{
		Key:             "key",
		Life:            life.Alive,
		Suspended:       true,
		SuspendedReason: "because I said so",
	}}:
	case <-c.Context().Done():
		c.Fatalf("timed out waiting to send change")
	}

	select {
	case <-s.changes:
	case <-c.Context().Done():
		c.Fatalf("timed out waiting for changes to be sent")
	}

	c.Assert(w.Report(), tc.DeepEquals, map[string]any{
		"consumer-relation-uuid":   s.consumerRelationUUID.String(),
		"offerer-application-uuid": s.offererApplicationUUID.String(),
		"life":                     "alive",
		"suspended":                true,
		"suspended-reason":         "because I said so",
	})

	workertest.CleanKill(c, w)
}

func (s *offererRelationsWorkerSuite) newConfig(c *tc.C) Config {
	return Config{
		Client:                 s.client,
		ConsumerRelationUUID:   s.consumerRelationUUID,
		OffererApplicationUUID: s.offererApplicationUUID,
		Macaroon:               s.macaroon,
		Changes:                s.changes,
		Clock:                  clock.WallClock,
		Logger:                 loggertesting.WrapCheckLog(c),
	}
}

func (s *offererRelationsWorkerSuite) newWorker(c *tc.C, cfg Config) *offererRelationsWorker {
	w, err := NewWorker(cfg)
	c.Assert(err, tc.ErrorIsNil)

	return w.(*offererRelationsWorker)
}

func (s *offererRelationsWorkerSuite) setupMocks(c *tc.C) *gomock.Controller {
	ctrl := gomock.NewController(c)

	s.client = NewMockRemoteModelRelationsClient(ctrl)

	c.Cleanup(func() {
		s.client = nil
	})

	return ctrl
}

func newMacaroon(c *tc.C, id string) *macaroon.Macaroon {
	mac, err := macaroon.New(nil, []byte(id), "", macaroon.LatestVersion)
	c.Assert(err, tc.ErrorIsNil)
	return mac
}
