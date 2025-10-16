// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package offererunitrelations

import (
	context "context"
	"testing"
	"time"

	"github.com/juju/clock"
	"github.com/juju/tc"
	"github.com/juju/worker/v4/workertest"
	"go.uber.org/goleak"
	"go.uber.org/mock/gomock"
	"gopkg.in/macaroon.v2"

	"github.com/juju/juju/api/watcher"
	coreapplication "github.com/juju/juju/core/application"
	"github.com/juju/juju/core/errors"
	"github.com/juju/juju/core/life"
	corerelation "github.com/juju/juju/core/relation"
	"github.com/juju/juju/core/watcher/watchertest"
	loggertesting "github.com/juju/juju/internal/logger/testing"
	"github.com/juju/juju/rpc/params"
)

type offererUnitRelationsWorker struct {
	client *MockRemoteModelRelationsClient

	consumerRelationUUID   corerelation.UUID
	offererApplicationUUID coreapplication.UUID

	macaroon *macaroon.Macaroon

	changes chan RelationUnitChange
}

func TestOffererUnitRelationsWorker(t *testing.T) {
	defer goleak.VerifyNone(t)
	tc.Run(t, &offererUnitRelationsWorker{})
}

func (s *offererUnitRelationsWorker) SetUpTest(c *tc.C) {
	s.consumerRelationUUID = tc.Must(c, corerelation.NewUUID)
	s.offererApplicationUUID = tc.Must(c, coreapplication.NewID)

	s.macaroon = newMacaroon(c, "test")

	s.changes = make(chan RelationUnitChange, 1)
}

func (s *offererUnitRelationsWorker) TestValidate(c *tc.C) {
	defer s.setupMocks(c).Finish()

	cfg := s.newConfig(c)
	err := cfg.Validate()
	c.Check(err, tc.ErrorIsNil)

	cfg = s.newConfig(c)
	cfg.Client = nil
	err = cfg.Validate()
	c.Check(err, tc.ErrorIs, errors.NotValid)

	cfg = s.newConfig(c)
	cfg.OffererApplicationUUID = ""
	err = cfg.Validate()
	c.Check(err, tc.ErrorIs, errors.NotValid)

	cfg = s.newConfig(c)
	cfg.ConsumerRelationUUID = ""
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

func (s *offererUnitRelationsWorker) TestStart(c *tc.C) {
	defer s.setupMocks(c).Finish()

	done := make(chan struct{})
	s.client.EXPECT().WatchRelationChanges(gomock.Any(),
		s.consumerRelationUUID.String(), s.offererApplicationUUID.String(), macaroon.Slice{s.macaroon}).
		DoAndReturn(func(ctx context.Context, s1, s2 string, s3 macaroon.Slice) (watcher.RemoteRelationWatcher, error) {
			defer close(done)
			return watchertest.NewMockWatcher(make(<-chan params.RemoteRelationChangeEvent)), nil
		})

	w := s.newWorker(c, s.newConfig(c))
	defer workertest.DirtyKill(c, w)

	select {
	case <-done:
	case <-c.Context().Done():
		c.Fatalf("timed out waiting for WatchRelationUnits to be called")
	}

	workertest.CleanKill(c, w)
}

func (s *offererUnitRelationsWorker) TestChangeEvent(c *tc.C) {
	defer s.setupMocks(c).Finish()

	ch := make(chan params.RemoteRelationChangeEvent)

	sync := make(chan struct{})
	s.client.EXPECT().WatchRelationChanges(gomock.Any(),
		s.consumerRelationUUID.String(), s.offererApplicationUUID.String(), macaroon.Slice{s.macaroon}).
		DoAndReturn(func(ctx context.Context, s1, s2 string, s3 macaroon.Slice) (watcher.RemoteRelationWatcher, error) {
			defer close(sync)
			return watchertest.NewMockWatcher(ch), nil
		})

	w := s.newWorker(c, s.newConfig(c))
	defer workertest.DirtyKill(c, w)

	select {
	case <-sync:
	case <-c.Context().Done():
		c.Fatalf("timed out waiting for WatchRelationUnits to be called")
	}

	select {
	case ch <- params.RemoteRelationChangeEvent{
		RelationToken: s.consumerRelationUUID.String(),
		ChangedUnits: []params.RemoteRelationUnitChange{{
			UnitId: 0,
			Settings: map[string]any{
				"foo": "baz",
			},
		}},
		ApplicationSettings: map[string]any{
			"foo": "bar",
		},
		UnitCount:       3,
		Life:            "alive",
		Suspended:       ptr(true),
		SuspendedReason: "because",
		DepartedUnits: []int{
			4,
		},
	}:
	case <-c.Context().Done():
		c.Fatalf("timed out waiting to send change event")
	}

	var change RelationUnitChange
	select {
	case change = <-s.changes:
	case <-c.Context().Done():
		c.Fatalf("timed out waiting for changes to be sent")
	}

	c.Assert(change, tc.DeepEquals, RelationUnitChange{
		ConsumerRelationUUID:   s.consumerRelationUUID,
		OffererApplicationUUID: s.offererApplicationUUID,
		ChangedUnits: []UnitChange{{
			UnitID: 0,
			Settings: map[string]string{
				"foo": "baz",
			},
		}},
		ApplicationSettings: map[string]string{
			"foo": "bar",
		},
		DeprecatedDepartedUnits: []int{
			4,
		},
		Life:            "alive",
		Suspended:       true,
		SuspendedReason: "because",
	})

	workertest.CleanKill(c, w)
}

func (s *offererUnitRelationsWorker) TestChangeEventIsEmpty(c *tc.C) {
	defer s.setupMocks(c).Finish()

	ch := make(chan params.RemoteRelationChangeEvent)

	sync := make(chan struct{})
	s.client.EXPECT().WatchRelationChanges(gomock.Any(),
		s.consumerRelationUUID.String(), s.offererApplicationUUID.String(), macaroon.Slice{s.macaroon}).
		DoAndReturn(func(ctx context.Context, s1, s2 string, s3 macaroon.Slice) (watcher.RemoteRelationWatcher, error) {
			defer close(sync)
			return watchertest.NewMockWatcher(ch), nil
		})

	w := s.newWorker(c, s.newConfig(c))
	defer workertest.DirtyKill(c, w)

	select {
	case <-sync:
	case <-c.Context().Done():
		c.Fatalf("timed out waiting for WatchRelationUnits to be called")
	}

	select {
	case ch <- params.RemoteRelationChangeEvent{}:
	case <-c.Context().Done():
		c.Fatalf("timed out waiting to send change event")
	}

	select {
	case <-s.changes:
		c.Fatalf("unexpected change received")
	case <-time.After(500 * time.Millisecond):
	}

	workertest.CleanKill(c, w)
}

func (s *offererUnitRelationsWorker) TestReport(c *tc.C) {
	defer s.setupMocks(c).Finish()

	ch := make(chan params.RemoteRelationChangeEvent)

	sync := make(chan struct{})
	s.client.EXPECT().WatchRelationChanges(gomock.Any(),
		s.consumerRelationUUID.String(), s.offererApplicationUUID.String(), macaroon.Slice{s.macaroon}).
		DoAndReturn(func(ctx context.Context, s1, s2 string, s3 macaroon.Slice) (watcher.RemoteRelationWatcher, error) {
			defer close(sync)
			return watchertest.NewMockWatcher(ch), nil
		})

	w := s.newWorker(c, s.newConfig(c))
	defer workertest.DirtyKill(c, w)

	select {
	case <-sync:
	case <-c.Context().Done():
		c.Fatalf("timed out waiting for WatchRelationUnits to be called")
	}

	c.Assert(w.Report(), tc.DeepEquals, map[string]any{
		"offerer-application-uuid": s.offererApplicationUUID.String(),
		"consumer-relation-uuid":   s.consumerRelationUUID.String(),
		"changed-units":            []map[string]any(nil),
		"settings":                 map[string]string(nil),
		"departed-units":           []int(nil),
		"life":                     life.Value(""),
		"suspended":                false,
		"suspended-reason":         "",
	})

	select {
	case ch <- params.RemoteRelationChangeEvent{
		RelationToken: s.consumerRelationUUID.String(),
		ChangedUnits: []params.RemoteRelationUnitChange{{
			UnitId: 0,
			Settings: map[string]any{
				"foo": "baz",
			},
		}},
		ApplicationSettings: map[string]any{
			"foo": "bar",
		},
		UnitCount:       3,
		Life:            "alive",
		Suspended:       ptr(true),
		SuspendedReason: "because",
		DepartedUnits: []int{
			4,
		},
	}:
	case <-c.Context().Done():
		c.Fatalf("timed out waiting to send change event")
	}

	c.Assert(w.Report(), tc.DeepEquals, map[string]any{
		"offerer-application-uuid": s.offererApplicationUUID.String(),
		"consumer-relation-uuid":   s.consumerRelationUUID.String(),
		"changed-units": []map[string]any{{
			"unit-id": 0,
			"settings": map[string]string{
				"foo": "baz",
			},
		}},
		"settings": map[string]string{
			"foo": "bar",
		},
		"departed-units": []int{
			4,
		},
		"life":             life.Alive,
		"suspended":        true,
		"suspended-reason": "because",
	})

	workertest.CleanKill(c, w)
}

func (s *offererUnitRelationsWorker) newConfig(c *tc.C) Config {
	return Config{
		Client:                 s.client,
		OffererApplicationUUID: s.offererApplicationUUID,
		ConsumerRelationUUID:   s.consumerRelationUUID,
		Macaroon:               s.macaroon,
		Changes:                s.changes,
		Clock:                  clock.WallClock,
		Logger:                 loggertesting.WrapCheckLog(c),
	}
}

func (s *offererUnitRelationsWorker) newWorker(c *tc.C, cfg Config) *remoteWorker {
	w, err := NewWorker(cfg)
	c.Assert(err, tc.ErrorIsNil)

	return w.(*remoteWorker)
}

func (s *offererUnitRelationsWorker) setupMocks(c *tc.C) *gomock.Controller {
	ctrl := gomock.NewController(c)

	s.client = NewMockRemoteModelRelationsClient(ctrl)

	c.Cleanup(func() {
		s.client = nil
	})

	return ctrl
}
