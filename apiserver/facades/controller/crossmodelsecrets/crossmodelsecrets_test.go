// Copyright 2023 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package crossmodelsecrets_test

import (
	"context"
	"testing"
	"time"

	"github.com/go-macaroon-bakery/macaroon-bakery/v3/bakery"
	"github.com/go-macaroon-bakery/macaroon-bakery/v3/bakery/checkers"
	"github.com/juju/clock"
	"github.com/juju/errors"
	"github.com/juju/names/v6"
	"github.com/juju/tc"
	"go.uber.org/mock/gomock"
	"gopkg.in/macaroon.v2"

	"github.com/juju/juju/apiserver/authentication"
	"github.com/juju/juju/apiserver/common"
	"github.com/juju/juju/apiserver/common/crossmodel"
	"github.com/juju/juju/apiserver/facades/controller/crossmodelsecrets"
	"github.com/juju/juju/apiserver/facades/controller/crossmodelsecrets/mocks"
	"github.com/juju/juju/core/model"
	coresecrets "github.com/juju/juju/core/secrets"
	unittesting "github.com/juju/juju/core/unit/testing"
	secretservice "github.com/juju/juju/domain/secret/service"
	secretbackendservice "github.com/juju/juju/domain/secretbackend/service"
	loggertesting "github.com/juju/juju/internal/logger/testing"
	"github.com/juju/juju/internal/secrets/provider"
	coretesting "github.com/juju/juju/internal/testing"
	"github.com/juju/juju/rpc/params"
)

func TestCrossModelSecretsSuite(t *testing.T) {
	tc.Run(t, &CrossModelSecretsSuite{})
}

type CrossModelSecretsSuite struct {
	coretesting.BaseSuite

	resources            *common.Resources
	secretService        *mocks.MockSecretService
	secretBackendService *mocks.MockSecretBackendService
	crossModelState      *mocks.MockCrossModelState
	stateBackend         *mocks.MockStateBackend

	facade *crossmodelsecrets.CrossModelSecretsAPI

	authContext *crossmodel.AuthContext
	bakery      authentication.ExpirableStorageBakery
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

func (m *mockBakery) ExpireStorageAfter(_ time.Duration) (authentication.ExpirableStorageBakery, error) {
	return m, nil
}

func (m *mockBakery) Auth(_ context.Context, mss ...macaroon.Slice) *bakery.AuthChecker {
	return m.Bakery.Checker.Auth(mss...)
}

func (m *mockBakery) NewMacaroon(ctx context.Context, version bakery.Version, caveats []checkers.Caveat, ops ...bakery.Op) (*bakery.Macaroon, error) {
	return m.Bakery.Oven.NewMacaroon(ctx, version, caveats, ops...)
}

func (s *CrossModelSecretsSuite) SetUpTest(c *tc.C) {
	s.BaseSuite.SetUpTest(c)
	s.resources = common.NewResources()
	s.AddCleanup(func(*tc.C) { s.resources.StopAll() })

	key, err := bakery.GenerateKey()
	c.Assert(err, tc.ErrorIsNil)
	locator := testLocator{key.Public}
	bakery := bakery.New(bakery.BakeryParams{
		Locator:       locator,
		Key:           bakery.MustGenerateKey(),
		OpsAuthorizer: crossmodel.CrossModelAuthorizer{},
	})
	s.bakery = &mockBakery{bakery}
	s.authContext, err = crossmodel.NewAuthContext(
		nil, nil, coretesting.ModelTag, key, crossmodel.NewOfferBakeryForTest(s.bakery, clock.WallClock),
	)
	c.Assert(err, tc.ErrorIsNil)
}

func (s *CrossModelSecretsSuite) setup(c *tc.C) *gomock.Controller {
	ctrl := gomock.NewController(c)

	s.secretService = mocks.NewMockSecretService(ctrl)
	s.secretBackendService = mocks.NewMockSecretBackendService(ctrl)
	s.crossModelState = mocks.NewMockCrossModelState(ctrl)
	s.stateBackend = mocks.NewMockStateBackend(ctrl)

	secretsServiceGetter := func(context.Context, model.UUID) (crossmodelsecrets.SecretService, error) {
		return s.secretService, nil
	}

	var err error
	s.facade, err = crossmodelsecrets.NewCrossModelSecretsAPI(
		s.resources,
		s.authContext,
		coretesting.ControllerTag.Id(),
		model.UUID(coretesting.ModelTag.Id()),
		secretsServiceGetter,
		s.secretBackendService,
		s.crossModelState,
		s.stateBackend,
		loggertesting.WrapCheckLog(c),
	)
	c.Assert(err, tc.ErrorIsNil)

	return ctrl
}

func ptr[T any](v T) *T {
	return &v
}

func (s *CrossModelSecretsSuite) TestGetSecretContentInfo(c *tc.C) {
	s.assertGetSecretContentInfo(c, false)
}

func (s *CrossModelSecretsSuite) TestGetSecretContentInfoNewConsumer(c *tc.C) {
	s.assertGetSecretContentInfo(c, true)
}

type backendConfigParamsMatcher struct {
	c        *tc.C
	expected any
}

func (m backendConfigParamsMatcher) Matches(x interface{}) bool {
	obtained, ok := x.(secretbackendservice.BackendConfigParams)
	if !ok {
		return false
	}
	m.c.Assert(obtained.GrantedSecretsGetter, tc.NotNil)
	obtained.GrantedSecretsGetter = nil
	m.c.Assert(obtained, tc.DeepEquals, m.expected)
	return true
}

func (m backendConfigParamsMatcher) String() string {
	return "Match the contents of BackendConfigParams"
}

func (s *CrossModelSecretsSuite) assertGetSecretContentInfo(c *tc.C, newConsumer bool) {
	defer s.setup(c).Finish()

	uri := coresecrets.NewURI().WithSource(coretesting.ModelTag.Id())
	app := names.NewApplicationTag("remote-app")
	relation := names.NewRelationTag("remote-app:foo local-app:foo")
	s.crossModelState.EXPECT().GetRemoteApplicationTag("token").Return(app, nil)
	s.stateBackend.EXPECT().HasEndpoint(relation.Id(), "remote-app").Return(true, nil)

	// Remote app 2 has incorrect relation.
	app2 := names.NewApplicationTag("remote-app2")
	s.crossModelState.EXPECT().GetRemoteApplicationTag("token2").Return(app2, nil)
	s.stateBackend.EXPECT().HasEndpoint(relation.Id(), "remote-app2").Return(false, nil)

	consumer := secretservice.SecretAccessor{
		Kind: secretservice.UnitAccessor,
		ID:   "remote-app/666",
	}
	s.secretService.EXPECT().UpdateRemoteConsumedRevision(gomock.Any(), uri, unittesting.GenNewName(c, "remote-app/666"), true).Return(667, nil)
	s.secretService.EXPECT().GetSecretValue(gomock.Any(), uri, 667, consumer).Return(
		nil,
		&coresecrets.ValueRef{
			BackendID:  "backend-id",
			RevisionID: "rev-id",
		}, nil,
	)
	s.secretBackendService.EXPECT().BackendConfigInfo(gomock.Any(), backendConfigParamsMatcher{c: c, expected: secretbackendservice.BackendConfigParams{
		Accessor:       consumer,
		ModelUUID:      model.UUID(coretesting.ModelTag.Id()),
		BackendIDs:     []string{"backend-id"},
		SameController: false,
	}}).Return(&provider.ModelBackendConfigInfo{
		ActiveID: "active-id",
		Configs: map[string]provider.ModelBackendConfig{
			"backend-id": {
				ControllerUUID: coretesting.ControllerTag.Id(),
				ModelUUID:      coretesting.ModelTag.Id(),
				ModelName:      "fred",
				BackendConfig: provider.BackendConfig{
					BackendType: "vault",
					Config:      map[string]interface{}{"foo": "bar"},
				},
			},
		},
	}, nil)

	mac, err := s.bakery.NewMacaroon(
		c.Context(),
		bakery.LatestVersion,
		[]checkers.Caveat{
			checkers.DeclaredCaveat("username", "mary"),
			checkers.DeclaredCaveat("offer-uuid", "some-offer"),
			checkers.DeclaredCaveat("source-model-uuid", coretesting.ModelTag.Id()),
			checkers.DeclaredCaveat("relation-key", relation.Id()),
		}, bakery.Op{"consume", "mysql-uuid"})
	c.Assert(err, tc.ErrorIsNil)

	args := params.GetRemoteSecretContentArgs{
		Args: []params.GetRemoteSecretContentArg{{
			SourceControllerUUID: "deadbeef-1bad-500d-9000-4b1d0d06f666",
			ApplicationToken:     "token",
			UnitId:               666,
			BakeryVersion:        3,
			Macaroons:            macaroon.Slice{mac.M()},
			URI:                  uri.String(),
			Refresh:              true,
		}, {
			URI: coresecrets.NewURI().String(),
		}, {
			URI: uri.String(),
		}, {
			SourceControllerUUID: "deadbeef-1bad-500d-9000-4b1d0d06f666",
			ApplicationToken:     "token2",
			UnitId:               666,
			BakeryVersion:        3,
			Macaroons:            macaroon.Slice{mac.M()},
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
			LatestRevision: ptr(667),
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
