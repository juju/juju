// Copyright 2023 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package crossmodelsecrets_test

import (
	"context"
	"testing"

	"github.com/go-macaroon-bakery/macaroon-bakery/v3/bakery"
	"github.com/go-macaroon-bakery/macaroon-bakery/v3/bakery/checkers"
	"github.com/juju/errors"
	"github.com/juju/names/v6"
	"github.com/juju/tc"
	"go.uber.org/mock/gomock"
	"gopkg.in/macaroon.v2"

	"github.com/juju/juju/apiserver/facades/controller/crossmodelsecrets"
	"github.com/juju/juju/core/application"
	"github.com/juju/juju/core/model"
	modeltesting "github.com/juju/juju/core/model/testing"
	"github.com/juju/juju/core/offer"
	"github.com/juju/juju/core/relation"
	coresecrets "github.com/juju/juju/core/secrets"
	"github.com/juju/juju/core/unit"
	secreterrors "github.com/juju/juju/domain/secret/errors"
	"github.com/juju/juju/domain/secret/service"
	secretbackendservice "github.com/juju/juju/domain/secretbackend/service"
	loggertesting "github.com/juju/juju/internal/logger/testing"
	"github.com/juju/juju/internal/secrets/provider"
	coretesting "github.com/juju/juju/internal/testing"
	"github.com/juju/juju/rpc/params"
)

type CrossModelSecretsSuite struct {
	coretesting.BaseSuite

	bakery *mockBakery

	modelUUID model.UUID

	authContext   *MockCrossModelAuthContext
	authenticator *MockMacaroonAuthenticator

	secretBackendService      *MockSecretBackendService
	secretService             *MockSecretService
	crossModelRelationService *MockCrossModelRelationService

	facade *crossmodelsecrets.CrossModelSecretsAPI
}

func TestCrossModelSecretsSuite(t *testing.T) {
	tc.Run(t, &CrossModelSecretsSuite{})
}

type testLocator struct {
	PublicKey bakery.PublicKey
}

func (b testLocator) ThirdPartyInfo(ctx context.Context, loc string) (bakery.ThirdPartyInfo, error) {
	if loc != "http://thirdparty" {
		return bakery.ThirdPartyInfo{}, errors.NotFoundf("location %v", loc)
	}
	return bakery.ThirdPartyInfo{
		PublicKey: b.PublicKey,
		Version:   bakery.LatestVersion,
	}, nil
}

type mockBakery struct {
	*bakery.Bakery
}

func (m *mockBakery) NewMacaroon(ctx context.Context, version bakery.Version, caveats []checkers.Caveat, ops ...bakery.Op) (*bakery.Macaroon, error) {
	return m.Bakery.Oven.NewMacaroon(ctx, version, caveats, ops...)
}

func (s *CrossModelSecretsSuite) SetUpTest(c *tc.C) {
	s.BaseSuite.SetUpTest(c)

	key, err := bakery.GenerateKey()
	c.Assert(err, tc.ErrorIsNil)
	locator := testLocator{key.Public}
	s.bakery = &mockBakery{Bakery: bakery.New(bakery.BakeryParams{
		Locator: locator,
		Key:     bakery.MustGenerateKey(),
	})}
}

func (s *CrossModelSecretsSuite) setup(c *tc.C) *gomock.Controller {
	ctrl := gomock.NewController(c)

	s.modelUUID = modeltesting.GenModelUUID(c)

	s.authenticator = NewMockMacaroonAuthenticator(ctrl)
	s.authContext = NewMockCrossModelAuthContext(ctrl)
	s.authContext.EXPECT().Authenticator().Return(s.authenticator).AnyTimes()

	s.secretBackendService = NewMockSecretBackendService(ctrl)
	s.secretService = NewMockSecretService(ctrl)
	s.crossModelRelationService = NewMockCrossModelRelationService(ctrl)

	secretServiceGetter := func(_ context.Context, modelUUID model.UUID) (crossmodelsecrets.SecretService, error) {
		return s.secretService, nil
	}
	crossModelServiceGetter := func(_ context.Context, modelUUID model.UUID) (crossmodelsecrets.CrossModelRelationService, error) {
		return s.crossModelRelationService, nil
	}

	var err error
	s.facade, err = crossmodelsecrets.NewCrossModelSecretsAPI(
		coretesting.ControllerTag.Id(),
		s.modelUUID,
		s.authContext,
		s.secretBackendService,
		secretServiceGetter,
		crossModelServiceGetter,
		loggertesting.WrapCheckLog(c),
	)
	c.Assert(err, tc.ErrorIsNil)

	s.AddCleanup(func(_ *tc.C) {
		s.authenticator = nil
		s.authContext = nil
		s.secretBackendService = nil
		s.secretService = nil
		s.crossModelRelationService = nil
	})

	return ctrl
}

func ptr[T any](v T) *T {
	return &v
}

func (s *CrossModelSecretsSuite) TestGetSecretContentInfo(c *tc.C) {
	defer s.setup(c).Finish()

	uri := coresecrets.NewURI().WithSource(coretesting.ModelTag.Id())

	appUUID := tc.Must(c, application.NewUUID)
	appUUID2 := tc.Must(c, application.NewUUID)

	s.crossModelRelationService.EXPECT().GetRemoteConsumerApplicationName(gomock.Any(), appUUID).Return("mediawiki", nil)
	s.crossModelRelationService.EXPECT().ProcessRemoteConsumerGetSecret(gomock.Any(), uri, unit.Name("mediawiki/666"), ptr(667), false, true).Return(
		nil,
		&coresecrets.ValueRef{
			BackendID:  "backend-id",
			RevisionID: "rev-id",
		}, 668, nil,
	)
	s.crossModelRelationService.EXPECT().GetRemoteConsumerApplicationName(gomock.Any(), appUUID2).Return("wordpress", nil)
	s.crossModelRelationService.EXPECT().ProcessRemoteConsumerGetSecret(gomock.Any(), uri, unit.Name("wordpress/666"), nil, false, true).Return(
		nil, nil, 0, secreterrors.PermissionDenied,
	)

	offerUUID := tc.Must(c, offer.NewUUID)

	mac, err := s.bakery.NewMacaroon(
		c.Context(),
		bakery.LatestVersion,
		[]checkers.Caveat{
			checkers.DeclaredCaveat("username", "mary"),
			checkers.DeclaredCaveat("offer-uuid", offerUUID.String()),
			checkers.DeclaredCaveat("source-model-uuid", uri.SourceUUID),
			checkers.DeclaredCaveat("relation-key", "mediawkik:server mysql:database"),
		}, bakery.Op{Entity: "consume", Action: "mysql-uuid"})
	c.Assert(err, tc.ErrorIsNil)
	mac2, err := s.bakery.NewMacaroon(
		c.Context(),
		bakery.LatestVersion,
		[]checkers.Caveat{
			checkers.DeclaredCaveat("username", "mary"),
			checkers.DeclaredCaveat("offer-uuid", offerUUID.String()),
			checkers.DeclaredCaveat("source-model-uuid", uri.SourceUUID),
			checkers.DeclaredCaveat("relation-key", "wordpress:server mysql:database"),
		}, bakery.Op{Entity: "consume", Action: "mysql-uuid"})
	c.Assert(err, tc.ErrorIsNil)

	relKey := tc.Must1(c, relation.NewKeyFromString, "mediawkik:server mysql:database")
	relTag := names.NewRelationTag(relKey.String())
	relKey2 := tc.Must1(c, relation.NewKeyFromString, "wordpress:server mysql:database")
	relTag2 := names.NewRelationTag(relKey2.String())
	s.crossModelRelationService.EXPECT().IsCrossModelRelationValidForApplication(gomock.Any(), relKey, "mediawiki").Return(true, nil)
	s.crossModelRelationService.EXPECT().IsCrossModelRelationValidForApplication(gomock.Any(), relKey2, "wordpress").Return(true, nil)
	s.authenticator.EXPECT().CheckRelationMacaroons(
		gomock.Any(), s.modelUUID.String(), offerUUID.String(), relTag, macaroon.Slice{mac.M()}, bakery.LatestVersion)
	s.authenticator.EXPECT().CheckRelationMacaroons(
		gomock.Any(), s.modelUUID.String(), offerUUID.String(), relTag2, macaroon.Slice{mac2.M()}, bakery.LatestVersion)

	s.secretBackendService.EXPECT().BackendConfigInfo(gomock.Any(), gomock.Any()).DoAndReturn(
		func(_ context.Context, params secretbackendservice.BackendConfigParams) (*provider.ModelBackendConfigInfo, error) {
			c.Assert(params.GrantedSecretsGetter, tc.NotNil)
			params.GrantedSecretsGetter = nil
			c.Assert(params, tc.DeepEquals, secretbackendservice.BackendConfigParams{
				Accessor: service.SecretAccessor{
					Kind: service.UnitAccessor,
					ID:   "mediawiki/666",
				},
				ModelUUID:      model.UUID(uri.SourceUUID),
				BackendIDs:     []string{"backend-id"},
				SameController: false,
			})
			return &provider.ModelBackendConfigInfo{
				ActiveID: "active-id",
				Configs: map[string]provider.ModelBackendConfig{
					"backend-id": {
						ControllerUUID: coretesting.ControllerTag.Id(),
						ModelUUID:      uri.SourceUUID,
						ModelName:      "fred",
						BackendConfig: provider.BackendConfig{
							BackendType: "vault",
							Config:      map[string]interface{}{"foo": "bar"},
						},
					},
				},
			}, nil
		})

	args := params.GetRemoteSecretContentArgs{
		Args: []params.GetRemoteSecretContentArg{{
			SourceControllerUUID: "deadbeef-1bad-500d-9000-4b1d0d06f666",
			ApplicationToken:     appUUID.String(),
			UnitId:               666,
			BakeryVersion:        3,
			Macaroons:            macaroon.Slice{mac.M()},
			URI:                  uri.String(),
			Revision:             ptr(667),
			Refresh:              true,
		}, {
			URI: coresecrets.NewURI().String(),
		}, {
			URI: uri.String(),
		}, {
			SourceControllerUUID: "deadbeef-1bad-500d-9000-4b1d0d06f666",
			ApplicationToken:     appUUID2.String(),
			UnitId:               666,
			BakeryVersion:        3,
			Macaroons:            macaroon.Slice{mac2.M()},
			URI:                  uri.String(),
			Refresh:              true,
		}},
	}
	results, err := s.facade.GetSecretContentInfo(c.Context(), args)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(results, tc.DeepEquals, params.SecretContentResults{
		Results: []params.SecretContentResult{{
			Content: params.SecretContentParams{
				ValueRef: &params.SecretValueRef{
					BackendID:  "backend-id",
					RevisionID: "rev-id",
				},
			},
			BackendConfig: &params.SecretBackendConfigResult{
				ControllerUUID: coretesting.ControllerTag.Id(),
				ModelUUID:      coretesting.ModelTag.Id(),
				ModelName:      "fred",
				Draining:       true,
				Config: params.SecretBackendConfig{
					BackendType: "vault",
					Params:      map[string]interface{}{"foo": "bar"},
				},
			},
			LatestRevision: ptr(668),
		}, {
			Error: &params.Error{
				Code:    "not valid",
				Message: "secret URI with empty source UUID not valid",
			},
		}, {
			Error: &params.Error{
				Code:    "not valid",
				Message: "empty secret revision not valid",
			},
		}, {
			Error: &params.Error{
				Code:    "unauthorized access",
				Message: "permission denied",
			},
		}},
	})
}
