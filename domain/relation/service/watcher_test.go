// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package service

import (
	"context"

	"github.com/juju/collections/set"
	jujutesting "github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	"go.uber.org/mock/gomock"
	gc "gopkg.in/check.v1"

	coreapplication "github.com/juju/juju/core/application"
	applicationtesting "github.com/juju/juju/core/application/testing"
	"github.com/juju/juju/core/changestream"
	changestreammock "github.com/juju/juju/core/changestream/mocks"
	"github.com/juju/juju/core/life"
	corerelation "github.com/juju/juju/core/relation"
	"github.com/juju/juju/core/relation/testing"
	"github.com/juju/juju/domain/relation"
	relationerrors "github.com/juju/juju/domain/relation/errors"
	"github.com/juju/juju/internal/charm"
	loggertesting "github.com/juju/juju/internal/logger/testing"
)

// Test the internal functionality of the complex watchers.

type watcherSuite struct {
	jujutesting.IsolationSuite

	state          *MockState
	watcherFactory *MockWatcherFactory

	service *WatchableService
}

var _ = gc.Suite(&watcherSuite{})

// TestSubordinateSendChangeEventRelationScopeGlobal tests that if the
// subordinate unit's relation endpoint is global scoped, an event is
// sent.
func (s *watcherSuite) TestSubordinateSendChangeEventRelationScopeGlobal(c *gc.C) {
	// Arrange
	defer s.setupMocks(c).Finish()
	relUUID := testing.GenRelationUUID(c)
	principalID := applicationtesting.GenApplicationUUID(c)
	subordinateID := applicationtesting.GenApplicationUUID(c)
	scope := charm.ScopeGlobal

	s.expectGetRelationEndpointScope(relUUID, subordinateID, scope, nil)

	// Act
	watcher := &subordinateLifeSuspendedStatusWatcher{
		s:           s.service,
		parentAppID: principalID,
		appID:       subordinateID,
	}
	change, err := watcher.watchNewRelation(
		context.Background(),
		relUUID,
	)

	// Assert
	c.Assert(err, gc.IsNil)
	c.Assert(change, jc.IsTrue)
}

// TestSubordinateSendChangeEventRelationAnotherSubordinate tests that if the
// subordinate unit's relation endpoint is container scoped, and the other
// end is also a subordinate, an event is sent.
func (s *watcherSuite) TestSubordinateSendChangeEventRelationAnotherSubordinate(c *gc.C) {
	// Arrange:
	defer s.setupMocks(c).Finish()
	relUUID := testing.GenRelationUUID(c)
	principalID := applicationtesting.GenApplicationUUID(c)
	subordinateID := applicationtesting.GenApplicationUUID(c)
	anotherSubordinateID := applicationtesting.GenApplicationUUID(c)
	scope := charm.ScopeContainer
	otherAppData := relation.OtherApplicationForWatcher{
		ApplicationID: anotherSubordinateID,
		Subordinate:   true,
	}

	s.expectGetRelationEndpointScope(relUUID, subordinateID, scope, nil)
	s.expectGetOtherRelatedEndpointApplicationData(relUUID, subordinateID, otherAppData, nil)

	// Act
	watcher := &subordinateLifeSuspendedStatusWatcher{
		s:           s.service,
		parentAppID: principalID,
		appID:       subordinateID,
	}
	change, err := watcher.watchNewRelation(
		context.Background(),
		relUUID,
	)

	// Assert
	c.Assert(err, gc.IsNil)
	c.Assert(change, jc.IsTrue)
}

// TestSubordinateSendChangeEventRelationPrincipal tests that if the
// subordinate unit's relation endpoint is container scoped, and the other
// end application is the principal application, an event is sent.
func (s *watcherSuite) TestSubordinateSendChangeEventRelationPrincipal(c *gc.C) {
	// Arrange:
	defer s.setupMocks(c).Finish()
	relUUID := testing.GenRelationUUID(c)
	principalID := applicationtesting.GenApplicationUUID(c)
	subordinateID := applicationtesting.GenApplicationUUID(c)
	scope := charm.ScopeContainer
	otherAppData := relation.OtherApplicationForWatcher{
		ApplicationID: principalID,
	}

	s.expectGetRelationEndpointScope(relUUID, subordinateID, scope, nil)
	s.expectGetOtherRelatedEndpointApplicationData(relUUID, subordinateID, otherAppData, nil)

	// Act
	watcher := &subordinateLifeSuspendedStatusWatcher{
		s:           s.service,
		parentAppID: principalID,
		appID:       subordinateID,
	}
	change, err := watcher.watchNewRelation(
		context.Background(),
		relUUID,
	)

	// Assert
	c.Assert(err, gc.IsNil)
	c.Assert(change, jc.IsTrue)
}

// TestSubordinateSendChangeEventRelationNoChange checks that no change is requested
// while all state calls are successful.
func (s *watcherSuite) TestSubordinateSendChangeEventRelationNoChange(c *gc.C) {
	// Arrange:
	defer s.setupMocks(c).Finish()
	relUUID := testing.GenRelationUUID(c)
	principalID := applicationtesting.GenApplicationUUID(c)
	subordinateID := applicationtesting.GenApplicationUUID(c)
	anotherID := applicationtesting.GenApplicationUUID(c)
	scope := charm.ScopeContainer
	otherAppData := relation.OtherApplicationForWatcher{
		ApplicationID: anotherID,
		Subordinate:   false,
	}

	s.expectGetRelationEndpointScope(relUUID, subordinateID, scope, nil)
	s.expectGetOtherRelatedEndpointApplicationData(relUUID, subordinateID, otherAppData, nil)

	// Act
	watcher := &subordinateLifeSuspendedStatusWatcher{
		s:           s.service,
		parentAppID: principalID,
		appID:       subordinateID,
	}
	change, err := watcher.watchNewRelation(
		context.Background(),
		relUUID,
	)

	// Assert
	c.Assert(err, gc.IsNil)
	c.Assert(change, jc.IsFalse)
}

func (s *watcherSuite) TestChangeEventsForSubordinateLifeSuspendedStatusMapper(c *gc.C) {
	// Arrange: setup a change to an existing relation being watched,
	// a new relation for the subordinate, a new relation for neither
	// the subordinate nor principal application.
	ctrl := s.setupMocks(c)
	defer ctrl.Finish()
	principalSubordinateRelUUID := testing.GenRelationUUID(c)
	newSubordinateRelUUID := testing.GenRelationUUID(c)
	unrelatedRelUUID := testing.GenRelationUUID(c)
	principalID := applicationtesting.GenApplicationUUID(c)
	subordinateID := applicationtesting.GenApplicationUUID(c)

	currentRelData := relation.RelationLifeSuspendedData{
		EndpointIdentifiers: []corerelation.EndpointIdentifier{
			{
				ApplicationName: "subordinate",
				EndpointName:    "seven",
				Role:            charm.RoleRequirer,
			}, {
				ApplicationName: "principal",
				EndpointName:    "two",
				Role:            charm.RoleProvider,
			},
		},
		Life:      life.Alive,
		Suspended: false,
	}
	newData := relation.RelationLifeSuspendedData{
		EndpointIdentifiers: []corerelation.EndpointIdentifier{
			{
				ApplicationName: "subordinate",
				EndpointName:    "seven",
				Role:            charm.RoleRequirer,
			}, {
				ApplicationName: "another",
				EndpointName:    "two",
				Role:            charm.RoleProvider,
			},
		},
		Life:      life.Alive,
		Suspended: true,
	}

	newRelData := currentRelData
	newRelData.Life = life.Dying
	s.expectGetMapperDataForWatchLifeSuspendedStatus(principalSubordinateRelUUID, subordinateID, newRelData, nil)
	s.expectGetMapperDataForWatchLifeSuspendedStatus(unrelatedRelUUID, subordinateID, relation.RelationLifeSuspendedData{}, relationerrors.ApplicationNotFoundForRelation)
	s.expectGetMapperDataForWatchLifeSuspendedStatus(newSubordinateRelUUID, subordinateID, newData, nil)
	s.expectGetRelationEndpointScope(newSubordinateRelUUID, subordinateID, charm.ScopeGlobal, nil)
	changeOne := s.expectChanged(ctrl, principalSubordinateRelUUID)
	changeTwo := s.expectChanged(ctrl, newSubordinateRelUUID)
	changeThree := s.expectChanged(ctrl, unrelatedRelUUID)
	changes := []changestream.ChangeEvent{
		changeTwo,
		changeThree,
		changeOne,
	}
	currentRelations := make(map[corerelation.UUID]relation.RelationLifeSuspendedData)
	currentRelations[principalSubordinateRelUUID] = currentRelData

	// Act
	watcher := &subordinateLifeSuspendedStatusWatcher{
		s:                s.service,
		parentAppID:      principalID,
		appID:            subordinateID,
		currentRelations: currentRelations,
		relationsIgnored: set.NewStrings(),
	}
	obtainedChanges, err := watcher.filterChangeEvents(
		context.Background(),
		changes,
	)

	// Assert: changes contain the existing relation and the new relation,
	// but not a relation for neither the principal nor subordinate.
	expectedRelations := make(map[corerelation.UUID]relation.RelationLifeSuspendedData)
	expectedRelations[newSubordinateRelUUID] = newData
	expectedRelations[principalSubordinateRelUUID] = newRelData

	expectedChangeZero, _ := corerelation.NewKey(newData.EndpointIdentifiers)
	expectedChangeOne, _ := corerelation.NewKey(newRelData.EndpointIdentifiers)
	expectedChanged := set.NewStrings(expectedChangeZero.String())
	expectedChanged.Add(expectedChangeOne.String())

	c.Assert(err, gc.IsNil)
	c.Assert(obtainedChanges, gc.HasLen, 2)
	for _, change := range obtainedChanges {
		c.Check(expectedChanged.Contains(change.Changed()), jc.IsTrue)
	}
	c.Check(watcher.currentRelations, gc.DeepEquals, currentRelations)
	c.Check(watcher.relationsIgnored.Contains(unrelatedRelUUID.String()), jc.IsTrue)
}

func (s *watcherSuite) setupMocks(c *gc.C) *gomock.Controller {
	ctrl := gomock.NewController(c)

	s.state = NewMockState(ctrl)
	s.watcherFactory = NewMockWatcherFactory(ctrl)
	s.service = NewWatchableService(s.state, s.watcherFactory, nil, loggertesting.WrapCheckLog(c))

	return ctrl
}

func (s *watcherSuite) expectGetRelationEndpointScope(
	uuid corerelation.UUID,
	id coreapplication.ID,
	scope charm.RelationScope,
	err error,
) {
	s.state.EXPECT().GetRelationEndpointScope(gomock.Any(), uuid, id).Return(scope, err)
}

func (s *watcherSuite) expectGetOtherRelatedEndpointApplicationData(
	relUUID corerelation.UUID,
	id coreapplication.ID,
	data relation.OtherApplicationForWatcher,
	err error,
) {
	s.state.EXPECT().GetOtherRelatedEndpointApplicationData(gomock.Any(), relUUID, id).Return(
		data, err)
}

func (s *watcherSuite) expectChanged(ctrl *gomock.Controller, uuid corerelation.UUID) changestream.ChangeEvent {
	change := changestreammock.NewMockChangeEvent(ctrl)
	change.EXPECT().Changed().Return(uuid.String())
	return change
}

func (s *watcherSuite) expectGetMapperDataForWatchLifeSuspendedStatus(
	relUUID corerelation.UUID,
	appID coreapplication.ID,
	data relation.RelationLifeSuspendedData,
	err error,
) {
	s.state.EXPECT().GetMapperDataForWatchLifeSuspendedStatus(gomock.Any(), relUUID, appID).Return(data, err)
}
