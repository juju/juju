// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package uniter

import (
	"context"
	"errors"
	"maps"
	"slices"

	"github.com/juju/collections/transform"
	"github.com/juju/names/v6"
	"github.com/juju/tc"
	"github.com/juju/worker/v4"
	"go.uber.org/mock/gomock"

	"github.com/juju/juju/apiserver/common"
	apiservererrors "github.com/juju/juju/apiserver/errors"
	"github.com/juju/juju/core/application"
	coreapplicationtesting "github.com/juju/juju/core/application/testing"
	"github.com/juju/juju/core/relation"
	corerelationtesting "github.com/juju/juju/core/relation/testing"
	"github.com/juju/juju/core/unit"
	coreunittesting "github.com/juju/juju/core/unit/testing"
	"github.com/juju/juju/core/watcher"
	"github.com/juju/juju/core/watcher/watchertest"
	domainrelation "github.com/juju/juju/domain/relation"
	relationerrors "github.com/juju/juju/domain/relation/errors"
	"github.com/juju/juju/internal/testhelpers"
	"github.com/juju/juju/rpc/params"
)

type watcherrelationunitSuite struct {
	testhelpers.IsolationSuite

	relationService *MockRelationService
	watcherRegistry *MockWatcherRegistry

	uniter *UniterAPI
}

var _ = tc.Suite(&watcherrelationunitSuite{})

func (s *watcherrelationunitSuite) setupMocks(c *tc.C) *gomock.Controller {
	ctrl := gomock.NewController(c)

	s.relationService = NewMockRelationService(ctrl)
	s.watcherRegistry = NewMockWatcherRegistry(ctrl)

	s.uniter = &UniterAPI{
		relationService: s.relationService,
		watcherRegistry: s.watcherRegistry,
	}

	return ctrl
}

func (s *watcherrelationunitSuite) TestWatchOneRelationUnitWrongUnitTag(c *tc.C) {
	// Arrange
	defer s.setupMocks(c).Finish()
	relationKey := "relation-app1.ep1#app2.ep2"
	unitTag := "##error##"

	// Act
	_, err := s.uniter.watchOneRelationUnit(context.Background(), func(tag names.Tag) bool {
		return true
	}, params.RelationUnit{
		Relation: relationKey,
		Unit:     unitTag,
	})

	// Assert
	c.Assert(err, tc.ErrorIs, apiservererrors.ErrPerm)
}

func (s *watcherrelationunitSuite) TestWatchOneRelationUnitCannotAccess(c *tc.C) {
	// Arrange
	defer s.setupMocks(c).Finish()
	relationKey := "relation-app1.ep1#app2.ep2"
	unitTag := "unit-app1-0"

	// Act
	_, err := s.uniter.watchOneRelationUnit(context.Background(), func(tag names.Tag) bool {
		return false // cannot access
	}, params.RelationUnit{
		Relation: relationKey,
		Unit:     unitTag,
	})

	// Assert
	c.Assert(err, tc.ErrorIs, apiservererrors.ErrPerm)
}

func (s *watcherrelationunitSuite) TestWatchOneRelationUnitWrongRelationKey(c *tc.C) {
	// Arrange
	defer s.setupMocks(c).Finish()
	relationKey := "##error##"
	unitTag := "unit-app1-0"

	// Act
	_, err := s.uniter.watchOneRelationUnit(context.Background(), func(tag names.Tag) bool {
		return true
	}, params.RelationUnit{
		Relation: relationKey,
		Unit:     unitTag,
	})

	// Assert
	c.Assert(err, tc.ErrorIs, apiservererrors.ErrPerm)
}

func (s *watcherrelationunitSuite) TestWatchOneRelationUnitKeyNotFound(c *tc.C) {
	// Arrange
	defer s.setupMocks(c).Finish()
	relationKey := "relation-app1.ep1#app2.ep2"
	unitTag := "unit-app1-0"

	s.relationService.EXPECT().GetRelationUUIDByKey(
		gomock.Any(), corerelationtesting.GenNewKey(c, "app1:ep1 app2:ep2")).Return("", relationerrors.RelationNotFound)

	// Act
	_, err := s.uniter.watchOneRelationUnit(context.Background(), func(tag names.Tag) bool {
		return true
	}, params.RelationUnit{
		Relation: relationKey,
		Unit:     unitTag,
	})

	// Assert
	c.Assert(err, tc.ErrorIs, apiservererrors.ErrPerm)
}

func (s *watcherrelationunitSuite) TestWatchOneRelationUnitKeyDomainError(c *tc.C) {
	// Arrange
	defer s.setupMocks(c).Finish()
	relationKey := "relation-app1.ep1#app2.ep2"
	unitTag := "unit-app1-0"
	domainError := errors.New("domain error")

	s.relationService.EXPECT().GetRelationUUIDByKey(
		gomock.Any(), corerelationtesting.GenNewKey(c, "app1:ep1 app2:ep2")).Return("", domainError)

	// Act
	_, err := s.uniter.watchOneRelationUnit(context.Background(), func(tag names.Tag) bool {
		return true
	}, params.RelationUnit{
		Relation: relationKey,
		Unit:     unitTag,
	})

	// Assert
	c.Assert(err, tc.ErrorIs, domainError)
}

// TestWatchOneRelationUnit tests the watchOneRelationUnit facade method. It
// tests that the initial event of the watcher is consumed correctly.
func (s *watcherrelationunitSuite) TestWatchOneRelationUnit(c *tc.C) {
	// Arrange
	defer s.setupMocks(c).Finish()
	relationKey := "relation-app1.ep1#app2.ep2"
	unitTag := "unit-app1-0"
	relationUUID := corerelationtesting.GenRelationUUID(c)
	s.relationService.EXPECT().GetRelationUUIDByKey(
		gomock.Any(), corerelationtesting.GenNewKey(c, "app1:ep1 app2:ep2")).Return(relationUUID, nil)

	unitUUIDByName := map[unit.Name]unit.UUID{
		"app1/0": coreunittesting.GenUnitUUID(c),
		"app2/0": coreunittesting.GenUnitUUID(c),
	}
	unitUUIDs := slices.Collect(maps.Values(unitUUIDByName))
	unitNames := slices.Collect(maps.Keys(unitUUIDByName))

	// Generate fake but consistent events from apiserver watcher
	// Initial event: all unit uuids
	initialEvent := transform.Slice(unitUUIDs, domainrelation.EncodeUnitUUID)

	// Initial change: all units.
	initialChange := domainrelation.RelationUnitsChange{
		Changed: transform.SliceToMap(unitNames, func(f unit.Name) (unit.Name, int64) {
			return f, 1
		}),
	}
	s.relationService.EXPECT().GetRelationUnitChanges(gomock.Any(), unitUUIDs, nil).Return(initialChange, nil)

	// Generate watcher id that will be returned by the watcher registry.
	watcherID := "watcher-id"
	var relUnitsWatcher common.RelationUnitsWatcher
	s.watcherRegistry.EXPECT().Register(gomock.Any()).DoAndReturn(func(worker worker.Worker) (string, error) {
		var ok bool
		relUnitsWatcher, ok = worker.(common.RelationUnitsWatcher)
		c.Assert(ok, tc.IsTrue)
		return watcherID, nil
	})

	// The notStartedSafeGuard stops the event producer go-routine leaking if the watcher never
	// starts.
	notStartedSafeGuard := make(chan struct{})
	defer func() {
		close(notStartedSafeGuard)
	}()
	s.relationService.EXPECT().WatchRelatedUnits(gomock.Any(), unit.Name("app1/0"),
		relationUUID).DoAndReturn(func(context.Context, unit.Name, relation.UUID) (watcher.StringsWatcher, error) {
		// Start the event producer, simulating the underlying domain watcher.
		ch := make(chan []string)
		w := watchertest.NewMockStringsWatcher(ch)
		go func() {
			select {
			case <-notStartedSafeGuard:
				c.Errorf("consumer was never started")
				return
				// Send the initial event for the watcher to process.
			case ch <- initialEvent:
			}
		}()
		return w, nil
	})

	// Act:
	result, err := s.uniter.watchOneRelationUnit(context.Background(), func(tag names.Tag) bool {
		return true
	}, params.RelationUnit{
		Relation: relationKey,
		Unit:     unitTag,
	})

	// Assert
	c.Assert(err, tc.IsNil)
	c.Assert(result, tc.DeepEquals, params.RelationUnitsWatchResult{
		RelationUnitsWatcherId: watcherID,
		Changes:                convertRelationUnitsChange(initialChange),
	})
	relUnitsWatcher.Kill()
}

// TestRelationUnitsWatcher checks that the watcher correctly processes and
// emits events.
func (s *watcherrelationunitSuite) TestRelationUnitsWatcher(c *tc.C) {
	// Arrange
	defer s.setupMocks(c).Finish()
	unitTag := names.NewUnitTag("app1/0")
	relationUUID := corerelationtesting.GenRelationUUID(c)

	appUUIDByName := map[string]application.ID{
		"app1": coreapplicationtesting.GenApplicationUUID(c),
		"app2": coreapplicationtesting.GenApplicationUUID(c),
	}
	unitUUIDByName := map[unit.Name]unit.UUID{
		"app1/0": coreunittesting.GenUnitUUID(c),
		"app2/0": coreunittesting.GenUnitUUID(c),
		"app1/1": coreunittesting.GenUnitUUID(c),
		"app2/1": coreunittesting.GenUnitUUID(c),
	}
	appUUIDs := slices.Collect(maps.Values(appUUIDByName))
	unitUUIDs := slices.Collect(maps.Values(unitUUIDByName))
	appNames := slices.Collect(maps.Keys(appUUIDByName))
	unitNames := slices.Collect(maps.Keys(unitUUIDByName))

	// Arrange: Generate fake but consistent events from domain watcher
	events := [][]string{
		// initial event: all unit uuids
		transform.Slice(unitUUIDs, domainrelation.EncodeUnitUUID),
		// second event: everything
		append(transform.Slice(unitUUIDs, domainrelation.EncodeUnitUUID),
			transform.Slice(appUUIDs, domainrelation.EncodeApplicationUUID)...),
		// Third event: all application (unit departed)
		transform.Slice(appUUIDs, domainrelation.EncodeApplicationUUID),
	}

	// Arrange: Generate expected watcher results.
	// First change: all units.
	initialChange := domainrelation.RelationUnitsChange{
		Changed: transform.SliceToMap(unitNames, func(f unit.Name) (unit.Name, int64) {
			return f, 1
		}),
	}
	initial := s.relationService.EXPECT().GetRelationUnitChanges(gomock.Any(), unitUUIDs,
		nil).Return(initialChange, nil)
	// Second change: all units and applications.
	withAppsChange := domainrelation.RelationUnitsChange{
		Changed: transform.SliceToMap(unitNames, func(f unit.Name) (unit.Name, int64) {
			return f, 2
		}),
		AppChanged: transform.SliceToMap(appNames, func(f string) (string, int64) {
			return f, 1
		}),
	}
	withApps := s.relationService.EXPECT().GetRelationUnitChanges(gomock.Any(), unitUUIDs,
		appUUIDs).Return(withAppsChange,
		nil)
	// Third change: all applications (unit departed).
	unitDepartedChange := domainrelation.RelationUnitsChange{
		AppChanged: transform.SliceToMap(appNames, func(f string) (string, int64) {
			return f, 2
		}),
		Departed: unitNames,
	}
	unitDeparted := s.relationService.EXPECT().GetRelationUnitChanges(gomock.Any(), nil, appUUIDs).Return(unitDepartedChange,
		nil)
	gomock.InOrder(initial, withApps, unitDeparted)
	expectedEvents := []domainrelation.RelationUnitsChange{initialChange, withAppsChange, unitDepartedChange}

	// Arrange: Start an event producer when WatchRelationUnits is called.
	unexpectedFinishSafeGuard := make(chan struct{}) // used as a safeguard to avoid deadlock if the event consumer finishes early.
	notStartedSafeGuard := make(chan struct{})       // used as a safeguard to avoid deadlock if the event consumer never starts.
	defer func() {
		close(notStartedSafeGuard)
	}()
	s.relationService.EXPECT().WatchRelatedUnits(gomock.Any(), unit.Name("app1/0"),
		relationUUID).DoAndReturn(func(context.Context, unit.Name, relation.UUID) (watcher.StringsWatcher, error) {
		// Start the event producer, simulating the underlying domain watcher.
		ch := make(chan []string)
		w := watchertest.NewMockStringsWatcher(ch)
		go func() {
			defer close(ch)
			for _, event := range events {
				select {
				case <-unexpectedFinishSafeGuard:
					c.Errorf("watcher finished early")
					return
				case <-notStartedSafeGuard:
					c.Errorf("consumer was never started")
					return
				case ch <- event:
				}
			}
		}()
		return w, nil
	})

	// Act:
	relUnitsWatcher, err := newRelationUnitsWatcher(unitTag, relationUUID, s.relationService)
	c.Assert(err, tc.ErrorIsNil)

	// Assert:
	// Start the watcher event consumer, simulating the uniter.
	watcherEvents := make([]params.RelationUnitsChange, 0, len(expectedEvents))
	for v := range relUnitsWatcher.Changes() { // consume all remaining events
		c.Logf("%+v", v)
		watcherEvents = append(watcherEvents, v)
	}
	c.Check(watcherEvents, tc.DeepEquals, transform.Slice(expectedEvents, convertRelationUnitsChange))
	close(unexpectedFinishSafeGuard)
	relUnitsWatcher.Kill()
}
