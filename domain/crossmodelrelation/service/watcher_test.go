// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package service

import (
	"context"
	stdtesting "testing"

	"github.com/juju/clock"
	"github.com/juju/tc"
	"go.uber.org/mock/gomock"

	coreapplication "github.com/juju/juju/core/application"
	"github.com/juju/juju/core/changestream"
	changestreammock "github.com/juju/juju/core/changestream/mocks"
	"github.com/juju/juju/core/database"
	"github.com/juju/juju/core/network"
	corerelation "github.com/juju/juju/core/relation"
	corerelationtesting "github.com/juju/juju/core/relation/testing"
	"github.com/juju/juju/core/watcher"
	"github.com/juju/juju/core/watcher/eventsource"
	"github.com/juju/juju/core/watcher/watchertest"
	"github.com/juju/juju/internal/errors"
	loggertesting "github.com/juju/juju/internal/logger/testing"
	"github.com/juju/juju/internal/testhelpers"
)

// Test the watcher functionality of the cross-model relation service.

type watcherServiceSuite struct {
	testhelpers.IsolationSuite

	controllerState *MockControllerState
	modelState      *MockModelState
	statusHistory   *MockStatusHistory
	watcherFactory  *MockWatcherFactory

	service *WatchableService
}

func TestWatcherServiceSuite(t *stdtesting.T) {
	tc.Run(t, &watcherServiceSuite{})
}

func (s *watcherServiceSuite) setupMocks(c *tc.C) *gomock.Controller {
	ctrl := gomock.NewController(c)

	s.controllerState = NewMockControllerState(ctrl)
	s.modelState = NewMockModelState(ctrl)
	s.statusHistory = NewMockStatusHistory(ctrl)
	s.watcherFactory = NewMockWatcherFactory(ctrl)

	s.service = NewWatchableService(
		s.controllerState,
		s.modelState,
		s.statusHistory,
		s.watcherFactory,
		clock.WallClock,
		loggertesting.WrapCheckLog(c),
	)

	c.Cleanup(func() {
		s.controllerState = nil
		s.modelState = nil
		s.statusHistory = nil
		s.watcherFactory = nil
		s.service = nil
	})

	return ctrl
}

// TestWatchRemoteApplicationConsumers tests that WatchRemoteApplicationConsumers
// creates a notify watcher for remote application consumer changes.
func (s *watcherServiceSuite) TestWatchRemoteApplicationConsumers(c *tc.C) {
	defer s.setupMocks(c).Finish()

	// Arrange
	namespace := "application_remote_consumer"
	s.modelState.EXPECT().NamespaceRemoteApplicationConsumers().Return(namespace)

	ch := make(chan struct{})
	mockWatcher := watchertest.NewMockNotifyWatcher(ch)
	s.watcherFactory.EXPECT().NewNotifyWatcher(
		gomock.Any(),
		"watch remote application consumer",
		gomock.Any(),
	).Return(mockWatcher, nil)

	// Act
	w, err := s.service.WatchRemoteApplicationConsumers(c.Context())

	// Assert
	c.Assert(err, tc.IsNil)
	c.Assert(w, tc.Equals, mockWatcher)
}

// TestWatchRemoteApplicationConsumersError tests error handling when
// watcher creation fails.
func (s *watcherServiceSuite) TestWatchRemoteApplicationConsumersError(c *tc.C) {
	defer s.setupMocks(c).Finish()

	// Arrange
	namespace := "application_remote_consumer"
	s.modelState.EXPECT().NamespaceRemoteApplicationConsumers().Return(namespace)

	expectedErr := errors.New("watcher creation failed")
	s.watcherFactory.EXPECT().NewNotifyWatcher(
		gomock.Any(),
		gomock.Any(),
		gomock.Any(),
	).Return(nil, expectedErr)

	// Act
	_, err := s.service.WatchRemoteApplicationConsumers(c.Context())

	// Assert
	c.Assert(err, tc.ErrorMatches, "watcher creation failed")
}

// TestWatchRemoteApplicationOfferers tests that WatchRemoteApplicationOfferers
// creates a notify watcher for remote application offerer changes.
func (s *watcherServiceSuite) TestWatchRemoteApplicationOfferers(c *tc.C) {
	defer s.setupMocks(c).Finish()

	// Arrange
	namespace := "application_remote_offerer"
	s.modelState.EXPECT().NamespaceRemoteApplicationOfferers().Return(namespace)

	ch := make(chan struct{})
	mockWatcher := watchertest.NewMockNotifyWatcher(ch)
	s.watcherFactory.EXPECT().NewNotifyWatcher(
		gomock.Any(),
		"watch remote application offerer",
		gomock.Any(),
	).Return(mockWatcher, nil)

	// Act
	w, err := s.service.WatchRemoteApplicationOfferers(c.Context())

	// Assert
	c.Assert(err, tc.IsNil)
	c.Assert(w, tc.Equals, mockWatcher)
}

// TestWatchRemoteApplicationOfferersError tests error handling when
// watcher creation fails.
func (s *watcherServiceSuite) TestWatchRemoteApplicationOfferersError(c *tc.C) {
	defer s.setupMocks(c).Finish()

	// Arrange
	namespace := "application_remote_offerer"
	s.modelState.EXPECT().NamespaceRemoteApplicationOfferers().Return(namespace)

	expectedErr := errors.New("watcher creation failed")
	s.watcherFactory.EXPECT().NewNotifyWatcher(
		gomock.Any(),
		gomock.Any(),
		gomock.Any(),
	).Return(nil, expectedErr)

	// Act
	_, err := s.service.WatchRemoteApplicationOfferers(c.Context())

	// Assert
	c.Assert(err, tc.ErrorMatches, "watcher creation failed")
}

// TestWatchRemoteConsumedSecretsChanges tests that
// WatchRemoteConsumedSecretsChanges creates a strings watcher for remote
// consumed secret changes.
func (s *watcherServiceSuite) TestWatchRemoteConsumedSecretsChanges(c *tc.C) {
	defer s.setupMocks(c).Finish()

	// Arrange
	appUUID := tc.Must(c, coreapplication.NewUUID)
	namespace := "secret_remote_consumer"
	query := func(ctx context.Context, db database.TxnRunner) ([]string, error) {
		return []string{}, nil
	}

	s.modelState.EXPECT().InitialWatchStatementForRemoteConsumedSecretsChangesFromOfferingSide(
		appUUID.String(),
	).Return(namespace, query)

	mockWatcher := watchertest.NewMockStringsWatcher(make(chan []string))
	s.watcherFactory.EXPECT().NewNamespaceWatcher(
		gomock.Any(),
		gomock.Any(),
		gomock.Any(),
		gomock.Any(),
	).DoAndReturn(func(ctx context.Context, initialQuery eventsource.NamespaceQuery, summary string, filterOption eventsource.FilterOption, filterOptions ...eventsource.FilterOption) (watcher.StringsWatcher, error) {
		// Create a test watcher that will be wrapped by secret.NewSecretStringWatcher
		return mockWatcher, nil
	})

	s.modelState.EXPECT().GetRemoteConsumedSecretURIsWithChangesFromOfferingSide(
		gomock.Any(),
		appUUID.String(),
	).Return([]string{}, nil).AnyTimes()

	// Act
	w, err := s.service.WatchRemoteConsumedSecretsChanges(c.Context(), appUUID)

	// Assert
	c.Assert(err, tc.IsNil)
	c.Assert(w, tc.NotNil)
}

// TestWatchRemoteConsumedSecretsChangesInvalidUUID tests that an invalid
// application UUID returns an error.
func (s *watcherServiceSuite) TestWatchRemoteConsumedSecretsChangesInvalidUUID(c *tc.C) {
	defer s.setupMocks(c).Finish()

	// Arrange
	invalidUUID := coreapplication.UUID("invalid")

	// Act
	_, err := s.service.WatchRemoteConsumedSecretsChanges(c.Context(), invalidUUID)

	// Assert
	c.Assert(err, tc.ErrorMatches, "validating application UUID:.*")
}

// TestWatchRemoteConsumedSecretsChangesWatcherError tests error handling
// when watcher creation fails.
func (s *watcherServiceSuite) TestWatchRemoteConsumedSecretsChangesWatcherError(c *tc.C) {
	defer s.setupMocks(c).Finish()

	// Arrange
	appUUID := tc.Must(c, coreapplication.NewUUID)
	namespace := "secret_remote_consumer"
	query := func(ctx context.Context, db database.TxnRunner) ([]string, error) {
		return []string{}, nil
	}

	s.modelState.EXPECT().InitialWatchStatementForRemoteConsumedSecretsChangesFromOfferingSide(
		appUUID.String(),
	).Return(namespace, query)

	expectedErr := errors.New("watcher creation failed")
	s.watcherFactory.EXPECT().NewNamespaceWatcher(
		gomock.Any(),
		gomock.Any(),
		gomock.Any(),
		gomock.Any(),
	).Return(nil, expectedErr)

	// Act
	_, err := s.service.WatchRemoteConsumedSecretsChanges(c.Context(), appUUID)

	// Assert
	c.Assert(err, tc.ErrorMatches, "watcher creation failed")
}

// TestWatchConsumerRelations tests that WatchConsumerRelations creates
// a strings watcher with mapper for consumer relations.
func (s *watcherServiceSuite) TestWatchConsumerRelations(c *tc.C) {
	defer s.setupMocks(c).Finish()

	// Arrange
	namespace := "relation"
	query := func(ctx context.Context, db database.TxnRunner) ([]string, error) {
		return []string{}, nil
	}

	s.modelState.EXPECT().InitialWatchStatementForConsumerRelations().Return(namespace, query)

	mockWatcher := watchertest.NewMockStringsWatcher(make(chan []string))
	s.watcherFactory.EXPECT().NewNamespaceMapperWatcher(
		gomock.Any(),
		gomock.Any(),
		"consumer relations watcher",
		gomock.Any(),
		gomock.Any(),
	).Return(mockWatcher, nil)

	// Act
	w, err := s.service.WatchConsumerRelations(c.Context())

	// Assert
	c.Assert(err, tc.IsNil)
	c.Assert(w, tc.Equals, mockWatcher)
}

// TestWatchConsumerRelationsError tests error handling when watcher
// creation fails.
func (s *watcherServiceSuite) TestWatchConsumerRelationsError(c *tc.C) {
	defer s.setupMocks(c).Finish()

	// Arrange
	namespace := "relation"
	query := func(ctx context.Context, db database.TxnRunner) ([]string, error) {
		return []string{}, nil
	}

	s.modelState.EXPECT().InitialWatchStatementForConsumerRelations().Return(namespace, query)

	expectedErr := errors.New("watcher creation failed")
	s.watcherFactory.EXPECT().NewNamespaceMapperWatcher(
		gomock.Any(),
		gomock.Any(),
		gomock.Any(),
		gomock.Any(),
		gomock.Any(),
	).Return(nil, expectedErr)

	// Act
	_, err := s.service.WatchConsumerRelations(c.Context())

	// Assert
	c.Assert(err, tc.ErrorMatches, "watcher creation failed")
}

// TestWatchConsumerRelationsMapperEmptyChanges tests that the mapper
// returns nil for empty changes.
func (s *watcherServiceSuite) TestWatchConsumerRelationsMapperEmptyChanges(c *tc.C) {
	defer s.setupMocks(c).Finish()

	// Arrange
	namespace := "relation"
	query := func(ctx context.Context, db database.TxnRunner) ([]string, error) {
		return []string{}, nil
	}

	s.modelState.EXPECT().InitialWatchStatementForConsumerRelations().Return(namespace, query)

	var capturedMapper eventsource.Mapper
	mockWatcher := watchertest.NewMockStringsWatcher(make(chan []string))
	s.watcherFactory.EXPECT().NewNamespaceMapperWatcher(
		gomock.Any(),
		gomock.Any(),
		gomock.Any(),
		gomock.Any(),
		gomock.Any(),
	).DoAndReturn(func(ctx context.Context, initialQuery eventsource.NamespaceQuery, summary string, mapper eventsource.Mapper, filterOption eventsource.FilterOption, filterOptions ...eventsource.FilterOption) (watcher.StringsWatcher, error) {
		capturedMapper = mapper
		return mockWatcher, nil
	})

	_, err := s.service.WatchConsumerRelations(c.Context())
	c.Assert(err, tc.IsNil)

	// Act - test the mapper with empty changes
	result, err := capturedMapper(c.Context(), []changestream.ChangeEvent{})

	// Assert
	c.Assert(err, tc.IsNil)
	c.Assert(result, tc.IsNil)
}

// TestWatchConsumerRelationsMapperFiltersRelations tests that the mapper
// correctly filters consumer relation UUIDs.
func (s *watcherServiceSuite) TestWatchConsumerRelationsMapperFiltersRelations(c *tc.C) {
	ctrl := s.setupMocks(c)
	defer ctrl.Finish()

	// Arrange
	namespace := "relation"
	query := func(ctx context.Context, db database.TxnRunner) ([]string, error) {
		return []string{}, nil
	}

	s.modelState.EXPECT().InitialWatchStatementForConsumerRelations().Return(namespace, query)

	var capturedMapper eventsource.Mapper
	mockWatcher := watchertest.NewMockStringsWatcher(make(chan []string))
	s.watcherFactory.EXPECT().NewNamespaceMapperWatcher(
		gomock.Any(),
		gomock.Any(),
		gomock.Any(),
		gomock.Any(),
		gomock.Any(),
	).DoAndReturn(func(ctx context.Context, initialQuery eventsource.NamespaceQuery, summary string, mapper eventsource.Mapper, filterOption eventsource.FilterOption, filterOptions ...eventsource.FilterOption) (watcher.StringsWatcher, error) {
		capturedMapper = mapper
		return mockWatcher, nil
	})

	_, err := s.service.WatchConsumerRelations(c.Context())
	c.Assert(err, tc.IsNil)

	// Create mock change events
	relUUID1 := corerelationtesting.GenRelationUUID(c)
	relUUID2 := corerelationtesting.GenRelationUUID(c)

	change1 := changestreammock.NewMockChangeEvent(ctrl)
	change1.EXPECT().Changed().Return(relUUID1.String()).AnyTimes()

	change2 := changestreammock.NewMockChangeEvent(ctrl)
	change2.EXPECT().Changed().Return(relUUID2.String()).AnyTimes()

	changes := []changestream.ChangeEvent{change1, change2}

	// Expect GetConsumerRelationUUIDs to filter and return only relUUID1
	s.modelState.EXPECT().GetConsumerRelationUUIDs(
		gomock.Any(),
		relUUID1.String(),
		relUUID2.String(),
	).Return([]string{relUUID1.String()}, nil)

	// Act - test the mapper with changes
	result, err := capturedMapper(c.Context(), changes)

	// Assert
	c.Assert(err, tc.IsNil)
	c.Assert(result, tc.DeepEquals, []string{relUUID1.String()})
}

// TestWatchConsumerRelationsMapperHandlesError tests that the mapper
// handles errors gracefully by logging and returning nil.
func (s *watcherServiceSuite) TestWatchConsumerRelationsMapperHandlesError(c *tc.C) {
	ctrl := s.setupMocks(c)
	defer ctrl.Finish()

	// Arrange
	namespace := "relation"
	query := func(ctx context.Context, db database.TxnRunner) ([]string, error) {
		return []string{}, nil
	}

	s.modelState.EXPECT().InitialWatchStatementForConsumerRelations().Return(namespace, query)

	var capturedMapper eventsource.Mapper
	mockWatcher := watchertest.NewMockStringsWatcher(make(chan []string))
	s.watcherFactory.EXPECT().NewNamespaceMapperWatcher(
		gomock.Any(),
		gomock.Any(),
		gomock.Any(),
		gomock.Any(),
		gomock.Any(),
	).DoAndReturn(func(ctx context.Context, initialQuery eventsource.NamespaceQuery, summary string, mapper eventsource.Mapper, filterOption eventsource.FilterOption, filterOptions ...eventsource.FilterOption) (watcher.StringsWatcher, error) {
		capturedMapper = mapper
		return mockWatcher, nil
	})

	_, err := s.service.WatchConsumerRelations(c.Context())
	c.Assert(err, tc.IsNil)

	// Create mock change event
	relUUID := corerelationtesting.GenRelationUUID(c)
	change := changestreammock.NewMockChangeEvent(ctrl)
	change.EXPECT().Changed().Return(relUUID.String()).AnyTimes()

	changes := []changestream.ChangeEvent{change}

	// Expect GetConsumerRelationUUIDs to return an error
	s.modelState.EXPECT().GetConsumerRelationUUIDs(
		gomock.Any(),
		relUUID.String(),
	).Return(nil, errors.New("database error"))

	// Act - test the mapper with error
	result, err := capturedMapper(c.Context(), changes)

	// Assert - should return nil, not error (error is logged)
	c.Assert(err, tc.IsNil)
	c.Assert(result, tc.IsNil)
}

// TestWatchOffererRelations tests that WatchOffererRelations creates
// a strings watcher with stateful caching mapper.
func (s *watcherServiceSuite) TestWatchOffererRelations(c *tc.C) {
	defer s.setupMocks(c).Finish()

	// Arrange
	relationNamespace := "relation"
	consumerNamespace := "application_remote_consumer"
	query := func(ctx context.Context, db database.TxnRunner) ([]string, error) {
		return []string{}, nil
	}

	s.modelState.EXPECT().InitialWatchStatementForOffererRelations().Return(relationNamespace, query)
	s.modelState.EXPECT().NamespaceRemoteApplicationConsumers().Return(consumerNamespace)

	mockWatcher := watchertest.NewMockStringsWatcher(make(chan []string))
	s.watcherFactory.EXPECT().NewNamespaceMapperWatcher(
		gomock.Any(),
		gomock.Any(),
		"offerer relations watcher",
		gomock.Any(),
		gomock.Any(),
		gomock.Any(),
	).Return(mockWatcher, nil)

	// Act
	w, err := s.service.WatchOffererRelations(c.Context())

	// Assert
	c.Assert(err, tc.IsNil)
	c.Assert(w, tc.Equals, mockWatcher)
}

// TestWatchOffererRelationsError tests error handling when watcher
// creation fails.
func (s *watcherServiceSuite) TestWatchOffererRelationsError(c *tc.C) {
	defer s.setupMocks(c).Finish()

	// Arrange
	relationNamespace := "relation"
	consumerNamespace := "application_remote_consumer"
	query := func(ctx context.Context, db database.TxnRunner) ([]string, error) {
		return []string{}, nil
	}

	s.modelState.EXPECT().InitialWatchStatementForOffererRelations().Return(relationNamespace, query)
	s.modelState.EXPECT().NamespaceRemoteApplicationConsumers().Return(consumerNamespace)

	expectedErr := errors.New("watcher creation failed")
	s.watcherFactory.EXPECT().NewNamespaceMapperWatcher(
		gomock.Any(),
		gomock.Any(),
		gomock.Any(),
		gomock.Any(),
		gomock.Any(),
		gomock.Any(),
	).Return(nil, expectedErr)

	// Act
	_, err := s.service.WatchOffererRelations(c.Context())

	// Assert
	c.Assert(err, tc.ErrorMatches, "watcher creation failed")
}

// TestWatchRelationIngressNetworks tests that WatchRelationIngressNetworks
// creates a notify watcher for ingress network changes.
func (s *watcherServiceSuite) TestWatchRelationIngressNetworks(c *tc.C) {
	defer s.setupMocks(c).Finish()

	// Arrange
	relationUUID := corerelationtesting.GenRelationUUID(c)
	namespace := "relation_network_ingress"

	s.modelState.EXPECT().NamespaceForRelationIngressNetworksWatcher().Return(namespace)

	mockWatcher := watchertest.NewMockNotifyWatcher(make(chan struct{}))
	s.watcherFactory.EXPECT().NewNotifyWatcher(
		gomock.Any(),
		"relation ingress networks watcher",
		gomock.Any(),
	).Return(mockWatcher, nil)

	// Act
	w, err := s.service.WatchRelationIngressNetworks(c.Context(), relationUUID)

	// Assert
	c.Assert(err, tc.IsNil)
	c.Assert(w, tc.Equals, mockWatcher)
}

// TestWatchRelationIngressNetworksInvalidUUID tests that an invalid
// relation UUID returns an error.
func (s *watcherServiceSuite) TestWatchRelationIngressNetworksInvalidUUID(c *tc.C) {
	defer s.setupMocks(c).Finish()

	// Arrange
	invalidUUID := corerelation.UUID("invalid")

	// Act
	_, err := s.service.WatchRelationIngressNetworks(c.Context(), invalidUUID)

	// Assert
	c.Assert(err, tc.ErrorMatches, ".*")
}

// TestWatchRelationIngressNetworksWatcherError tests error handling
// when watcher creation fails.
func (s *watcherServiceSuite) TestWatchRelationIngressNetworksWatcherError(c *tc.C) {
	defer s.setupMocks(c).Finish()

	// Arrange
	relationUUID := corerelationtesting.GenRelationUUID(c)
	namespace := "relation_network_ingress"

	s.modelState.EXPECT().NamespaceForRelationIngressNetworksWatcher().Return(namespace)

	expectedErr := errors.New("watcher creation failed")
	s.watcherFactory.EXPECT().NewNotifyWatcher(
		gomock.Any(),
		gomock.Any(),
		gomock.Any(),
	).Return(nil, expectedErr)

	// Act
	_, err := s.service.WatchRelationIngressNetworks(c.Context(), relationUUID)

	// Assert
	c.Assert(err, tc.ErrorMatches, "watcher creation failed")
}

// TestWatchRelationEgressNetworks tests that WatchRelationEgressNetworks
// creates a strings watcher monitoring multiple namespaces.
func (s *watcherServiceSuite) TestWatchRelationEgressNetworks(c *tc.C) {
	defer s.setupMocks(c).Finish()

	// Arrange
	relationUUID := corerelationtesting.GenRelationUUID(c)
	relationEgressNS := "relation_network_egress"
	modelConfigNS := "model_config"
	ipAddressNS := "ip_address"

	query := func(ctx context.Context, db database.TxnRunner) ([]string, error) {
		return []string{}, nil
	}

	s.modelState.EXPECT().NamespacesForRelationEgressNetworksWatcher().Return(
		relationEgressNS, modelConfigNS, ipAddressNS,
	)
	s.modelState.EXPECT().InitialWatchStatementForRelationEgressNetworks(
		relationUUID.String(),
	).Return(query)

	mockWatcher := watchertest.NewMockStringsWatcher(make(chan []string))
	s.watcherFactory.EXPECT().NewNamespaceMapperWatcher(
		gomock.Any(),
		gomock.Any(),
		gomock.Any(),
		gomock.Any(),
		gomock.Any(),
		gomock.Any(),
		gomock.Any(),
	).Return(mockWatcher, nil)

	// Act
	w, err := s.service.WatchRelationEgressNetworks(c.Context(), relationUUID)

	// Assert
	c.Assert(err, tc.IsNil)
	c.Assert(w, tc.Equals, mockWatcher)
}

// TestWatchRelationEgressNetworksInvalidUUID tests that an invalid
// relation UUID returns an error.
func (s *watcherServiceSuite) TestWatchRelationEgressNetworksInvalidUUID(c *tc.C) {
	defer s.setupMocks(c).Finish()

	// Arrange
	invalidUUID := corerelation.UUID("invalid")

	// Act
	_, err := s.service.WatchRelationEgressNetworks(c.Context(), invalidUUID)

	// Assert
	c.Assert(err, tc.ErrorMatches, ".*")
}

// TestWatchRelationEgressNetworksWatcherError tests error handling
// when watcher creation fails.
func (s *watcherServiceSuite) TestWatchRelationEgressNetworksWatcherError(c *tc.C) {
	defer s.setupMocks(c).Finish()

	// Arrange
	relationUUID := corerelationtesting.GenRelationUUID(c)
	relationEgressNS := "relation_network_egress"
	modelConfigNS := "model_config"
	ipAddressNS := "ip_address"

	query := func(ctx context.Context, db database.TxnRunner) ([]string, error) {
		return []string{}, nil
	}

	s.modelState.EXPECT().NamespacesForRelationEgressNetworksWatcher().Return(
		relationEgressNS, modelConfigNS, ipAddressNS,
	)
	s.modelState.EXPECT().InitialWatchStatementForRelationEgressNetworks(
		relationUUID.String(),
	).Return(query)

	expectedErr := errors.New("watcher creation failed")
	s.watcherFactory.EXPECT().NewNamespaceMapperWatcher(
		gomock.Any(),
		gomock.Any(),
		gomock.Any(),
		gomock.Any(),
		gomock.Any(),
		gomock.Any(),
		gomock.Any(),
	).Return(nil, expectedErr)

	// Act
	_, err := s.service.WatchRelationEgressNetworks(c.Context(), relationUUID)

	// Assert
	c.Assert(err, tc.ErrorMatches, "watcher creation failed")
}

// TestGetEgressCIDRsEmptyUnits tests that getEgressCIDRs returns empty
// when there are no units in the relation.
func (s *watcherServiceSuite) TestGetEgressCIDRsEmptyUnits(c *tc.C) {
	defer s.setupMocks(c).Finish()

	// Arrange
	relationUUID := corerelationtesting.GenRelationUUID(c)

	s.modelState.EXPECT().GetUnitAddressesForRelation(
		gomock.Any(),
		relationUUID.String(),
	).Return(map[string]network.SpaceAddresses{}, nil)

	// Act
	result, err := s.service.getEgressCIDRs(c.Context(), relationUUID.String())

	// Assert
	c.Assert(err, tc.IsNil)
	c.Assert(result, tc.DeepEquals, []string{})
}

// TestGetEgressCIDRsRelationSpecific tests that getEgressCIDRs returns
// relation-specific egress CIDRs when available (priority 1).
func (s *watcherServiceSuite) TestGetEgressCIDRsRelationSpecific(c *tc.C) {
	defer s.setupMocks(c).Finish()

	// Arrange
	relationUUID := corerelationtesting.GenRelationUUID(c)
	unitAddresses := map[string]network.SpaceAddresses{
		"unit-1": {},
	}
	relationCIDRs := []string{"10.0.0.0/24"}

	s.modelState.EXPECT().GetUnitAddressesForRelation(
		gomock.Any(),
		relationUUID.String(),
	).Return(unitAddresses, nil)

	s.modelState.EXPECT().GetRelationNetworkEgress(
		gomock.Any(),
		relationUUID.String(),
	).Return(relationCIDRs, nil)

	// Act
	result, err := s.service.getEgressCIDRs(c.Context(), relationUUID.String())

	// Assert
	c.Assert(err, tc.IsNil)
	c.Assert(result, tc.DeepEquals, relationCIDRs)
}

// TestGetEgressCIDRsModelConfig tests that getEgressCIDRs returns
// model config egress-subnets when available and no relation-specific CIDRs
// exist (priority 2).
func (s *watcherServiceSuite) TestGetEgressCIDRsModelConfig(c *tc.C) {
	defer s.setupMocks(c).Finish()

	// Arrange
	relationUUID := corerelationtesting.GenRelationUUID(c)
	unitAddresses := map[string]network.SpaceAddresses{
		"unit-1": {},
	}
	modelCIDRs := []string{"192.168.0.0/16"}

	s.modelState.EXPECT().GetUnitAddressesForRelation(
		gomock.Any(),
		relationUUID.String(),
	).Return(unitAddresses, nil)

	s.modelState.EXPECT().GetRelationNetworkEgress(
		gomock.Any(),
		relationUUID.String(),
	).Return([]string{}, nil)

	s.modelState.EXPECT().GetModelEgressSubnets(
		gomock.Any(),
	).Return(modelCIDRs, nil)

	// Act
	result, err := s.service.getEgressCIDRs(c.Context(), relationUUID.String())

	// Assert
	c.Assert(err, tc.IsNil)
	c.Assert(result, tc.DeepEquals, modelCIDRs)
}

// TestGetEgressCIDRsUnitAddresses tests that getEgressCIDRs returns
// unit public addresses converted to CIDRs when no relation-specific or
// model config CIDRs exist (priority 3).
func (s *watcherServiceSuite) TestGetEgressCIDRsUnitAddresses(c *tc.C) {
	defer s.setupMocks(c).Finish()

	// Arrange
	relationUUID := corerelationtesting.GenRelationUUID(c)
	unitAddresses := map[string]network.SpaceAddresses{
		"unit-1": network.NewSpaceAddresses("10.0.0.1"),
	}

	s.modelState.EXPECT().GetUnitAddressesForRelation(
		gomock.Any(),
		relationUUID.String(),
	).Return(unitAddresses, nil)

	s.modelState.EXPECT().GetRelationNetworkEgress(
		gomock.Any(),
		relationUUID.String(),
	).Return([]string{}, nil)

	s.modelState.EXPECT().GetModelEgressSubnets(
		gomock.Any(),
	).Return([]string{}, nil)

	// Act
	result, err := s.service.getEgressCIDRs(c.Context(), relationUUID.String())

	// Assert
	c.Assert(err, tc.IsNil)
	// Should have converted the address to a /32 CIDR
	c.Assert(len(result), tc.Equals, 1)
	c.Assert(result[0], tc.Matches, "10\\.0\\.0\\.1/.*")
}

// TestGetEgressCIDRsUnitAddressesError tests error handling when
// getting unit addresses fails.
func (s *watcherServiceSuite) TestGetEgressCIDRsUnitAddressesError(c *tc.C) {
	defer s.setupMocks(c).Finish()

	// Arrange
	relationUUID := corerelationtesting.GenRelationUUID(c)

	expectedErr := errors.New("database error")
	s.modelState.EXPECT().GetUnitAddressesForRelation(
		gomock.Any(),
		relationUUID.String(),
	).Return(nil, expectedErr)

	// Act
	_, err := s.service.getEgressCIDRs(c.Context(), relationUUID.String())

	// Assert
	c.Assert(err, tc.ErrorMatches, "database error")
}

// TestGetEgressCIDRsRelationNetworkEgressError tests error handling when
// getting relation network egress fails.
func (s *watcherServiceSuite) TestGetEgressCIDRsRelationNetworkEgressError(c *tc.C) {
	defer s.setupMocks(c).Finish()

	// Arrange
	relationUUID := corerelationtesting.GenRelationUUID(c)
	unitAddresses := map[string]network.SpaceAddresses{
		"unit-1": {},
	}

	s.modelState.EXPECT().GetUnitAddressesForRelation(
		gomock.Any(),
		relationUUID.String(),
	).Return(unitAddresses, nil)

	expectedErr := errors.New("database error")
	s.modelState.EXPECT().GetRelationNetworkEgress(
		gomock.Any(),
		relationUUID.String(),
	).Return(nil, expectedErr)

	// Act
	_, err := s.service.getEgressCIDRs(c.Context(), relationUUID.String())

	// Assert
	c.Assert(err, tc.ErrorMatches, "database error")
}

// TestGetEgressCIDRsModelEgressSubnetsError tests error handling when
// getting model egress subnets fails.
func (s *watcherServiceSuite) TestGetEgressCIDRsModelEgressSubnetsError(c *tc.C) {
	defer s.setupMocks(c).Finish()

	// Arrange
	relationUUID := corerelationtesting.GenRelationUUID(c)
	unitAddresses := map[string]network.SpaceAddresses{
		"unit-1": {},
	}

	s.modelState.EXPECT().GetUnitAddressesForRelation(
		gomock.Any(),
		relationUUID.String(),
	).Return(unitAddresses, nil)

	s.modelState.EXPECT().GetRelationNetworkEgress(
		gomock.Any(),
		relationUUID.String(),
	).Return([]string{}, nil)

	expectedErr := errors.New("database error")
	s.modelState.EXPECT().GetModelEgressSubnets(
		gomock.Any(),
	).Return(nil, expectedErr)

	// Act
	_, err := s.service.getEgressCIDRs(c.Context(), relationUUID.String())

	// Assert
	c.Assert(err, tc.ErrorMatches, "database error")
}
