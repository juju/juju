// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package localunitrelations

import (
	context "context"
	"testing"

	"github.com/juju/clock"
	"github.com/juju/tc"
	"github.com/juju/worker/v4/workertest"
	"go.uber.org/goleak"
	"go.uber.org/mock/gomock"

	coreapplication "github.com/juju/juju/core/application"
	"github.com/juju/juju/core/errors"
	corerelation "github.com/juju/juju/core/relation"
	watcher "github.com/juju/juju/core/watcher"
	"github.com/juju/juju/core/watcher/watchertest"
	relation "github.com/juju/juju/domain/relation"
	loggertesting "github.com/juju/juju/internal/logger/testing"
)

type localUnitRelationsWorker struct {
	service *MockService

	consumerRelationUUID    corerelation.UUID
	consumerApplicationUUID coreapplication.UUID

	changes chan relation.RelationUnitChange
}

func TestRemoteRelationsWorker(t *testing.T) {
	defer goleak.VerifyNone(t)
	tc.Run(t, &localUnitRelationsWorker{})
}

func (s *localUnitRelationsWorker) SetUpTest(c *tc.C) {
	s.consumerRelationUUID = tc.Must(c, corerelation.NewUUID)
	s.consumerApplicationUUID = tc.Must(c, coreapplication.NewID)

	s.changes = make(chan relation.RelationUnitChange, 1)
}

func (s *localUnitRelationsWorker) TestValidate(c *tc.C) {
	defer s.setupMocks(c).Finish()

	cfg := s.newConfig(c)
	err := cfg.Validate()
	c.Check(err, tc.ErrorIsNil)

	cfg = s.newConfig(c)
	cfg.Service = nil
	err = cfg.Validate()
	c.Check(err, tc.ErrorIs, errors.NotValid)

	cfg = s.newConfig(c)
	cfg.ConsumerApplicationUUID = ""
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

func (s *localUnitRelationsWorker) TestStart(c *tc.C) {
	defer s.setupMocks(c).Finish()

	done := make(chan struct{})
	s.service.EXPECT().WatchRelationUnits(gomock.Any(), s.consumerApplicationUUID).DoAndReturn(func(context.Context, coreapplication.UUID) (watcher.NotifyWatcher, error) {
		defer close(done)
		return watchertest.NewMockNotifyWatcher(make(<-chan struct{})), nil
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

func (s *localUnitRelationsWorker) TestChangeEvent(c *tc.C) {
	defer s.setupMocks(c).Finish()

	ch := make(chan struct{})

	sync := make(chan struct{})
	s.service.EXPECT().WatchRelationUnits(gomock.Any(), s.consumerApplicationUUID).
		DoAndReturn(func(context.Context, coreapplication.UUID) (watcher.NotifyWatcher, error) {
			defer close(sync)
			return watchertest.NewMockNotifyWatcher(ch), nil
		})
	s.service.EXPECT().GetRelationUnits(gomock.Any(), s.consumerApplicationUUID).
		Return(relation.RelationUnitChange{
			ChangedUnits: []relation.UnitChange{{
				UnitID: 0,
				Settings: map[string]any{
					"foo": "baz",
				},
			}},
			AvailableUnits: []int{
				0, 1, 2,
			},
			ApplicationSettings: map[string]any{
				"foo": "bar",
			},
			UnitCount: 3,
			LegacyDepartedUnits: []int{
				4,
			},
		}, nil)

	w := s.newWorker(c, s.newConfig(c))
	defer workertest.DirtyKill(c, w)

	select {
	case <-sync:
	case <-c.Context().Done():
		c.Fatalf("timed out waiting for WatchRelationUnits to be called")
	}

	select {
	case ch <- struct{}{}:
	case <-c.Context().Done():
		c.Fatalf("timed out waiting to send change event")
	}

	var change relation.RelationUnitChange
	select {
	case change = <-s.changes:
	case <-c.Context().Done():
		c.Fatalf("timed out waiting for changes to be sent")
	}

	c.Assert(change, tc.DeepEquals, relation.RelationUnitChange{
		ChangedUnits: []relation.UnitChange{{
			UnitID: 0,
			Settings: map[string]any{
				"foo": "baz",
			},
		}},
		AvailableUnits: []int{0, 1, 2},
		ApplicationSettings: map[string]any{
			"foo": "bar",
		},
		UnitCount: 3,
		LegacyDepartedUnits: []int{
			4,
		},
	})

	workertest.CleanKill(c, w)
}

func (s *localUnitRelationsWorker) newConfig(c *tc.C) Config {
	return Config{
		Service:                 s.service,
		ConsumerApplicationUUID: s.consumerApplicationUUID,
		ConsumerRelationUUID:    s.consumerRelationUUID,
		Changes:                 s.changes,
		Clock:                   clock.WallClock,
		Logger:                  loggertesting.WrapCheckLog(c),
	}
}

func (s *localUnitRelationsWorker) newWorker(c *tc.C, cfg Config) *localWorker {
	w, err := NewWorker(cfg)
	c.Assert(err, tc.ErrorIsNil)

	return w.(*localWorker)
}

func (s *localUnitRelationsWorker) setupMocks(c *tc.C) *gomock.Controller {
	ctrl := gomock.NewController(c)

	s.service = NewMockService(ctrl)

	c.Cleanup(func() {
		s.service = nil
	})

	return ctrl
}
