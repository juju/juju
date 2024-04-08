// Copyright 2023 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package crossmodelsecrets_test

import (
	"context"
	"time"

	"github.com/go-macaroon-bakery/macaroon-bakery/v3/bakery"
	"github.com/go-macaroon-bakery/macaroon-bakery/v3/bakery/checkers"
	"github.com/juju/clock"
	"github.com/juju/errors"
	"github.com/juju/loggo/v2"
	"github.com/juju/names/v5"
	jc "github.com/juju/testing/checkers"
	"go.uber.org/mock/gomock"
	gc "gopkg.in/check.v1"
	"gopkg.in/macaroon.v2"

	"github.com/juju/juju/apiserver/authentication"
	"github.com/juju/juju/apiserver/common"
	"github.com/juju/juju/apiserver/common/crossmodel"
	"github.com/juju/juju/apiserver/facades/controller/crossmodelsecrets"
	"github.com/juju/juju/apiserver/facades/controller/crossmodelsecrets/mocks"
	corelogger "github.com/juju/juju/core/logger"
	coresecrets "github.com/juju/juju/core/secrets"
	secretservice "github.com/juju/juju/domain/secret/service"
	"github.com/juju/juju/internal/secrets/provider"
	"github.com/juju/juju/rpc/params"
	coretesting "github.com/juju/juju/testing"
)

var _ = gc.Suite(&CrossModelSecretsSuite{})

type CrossModelSecretsSuite struct {
	coretesting.BaseSuite

	resources       *common.Resources
	secretService   *mocks.MockSecretService
	crossModelState *mocks.MockCrossModelState
	stateBackend    *mocks.MockStateBackend

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

func (s *CrossModelSecretsSuite) SetUpTest(c *gc.C) {
	s.BaseSuite.SetUpTest(c)
	s.resources = common.NewResources()
	s.AddCleanup(func(*gc.C) { s.resources.StopAll() })

	key, err := bakery.GenerateKey()
	c.Assert(err, jc.ErrorIsNil)
	locator := testLocator{key.Public}
	bakery := bakery.New(bakery.BakeryParams{
		Locator:       locator,
		Key:           bakery.MustGenerateKey(),
		OpsAuthorizer: crossmodel.CrossModelAuthorizer{},
	})
	s.bakery = &mockBakery{bakery}
	s.authContext, err = crossmodel.NewAuthContext(
		nil, key, crossmodel.NewOfferBakeryForTest(s.bakery, clock.WallClock),
	)
	c.Assert(err, jc.ErrorIsNil)
}

func (s *CrossModelSecretsSuite) setup(c *gc.C) *gomock.Controller {
	ctrl := gomock.NewController(c)

	s.secretService = mocks.NewMockSecretService(ctrl)
	s.crossModelState = mocks.NewMockCrossModelState(ctrl)
	s.stateBackend = mocks.NewMockStateBackend(ctrl)

	secretsStateGetter := func(modelUUID string) crossmodelsecrets.SecretService {
		return s.secretService
	}
	backendConfigGetter := func(_ context.Context, modelUUID string, sameController bool, backendID string, consumer names.Tag) (*provider.ModelBackendConfigInfo, error) {
		c.Assert(sameController, jc.IsFalse)
		c.Assert(backendID, gc.Equals, "backend-id")
		c.Assert(consumer.String(), gc.Equals, "unit-remote-app-666")
		return &provider.ModelBackendConfigInfo{
			ActiveID: "active-id",
			Configs: map[string]provider.ModelBackendConfig{
				"backend-id": {
					ControllerUUID: coretesting.ControllerTag.Id(),
					ModelUUID:      modelUUID,
					ModelName:      "fred",
					BackendConfig: provider.BackendConfig{
						BackendType: "vault",
						Config:      map[string]interface{}{"foo": "bar"},
					},
				},
			},
		}, nil
	}
	var err error
	s.facade, err = crossmodelsecrets.NewCrossModelSecretsAPI(
		s.resources,
		s.authContext,
		coretesting.ControllerTag.Id(),
		coretesting.ModelTag.Id(),
		secretsStateGetter,
		backendConfigGetter,
		s.crossModelState,
		s.stateBackend,
		loggo.GetLoggerWithTags("juju.apiserver.crossmodelsecrets", corelogger.SECRETS),
	)
	c.Assert(err, jc.ErrorIsNil)

	return ctrl
}

func ptr[T any](v T) *T {
	return &v
}

func (s *CrossModelSecretsSuite) TestGetSecretContentInfo(c *gc.C) {
	s.assertGetSecretContentInfo(c, false)
}

func (s *CrossModelSecretsSuite) TestGetSecretContentInfoNewConsumer(c *gc.C) {
	s.assertGetSecretContentInfo(c, true)
}

func (s *CrossModelSecretsSuite) assertGetSecretContentInfo(c *gc.C, newConsumer bool) {
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
	if newConsumer {
		s.secretService.EXPECT().GetSecretRemoteConsumer(gomock.Any(), uri, "remote-app/666").Return(nil, errors.NotFoundf(""))
	} else {
		s.secretService.EXPECT().GetSecretRemoteConsumer(gomock.Any(), uri, "remote-app/666").Return(&coresecrets.SecretConsumerMetadata{CurrentRevision: 69}, nil)
	}
	s.secretService.EXPECT().GetSecret(gomock.Any(), uri).Return(&coresecrets.SecretMetadata{
		LatestRevision: 667,
	}, nil)
	s.secretService.EXPECT().SaveSecretRemoteConsumer(gomock.Any(), uri, "remote-app/666", &coresecrets.SecretConsumerMetadata{
		CurrentRevision: 667,
		LatestRevision:  667,
	}).Return(nil)
	s.secretService.EXPECT().GetSecretAccess(gomock.Any(), uri, consumer).Return(coresecrets.RoleView, nil)
	s.secretService.EXPECT().GetSecretValue(gomock.Any(), uri, 667).Return(
		nil,
		&coresecrets.ValueRef{
			BackendID:  "backend-id",
			RevisionID: "rev-id",
		}, nil,
	)

	mac, err := s.bakery.NewMacaroon(
		context.Background(),
		bakery.LatestVersion,
		[]checkers.Caveat{
			checkers.DeclaredCaveat("username", "mary"),
			checkers.DeclaredCaveat("offer-uuid", "some-offer"),
			checkers.DeclaredCaveat("source-model-uuid", coretesting.ModelTag.Id()),
			checkers.DeclaredCaveat("relation-key", relation.Id()),
		}, bakery.Op{"consume", "mysql-uuid"})
	c.Assert(err, jc.ErrorIsNil)

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
	results, err := s.facade.GetSecretContentInfo(context.Background(), args)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(results, jc.DeepEquals, params.SecretContentResults{
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
