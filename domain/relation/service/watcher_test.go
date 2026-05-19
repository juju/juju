// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package service

import (
	stdtesting "testing"

	"github.com/juju/collections/set"
	"github.com/juju/tc"
	"github.com/canonical/gomock/gomock"

	coreapplication "github.com/juju/juju/core/application"
	"github.com/juju/juju/core/changestream"
	changestreammock "github.com/juju/juju/core/changestream/mocks"
	"github.com/juju/juju/core/life"
	corerelation "github.com/juju/juju/core/relation"
	"github.com/juju/juju/core/relation/testing"
	applicationerrors "github.com/juju/juju/domain/application/errors"
	"github.com/juju/juju/domain/deployment/charm"
	"github.com/juju/juju/domain/relation"
	relationerrors "github.com/juju/juju/domain/relation/errors"
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

func TestWatcherSuite(t *stdtesting.T) {
	tc.Run(t, &watcherSuite{})
}

// TestSubordinateSendChangeEventRelationScopeGlobal tests that if the
// subordinate unit's relation endpoint is global scoped, an event is
// sent.
func (s *watcherSuite) TestSubordinateSendChangeEventRelationScopeGlobal(c *tc.C) {
	// Arrange
	defer s.setupMocks(c).Finish()
	relUUID := testing.GenRelationUUID(c)
	principalID := tc.Must(c, coreapplication.NewUUID)
	subordinateID := tc.Must(c, coreapplication.NewUUID)
	scope := charm.ScopeGlobal

	s.expectGetRelationEndpointScope(relUUID, subordinateID, scope, nil)

	// Act
	watcher := s.getSubordinateWatcher(principalID, subordinateID)
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
	principalID := tc.Must(c, coreapplication.NewUUID)
	subordinateID := tc.Must(c, coreapplication.NewUUID)
	anotherSubordinateID := tc.Must(c, coreapplication.NewUUID)
	scope := charm.ScopeContainer
	otherAppData := relation.OtherApplicationForWatcher{
		ApplicationID: anotherSubordinateID,
		Subordinate:   true,
	}

	s.expectGetRelationEndpointScope(relUUID, subordinateID, scope, nil)
	s.expectGetOtherRelatedEndpointApplicationData(relUUID, subordinateID, otherAppData, nil)

	// Act
	watcher := s.getSubordinateWatcher(principalID, subordinateID)
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
	principalID := tc.Must(c, coreapplication.NewUUID)
	subordinateID := tc.Must(c, coreapplication.NewUUID)
	scope := charm.ScopeContainer
	otherAppData := relation.OtherApplicationForWatcher{
		ApplicationID: principalID,
	}

	s.expectGetRelationEndpointScope(relUUID, subordinateID, scope, nil)
	s.expectGetOtherRelatedEndpointApplicationData(relUUID, subordinateID, otherAppData, nil)

	// Act
	watcher := s.getSubordinateWatcher(principalID, subordinateID)
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
	principalID := tc.Must(c, coreapplication.NewUUID)
	subordinateID := tc.Must(c, coreapplication.NewUUID)
	anotherID := tc.Must(c, coreapplication.NewUUID)
	scope := charm.ScopeContainer
	otherAppData := relation.OtherApplicationForWatcher{
		ApplicationID: anotherID,
		Subordinate:   false,
	}

	s.expectGetRelationEndpointScope(relUUID, subordinateID, scope, nil)
	s.expectGetOtherRelatedEndpointApplicationData(relUUID, subordinateID, otherAppData, nil)

	// Act
	watcher := s.getSubordinateWatcher(principalID, subordinateID)
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
	principalID := tc.Must(c, coreapplication.NewUUID)
	subordinateID := tc.Must(c, coreapplication.NewUUID)

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
	watcher := s.getSubordinateWatcher(principalID, subordinateID)
	watcher.currentRelations = currentRelations

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

	expectedChangeZero := corerelation.Key(newData.EndpointIdentifiers)
	expectedChangeOne := corerelation.Key(newRelData.EndpointIdentifiers)
	expectedChanged := set.NewStrings(expectedChangeZero.String())
	expectedChanged.Add(expectedChangeOne.String())

	c.Assert(err, tc.IsNil)
	c.Assert(obtainedChanges, tc.HasLen, 2)
	for _, change := range obtainedChanges {
		c.Check(expectedChanged.Contains(change), tc.IsTrue)
	}
	c.Check(watcher.currentRelations, tc.DeepEquals, currentRelations)
	c.Check(relationsIgnored.Contains(unrelatedRelUUID.String()), tc.IsTrue)
}

// TestSubordinateRelationRemovedKnown verifies that when a known relation is
// removed (RelationNotFound), filterChangeEvents emits the endpoint key so
// that key-based consumers (e.g. the uniter remote-state watcher) can clean
// up their state.
func (s *watcherSuite) TestSubordinateRelationRemovedKnown(c *tc.C) {
	// Arrange
	ctrl := s.setupMocks(c)
	defer ctrl.Finish()

	relUUID := testing.GenRelationUUID(c)
	principalID := tc.Must(c, coreapplication.NewUUID)
	subordinateID := tc.Must(c, coreapplication.NewUUID)

	existingData := relation.RelationLifeSuspendedData{
		EndpointIdentifiers: []corerelation.EndpointIdentifier{
			{ApplicationName: "subordinate", EndpointName: "ep", Role: charm.RoleRequirer},
			{ApplicationName: "principal", EndpointName: "ep", Role: charm.RoleProvider},
		},
		Life:      life.Alive,
		Suspended: false,
	}

	// The relation has been removed from the database.
	s.expectGetMapperDataForWatchLifeSuspendedStatus(
		relUUID, subordinateID, relation.RelationLifeSuspendedData{}, relationerrors.RelationNotFound,
	)
	change := s.expectChanged(ctrl, relUUID)

	watcher := s.getSubordinateWatcher(principalID, subordinateID)
	watcher.currentRelations = map[corerelation.UUID]relation.RelationLifeSuspendedData{
		relUUID: existingData,
	}

	// Act
	relationsIgnored := set.NewStrings()
	obtained, err := watcher.filterChangeEvents(
		c.Context(),
		[]changestream.ChangeEvent{change},
		relationsIgnored,
	)

	// Assert: a single change event is emitted with the endpoint key, so
	// the consumer can clean up its key-indexed state.
	c.Assert(err, tc.IsNil)
	c.Assert(obtained, tc.HasLen, 1)
	expectedKey := corerelation.Key(existingData.EndpointIdentifiers)
	c.Check(obtained[0], tc.Equals, expectedKey.String())
	// The relation must be removed from currentRelations.
	c.Check(watcher.currentRelations, tc.HasLen, 0)
	c.Check(relationsIgnored.IsEmpty(), tc.IsTrue)
}

// TestSubordinateRelationRemovedUnknown verifies that when a relation the
// watcher has never seen is removed (RelationNotFound), no change event is
// emitted.
func (s *watcherSuite) TestSubordinateRelationRemovedUnknown(c *tc.C) {
	// Arrange
	ctrl := s.setupMocks(c)
	defer ctrl.Finish()

	relUUID := testing.GenRelationUUID(c)
	principalID := tc.Must(c, coreapplication.NewUUID)
	subordinateID := tc.Must(c, coreapplication.NewUUID)

	// The relation has been removed but was never tracked.
	s.expectGetMapperDataForWatchLifeSuspendedStatus(
		relUUID, subordinateID, relation.RelationLifeSuspendedData{}, relationerrors.RelationNotFound,
	)
	change := s.expectChanged(ctrl, relUUID)

	watcher := s.getSubordinateWatcher(principalID, subordinateID)
	// currentRelations is empty: this UUID was never seen.

	// Act
	relationsIgnored := set.NewStrings()
	obtained, err := watcher.filterChangeEvents(
		c.Context(),
		[]changestream.ChangeEvent{change},
		relationsIgnored,
	)

	// Assert: nothing to clean up, no event emitted.
	c.Assert(err, tc.IsNil)
	c.Check(obtained, tc.HasLen, 0)
	c.Check(relationsIgnored.IsEmpty(), tc.IsTrue)
}

// TestSubordinateAppNotFoundForTrackedRelation verifies that when
// ApplicationNotFoundForRelation is returned for a relation that was
// previously tracked, the watcher emits the old key and removes the
// relation from currentRelations.
func (s *watcherSuite) TestSubordinateAppNotFoundForTrackedRelation(c *tc.C) {
	// Arrange
	ctrl := s.setupMocks(c)
	defer ctrl.Finish()

	relUUID := testing.GenRelationUUID(c)
	principalID := tc.Must(c, coreapplication.NewUUID)
	subordinateID := tc.Must(c, coreapplication.NewUUID)

	existingData := relation.RelationLifeSuspendedData{
		EndpointIdentifiers: []corerelation.EndpointIdentifier{
			{ApplicationName: "subordinate", EndpointName: "ep", Role: charm.RoleRequirer},
			{ApplicationName: "principal", EndpointName: "ep", Role: charm.RoleProvider},
		},
		Life:      life.Alive,
		Suspended: false,
	}

	// relation_endpoint rows removed before relation row during teardown.
	s.expectGetMapperDataForWatchLifeSuspendedStatus(
		relUUID, subordinateID, relation.RelationLifeSuspendedData{}, relationerrors.ApplicationNotFoundForRelation,
	)
	change := s.expectChanged(ctrl, relUUID)

	watcher := s.getSubordinateWatcher(principalID, subordinateID)
	watcher.currentRelations = map[corerelation.UUID]relation.RelationLifeSuspendedData{
		relUUID: existingData,
	}

	// Act
	relationsIgnored := set.NewStrings()
	obtained, err := watcher.filterChangeEvents(
		c.Context(),
		[]changestream.ChangeEvent{change},
		relationsIgnored,
	)

	// Assert: the old key is emitted and the relation is cleaned up.
	c.Assert(err, tc.IsNil)
	c.Assert(obtained, tc.HasLen, 1)
	expectedKey := corerelation.Key(existingData.EndpointIdentifiers)
	c.Check(obtained[0], tc.Equals, expectedKey.String())
	c.Check(watcher.currentRelations, tc.HasLen, 0)
	c.Check(relationsIgnored.IsEmpty(), tc.IsTrue)
}

// TestSubordinateAppNotFoundForUntrackedRelation verifies that when
// ApplicationNotFoundForRelation is returned for a relation that was
// never tracked, the relation is added to relationsIgnored.
func (s *watcherSuite) TestSubordinateAppNotFoundForUntrackedRelation(c *tc.C) {
	// Arrange
	ctrl := s.setupMocks(c)
	defer ctrl.Finish()

	relUUID := testing.GenRelationUUID(c)
	principalID := tc.Must(c, coreapplication.NewUUID)
	subordinateID := tc.Must(c, coreapplication.NewUUID)

	s.expectGetMapperDataForWatchLifeSuspendedStatus(
		relUUID, subordinateID, relation.RelationLifeSuspendedData{}, relationerrors.ApplicationNotFoundForRelation,
	)
	change := s.expectChanged(ctrl, relUUID)

	watcher := s.getSubordinateWatcher(principalID, subordinateID)

	// Act
	relationsIgnored := set.NewStrings()
	obtained, err := watcher.filterChangeEvents(
		c.Context(),
		[]changestream.ChangeEvent{change},
		relationsIgnored,
	)

	// Assert: no event emitted, relation is ignored.
	c.Assert(err, tc.IsNil)
	c.Check(obtained, tc.HasLen, 0)
	c.Check(relationsIgnored.Contains(relUUID.String()), tc.IsTrue)
}

// TestPrincipalAppNotFoundForTrackedRelation verifies that when
// ApplicationNotFoundForRelation is returned for a previously tracked
// relation, the principal watcher emits the old key.
func (s *watcherSuite) TestPrincipalAppNotFoundForTrackedRelation(c *tc.C) {
	// Arrange
	ctrl := s.setupMocks(c)
	defer ctrl.Finish()

	relUUID := testing.GenRelationUUID(c)
	appID := tc.Must(c, coreapplication.NewUUID)

	existingData := relation.RelationLifeSuspendedData{
		EndpointIdentifiers: []corerelation.EndpointIdentifier{
			{ApplicationName: "app", EndpointName: "ep", Role: charm.RoleRequirer},
			{ApplicationName: "other", EndpointName: "ep", Role: charm.RoleProvider},
		},
		Life:      life.Alive,
		Suspended: false,
	}

	s.expectGetMapperDataForWatchLifeSuspendedStatus(
		relUUID, appID, relation.RelationLifeSuspendedData{}, relationerrors.ApplicationNotFoundForRelation,
	)
	change := s.expectChanged(ctrl, relUUID)

	watcher := s.getPrincipalWatcher(appID)
	watcher.currentRelations = map[corerelation.UUID]relation.RelationLifeSuspendedData{
		relUUID: existingData,
	}

	// Act
	relationsIgnored := set.NewStrings()
	obtained, err := watcher.filterChangeEvents(
		c.Context(),
		[]changestream.ChangeEvent{change},
		relationsIgnored,
	)

	// Assert: the old key is emitted and the relation is cleaned up.
	c.Assert(err, tc.IsNil)
	c.Assert(obtained, tc.HasLen, 1)
	expectedKey := corerelation.Key(existingData.EndpointIdentifiers)
	c.Check(obtained[0], tc.Equals, expectedKey.String())
	c.Check(watcher.currentRelations, tc.HasLen, 0)
	c.Check(relationsIgnored.IsEmpty(), tc.IsTrue)
}

// TestPrincipalAppNotFoundForUntrackedRelation verifies that when
// ApplicationNotFoundForRelation is returned for a never-tracked
// relation, the principal watcher adds it to relationsIgnored.
func (s *watcherSuite) TestPrincipalAppNotFoundForUntrackedRelation(c *tc.C) {
	// Arrange
	ctrl := s.setupMocks(c)
	defer ctrl.Finish()

	relUUID := testing.GenRelationUUID(c)
	appID := tc.Must(c, coreapplication.NewUUID)

	s.expectGetMapperDataForWatchLifeSuspendedStatus(
		relUUID, appID, relation.RelationLifeSuspendedData{}, relationerrors.ApplicationNotFoundForRelation,
	)
	change := s.expectChanged(ctrl, relUUID)

	watcher := s.getPrincipalWatcher(appID)

	// Act
	relationsIgnored := set.NewStrings()
	obtained, err := watcher.filterChangeEvents(
		c.Context(),
		[]changestream.ChangeEvent{change},
		relationsIgnored,
	)

	// Assert: no event emitted, relation is ignored.
	c.Assert(err, tc.IsNil)
	c.Check(obtained, tc.HasLen, 0)
	c.Check(relationsIgnored.Contains(relUUID.String()), tc.IsTrue)
}

func (s *watcherSuite) TestWatchRelationsLifeSuspendedStatusForApplicationApplicationNotFound(c *tc.C) {
	defer s.setupMocks(c).Finish()

	s.state.EXPECT().ApplicationExists(gomock.Any(), gomock.Any()).Return(applicationerrors.ApplicationNotFound)

	_, err := s.service.WatchRelationsLifeSuspendedStatusForApplication(c.Context(), tc.Must(c, coreapplication.NewUUID))
	c.Assert(err, tc.ErrorIs, applicationerrors.ApplicationNotFound)
}

func (s *watcherSuite) setupMocks(c *tc.C) *gomock.Controller {
	ctrl := gomock.NewController(c)

	s.state = NewMockState(ctrl)
	s.watcherFactory = NewMockWatcherFactory(ctrl)
	s.service = NewWatchableService(s.state, s.watcherFactory, nil, nil, loggertesting.WrapCheckLog(c))

	c.Cleanup(func() {
		s.state = nil
		s.service = nil
		s.watcherFactory = nil
	})
	return ctrl
}

func (s *watcherSuite) expectGetRelationEndpointScope(
	uuid corerelation.UUID,
	id coreapplication.UUID,
	scope charm.RelationScope,
	err error,
) {
	s.state.EXPECT().GetRelationEndpointScope(gomock.Any(), uuid, id).Return(scope, err)
}

func (s *watcherSuite) expectGetOtherRelatedEndpointApplicationData(
	relUUID corerelation.UUID,
	id coreapplication.UUID,
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
	appUUID coreapplication.UUID,
	data relation.RelationLifeSuspendedData,
	err error,
) {
	s.state.EXPECT().GetMapperDataForWatchLifeSuspendedStatus(gomock.Any(), relUUID, appUUID).Return(data, err)
}

func (s *watcherSuite) getSubordinateWatcher(principalID, subordinateID coreapplication.UUID) *subordinateLifeSuspendedStatusWatcher {
	w := &subordinateLifeSuspendedStatusWatcher{
		parentAppID: principalID,
	}
	w.lifeSuspendedStatusWatcher = lifeSuspendedStatusWatcher[corerelation.Key]{
		s:             s.service,
		appUUID:       subordinateID,
		processChange: w.processChange,
	}
	return w
}

func (s *watcherSuite) getPrincipalWatcher(appID coreapplication.UUID) *principalLifeSuspendedStatusWatcher {
	w := &principalLifeSuspendedStatusWatcher{}
	w.lifeSuspendedStatusWatcher = lifeSuspendedStatusWatcher[corerelation.Key]{
		s:                s.service,
		appUUID:          appID,
		currentRelations: make(map[corerelation.UUID]relation.RelationLifeSuspendedData),
		processChange:    w.processChange,
	}
	return w
}
