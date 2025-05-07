// Copyright 2021 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package secretsmanager_test

import (
	"context"
	"time"

	"github.com/juju/clock"
	"github.com/juju/clock/testclock"
	"github.com/juju/errors"
	"github.com/juju/names/v6"
	"github.com/juju/tc"
	"github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	"go.uber.org/mock/gomock"
	"gopkg.in/macaroon.v2"

	facademocks "github.com/juju/juju/apiserver/facade/mocks"
	"github.com/juju/juju/apiserver/facades/agent/secretsmanager"
	"github.com/juju/juju/apiserver/facades/agent/secretsmanager/mocks"
	"github.com/juju/juju/core/model"
	coresecrets "github.com/juju/juju/core/secrets"
	unittesting "github.com/juju/juju/core/unit/testing"
	corewatcher "github.com/juju/juju/core/watcher"
	secreterrors "github.com/juju/juju/domain/secret/errors"
	secretservice "github.com/juju/juju/domain/secret/service"
	secretbackendservice "github.com/juju/juju/domain/secretbackend/service"
	"github.com/juju/juju/internal/secrets"
	"github.com/juju/juju/internal/secrets/provider"
	coretesting "github.com/juju/juju/internal/testing"
	jujutesting "github.com/juju/juju/juju/testing"
	"github.com/juju/juju/rpc/params"
)

type SecretsManagerSuite struct {
	testing.IsolationSuite

	authorizer      *facademocks.MockAuthorizer
	watcherRegistry *facademocks.MockWatcherRegistry

	leadership            *mocks.MockChecker
	token                 *mocks.MockToken
	secretBackendService  *mocks.MockSecretBackendService
	secretService         *mocks.MockSecretService
	secretsConsumer       *mocks.MockSecretsConsumer
	crossModelState       *mocks.MockCrossModelState
	remoteClient          *mocks.MockCrossModelSecretsClient
	secretsWatcher        *mocks.MockStringsWatcher
	secretTriggers        *mocks.MockSecretTriggers
	secretsTriggerWatcher *mocks.MockSecretTriggerWatcher
	authTag               names.Tag
	clock                 clock.Clock

	facade *secretsmanager.SecretsManagerAPI
}

var _ = tc.Suite(&SecretsManagerSuite{})

func (s *SecretsManagerSuite) SetUpTest(c *tc.C) {
	s.IsolationSuite.SetUpTest(c)

	s.authTag = names.NewUnitTag("mariadb/0")
}

func (s *SecretsManagerSuite) setup(c *tc.C) *gomock.Controller {
	ctrl := gomock.NewController(c)

	s.authorizer = facademocks.NewMockAuthorizer(ctrl)
	s.watcherRegistry = facademocks.NewMockWatcherRegistry(ctrl)

	s.leadership = mocks.NewMockChecker(ctrl)
	s.token = mocks.NewMockToken(ctrl)
	s.secretService = mocks.NewMockSecretService(ctrl)
	s.secretBackendService = mocks.NewMockSecretBackendService(ctrl)
	s.secretsConsumer = mocks.NewMockSecretsConsumer(ctrl)
	s.crossModelState = mocks.NewMockCrossModelState(ctrl)
	s.secretsWatcher = mocks.NewMockStringsWatcher(ctrl)
	s.secretTriggers = mocks.NewMockSecretTriggers(ctrl)
	s.secretsTriggerWatcher = mocks.NewMockSecretTriggerWatcher(ctrl)
	s.expectAuthUnitAgent()

	s.clock = testclock.NewClock(time.Now())

	remoteClientGetter := func(_ context.Context, uri *coresecrets.URI) (secretsmanager.CrossModelSecretsClient, error) {
		return s.remoteClient, nil
	}

	var err error
	s.facade, err = secretsmanager.NewTestAPI(
		c,
		s.authorizer, s.watcherRegistry, s.leadership, s.secretService, s.secretsConsumer,
		s.secretTriggers, s.secretBackendService, remoteClientGetter,
		s.crossModelState, s.authTag, s.clock,
	)
	c.Assert(err, jc.ErrorIsNil)

	return ctrl
}

func (s *SecretsManagerSuite) expectAuthUnitAgent() {
	s.authorizer.EXPECT().AuthUnitAgent().Return(true)
}

func ptr[T any](v T) *T {
	return &v
}

type backendConfigParamsMatcher struct {
	c        *tc.C
	expected any
}

func (m backendConfigParamsMatcher) Matches(x interface{}) bool {
	if obtained, ok := x.(secretbackendservice.BackendConfigParams); ok {
		m.c.Assert(obtained.GrantedSecretsGetter, tc.NotNil)
		obtained.GrantedSecretsGetter = nil
		m.c.Assert(obtained, jc.DeepEquals, m.expected)
		return true
	}
	obtained, ok := x.(secretbackendservice.DrainBackendConfigParams)
	if !ok {
		return false
	}
	m.c.Assert(obtained.GrantedSecretsGetter, tc.NotNil)
	obtained.GrantedSecretsGetter = nil
	m.c.Assert(obtained, jc.DeepEquals, m.expected)
	return true
}

func (m backendConfigParamsMatcher) String() string {
	return "Match the contents of BackendConfigParams"
}

func (s *SecretsManagerSuite) TestGetSecretBackendConfigs(c *tc.C) {
	defer s.setup(c).Finish()

	s.leadership.EXPECT().LeadershipCheck("mariadb", "mariadb/0").Return(s.token)
	s.secretBackendService.EXPECT().BackendConfigInfo(gomock.Any(), backendConfigParamsMatcher{c: c,
		expected: secretbackendservice.BackendConfigParams{
			LeaderToken: s.token,
			Accessor: secretservice.SecretAccessor{
				Kind: secretservice.UnitAccessor,
				ID:   "mariadb/0",
			},
			ModelUUID:      model.UUID(coretesting.ModelTag.Id()),
			BackendIDs:     []string{"backend-id"},
			SameController: true,
		}}).Return(&provider.ModelBackendConfigInfo{
		ActiveID: "backend-id",
		Configs: map[string]provider.ModelBackendConfig{
			"backend-id": {
				ControllerUUID: coretesting.ControllerTag.Id(),
				ModelUUID:      coretesting.ModelTag.Id(),
				ModelName:      "fred",
				BackendConfig: provider.BackendConfig{
					BackendType: "some-backend",
					Config:      map[string]interface{}{"foo": "bar"},
				},
			},
		},
	}, nil)

	result, err := s.facade.GetSecretBackendConfigs(context.Background(), params.SecretBackendArgs{
		BackendIDs: []string{"backend-id"},
	})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(result, jc.DeepEquals, params.SecretBackendConfigResults{
		ActiveID: "backend-id",
		Results: map[string]params.SecretBackendConfigResult{
			"backend-id": {
				ControllerUUID: coretesting.ControllerTag.Id(),
				ModelUUID:      coretesting.ModelTag.Id(),
				ModelName:      "fred",
				Draining:       false,
				Config: params.SecretBackendConfig{
					BackendType: "some-backend",
					Params:      map[string]interface{}{"foo": "bar"},
				},
			},
		},
	})
}

func (s *SecretsManagerSuite) TestGetSecretBackendConfigsForDrain(c *tc.C) {
	defer s.setup(c).Finish()

	s.leadership.EXPECT().LeadershipCheck("mariadb", "mariadb/0").Return(s.token)
	s.secretBackendService.EXPECT().DrainBackendConfigInfo(gomock.Any(), backendConfigParamsMatcher{c: c,
		expected: secretbackendservice.DrainBackendConfigParams{
			LeaderToken: s.token,
			Accessor: secretservice.SecretAccessor{
				Kind: secretservice.UnitAccessor,
				ID:   "mariadb/0",
			},
			ModelUUID: model.UUID(coretesting.ModelTag.Id()),
			BackendID: "backend-id",
		}}).Return(&provider.ModelBackendConfigInfo{
		ActiveID: "backend-id",
		Configs: map[string]provider.ModelBackendConfig{
			"backend-id": {
				ControllerUUID: coretesting.ControllerTag.Id(),
				ModelUUID:      coretesting.ModelTag.Id(),
				ModelName:      "fred",
				BackendConfig: provider.BackendConfig{
					BackendType: "some-backend",
					Config:      map[string]interface{}{"foo": "admin"},
				},
			},
		},
	}, nil)

	result, err := s.facade.GetSecretBackendConfigs(context.Background(), params.SecretBackendArgs{
		ForDrain:   true,
		BackendIDs: []string{"backend-id"},
	})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(result, jc.DeepEquals, params.SecretBackendConfigResults{
		ActiveID: "backend-id",
		Results: map[string]params.SecretBackendConfigResult{
			"backend-id": {
				ControllerUUID: coretesting.ControllerTag.Id(),
				ModelUUID:      coretesting.ModelTag.Id(),
				ModelName:      "fred",
				Draining:       true,
				Config: params.SecretBackendConfig{
					BackendType: "some-backend",
					Params:      map[string]interface{}{"foo": "admin"},
				},
			},
		},
	})
}

func (s *SecretsManagerSuite) TestCreateSecretURIs(c *tc.C) {
	defer s.setup(c).Finish()

	uri1 := coresecrets.NewURI()
	uri2 := coresecrets.NewURI()
	s.secretService.EXPECT().CreateSecretURIs(gomock.Any(), 2).Return([]*coresecrets.URI{uri1, uri2}, nil)

	results, err := s.facade.CreateSecretURIs(context.Background(), params.CreateSecretURIsArg{
		Count: 2,
	})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(results.Results, tc.HasLen, 2)
	for _, r := range results.Results {
		_, err := coresecrets.ParseURI(r.Result)
		c.Assert(err, jc.ErrorIsNil)
	}
}

func (s *SecretsManagerSuite) TestGetConsumerSecretsRevisionInfoHavingConsumerLabel(c *tc.C) {
	defer s.setup(c).Finish()

	uri := coresecrets.NewURI()
	s.secretsConsumer.EXPECT().GetSecretConsumerAndLatest(gomock.Any(), uri, unittesting.GenNewName(c, "mariadb/0")).Return(
		&coresecrets.SecretConsumerMetadata{
			Label: "label",
		}, 666, nil)

	results, err := s.facade.GetConsumerSecretsRevisionInfo(context.Background(), params.GetSecretConsumerInfoArgs{
		ConsumerTag: "unit-mariadb/0",
		URIs:        []string{uri.String()},
	})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(results, jc.DeepEquals, params.SecretConsumerInfoResults{
		Results: []params.SecretConsumerInfoResult{{
			Label:    "label",
			Revision: 666,
		}},
	})
}

func (s *SecretsManagerSuite) TestGetConsumerSecretsRevisionInfoHavingNoConsumerLabel(c *tc.C) {
	defer s.setup(c).Finish()

	uri := coresecrets.NewURI()
	s.secretsConsumer.EXPECT().GetSecretConsumerAndLatest(gomock.Any(), uri, unittesting.GenNewName(c, "mariadb/0")).Return(
		&coresecrets.SecretConsumerMetadata{
			CurrentRevision: 665,
		}, 666, nil)

	results, err := s.facade.GetConsumerSecretsRevisionInfo(context.Background(), params.GetSecretConsumerInfoArgs{
		ConsumerTag: "unit-mariadb/0",
		URIs:        []string{uri.String()},
	})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(results, jc.DeepEquals, params.SecretConsumerInfoResults{
		Results: []params.SecretConsumerInfoResult{{
			Revision: 666,
		}},
	})
}

func (s *SecretsManagerSuite) TestGetConsumerSecretsRevisionInfoForPeerUnitsAccessingAppOwnedSecrets(c *tc.C) {
	defer s.setup(c).Finish()

	uri := coresecrets.NewURI()
	s.secretsConsumer.EXPECT().GetSecretConsumerAndLatest(gomock.Any(), uri, unittesting.GenNewName(c, "mariadb/0")).Return(
		&coresecrets.SecretConsumerMetadata{
			CurrentRevision: 665,
			Label:           "owner-label",
		}, 666, nil)

	results, err := s.facade.GetConsumerSecretsRevisionInfo(context.Background(), params.GetSecretConsumerInfoArgs{
		ConsumerTag: "unit-mariadb/0",
		URIs:        []string{uri.String()},
	})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(results, jc.DeepEquals, params.SecretConsumerInfoResults{
		Results: []params.SecretConsumerInfoResult{{
			Label:    "owner-label",
			Revision: 666,
		}},
	})
}

func (s *SecretsManagerSuite) TestGetSecretMetadata(c *tc.C) {
	defer s.setup(c).Finish()

	s.leadership.EXPECT().LeadershipCheck("mariadb", "mariadb/0").Return(s.token)
	s.token.EXPECT().Check().Return(nil)

	now := time.Now()
	uri := coresecrets.NewURI()
	s.secretService.EXPECT().ListCharmSecrets(gomock.Any(), []secretservice.CharmSecretOwner{{
		Kind: secretservice.UnitOwner,
		ID:   "mariadb/0",
	}, {
		Kind: secretservice.ApplicationOwner,
		ID:   "mariadb",
	}}).Return([]*coresecrets.SecretMetadata{{
		URI:                    uri,
		Owner:                  coresecrets.Owner{Kind: coresecrets.ApplicationOwner, ID: "mariadb"},
		Description:            "description",
		Label:                  "label",
		RotatePolicy:           coresecrets.RotateHourly,
		LatestRevision:         666,
		LatestRevisionChecksum: "deadbeef",
		LatestExpireTime:       &now,
		NextRotateTime:         &now,
	}}, [][]*coresecrets.SecretRevisionMetadata{{
		{
			Revision: 666,
			ValueRef: &coresecrets.ValueRef{
				BackendID:  "backend-id",
				RevisionID: "rev-id",
			},
		}, {
			Revision: 667,
		},
	}}, nil)
	s.secretService.EXPECT().GetSecretGrants(gomock.Any(), uri, coresecrets.RoleView).Return([]secretservice.SecretAccess{
		{
			Scope: secretservice.SecretAccessScope{
				Kind: secretservice.RelationAccessScope,
				ID:   "gitlab:server mysql:db",
			},
			Subject: secretservice.SecretAccessor{
				Kind: secretservice.ApplicationAccessor,
				ID:   "gitlab",
			},
			Role: coresecrets.RoleView,
		},
	}, nil)

	results, err := s.facade.GetSecretMetadata(context.Background())
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(results, jc.DeepEquals, params.ListSecretResults{
		Results: []params.ListSecretResult{{
			URI:                    uri.String(),
			OwnerTag:               "application-mariadb",
			Description:            "description",
			Label:                  "label",
			RotatePolicy:           coresecrets.RotateHourly.String(),
			LatestRevision:         666,
			LatestRevisionChecksum: "deadbeef",
			LatestExpireTime:       &now,
			NextRotateTime:         &now,
			Revisions: []params.SecretRevision{{
				Revision: 666,
				ValueRef: &params.SecretValueRef{
					BackendID:  "backend-id",
					RevisionID: "rev-id",
				},
			}, {
				Revision: 667,
			}},
			Access: []params.AccessInfo{
				{TargetTag: "application-gitlab", ScopeTag: "relation-gitlab.server#mysql.db", Role: "view"},
			},
		}},
	})
}

func (s *SecretsManagerSuite) TestGetSecretContentInvalidArg(c *tc.C) {
	defer s.setup(c).Finish()

	results, err := s.facade.GetSecretContentInfo(context.Background(), params.GetSecretContentArgs{
		Args: []params.GetSecretContentArg{{}},
	})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(results.Results[0].Error, tc.ErrorMatches, `both uri and label are empty`)
}

func (s *SecretsManagerSuite) TestGetSecretContentForOwnerSecretURIArg(c *tc.C) {
	defer s.setup(c).Finish()

	data := map[string]string{"foo": "bar"}
	val := coresecrets.NewSecretValue(data)
	uri := coresecrets.NewURI()

	s.secretService.EXPECT().ProcessCharmSecretConsumerLabel(gomock.Any(), unittesting.GenNewName(c, "mariadb/0"), uri, "").Return(uri, nil, nil)

	s.secretsConsumer.EXPECT().GetConsumedRevision(gomock.Any(), uri, unittesting.GenNewName(c, "mariadb/0"), false, false, nil).
		Return(668, nil)

	s.secretService.EXPECT().GetSecretValue(gomock.Any(), uri, 668, secretservice.SecretAccessor{
		Kind: secretservice.UnitAccessor,
		ID:   "mariadb/0",
	}).Return(
		val, nil, nil,
	)

	results, err := s.facade.GetSecretContentInfo(context.Background(), params.GetSecretContentArgs{
		Args: []params.GetSecretContentArg{
			{URI: uri.String()},
		},
	})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(results, jc.DeepEquals, params.SecretContentResults{
		Results: []params.SecretContentResult{{
			Content: params.SecretContentParams{Data: data},
		}},
	})
}

func (s *SecretsManagerSuite) TestGetSecretContentForOwnerSecretLabelArg(c *tc.C) {
	defer s.setup(c).Finish()

	data := map[string]string{"foo": "bar"}
	val := coresecrets.NewSecretValue(data)
	uri := coresecrets.NewURI()

	s.secretService.EXPECT().ProcessCharmSecretConsumerLabel(gomock.Any(), unittesting.GenNewName(c, "mariadb/0"), nil, "foo").Return(uri, nil, nil)

	s.secretsConsumer.EXPECT().GetConsumedRevision(gomock.Any(), uri, unittesting.GenNewName(c, "mariadb/0"), false, false, nil).
		Return(668, nil)

	s.secretService.EXPECT().GetSecretValue(gomock.Any(), uri, 668, secretservice.SecretAccessor{
		Kind: secretservice.UnitAccessor,
		ID:   "mariadb/0",
	}).Return(
		val, nil, nil,
	)

	results, err := s.facade.GetSecretContentInfo(context.Background(), params.GetSecretContentArgs{
		Args: []params.GetSecretContentArg{
			{Label: "foo"},
		},
	})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(results, jc.DeepEquals, params.SecretContentResults{
		Results: []params.SecretContentResult{{
			Content: params.SecretContentParams{Data: data},
		}},
	})
}

func (s *SecretsManagerSuite) TestGetSecretContentForAppSecretUpdateLabel(c *tc.C) {
	defer s.setup(c).Finish()

	data := map[string]string{"foo": "bar"}
	val := coresecrets.NewSecretValue(data)
	uri := coresecrets.NewURI()

	s.secretService.EXPECT().ProcessCharmSecretConsumerLabel(gomock.Any(), unittesting.GenNewName(c, "mariadb/0"), uri, "foo").Return(uri, nil, nil)

	s.secretsConsumer.EXPECT().GetConsumedRevision(gomock.Any(), uri, unittesting.GenNewName(c, "mariadb/0"), false, false, nil).
		Return(668, nil)
	s.secretService.EXPECT().GetSecretValue(gomock.Any(), uri, 668, secretservice.SecretAccessor{
		Kind: secretservice.UnitAccessor,
		ID:   "mariadb/0",
	}).Return(
		val, nil, nil,
	)

	results, err := s.facade.GetSecretContentInfo(context.Background(), params.GetSecretContentArgs{
		Args: []params.GetSecretContentArg{
			{URI: uri.String(), Label: "foo"},
		},
	})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(results, jc.DeepEquals, params.SecretContentResults{
		Results: []params.SecretContentResult{{
			Content: params.SecretContentParams{Data: data},
		}},
	})
}

func (s *SecretsManagerSuite) TestGetSecretContentForUnitAccessApplicationOwnedSecret(c *tc.C) {
	defer s.setup(c).Finish()

	data := map[string]string{"foo": "bar"}
	val := coresecrets.NewSecretValue(data)
	uri := coresecrets.NewURI()

	s.secretService.EXPECT().ProcessCharmSecretConsumerLabel(gomock.Any(), unittesting.GenNewName(c, "mariadb/0"), nil, "foo").Return(uri, nil, nil)

	s.secretsConsumer.EXPECT().GetConsumedRevision(gomock.Any(), uri, unittesting.GenNewName(c, "mariadb/0"), false, false, nil).
		Return(668, nil)

	s.secretService.EXPECT().GetSecretValue(gomock.Any(), uri, 668, secretservice.SecretAccessor{
		Kind: secretservice.UnitAccessor,
		ID:   "mariadb/0",
	}).Return(
		val, nil, nil,
	)

	results, err := s.facade.GetSecretContentInfo(context.Background(), params.GetSecretContentArgs{
		Args: []params.GetSecretContentArg{
			{Label: "foo"},
		},
	})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(results, jc.DeepEquals, params.SecretContentResults{
		Results: []params.SecretContentResult{{
			Content: params.SecretContentParams{Data: data},
		}},
	})
}

func (s *SecretsManagerSuite) TestGetSecretContentConsumerUnitAgent(c *tc.C) {
	s.authTag = names.NewUnitTag("mariadb/0")

	defer s.setup(c).Finish()

	data := map[string]string{"foo": "bar"}
	val := coresecrets.NewSecretValue(data)
	uri := coresecrets.NewURI()

	s.secretService.EXPECT().ProcessCharmSecretConsumerLabel(gomock.Any(), unittesting.GenNewName(c, "mariadb/0"), uri, "").Return(uri, nil, nil)
	s.secretsConsumer.EXPECT().GetConsumedRevision(gomock.Any(), uri, unittesting.GenNewName(c, "mariadb/0"), false, false, nil).
		Return(666, nil)
	s.secretService.EXPECT().GetSecretValue(gomock.Any(), uri, 666, secretservice.SecretAccessor{
		Kind: secretservice.UnitAccessor,
		ID:   "mariadb/0",
	}).Return(
		val, nil, nil,
	)

	results, err := s.facade.GetSecretContentInfo(context.Background(), params.GetSecretContentArgs{
		Args: []params.GetSecretContentArg{
			{URI: uri.String()},
		},
	})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(results, jc.DeepEquals, params.SecretContentResults{
		Results: []params.SecretContentResult{{
			Content: params.SecretContentParams{Data: data},
		}},
	})
}

func (s *SecretsManagerSuite) TestGetSecretContentConsumerLabelOnly(c *tc.C) {
	defer s.setup(c).Finish()

	data := map[string]string{"foo": "bar"}
	val := coresecrets.NewSecretValue(data)
	uri := coresecrets.NewURI()

	s.secretService.EXPECT().ProcessCharmSecretConsumerLabel(gomock.Any(), unittesting.GenNewName(c, "mariadb/0"), nil, "label").Return(uri, nil, nil)
	s.secretsConsumer.EXPECT().GetConsumedRevision(gomock.Any(), uri, unittesting.GenNewName(c, "mariadb/0"), false, false, nil).
		Return(666, nil)
	s.secretService.EXPECT().GetSecretValue(gomock.Any(), uri, 666, secretservice.SecretAccessor{
		Kind: secretservice.UnitAccessor,
		ID:   "mariadb/0",
	}).Return(
		val, nil, nil,
	)

	results, err := s.facade.GetSecretContentInfo(context.Background(), params.GetSecretContentArgs{
		Args: []params.GetSecretContentArg{
			{Label: "label"},
		},
	})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(results, jc.DeepEquals, params.SecretContentResults{
		Results: []params.SecretContentResult{{
			Content: params.SecretContentParams{Data: data},
		}},
	})
}

func (s *SecretsManagerSuite) TestGetSecretContentConsumerUpdateArg(c *tc.C) {
	defer s.setup(c).Finish()

	data := map[string]string{"foo": "bar"}
	val := coresecrets.NewSecretValue(data)
	uri := coresecrets.NewURI()

	s.secretService.EXPECT().ProcessCharmSecretConsumerLabel(gomock.Any(), unittesting.GenNewName(c, "mariadb/0"), uri, "label").Return(uri, ptr("label"), nil)
	s.secretsConsumer.EXPECT().GetConsumedRevision(gomock.Any(), uri, unittesting.GenNewName(c, "mariadb/0"), true, false, ptr("label")).
		Return(668, nil)

	s.secretService.EXPECT().GetSecretValue(gomock.Any(), uri, 668, secretservice.SecretAccessor{
		Kind: secretservice.UnitAccessor,
		ID:   "mariadb/0",
	}).Return(
		val, nil, nil,
	)

	results, err := s.facade.GetSecretContentInfo(context.Background(), params.GetSecretContentArgs{
		Args: []params.GetSecretContentArg{
			{URI: uri.String(), Label: "label", Refresh: true},
		},
	})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(results, jc.DeepEquals, params.SecretContentResults{
		Results: []params.SecretContentResult{{
			Content: params.SecretContentParams{Data: data},
		}},
	})
}

func (s *SecretsManagerSuite) TestGetSecretContentConsumerPeekArg(c *tc.C) {
	defer s.setup(c).Finish()

	data := map[string]string{"foo": "bar"}
	val := coresecrets.NewSecretValue(data)
	uri := coresecrets.NewURI()

	s.secretService.EXPECT().ProcessCharmSecretConsumerLabel(gomock.Any(), unittesting.GenNewName(c, "mariadb/0"), uri, "").Return(uri, nil, nil)
	s.secretsConsumer.EXPECT().GetConsumedRevision(gomock.Any(), uri, unittesting.GenNewName(c, "mariadb/0"), false, true, nil).
		Return(668, nil)
	s.secretService.EXPECT().GetSecretValue(gomock.Any(), uri, 668, secretservice.SecretAccessor{
		Kind: secretservice.UnitAccessor,
		ID:   "mariadb/0",
	}).Return(
		val, nil, nil,
	)

	results, err := s.facade.GetSecretContentInfo(context.Background(), params.GetSecretContentArgs{
		Args: []params.GetSecretContentArg{
			{URI: uri.String(), Peek: true},
		},
	})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(results, jc.DeepEquals, params.SecretContentResults{
		Results: []params.SecretContentResult{{
			Content: params.SecretContentParams{Data: data},
		}},
	})
}

func (s *SecretsManagerSuite) TestGetSecretContentCrossModelExistingConsumerNoRefresh(c *tc.C) {
	ctrl := s.setup(c)
	defer ctrl.Finish()

	anotherUUID := "deadbeef-0bad-0666-8000-4b1d0d06f66d"
	uri := coresecrets.NewURI().WithSource(anotherUUID)

	scopeTag := names.NewRelationTag("foo:bar baz:bar")
	mac := jujutesting.MustNewMacaroon("id")

	s.remoteClient = mocks.NewMockCrossModelSecretsClient(ctrl)
	s.remoteClient.EXPECT().Close().Return(nil)

	s.crossModelState.EXPECT().GetToken(names.NewApplicationTag("mariadb")).Return("token", nil)
	s.secretService.EXPECT().ProcessCharmSecretConsumerLabel(gomock.Any(), unittesting.GenNewName(c, "mariadb/0"), uri, "").Return(uri, nil, nil)
	s.secretsConsumer.EXPECT().GetSecretConsumer(gomock.Any(), uri, unittesting.GenNewName(c, "mariadb/0")).Return(&coresecrets.SecretConsumerMetadata{
		CurrentRevision: 665,
	}, nil)

	s.remoteClient.EXPECT().GetSecretAccessScope(gomock.Any(), uri, "token", 0).Return("scope-token", nil)
	s.crossModelState.EXPECT().GetRemoteEntity("scope-token").Return(scopeTag, nil)
	s.crossModelState.EXPECT().GetMacaroon(scopeTag).Return(mac, nil)

	s.remoteClient.EXPECT().GetRemoteSecretContentInfo(gomock.Any(), uri, 665, false, false, coretesting.ControllerTag.Id(), "token", 0, macaroon.Slice{mac}).Return(
		&secrets.ContentParams{
			ValueRef: &coresecrets.ValueRef{
				BackendID:  "backend-id",
				RevisionID: "rev-id",
			},
		}, &provider.ModelBackendConfig{
			ControllerUUID: coretesting.ControllerTag.Id(),
			ModelUUID:      coretesting.ModelTag.Id(),
			ModelName:      "fred",
			BackendConfig: provider.BackendConfig{
				BackendType: "vault",
				Config:      map[string]interface{}{"foo": "bar"},
			},
		}, 666, true, nil)

	results, err := s.facade.GetSecretContentInfo(context.Background(), params.GetSecretContentArgs{
		Args: []params.GetSecretContentArg{
			{URI: uri.String()},
		},
	})
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
		}},
	})
}

func (s *SecretsManagerSuite) TestGetSecretContentCrossModelExistingConsumerNoRefreshUpdateLabel(c *tc.C) {
	ctrl := s.setup(c)
	defer ctrl.Finish()

	anotherUUID := "deadbeef-0bad-0666-8000-4b1d0d06f66d"
	uri := coresecrets.NewURI().WithSource(anotherUUID)

	scopeTag := names.NewRelationTag("foo:bar baz:bar")
	mac := jujutesting.MustNewMacaroon("id")

	s.remoteClient = mocks.NewMockCrossModelSecretsClient(ctrl)
	s.remoteClient.EXPECT().Close().Return(nil)

	s.crossModelState.EXPECT().GetToken(names.NewApplicationTag("mariadb")).Return("token", nil)
	s.secretService.EXPECT().ProcessCharmSecretConsumerLabel(gomock.Any(), unittesting.GenNewName(c, "mariadb/0"), uri, "foo").Return(uri, nil, nil)
	s.secretsConsumer.EXPECT().GetSecretConsumer(gomock.Any(), uri, unittesting.GenNewName(c, "mariadb/0")).Return(&coresecrets.SecretConsumerMetadata{
		CurrentRevision: 665,
	}, nil)

	s.remoteClient.EXPECT().GetSecretAccessScope(gomock.Any(), uri, "token", 0).Return("scope-token", nil)
	s.crossModelState.EXPECT().GetRemoteEntity("scope-token").Return(scopeTag, nil)
	s.crossModelState.EXPECT().GetMacaroon(scopeTag).Return(mac, nil)

	s.remoteClient.EXPECT().GetRemoteSecretContentInfo(gomock.Any(), uri, 665, false, false, coretesting.ControllerTag.Id(), "token", 0, macaroon.Slice{mac}).Return(
		&secrets.ContentParams{
			ValueRef: &coresecrets.ValueRef{
				BackendID:  "backend-id",
				RevisionID: "rev-id",
			},
		}, &provider.ModelBackendConfig{
			ControllerUUID: coretesting.ControllerTag.Id(),
			ModelUUID:      coretesting.ModelTag.Id(),
			ModelName:      "fred",
			BackendConfig: provider.BackendConfig{
				BackendType: "vault",
				Config:      map[string]interface{}{"foo": "bar"},
			},
		}, 666, true, nil)

	results, err := s.facade.GetSecretContentInfo(context.Background(), params.GetSecretContentArgs{
		Args: []params.GetSecretContentArg{
			{URI: uri.String(), Label: "foo"},
		},
	})
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
		}},
	})
}

func (s *SecretsManagerSuite) TestGetSecretContentCrossModelExistingConsumerRefresh(c *tc.C) {
	ctrl := s.setup(c)
	defer ctrl.Finish()

	anotherUUID := "deadbeef-0bad-0666-8000-4b1d0d06f66d"
	uri := coresecrets.NewURI().WithSource(anotherUUID)

	scopeTag := names.NewRelationTag("foo:bar baz:bar")
	mac := jujutesting.MustNewMacaroon("id")

	s.remoteClient = mocks.NewMockCrossModelSecretsClient(ctrl)
	s.remoteClient.EXPECT().Close().Return(nil)

	s.crossModelState.EXPECT().GetToken(names.NewApplicationTag("mariadb")).Return("token", nil)
	s.secretService.EXPECT().ProcessCharmSecretConsumerLabel(gomock.Any(), unittesting.GenNewName(c, "mariadb/0"), uri, "").Return(uri, nil, nil)
	s.secretsConsumer.EXPECT().GetSecretConsumer(gomock.Any(), uri, unittesting.GenNewName(c, "mariadb/0")).Return(&coresecrets.SecretConsumerMetadata{
		CurrentRevision: 665,
	}, nil)

	s.remoteClient.EXPECT().GetSecretAccessScope(gomock.Any(), uri, "token", 0).Return("scope-token", nil)
	s.crossModelState.EXPECT().GetRemoteEntity("scope-token").Return(scopeTag, nil)
	s.crossModelState.EXPECT().GetMacaroon(scopeTag).Return(mac, nil)

	s.remoteClient.EXPECT().GetRemoteSecretContentInfo(gomock.Any(), uri, 665, true, false, coretesting.ControllerTag.Id(), "token", 0, macaroon.Slice{mac}).Return(
		&secrets.ContentParams{
			ValueRef: &coresecrets.ValueRef{
				BackendID:  "backend-id",
				RevisionID: "rev-id",
			},
		}, &provider.ModelBackendConfig{
			ControllerUUID: coretesting.ControllerTag.Id(),
			ModelUUID:      coretesting.ModelTag.Id(),
			ModelName:      "fred",
			BackendConfig: provider.BackendConfig{
				BackendType: "vault",
				Config:      map[string]interface{}{"foo": "bar"},
			},
		}, 666, true, nil)

	s.secretsConsumer.EXPECT().SaveSecretConsumer(gomock.Any(), uri, unittesting.GenNewName(c, "mariadb/0"), &coresecrets.SecretConsumerMetadata{
		CurrentRevision: 666,
	})

	results, err := s.facade.GetSecretContentInfo(context.Background(), params.GetSecretContentArgs{
		Args: []params.GetSecretContentArg{
			{URI: uri.String(), Refresh: true},
		},
	})
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
		}},
	})
}

func (s *SecretsManagerSuite) TestGetSecretContentCrossModelNewConsumer(c *tc.C) {
	s.assertGetSecretContentCrossModelNewConsumer(c, secreterrors.SecretConsumerNotFound)
}

func (s *SecretsManagerSuite) TestGetSecretContentCrossModelNewConsumerAndSecret(c *tc.C) {
	s.assertGetSecretContentCrossModelNewConsumer(c, secreterrors.SecretNotFound)
}

func (s *SecretsManagerSuite) assertGetSecretContentCrossModelNewConsumer(c *tc.C, consumerErr error) {
	ctrl := s.setup(c)
	defer ctrl.Finish()

	anotherUUID := "deadbeef-0bad-0666-8000-4b1d0d06f66d"
	uri := coresecrets.NewURI().WithSource(anotherUUID)

	scopeTag := names.NewRelationTag("foo:bar baz:bar")
	mac := jujutesting.MustNewMacaroon("id")

	s.remoteClient = mocks.NewMockCrossModelSecretsClient(ctrl)
	s.remoteClient.EXPECT().Close().Return(nil)

	s.crossModelState.EXPECT().GetToken(names.NewApplicationTag("mariadb")).Return("token", nil)
	s.secretsConsumer.EXPECT().GetSecretConsumer(gomock.Any(), uri, unittesting.GenNewName(c, "mariadb/0")).Return(nil, consumerErr)
	s.secretService.EXPECT().ProcessCharmSecretConsumerLabel(gomock.Any(), unittesting.GenNewName(c, "mariadb/0"), uri, "").Return(uri, nil, nil)

	s.remoteClient.EXPECT().GetSecretAccessScope(gomock.Any(), uri, "token", 0).Return("scope-token", nil)
	s.crossModelState.EXPECT().GetRemoteEntity("scope-token").Return(scopeTag, nil)
	s.crossModelState.EXPECT().GetMacaroon(scopeTag).Return(mac, nil)

	s.remoteClient.EXPECT().GetRemoteSecretContentInfo(gomock.Any(), uri, 0, true, false, coretesting.ControllerTag.Id(), "token", 0, macaroon.Slice{mac}).Return(
		&secrets.ContentParams{
			ValueRef: &coresecrets.ValueRef{
				BackendID:  "backend-id",
				RevisionID: "rev-id",
			},
		}, &provider.ModelBackendConfig{
			ControllerUUID: coretesting.ControllerTag.Id(),
			ModelUUID:      coretesting.ModelTag.Id(),
			ModelName:      "fred",
			BackendConfig: provider.BackendConfig{
				BackendType: "vault",
				Config:      map[string]interface{}{"foo": "bar"},
			},
		}, 666, true, nil)

	s.secretsConsumer.EXPECT().SaveSecretConsumer(gomock.Any(), uri, unittesting.GenNewName(c, "mariadb/0"), &coresecrets.SecretConsumerMetadata{
		CurrentRevision: 666,
	})

	results, err := s.facade.GetSecretContentInfo(context.Background(), params.GetSecretContentArgs{
		Args: []params.GetSecretContentArg{
			{URI: uri.String(), Refresh: true},
		},
	})
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
		}},
	})
}

func (s *SecretsManagerSuite) TestWatchConsumedSecretsChanges(c *tc.C) {
	defer s.setup(c).Finish()

	s.secretsConsumer.EXPECT().WatchConsumedSecretsChanges(gomock.Any(), unittesting.GenNewName(c, "mariadb/0")).Return(
		s.secretsWatcher, nil,
	)
	s.watcherRegistry.EXPECT().Register(s.secretsWatcher).Return("1", nil)

	uri := coresecrets.NewURI()
	watchChan := make(chan []string, 1)
	watchChan <- []string{uri.String()}
	s.secretsWatcher.EXPECT().Changes().Return(watchChan)

	result, err := s.facade.WatchConsumedSecretsChanges(context.Background(), params.Entities{
		Entities: []params.Entity{{
			Tag: "unit-mariadb-0",
		}, {
			Tag: "unit-foo-0",
		}},
	})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(result, jc.DeepEquals, params.StringsWatchResults{
		Results: []params.StringsWatchResult{{
			StringsWatcherId: "1",
			Changes:          []string{uri.String()},
		}, {
			Error: &params.Error{Code: "unauthorized access", Message: "permission denied"},
		}},
	})
}

func (s *SecretsManagerSuite) TestGetSecretRevisionContentInfo(c *tc.C) {
	defer s.setup(c).Finish()

	uri := coresecrets.NewURI()
	s.secretService.EXPECT().GetSecretValue(gomock.Any(), uri, 666, secretservice.SecretAccessor{
		Kind: secretservice.UnitAccessor,
		ID:   "mariadb/0",
	}).Return(
		nil, &coresecrets.ValueRef{
			BackendID:  "backend-id",
			RevisionID: "rev-id",
		}, nil,
	)
	s.leadership.EXPECT().LeadershipCheck("mariadb", "mariadb/0").Return(s.token)
	s.secretBackendService.EXPECT().BackendConfigInfo(gomock.Any(), backendConfigParamsMatcher{c: c,
		expected: secretbackendservice.BackendConfigParams{
			LeaderToken: s.token,
			Accessor: secretservice.SecretAccessor{
				Kind: secretservice.UnitAccessor,
				ID:   "mariadb/0",
			},
			ModelUUID:      model.UUID(coretesting.ModelTag.Id()),
			BackendIDs:     []string{"backend-id"},
			SameController: true,
		}}).Return(&provider.ModelBackendConfigInfo{
		ActiveID: "backend-id",
		Configs: map[string]provider.ModelBackendConfig{
			"backend-id": {
				ControllerUUID: coretesting.ControllerTag.Id(),
				ModelUUID:      coretesting.ModelTag.Id(),
				ModelName:      "fred",
				BackendConfig: provider.BackendConfig{
					BackendType: "some-backend",
					Config:      map[string]interface{}{"foo": "bar"},
				},
			},
		},
	}, nil)

	results, err := s.facade.GetSecretRevisionContentInfo(context.Background(), params.SecretRevisionArg{
		URI:       uri.String(),
		Revisions: []int{666},
	})
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
				Draining:       false,
				Config: params.SecretBackendConfig{
					BackendType: "some-backend",
					Params:      map[string]interface{}{"foo": "bar"},
				},
			},
		}},
	})
}

func (s *SecretsManagerSuite) TestWatchObsolete(c *tc.C) {
	defer s.setup(c).Finish()

	s.leadership.EXPECT().LeadershipCheck("mariadb", "mariadb/0").Return(s.token)
	s.token.EXPECT().Check().Return(nil)
	s.secretTriggers.EXPECT().WatchObsolete(gomock.Any(), []secretservice.CharmSecretOwner{{
		Kind: secretservice.UnitOwner,
		ID:   "mariadb/0",
	}, {
		Kind: secretservice.ApplicationOwner,
		ID:   "mariadb",
	}}).Return(
		s.secretsWatcher, nil,
	)
	s.watcherRegistry.EXPECT().Register(s.secretsWatcher).Return("1", nil)

	uri := coresecrets.NewURI()
	watchChan := make(chan []string, 1)
	watchChan <- []string{uri.String()}
	s.secretsWatcher.EXPECT().Changes().Return(watchChan)

	result, err := s.facade.WatchObsolete(context.Background(), params.Entities{
		Entities: []params.Entity{{
			Tag: "unit-mariadb-0",
		}, {
			Tag: "application-mariadb",
		}},
	})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(result, jc.DeepEquals, params.StringsWatchResult{
		StringsWatcherId: "1",
		Changes:          []string{uri.String()},
	})
}

func (s *SecretsManagerSuite) TestWatchSecretsRotationChanges(c *tc.C) {
	defer s.setup(c).Finish()

	s.leadership.EXPECT().LeadershipCheck("mariadb", "mariadb/0").Return(s.token)
	s.token.EXPECT().Check().Return(nil)
	s.secretTriggers.EXPECT().WatchSecretsRotationChanges(gomock.Any(),
		[]secretservice.CharmSecretOwner{{
			Kind: secretservice.UnitOwner,
			ID:   "mariadb/0",
		}, {
			Kind: secretservice.ApplicationOwner,
			ID:   "mariadb",
		}}).Return(
		s.secretsTriggerWatcher, nil,
	)
	s.watcherRegistry.EXPECT().Register(s.secretsTriggerWatcher).Return("1", nil)

	next := time.Now().Add(time.Hour)
	uri := coresecrets.NewURI()
	rotateChan := make(chan []corewatcher.SecretTriggerChange, 1)
	rotateChan <- []corewatcher.SecretTriggerChange{{
		URI:             uri,
		NextTriggerTime: next,
	}}
	s.secretsTriggerWatcher.EXPECT().Changes().Return(rotateChan)

	result, err := s.facade.WatchSecretsRotationChanges(context.Background(), params.Entities{
		Entities: []params.Entity{{
			Tag: "unit-mariadb-0",
		}, {
			Tag: "application-mariadb",
		}},
	})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(result, jc.DeepEquals, params.SecretTriggerWatchResult{
		WatcherId: "1",
		Changes: []params.SecretTriggerChange{{
			URI:             uri.ID,
			NextTriggerTime: next,
		}},
	})
}

func (s *SecretsManagerSuite) TestSecretsRotated(c *tc.C) {
	defer s.setup(c).Finish()

	uri := coresecrets.NewURI()
	s.secretTriggers.EXPECT().SecretRotated(gomock.Any(), uri, secretservice.SecretRotatedParams{
		Accessor: secretservice.SecretAccessor{
			Kind: secretservice.UnitAccessor,
			ID:   "mariadb/0",
		},
		OriginalRevision: 666,
		Skip:             false,
	}).Return(errors.New("boom"))

	result, err := s.facade.SecretsRotated(context.Background(), params.SecretRotatedArgs{
		Args: []params.SecretRotatedArg{{
			URI:              uri.ID,
			OriginalRevision: 666,
		}, {
			URI: "bad",
		}},
	})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(result, jc.DeepEquals, params.ErrorResults{
		Results: []params.ErrorResult{
			{
				Error: &params.Error{Code: "", Message: `boom`},
			},
			{
				Error: &params.Error{Code: params.CodeNotValid, Message: `secret URI "bad" not valid`},
			},
		},
	})
}

func (s *SecretsManagerSuite) TestSecretsRotatedRetry(c *tc.C) {
	defer s.setup(c).Finish()

	uri := coresecrets.NewURI()
	s.secretTriggers.EXPECT().SecretRotated(gomock.Any(), uri, secretservice.SecretRotatedParams{
		Accessor: secretservice.SecretAccessor{
			Kind: secretservice.UnitAccessor,
			ID:   "mariadb/0",
		},
		OriginalRevision: 666,
		Skip:             false,
	}).Return(errors.New("boom"))

	result, err := s.facade.SecretsRotated(context.Background(), params.SecretRotatedArgs{
		Args: []params.SecretRotatedArg{{
			URI:              uri.ID,
			OriginalRevision: 666,
		}},
	})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(result, jc.DeepEquals, params.ErrorResults{
		Results: []params.ErrorResult{
			{
				Error: &params.Error{Code: "", Message: `boom`},
			},
		},
	})
}

func (s *SecretsManagerSuite) TestSecretsRotatedForce(c *tc.C) {
	defer s.setup(c).Finish()

	uri := coresecrets.NewURI()
	s.secretTriggers.EXPECT().SecretRotated(gomock.Any(), uri, secretservice.SecretRotatedParams{
		Accessor: secretservice.SecretAccessor{
			Kind: secretservice.UnitAccessor,
			ID:   "mariadb/0",
		},
		OriginalRevision: 666,
		Skip:             false,
	}).Return(errors.New("boom"))

	result, err := s.facade.SecretsRotated(context.Background(), params.SecretRotatedArgs{
		Args: []params.SecretRotatedArg{{
			URI:              uri.ID,
			OriginalRevision: 666,
		}},
	})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(result, jc.DeepEquals, params.ErrorResults{
		Results: []params.ErrorResult{
			{
				Error: &params.Error{Code: "", Message: `boom`},
			},
		},
	})
}

func (s *SecretsManagerSuite) TestWatchSecretRevisionsExpiryChanges(c *tc.C) {
	defer s.setup(c).Finish()

	s.leadership.EXPECT().LeadershipCheck("mariadb", "mariadb/0").Return(s.token)
	s.token.EXPECT().Check().Return(nil)
	s.secretTriggers.EXPECT().WatchSecretRevisionsExpiryChanges(gomock.Any(),
		[]secretservice.CharmSecretOwner{{
			Kind: secretservice.UnitOwner,
			ID:   "mariadb/0",
		}, {
			Kind: secretservice.ApplicationOwner,
			ID:   "mariadb",
		}}).Return(
		s.secretsTriggerWatcher, nil,
	)
	s.watcherRegistry.EXPECT().Register(s.secretsTriggerWatcher).Return("1", nil)

	next := time.Now().Add(time.Hour)
	uri := coresecrets.NewURI()
	expiryChan := make(chan []corewatcher.SecretTriggerChange, 1)
	expiryChan <- []corewatcher.SecretTriggerChange{{
		URI:             uri,
		Revision:        666,
		NextTriggerTime: next,
	}}
	s.secretsTriggerWatcher.EXPECT().Changes().Return(expiryChan)

	result, err := s.facade.WatchSecretRevisionsExpiryChanges(context.Background(), params.Entities{
		Entities: []params.Entity{{
			Tag: "unit-mariadb-0",
		}, {
			Tag: "application-mariadb",
		}},
	})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(result, jc.DeepEquals, params.SecretTriggerWatchResult{
		WatcherId: "1",
		Changes: []params.SecretTriggerChange{{
			URI:             uri.ID,
			Revision:        666,
			NextTriggerTime: next,
		}},
	})
}
