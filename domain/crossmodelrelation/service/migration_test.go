// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package service

import (
	"testing"

	"github.com/juju/tc"
	"go.uber.org/mock/gomock"

	"github.com/juju/juju/domain/application/charm"
	applicationerrors "github.com/juju/juju/domain/application/errors"
	"github.com/juju/juju/domain/crossmodelrelation"
	loggertesting "github.com/juju/juju/internal/logger/testing"
	"github.com/juju/juju/internal/uuid"
)

type migrationSuite struct {
	modelMigrationState *MockModelMigrationState
}

func TestMigrationSuite(t *testing.T) {
	tc.Run(t, &migrationSuite{})
}

func (s *migrationSuite) TestImportOffers(c *tc.C) {
	defer s.setupMocks(c).Finish()

	// Arrange
	input := []crossmodelrelation.OfferImport{
		{
			UUID:            uuid.MustNewUUID(),
			Name:            "test",
			ApplicationName: "test",
			Endpoints:       []string{"db-admin"},
		}, {
			UUID:            uuid.MustNewUUID(),
			Name:            "second",
			ApplicationName: "apple",
			Endpoints:       []string{"identity"},
		},
	}
	s.modelMigrationState.EXPECT().ImportOffers(gomock.Any(), input).Return(nil)

	// Act
	err := s.service(c).ImportOffers(c.Context(), input)

	// Assert
	c.Assert(err, tc.ErrorIsNil)
}

func (s *migrationSuite) TestImportOffersFail(c *tc.C) {
	defer s.setupMocks(c).Finish()

	// Arrange
	input := []crossmodelrelation.OfferImport{
		{
			UUID:            uuid.MustNewUUID(),
			Name:            "second",
			ApplicationName: "apple",
			Endpoints:       []string{"identity"},
		},
	}
	s.modelMigrationState.EXPECT().ImportOffers(gomock.Any(), input).Return(applicationerrors.ApplicationNotFound)

	// Act
	err := s.service(c).ImportOffers(c.Context(), input)

	// Assert
	c.Assert(err, tc.ErrorIs, applicationerrors.ApplicationNotFound)
}

func (s *migrationSuite) TestImportRemoteApplications(c *tc.C) {
	defer s.setupMocks(c).Finish()

	// Arrange
	input := []RemoteApplicationImport{
		{
			Name:            "remote-app1",
			OfferUUID:       uuid.MustNewUUID().String(),
			URL:             "ctrl:admin/model.app1",
			SourceModelUUID: uuid.MustNewUUID().String(),
			Macaroon:        "macaroon-data",
			Endpoints: []crossmodelrelation.RemoteApplicationEndpoint{
				{
					Name:      "db",
					Role:      "provider",
					Interface: "mysql",
				},
			},
			Bindings:        map[string]string{"db": "alpha"},
			IsConsumerProxy: false,
		},
		{
			Name:            "remote-app2",
			OfferUUID:       uuid.MustNewUUID().String(),
			URL:             "ctrl:admin/model.app2",
			SourceModelUUID: uuid.MustNewUUID().String(),
			Macaroon:        "macaroon-data-2",
			Endpoints: []crossmodelrelation.RemoteApplicationEndpoint{
				{
					Name:      "endpoint",
					Role:      "requirer",
					Interface: "http",
				},
			},
			Bindings:        map[string]string{"endpoint": "beta"},
			IsConsumerProxy: false,
		},
	}
	// Verify the service builds synthetic charms correctly
	s.modelMigrationState.EXPECT().ImportRemoteApplicationOfferers(
		gomock.Any(),
		syntheticCharmMatcher{
			expectedApps: input,
		},
	).Return(nil)

	// Act
	err := s.service(c).ImportRemoteApplications(c.Context(), input)

	// Assert
	c.Assert(err, tc.ErrorIsNil)
}

func (s *migrationSuite) TestImportRemoteApplicationsEmpty(c *tc.C) {
	defer s.setupMocks(c).Finish()

	// Arrange
	input := []RemoteApplicationImport{}
	s.modelMigrationState.EXPECT().ImportRemoteApplicationOfferers(
		gomock.Any(),
		syntheticCharmMatcher{
			expectedApps: input,
		},
	).Return(nil)

	// Act
	err := s.service(c).ImportRemoteApplications(c.Context(), input)

	// Assert
	c.Assert(err, tc.ErrorIsNil)
}

func (s *migrationSuite) TestImportRemoteApplicationsFail(c *tc.C) {
	defer s.setupMocks(c).Finish()

	// Arrange
	input := []RemoteApplicationImport{
		{
			Name:            "remote-app",
			OfferUUID:       uuid.MustNewUUID().String(),
			URL:             "ctrl:admin/model.app",
			SourceModelUUID: uuid.MustNewUUID().String(),
			Macaroon:        "macaroon-data",
			Endpoints: []crossmodelrelation.RemoteApplicationEndpoint{
				{
					Name:      "db",
					Role:      "provider",
					Interface: "mysql",
				},
			},
			IsConsumerProxy: false,
		},
	}
	s.modelMigrationState.EXPECT().ImportRemoteApplicationOfferers(
		gomock.Any(),
		syntheticCharmMatcher{
			expectedApps: input,
		},
	).Return(applicationerrors.ApplicationNotFound)

	// Act
	err := s.service(c).ImportRemoteApplications(c.Context(), input)

	// Assert
	c.Assert(err, tc.ErrorIs, applicationerrors.ApplicationNotFound)
}

func (s *migrationSuite) TestImportRemoteApplicationsPeerIgnored(c *tc.C) {
	defer s.setupMocks(c).Finish()

	// Arrange - import with a peer endpoint to verify it's ignored in synthetic charm
	input := []RemoteApplicationImport{
		{
			Name:            "remote-app",
			OfferUUID:       uuid.MustNewUUID().String(),
			URL:             "ctrl:admin/model.app",
			SourceModelUUID: uuid.MustNewUUID().String(),
			Macaroon:        "macaroon-data",
			Endpoints: []crossmodelrelation.RemoteApplicationEndpoint{
				{
					Name:      "provider-ep",
					Role:      "provider",
					Interface: "http",
				},
				{
					Name:      "peer-ep",
					Role:      "peer",
					Interface: "cluster",
				},
				{
					Name:      "requirer-ep",
					Role:      "requirer",
					Interface: "db",
				},
			},
			IsConsumerProxy: false,
		},
	}
	// Verify the synthetic charm excludes peer endpoints
	s.modelMigrationState.EXPECT().ImportRemoteApplicationOfferers(
		gomock.Any(),
		syntheticCharmMatcher{
			expectedApps: input,
		},
	).Return(nil)

	// Act
	err := s.service(c).ImportRemoteApplications(c.Context(), input)

	// Assert
	c.Assert(err, tc.ErrorIsNil)
}

func (s *migrationSuite) TestImportRemoteApplicationsConsumerProxyFiltered(c *tc.C) {
	defer s.setupMocks(c).Finish()

	// Arrange - import both a consumer proxy and a regular remote app
	// Consumer proxies should be filtered out by the service layer
	input := []RemoteApplicationImport{
		{
			Name:            "remote-consumer-proxy",
			OfferUUID:       uuid.MustNewUUID().String(),
			URL:             "ctrl:admin/model.app",
			SourceModelUUID: uuid.MustNewUUID().String(),
			Macaroon:        "macaroon-proxy",
			Endpoints: []crossmodelrelation.RemoteApplicationEndpoint{
				{
					Name:      "endpoint",
					Role:      "provider",
					Interface: "http",
				},
			},
			IsConsumerProxy: true, // Should be filtered out
		},
		{
			Name:            "remote-normal",
			OfferUUID:       uuid.MustNewUUID().String(),
			URL:             "ctrl:admin/model.normal",
			SourceModelUUID: uuid.MustNewUUID().String(),
			Macaroon:        "macaroon-normal",
			Endpoints: []crossmodelrelation.RemoteApplicationEndpoint{
				{
					Name:      "endpoint",
					Role:      "provider",
					Interface: "http",
				},
			},
			IsConsumerProxy: false, // Should be imported
		},
	}

	// Expected: only the non-consumer-proxy should be passed to state
	expectedToState := []RemoteApplicationImport{input[1]}
	s.modelMigrationState.EXPECT().ImportRemoteApplicationOfferers(
		gomock.Any(),
		syntheticCharmMatcher{
			expectedApps: expectedToState,
		},
	).Return(nil)

	// Act
	err := s.service(c).ImportRemoteApplications(c.Context(), input)

	// Assert
	c.Assert(err, tc.ErrorIsNil)
}

func (s *migrationSuite) TestImportRemoteApplicationsAllConsumerProxiesFiltered(c *tc.C) {
	defer s.setupMocks(c).Finish()

	// Arrange - all imports are consumer proxies
	input := []RemoteApplicationImport{
		{
			Name:            "remote-consumer-proxy-1",
			OfferUUID:       uuid.MustNewUUID().String(),
			URL:             "ctrl:admin/model.app1",
			SourceModelUUID: uuid.MustNewUUID().String(),
			Macaroon:        "macaroon-1",
			Endpoints: []crossmodelrelation.RemoteApplicationEndpoint{
				{
					Name:      "endpoint",
					Role:      "provider",
					Interface: "http",
				},
			},
			IsConsumerProxy: true,
		},
		{
			Name:            "remote-consumer-proxy-2",
			OfferUUID:       uuid.MustNewUUID().String(),
			URL:             "ctrl:admin/model.app2",
			SourceModelUUID: uuid.MustNewUUID().String(),
			Macaroon:        "macaroon-2",
			Endpoints: []crossmodelrelation.RemoteApplicationEndpoint{
				{
					Name:      "endpoint",
					Role:      "provider",
					Interface: "http",
				},
			},
			IsConsumerProxy: true,
		},
	}

	// Expected: empty slice passed to state
	s.modelMigrationState.EXPECT().ImportRemoteApplicationOfferers(
		gomock.Any(),
		syntheticCharmMatcher{
			expectedApps: []RemoteApplicationImport{},
		},
	).Return(nil)

	// Act
	err := s.service(c).ImportRemoteApplications(c.Context(), input)

	// Assert
	c.Assert(err, tc.ErrorIsNil)
}

// syntheticCharmMatcher is a custom gomock matcher that verifies
// RemoteApplicationImport slices have correctly built synthetic charms.
type syntheticCharmMatcher struct {
	expectedApps []RemoteApplicationImport
}

func (m syntheticCharmMatcher) Matches(x interface{}) bool {
	actual, ok := x.([]crossmodelrelation.RemoteApplicationImport)
	if !ok {
		return false
	}

	if len(actual) != len(m.expectedApps) {
		return false
	}

	for i, app := range actual {
		expected := m.expectedApps[i]

		// Verify basic fields match
		if app.Name != expected.Name ||
			app.OfferUUID != expected.OfferUUID ||
			app.URL != expected.URL ||
			app.SourceModelUUID != expected.SourceModelUUID ||
			app.Macaroon != expected.Macaroon {
			return false
		}

		// Verify endpoints match
		if len(app.Endpoints) != len(expected.Endpoints) {
			return false
		}
		for j, ep := range app.Endpoints {
			if ep.Name != expected.Endpoints[j].Name ||
				ep.Role != expected.Endpoints[j].Role ||
				ep.Interface != expected.Endpoints[j].Interface {
				return false
			}
		}

		// Verify synthetic charm was built correctly
		if app.SyntheticCharm.Metadata.Name != app.Name {
			return false
		}
		if app.SyntheticCharm.Source != charm.CMRSource {
			return false
		}
		if app.SyntheticCharm.ReferenceName != app.Name {
			return false
		}

		// Verify charm endpoints match input endpoints
		for _, ep := range expected.Endpoints {
			switch ep.Role {
			case "provider":
				rel, ok := app.SyntheticCharm.Metadata.Provides[ep.Name]
				if !ok || rel.Interface != ep.Interface {
					return false
				}
			case "requirer":
				rel, ok := app.SyntheticCharm.Metadata.Requires[ep.Name]
				if !ok || rel.Interface != ep.Interface {
					return false
				}
			case "peer":
				// Peer relations should not be in synthetic charm
				if _, inProvides := app.SyntheticCharm.Metadata.Provides[ep.Name]; inProvides {
					return false
				}
				if _, inRequires := app.SyntheticCharm.Metadata.Requires[ep.Name]; inRequires {
					return false
				}
			}
		}
	}

	return true
}

func (m syntheticCharmMatcher) String() string {
	return "matches RemoteApplicationImport with correctly built synthetic charms"
}

func (s *migrationSuite) service(c *tc.C) *MigrationService {
	return &MigrationService{
		modelState: s.modelMigrationState,
		logger:     loggertesting.WrapCheckLog(c),
	}
}

func (s *migrationSuite) setupMocks(c *tc.C) *gomock.Controller {
	ctrl := gomock.NewController(c)
	s.modelMigrationState = NewMockModelMigrationState(ctrl)

	c.Cleanup(func() {
		s.modelMigrationState = nil
	})
	return ctrl
}
