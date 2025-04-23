// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package uniter

import (
	"context"
	"errors"
	"maps"
	"slices"
	"sync"

	"github.com/juju/collections/transform"
	"github.com/juju/names/v6"
	"github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	"github.com/juju/worker/v4"
	"go.uber.org/mock/gomock"
	gc "gopkg.in/check.v1"

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
	"github.com/juju/juju/rpc/params"
)

type watcherrelationunitSuite struct {
	testing.IsolationSuite

	relationService *MockRelationService
	watcherRegistry *MockWatcherRegistry

	uniter *UniterAPI
}

var _ = gc.Suite(&watcherrelationunitSuite{})

func (s *watcherrelationunitSuite) setupMocks(c *gc.C) *gomock.Controller {
	ctrl := gomock.NewController(c)

	s.relationService = NewMockRelationService(ctrl)
	s.watcherRegistry = NewMockWatcherRegistry(ctrl)

	s.uniter = &UniterAPI{
		relationService: s.relationService,
		watcherRegistry: s.watcherRegistry,
	}

	return ctrl
}

func (s *watcherrelationunitSuite) TestWatchOneRelationUnitWrongUnitTag(c *gc.C) {
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
	c.Assert(err, jc.ErrorIs, apiservererrors.ErrPerm)
}

func (s *watcherrelationunitSuite) TestWatchOneRelationUnitCannotAccess(c *gc.C) {
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
	c.Assert(err, jc.ErrorIs, apiservererrors.ErrPerm)
}

func (s *watcherrelationunitSuite) TestWatchOneRelationUnitWrongRelationKey(c *gc.C) {
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
	c.Assert(err, jc.ErrorIs, apiservererrors.ErrPerm)
}

func (s *watcherrelationunitSuite) TestWatchOneRelationUnitKeyNotFound(c *gc.C) {
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
	c.Assert(err, jc.ErrorIs, apiservererrors.ErrPerm)
}

func (s *watcherrelationunitSuite) TestWatchOneRelationUnitKeyDomainError(c *gc.C) {
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
	c.Assert(err, jc.ErrorIs, domainError)
}

// TestWatchOneRelationUnit tests the functionality of watching a single relation unit and handling the associated events.
func (s *watcherrelationunitSuite) TestWatchOneRelationUnit(c *gc.C) {
	// Arrange
	defer s.setupMocks(c).Finish()
	relationKey := "relation-app1.ep1#app2.ep2"
	unitTag := "unit-app1-0"
	relationUUID := corerelationtesting.GenRelationUUID(c)
	s.relationService.EXPECT().GetRelationUUIDByKey(
		gomock.Any(), corerelationtesting.GenNewKey(c, "app1:ep1 app2:ep2")).Return(relationUUID, nil)

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

	// Generate fake but consistent events from domain watcher
	events := [][]string{
		// initial event: all unit uuids
		transform.Slice(unitUUIDs, domainrelation.EncodeUnitUUID),
		// second event: everything
		append(transform.Slice(unitUUIDs, domainrelation.EncodeUnitUUID),
			transform.Slice(appUUIDs, domainrelation.EncodeApplicationUUID)...),
		// Third event: all application (unit departed)
		transform.Slice(appUUIDs, domainrelation.EncodeApplicationUUID),
	}

	// Generate fake but consistent events from apiserver watcher
	// Initial change: all units.
	initialChange := watcher.RelationUnitsChange{
		Changed: transform.SliceToMap(unitNames, func(f unit.Name) (string, watcher.UnitSettings) {
			return f.String(), watcher.UnitSettings{Version: 1}
		}),
	}
	// Second change: all units and applications.
	withAppsChange := watcher.RelationUnitsChange{
		Changed: transform.SliceToMap(unitNames, func(f unit.Name) (string, watcher.UnitSettings) {
			return f.String(), watcher.UnitSettings{Version: 2}
		}),
		AppChanged: transform.SliceToMap(appNames, func(f string) (string, int64) {
			return f, 1
		}),
	}
	// Third change: all applications (unit departed).
	unitDepartedChange := watcher.RelationUnitsChange{
		AppChanged: transform.SliceToMap(appNames, func(f string) (string, int64) {
			return f, 2
		}),
		Departed: transform.Slice(unitNames, unit.Name.String),
	}

	// Generate watcher id that will be returned by the watcher registry.
	watcherID := "watcher-id"

	initial := s.relationService.EXPECT().GetRelationUnitChanges(gomock.Any(), unitUUIDs,
		nil).Return(initialChange, nil)
	withApps := s.relationService.EXPECT().GetRelationUnitChanges(gomock.Any(), unitUUIDs,
		appUUIDs).Return(withAppsChange,
		nil)
	unitDeparted := s.relationService.EXPECT().GetRelationUnitChanges(gomock.Any(), nil, appUUIDs).Return(unitDepartedChange,
		nil)

	// Check that the event will be received in order by the proxy watcher
	gomock.InOrder(initial, withApps, unitDeparted)

	// Test the proxy watcher
	defer s.expectWatchRelatedUnitWithEvents(c, relationUUID, watcherInitParams{
		events:          events,
		firstEventFetch: initial,
		watcherID:       watcherID,
		expectedEvents:  []watcher.RelationUnitsChange{withAppsChange, unitDepartedChange},
	}).Finish(c)

	// Act
	result, err := s.uniter.watchOneRelationUnit(context.Background(), func(tag names.Tag) bool {
		return true
	}, params.RelationUnit{
		Relation: relationKey,
		Unit:     unitTag,
	})

	// Assert
	c.Assert(err, gc.IsNil)
	c.Assert(result, gc.DeepEquals, params.RelationUnitsWatchResult{
		RelationUnitsWatcherId: watcherID,
		Changes:                convertRelationUnitsChange(initialChange),
	})
}

func (s *watcherrelationunitSuite) TestWatchOneRelationUnitNoEvent(c *gc.C) {
	// Arrange
	defer s.setupMocks(c).Finish()
	relationKey := "relation-app1.ep1#app2.ep2"
	unitTag := "unit-app1-0"
	relationUUID := corerelationtesting.GenRelationUUID(c)

	s.relationService.EXPECT().GetRelationUUIDByKey(
		gomock.Any(), corerelationtesting.GenNewKey(c, "app1:ep1 app2:ep2")).Return(relationUUID, nil)
	s.expectWatchRelatedUnitNoEvent(c, relationUUID)

	// Act
	result, err := s.uniter.watchOneRelationUnit(context.Background(), func(tag names.Tag) bool {
		return true
	}, params.RelationUnit{
		Relation: relationKey,
		Unit:     unitTag,
	})

	// Assert
	c.Assert(err, gc.IsNil)
	c.Assert(result, gc.DeepEquals, params.RelationUnitsWatchResult{})
}

type finish func(c *gc.C)

func (f finish) Finish(c *gc.C) {
	f(c)
}

func (s *watcherrelationunitSuite) expectWatchRelatedUnitNoEvent(c *gc.C, relationUUID relation.UUID) {
	s.relationService.EXPECT().WatchRelatedUnits(gomock.Any(), unit.Name("app1/0"),
		relationUUID).DoAndReturn(func(context.Context, unit.Name, relation.UUID) (watcher.StringsWatcher, error) {
		ch := make(chan []string)
		defer close(ch)
		return watchertest.NewMockStringsWatcher(ch), nil
	})
}

type watcherInitParams struct {
	// events are the events that will be returned by the domain watcher and translated by the proxy watcher.
	events [][]string
	// firstEventFetch represents the initial event fetch operation performed by the watcher during initialization.
	// it should be called before registering
	firstEventFetch any
	// watcherID is a unique identifier for the watcher, utilized for tracking and managing watcher instances.
	watcherID string
	// expectedEvents are the events that will be returned by the proxy watcher, except the initial one.
	expectedEvents []watcher.RelationUnitsChange
}

// expectWatchRelatedUnitWithEvents sets up and validates a mock watcher for related unit events in a specific relation context.
// It relies on a MockStringWatcher to mimics the behavior of the domain watcher, and run two goroutines:
// - one that will consume the events from the domain watcher and send them to the proxy watcher
// - one that will consume the events from the proxy watcher and validate them.
// It returns a function finisher to be sure everything is cleaned up properly and not before watcher have stops.
func (s *watcherrelationunitSuite) expectWatchRelatedUnitWithEvents(c *gc.C, relationUUID relation.UUID,
	init watcherInitParams) finish {
	wg := sync.WaitGroup{}
	notStartedSafeGuard := make(chan struct{})       // used as a safeguard to avoid deadlock if the watcher is not started
	unexpectedFinishSafeGuard := make(chan struct{}) // used as a safeguard to avoid deadlock if the proxy finish early
	finisher := func(c *gc.C) {
		if notStartedSafeGuard != nil {
			close(notStartedSafeGuard)
		}
		wg.Wait()
	}
	runWatcher := s.relationService.EXPECT().WatchRelatedUnits(gomock.Any(), unit.Name("app1/0"),
		relationUUID).DoAndReturn(func(context.Context, unit.Name, relation.UUID) (watcher.StringsWatcher, error) {
		ch := make(chan []string)
		w := watchertest.NewMockStringsWatcher(ch)
		wg.Add(1)
		go func() {
			defer close(ch)
			defer wg.Done()
			for _, event := range init.events {
				select {
				case <-notStartedSafeGuard:
					c.Errorf("watcher did not start")
					return
				case <-unexpectedFinishSafeGuard:
					c.Errorf("watcher finished early")
					return
				case ch <- event:
				}
			}
		}()
		return w, nil
	})
	registerWatcher := s.watcherRegistry.EXPECT().Register(gomock.Any()).DoAndReturn(func(worker worker.Worker) (string, error) {
		w, ok := worker.(common.RelationUnitsWatcher)
		c.Assert(ok, jc.IsTrue)
		notStartedSafeGuard = nil // allow watcher to finish, since it is started
		go func() {
			wg.Add(1)
			defer wg.Done()
			watcherEvents := make([]params.RelationUnitsChange, 0, len(init.expectedEvents))
			for v := range w.Changes() { // consume all remaining events
				c.Logf("%+v", v)
				watcherEvents = append(watcherEvents, v)
			}
			c.Check(watcherEvents, gc.DeepEquals, transform.Slice(init.expectedEvents, convertRelationUnitsChange))
			close(unexpectedFinishSafeGuard)
		}()

		return init.watcherID, nil
	})

	gomock.InOrder(
		runWatcher,
		init.firstEventFetch,
		registerWatcher,
	)

	return finisher
}
