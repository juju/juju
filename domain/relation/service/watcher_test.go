// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package service

import (
	"github.com/juju/collections/set"
	"github.com/juju/tc"
	"go.uber.org/mock/gomock"

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
	"github.com/juju/juju/internal/testhelpers"
)

// Test the internal functionality of the complex watchers.

type watcherSuite struct {
	testhelpers.IsolationSuite

	state          *MockState
	watcherFactory *MockWatcherFactory

	service *WatchableService
}

var _ = tc.Suite(&watcherSuite{})

// TestSubordinateSendChangeEventRelationScopeGlobal tests that if the
// subordinate unit's relation endpoint is global scoped, an event is
// sent.
func (s *watcherSuite) TestSubordinateSendChangeEventRelationScopeGlobal(c *tc.C) {
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
		c.Context(),
		relUUID,
	)

	// Assert
	c.Assert(err, tc.IsNil)
	c.Assert(change, tc.IsTrue)
}

// TestSubordinateSendChangeEventRelationAnotherSubordinate tests that if the
// subordinate unit's relation endpoint is container scoped, and the other
// end is also a subordinate, an event is sent.
func (s *watcherSuite) TestSubordinateSendChangeEventRelationAnotherSubordinate(c *tc.C) {
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
		c.Context(),
		relUUID,
	)

	// Assert
	c.Assert(err, tc.IsNil)
	c.Assert(change, tc.IsTrue)
}

// TestSubordinateSendChangeEventRelationPrincipal tests that if the
// subordinate unit's relation endpoint is container scoped, and the other
// end application is the principal application, an event is sent.
func (s *watcherSuite) TestSubordinateSendChangeEventRelationPrincipal(c *tc.C) {
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
		c.Context(),
		relUUID,
	)

	// Assert
	c.Assert(err, tc.IsNil)
	c.Assert(change, tc.IsTrue)
}

// TestSubordinateSendChangeEventRelationNoChange checks that no change is
// requested while all state calls are successful. Rhe relation is container
// scoped but the application in the relation is not the subordinate's principal.
func (s *watcherSuite) TestSubordinateSendChangeEventRelationNoChange(c *tc.C) {
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
		c.Context(),
		relUUID,
	)

	// Assert
	c.Assert(err, tc.IsNil)
	c.Assert(change, tc.IsFalse)
}

func (s *watcherSuite) TestChangeEventsForSubordinateLifeSuspendedStatusMapper(c *tc.C) {
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
	}

	relationsIgnored := set.NewStrings()
	obtainedChanges, err := watcher.filterChangeEvents(
		c.Context(),
		changes,
		relationsIgnored,
	)

	// Assert: changes contain the existing relation and the new relation,
	// but not a relation for either the principal nor subordinate.
	expectedRelations := make(map[corerelation.UUID]relation.RelationLifeSuspendedData)
	expectedRelations[newSubordinateRelUUID] = newData
	expectedRelations[principalSubordinateRelUUID] = newRelData

	expectedChangeZero, _ := corerelation.NewKey(newData.EndpointIdentifiers)
	expectedChangeOne, _ := corerelation.NewKey(newRelData.EndpointIdentifiers)
	expectedChanged := set.NewStrings(expectedChangeZero.String())
	expectedChanged.Add(expectedChangeOne.String())

	c.Assert(err, tc.IsNil)
	c.Assert(obtainedChanges, tc.HasLen, 2)
	for _, change := range obtainedChanges {
		c.Check(expectedChanged.Contains(change.Changed()), tc.IsTrue)
	}
	c.Check(watcher.currentRelations, tc.DeepEquals, currentRelations)
	c.Check(relationsIgnored.Contains(unrelatedRelUUID.String()), tc.IsTrue)
}

func (s *watcherSuite) setupMocks(c *tc.C) *gomock.Controller {
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
