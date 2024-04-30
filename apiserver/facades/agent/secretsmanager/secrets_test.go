// Copyright 2021 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package secretsmanager_test

import (
	"context"
	"fmt"
	"time"

	"github.com/juju/clock"
	"github.com/juju/clock/testclock"
	"github.com/juju/errors"
	"github.com/juju/names/v5"
	"github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	"go.uber.org/mock/gomock"
	gc "gopkg.in/check.v1"
	"gopkg.in/macaroon.v2"

	facademocks "github.com/juju/juju/apiserver/facade/mocks"
	"github.com/juju/juju/apiserver/facades/agent/secretsmanager"
	"github.com/juju/juju/apiserver/facades/agent/secretsmanager/mocks"
	"github.com/juju/juju/core/model"
	coresecrets "github.com/juju/juju/core/secrets"
	corewatcher "github.com/juju/juju/core/watcher"
	secreterrors "github.com/juju/juju/domain/secret/errors"
	secretservice "github.com/juju/juju/domain/secret/service"
	secretbackendservice "github.com/juju/juju/domain/secretbackend/service"
	"github.com/juju/juju/internal/secrets"
	"github.com/juju/juju/internal/secrets/provider"
	jujutesting "github.com/juju/juju/juju/testing"
	"github.com/juju/juju/rpc/params"
	"github.com/juju/juju/state"
	coretesting "github.com/juju/juju/testing"
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

var _ = gc.Suite(&SecretsManagerSuite{})

func (s *SecretsManagerSuite) SetUpTest(c *gc.C) {
	s.IsolationSuite.SetUpTest(c)

	s.authTag = names.NewUnitTag("mariadb/0")
}

func (s *SecretsManagerSuite) setup(c *gc.C) *gomock.Controller {
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
	c        *gc.C
	expected any
}

func (m backendConfigParamsMatcher) Matches(x interface{}) bool {
	if obtained, ok := x.(secretbackendservice.BackendConfigParams); ok {
		m.c.Assert(obtained.GrantedSecretsGetter, gc.NotNil)
		obtained.GrantedSecretsGetter = nil
		m.c.Assert(obtained, jc.DeepEquals, m.expected)
		return true
	}
	obtained, ok := x.(secretbackendservice.DrainBackendConfigParams)
	if !ok {
		return false
	}
	m.c.Assert(obtained.GrantedSecretsGetter, gc.NotNil)
	obtained.GrantedSecretsGetter = nil
	m.c.Assert(obtained, jc.DeepEquals, m.expected)
	return true
}

func (m backendConfigParamsMatcher) String() string {
	return "Match the contents of BackendConfigParams"
}

func (s *SecretsManagerSuite) TestGetSecretBackendConfigs(c *gc.C) {
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

func (s *SecretsManagerSuite) TestGetSecretBackendConfigsForDrain(c *gc.C) {
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

func (s *SecretsManagerSuite) TestCreateSecretURIs(c *gc.C) {
	defer s.setup(c).Finish()

	uri1 := coresecrets.NewURI()
	uri2 := coresecrets.NewURI()
	s.secretService.EXPECT().CreateSecretURIs(gomock.Any(), 2).Return([]*coresecrets.URI{uri1, uri2}, nil)

	results, err := s.facade.CreateSecretURIs(context.Background(), params.CreateSecretURIsArg{
		Count: 2,
	})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(results.Results, gc.HasLen, 2)
	for _, r := range results.Results {
		_, err := coresecrets.ParseURI(r.Result)
		c.Assert(err, jc.ErrorIsNil)
	}
}

func (s *SecretsManagerSuite) TestCreateSecrets(c *gc.C) {
	defer s.setup(c).Finish()

	p := secretservice.CreateSecretParams{
		Version:    secrets.Version,
		CharmOwner: &secretservice.CharmSecretOwner{Kind: secretservice.ApplicationOwner, ID: "mariadb"},
		UpdateSecretParams: secretservice.UpdateSecretParams{
			LeaderToken:  s.token,
			RotatePolicy: ptr(coresecrets.RotateDaily),
			ExpireTime:   ptr(s.clock.Now()),
			Description:  ptr("my secret"),
			Label:        ptr("foobar"),
			Params:       map[string]interface{}{"param": 1},
			Data:         map[string]string{"foo": "bar"},
		},
	}
	var gotURI *coresecrets.URI
	s.leadership.EXPECT().LeadershipCheck("mariadb", "mariadb/0").Return(s.token)
	s.secretService.EXPECT().CreateSecret(gomock.Any(), gomock.Any(), p).DoAndReturn(
		func(ctx context.Context, uri *coresecrets.URI, p secretservice.CreateSecretParams) error {
			gotURI = uri
			return nil
		},
	)

	results, err := s.facade.CreateSecrets(context.Background(), params.CreateSecretArgs{
		Args: []params.CreateSecretArg{{
			OwnerTag: "application-mariadb",
			UpsertSecretArg: params.UpsertSecretArg{
				RotatePolicy: ptr(coresecrets.RotateDaily),
				ExpireTime:   ptr(s.clock.Now()),
				Description:  ptr("my secret"),
				Label:        ptr("foobar"),
				Params:       map[string]interface{}{"param": 1},
				Content:      params.SecretContentParams{Data: map[string]string{"foo": "bar"}},
			},
		}, {
			UpsertSecretArg: params.UpsertSecretArg{
				//Content: params.SecretContentParams{},
			},
		}, {
			OwnerTag: "application-mysql",
			UpsertSecretArg: params.UpsertSecretArg{
				Content: params.SecretContentParams{Data: map[string]string{"foo": "bar"}},
			},
		}},
	})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(results, jc.DeepEquals, params.StringResults{
		Results: []params.StringResult{{
			Result: gotURI.String(),
		}, {
			Error: &params.Error{Message: `empty secret value not valid`, Code: params.CodeNotValid},
		}, {
			Error: &params.Error{Message: `permission denied`, Code: params.CodeUnauthorized},
		}},
	})
}

func (s *SecretsManagerSuite) TestCreateSecretDuplicateLabel(c *gc.C) {
	defer s.setup(c).Finish()

	p := secretservice.CreateSecretParams{
		Version:    secrets.Version,
		CharmOwner: &secretservice.CharmSecretOwner{Kind: secretservice.ApplicationOwner, ID: "mariadb"},
		UpdateSecretParams: secretservice.UpdateSecretParams{
			LeaderToken: s.token,
			Label:       ptr("foobar"),
			Data:        map[string]string{"foo": "bar"},
		},
	}
	s.leadership.EXPECT().LeadershipCheck("mariadb", "mariadb/0").Return(s.token)
	s.secretService.EXPECT().CreateSecret(gomock.Any(), gomock.Any(), p).Return(
		fmt.Errorf("dup label %w", state.LabelExists),
	)

	results, err := s.facade.CreateSecrets(context.Background(), params.CreateSecretArgs{
		Args: []params.CreateSecretArg{{
			OwnerTag: "application-mariadb",
			UpsertSecretArg: params.UpsertSecretArg{
				Label:   ptr("foobar"),
				Content: params.SecretContentParams{Data: map[string]string{"foo": "bar"}},
			},
		}},
	})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(results, jc.DeepEquals, params.StringResults{
		Results: []params.StringResult{{
			Error: &params.Error{Message: `secret with label "foobar" already exists`, Code: params.CodeAlreadyExists},
		}},
	})
}

func (s *SecretsManagerSuite) TestUpdateSecrets(c *gc.C) {
	defer s.setup(c).Finish()

	p := secretservice.UpdateSecretParams{
		LeaderToken: s.token,
		Accessor: secretservice.SecretAccessor{
			Kind: secretservice.UnitAccessor,
			ID:   "mariadb/0",
		},
		RotatePolicy: ptr(coresecrets.RotateDaily),
		ExpireTime:   ptr(s.clock.Now()),
		Description:  ptr("my secret"),
		Label:        ptr("foobar"),
		Params:       map[string]interface{}{"param": 1},
		Data:         map[string]string{"foo": "bar"},
	}
	pWithBackendId := p
	p.ValueRef = &coresecrets.ValueRef{
		BackendID:  "backend-id",
		RevisionID: "rev-id",
	}
	p.Data = nil
	uri := coresecrets.NewURI()
	expectURI := *uri
	s.secretService.EXPECT().UpdateSecret(gomock.Any(), &expectURI, p).Return(nil)
	s.secretService.EXPECT().UpdateSecret(gomock.Any(), &expectURI, pWithBackendId).Return(nil)
	s.leadership.EXPECT().LeadershipCheck("mariadb", "mariadb/0").Return(s.token).Times(2)

	results, err := s.facade.UpdateSecrets(context.Background(), params.UpdateSecretArgs{
		Args: []params.UpdateSecretArg{{
			URI: uri.String(),
			UpsertSecretArg: params.UpsertSecretArg{
				RotatePolicy: ptr(coresecrets.RotateDaily),
				ExpireTime:   ptr(s.clock.Now()),
				Description:  ptr("my secret"),
				Label:        ptr("foobar"),
				Params:       map[string]interface{}{"param": 1},
				Content:      params.SecretContentParams{Data: map[string]string{"foo": "bar"}},
			},
		}, {
			URI: uri.String(),
			UpsertSecretArg: params.UpsertSecretArg{
				RotatePolicy: ptr(coresecrets.RotateDaily),
				ExpireTime:   ptr(s.clock.Now()),
				Description:  ptr("my secret"),
				Label:        ptr("foobar"),
				Params:       map[string]interface{}{"param": 1},
				Content: params.SecretContentParams{ValueRef: &params.SecretValueRef{
					BackendID:  "backend-id",
					RevisionID: "rev-id",
				}},
			},
		}, {
			URI: uri.String(),
		}},
	})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(results, jc.DeepEquals, params.ErrorResults{
		Results: []params.ErrorResult{{}, {}, {
			Error: &params.Error{Message: `at least one attribute to update must be specified`},
		}},
	})
}

func (s *SecretsManagerSuite) TestRemoveSecrets(c *gc.C) {
	defer s.setup(c).Finish()

	s.leadership.EXPECT().LeadershipCheck("mariadb", "mariadb/0").Return(s.token)

	uri := coresecrets.NewURI()
	expectURI := *uri
	s.secretService.EXPECT().DeleteSecret(gomock.Any(), &expectURI, secretservice.DeleteSecretParams{
		LeaderToken: s.token,
		Accessor: secretservice.SecretAccessor{
			Kind: secretservice.UnitAccessor,
			ID:   "mariadb/0",
		},
	}).Return(nil)

	results, err := s.facade.RemoveSecrets(context.Background(), params.DeleteSecretArgs{
		Args: []params.DeleteSecretArg{{
			URI: expectURI.String(),
		}},
	})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(results, jc.DeepEquals, params.ErrorResults{
		Results: []params.ErrorResult{{}},
	})
}

func (s *SecretsManagerSuite) TestRemoveSecretRevision(c *gc.C) {
	defer s.setup(c).Finish()

	s.leadership.EXPECT().LeadershipCheck("mariadb", "mariadb/0").Return(s.token)

	uri := coresecrets.NewURI()
	expectURI := *uri
	s.secretService.EXPECT().DeleteSecret(gomock.Any(), &expectURI, secretservice.DeleteSecretParams{
		LeaderToken: s.token,
		Accessor: secretservice.SecretAccessor{
			Kind: secretservice.UnitAccessor,
			ID:   "mariadb/0",
		},
		Revisions: []int{666},
	}).Return(nil)

	results, err := s.facade.RemoveSecrets(context.Background(), params.DeleteSecretArgs{
		Args: []params.DeleteSecretArg{{
			URI:       expectURI.String(),
			Revisions: []int{666},
		}},
	})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(results, jc.DeepEquals, params.ErrorResults{
		Results: []params.ErrorResult{{}},
	})
}

func (s *SecretsManagerSuite) TestRemoveSecretNotFound(c *gc.C) {
	defer s.setup(c).Finish()

	s.leadership.EXPECT().LeadershipCheck("mariadb", "mariadb/0").Return(s.token)

	uri := coresecrets.NewURI()
	expectURI := *uri
	s.secretService.EXPECT().DeleteSecret(gomock.Any(), &expectURI, secretservice.DeleteSecretParams{
		LeaderToken: s.token,
		Accessor: secretservice.SecretAccessor{
			Kind: secretservice.UnitAccessor,
			ID:   "mariadb/0",
		},
		Revisions: []int{666},
	}).Return(secreterrors.SecretNotFound)

	results, err := s.facade.RemoveSecrets(context.Background(), params.DeleteSecretArgs{
		Args: []params.DeleteSecretArg{{
			URI:       expectURI.String(),
			Revisions: []int{666},
		}},
	})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(results.Results[0].Error, jc.Satisfies, params.IsCodeSecretNotFound)
}

func (s *SecretsManagerSuite) TestGetConsumerSecretsRevisionInfoHavingConsumerLabel(c *gc.C) {
	defer s.setup(c).Finish()

	uri := coresecrets.NewURI()
	s.secretsConsumer.EXPECT().GetSecretConsumerAndLatest(gomock.Any(), uri, "mariadb/0").Return(
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

func (s *SecretsManagerSuite) TestGetConsumerSecretsRevisionInfoHavingNoConsumerLabel(c *gc.C) {
	defer s.setup(c).Finish()

	uri := coresecrets.NewURI()
	s.secretsConsumer.EXPECT().GetSecretConsumerAndLatest(gomock.Any(), uri, "mariadb/0").Return(
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

func (s *SecretsManagerSuite) TestGetConsumerSecretsRevisionInfoForPeerUnitsAccessingAppOwnedSecrets(c *gc.C) {
	defer s.setup(c).Finish()

	uri := coresecrets.NewURI()
	s.secretsConsumer.EXPECT().GetSecretConsumerAndLatest(gomock.Any(), uri, "mariadb/0").Return(
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

func (s *SecretsManagerSuite) TestGetSecretMetadata(c *gc.C) {
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
		URI:              uri,
		Owner:            coresecrets.Owner{Kind: coresecrets.ApplicationOwner, ID: "mariadb"},
		Description:      "description",
		Label:            "label",
		RotatePolicy:     coresecrets.RotateHourly,
		LatestRevision:   666,
		LatestExpireTime: &now,
		NextRotateTime:   &now,
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
			URI:              uri.String(),
			OwnerTag:         "application-mariadb",
			Description:      "description",
			Label:            "label",
			RotatePolicy:     coresecrets.RotateHourly.String(),
			LatestRevision:   666,
			LatestExpireTime: &now,
			NextRotateTime:   &now,
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

func (s *SecretsManagerSuite) TestGetSecretContentInvalidArg(c *gc.C) {
	defer s.setup(c).Finish()

	results, err := s.facade.GetSecretContentInfo(context.Background(), params.GetSecretContentArgs{
		Args: []params.GetSecretContentArg{{}},
	})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(results.Results[0].Error, gc.ErrorMatches, `both uri and label are empty`)
}

func (s *SecretsManagerSuite) TestGetSecretContentForOwnerSecretURIArg(c *gc.C) {
	defer s.setup(c).Finish()

	data := map[string]string{"foo": "bar"}
	val := coresecrets.NewSecretValue(data)
	uri := coresecrets.NewURI()

	s.leadership.EXPECT().LeadershipCheck("mariadb", "mariadb/0").Return(s.token)
	s.secretService.EXPECT().ProcessSecretConsumerLabel(gomock.Any(), "mariadb/0", uri, "", gomock.Any()).Return(uri, nil, nil)

	s.secretsConsumer.EXPECT().GetConsumedRevision(gomock.Any(), uri, "mariadb/0", false, false, nil).
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

func (s *SecretsManagerSuite) TestGetSecretContentForOwnerSecretLabelArg(c *gc.C) {
	defer s.setup(c).Finish()

	data := map[string]string{"foo": "bar"}
	val := coresecrets.NewSecretValue(data)
	uri := coresecrets.NewURI()

	s.leadership.EXPECT().LeadershipCheck("mariadb", "mariadb/0").Return(s.token)
	s.secretService.EXPECT().ProcessSecretConsumerLabel(gomock.Any(), "mariadb/0", nil, "foo", gomock.Any()).Return(uri, nil, nil)

	s.secretsConsumer.EXPECT().GetConsumedRevision(gomock.Any(), uri, "mariadb/0", false, false, nil).
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

func (s *SecretsManagerSuite) TestGetSecretContentForAppSecretUpdateLabel(c *gc.C) {
	defer s.setup(c).Finish()

	data := map[string]string{"foo": "bar"}
	val := coresecrets.NewSecretValue(data)
	uri := coresecrets.NewURI()

	s.leadership.EXPECT().LeadershipCheck("mariadb", "mariadb/0").Return(s.token)
	s.secretService.EXPECT().ProcessSecretConsumerLabel(gomock.Any(), "mariadb/0", uri, "foo", gomock.Any()).Return(uri, nil, nil)

	s.secretsConsumer.EXPECT().GetConsumedRevision(gomock.Any(), uri, "mariadb/0", false, false, nil).
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

func (s *SecretsManagerSuite) TestGetSecretContentForUnitAccessApplicationOwnedSecret(c *gc.C) {
	defer s.setup(c).Finish()

	data := map[string]string{"foo": "bar"}
	val := coresecrets.NewSecretValue(data)
	uri := coresecrets.NewURI()

	s.leadership.EXPECT().LeadershipCheck("mariadb", "mariadb/0").Return(s.token)
	s.secretService.EXPECT().ProcessSecretConsumerLabel(gomock.Any(), "mariadb/0", nil, "foo", gomock.Any()).Return(uri, nil, nil)

	s.secretsConsumer.EXPECT().GetConsumedRevision(gomock.Any(), uri, "mariadb/0", false, false, nil).
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

func (s *SecretsManagerSuite) TestGetSecretContentConsumerUnitAgent(c *gc.C) {
	s.authTag = names.NewUnitTag("mariadb/0")

	defer s.setup(c).Finish()

	data := map[string]string{"foo": "bar"}
	val := coresecrets.NewSecretValue(data)
	uri := coresecrets.NewURI()

	s.leadership.EXPECT().LeadershipCheck("mariadb", "mariadb/0").Return(s.token)
	s.secretService.EXPECT().ProcessSecretConsumerLabel(gomock.Any(), "mariadb/0", uri, "", gomock.Any()).Return(uri, nil, nil)
	s.secretsConsumer.EXPECT().GetConsumedRevision(gomock.Any(), uri, "mariadb/0", false, false, nil).
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

func (s *SecretsManagerSuite) TestGetSecretContentConsumerLabelOnly(c *gc.C) {
	defer s.setup(c).Finish()

	data := map[string]string{"foo": "bar"}
	val := coresecrets.NewSecretValue(data)
	uri := coresecrets.NewURI()

	s.leadership.EXPECT().LeadershipCheck("mariadb", "mariadb/0").Return(s.token)
	s.secretService.EXPECT().ProcessSecretConsumerLabel(gomock.Any(), "mariadb/0", nil, "label", gomock.Any()).Return(uri, nil, nil)
	s.secretsConsumer.EXPECT().GetConsumedRevision(gomock.Any(), uri, "mariadb/0", false, false, nil).
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

func (s *SecretsManagerSuite) TestGetSecretContentConsumerUpdateArg(c *gc.C) {
	defer s.setup(c).Finish()

	data := map[string]string{"foo": "bar"}
	val := coresecrets.NewSecretValue(data)
	uri := coresecrets.NewURI()

	s.leadership.EXPECT().LeadershipCheck("mariadb", "mariadb/0").Return(s.token)
	s.secretService.EXPECT().ProcessSecretConsumerLabel(gomock.Any(), "mariadb/0", uri, "label", gomock.Any()).Return(uri, ptr("label"), nil)
	s.secretsConsumer.EXPECT().GetConsumedRevision(gomock.Any(), uri, "mariadb/0", true, false, ptr("label")).
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

func (s *SecretsManagerSuite) TestGetSecretContentConsumerPeekArg(c *gc.C) {
	defer s.setup(c).Finish()

	data := map[string]string{"foo": "bar"}
	val := coresecrets.NewSecretValue(data)
	uri := coresecrets.NewURI()

	s.leadership.EXPECT().LeadershipCheck("mariadb", "mariadb/0").Return(s.token)
	s.secretService.EXPECT().ProcessSecretConsumerLabel(gomock.Any(), "mariadb/0", uri, "", gomock.Any()).Return(uri, nil, nil)
	s.secretsConsumer.EXPECT().GetConsumedRevision(gomock.Any(), uri, "mariadb/0", false, true, nil).
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

func (s *SecretsManagerSuite) TestGetSecretContentCrossModelExistingConsumerNoRefresh(c *gc.C) {
	ctrl := s.setup(c)
	defer ctrl.Finish()

	anotherUUID := "deadbeef-0bad-0666-8000-4b1d0d06f66d"
	uri := coresecrets.NewURI().WithSource(anotherUUID)

	scopeTag := names.NewRelationTag("foo:bar baz:bar")
	mac := jujutesting.MustNewMacaroon("id")

	s.remoteClient = mocks.NewMockCrossModelSecretsClient(ctrl)
	s.remoteClient.EXPECT().Close().Return(nil)

	s.crossModelState.EXPECT().GetToken(names.NewApplicationTag("mariadb")).Return("token", nil)
	s.leadership.EXPECT().LeadershipCheck("mariadb", "mariadb/0").Return(s.token)
	s.secretService.EXPECT().ProcessSecretConsumerLabel(gomock.Any(), "mariadb/0", uri, "", gomock.Any()).Return(uri, nil, nil)
	s.secretsConsumer.EXPECT().GetSecretConsumer(gomock.Any(), uri, "mariadb/0").Return(&coresecrets.SecretConsumerMetadata{
		CurrentRevision: 665,
	}, nil)

	s.remoteClient.EXPECT().GetSecretAccessScope(uri, "token", 0).Return("scope-token", nil)
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

func (s *SecretsManagerSuite) TestGetSecretContentCrossModelExistingConsumerNoRefreshUpdateLabel(c *gc.C) {
	ctrl := s.setup(c)
	defer ctrl.Finish()

	anotherUUID := "deadbeef-0bad-0666-8000-4b1d0d06f66d"
	uri := coresecrets.NewURI().WithSource(anotherUUID)

	scopeTag := names.NewRelationTag("foo:bar baz:bar")
	mac := jujutesting.MustNewMacaroon("id")

	s.remoteClient = mocks.NewMockCrossModelSecretsClient(ctrl)
	s.remoteClient.EXPECT().Close().Return(nil)

	s.crossModelState.EXPECT().GetToken(names.NewApplicationTag("mariadb")).Return("token", nil)
	s.leadership.EXPECT().LeadershipCheck("mariadb", "mariadb/0").Return(s.token)
	s.secretService.EXPECT().ProcessSecretConsumerLabel(gomock.Any(), "mariadb/0", uri, "foo", gomock.Any()).Return(uri, nil, nil)
	s.secretsConsumer.EXPECT().GetSecretConsumer(gomock.Any(), uri, "mariadb/0").Return(&coresecrets.SecretConsumerMetadata{
		CurrentRevision: 665,
	}, nil)

	s.remoteClient.EXPECT().GetSecretAccessScope(uri, "token", 0).Return("scope-token", nil)
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

func (s *SecretsManagerSuite) TestGetSecretContentCrossModelExistingConsumerRefresh(c *gc.C) {
	ctrl := s.setup(c)
	defer ctrl.Finish()

	anotherUUID := "deadbeef-0bad-0666-8000-4b1d0d06f66d"
	uri := coresecrets.NewURI().WithSource(anotherUUID)

	scopeTag := names.NewRelationTag("foo:bar baz:bar")
	mac := jujutesting.MustNewMacaroon("id")

	s.remoteClient = mocks.NewMockCrossModelSecretsClient(ctrl)
	s.remoteClient.EXPECT().Close().Return(nil)

	s.crossModelState.EXPECT().GetToken(names.NewApplicationTag("mariadb")).Return("token", nil)
	s.leadership.EXPECT().LeadershipCheck("mariadb", "mariadb/0").Return(s.token)
	s.secretService.EXPECT().ProcessSecretConsumerLabel(gomock.Any(), "mariadb/0", uri, "", gomock.Any()).Return(uri, nil, nil)
	s.secretsConsumer.EXPECT().GetSecretConsumer(gomock.Any(), uri, "mariadb/0").Return(&coresecrets.SecretConsumerMetadata{
		CurrentRevision: 665,
	}, nil)

	s.remoteClient.EXPECT().GetSecretAccessScope(uri, "token", 0).Return("scope-token", nil)
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

	s.secretsConsumer.EXPECT().SaveSecretConsumer(gomock.Any(), uri, "mariadb/0", &coresecrets.SecretConsumerMetadata{
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

func (s *SecretsManagerSuite) TestGetSecretContentCrossModelNewConsumer(c *gc.C) {
	s.assertGetSecretContentCrossModelNewConsumer(c, secreterrors.SecretConsumerNotFound)
}

func (s *SecretsManagerSuite) TestGetSecretContentCrossModelNewConsumerAndSecret(c *gc.C) {
	s.assertGetSecretContentCrossModelNewConsumer(c, secreterrors.SecretNotFound)
}

func (s *SecretsManagerSuite) assertGetSecretContentCrossModelNewConsumer(c *gc.C, consumerErr error) {
	ctrl := s.setup(c)
	defer ctrl.Finish()

	anotherUUID := "deadbeef-0bad-0666-8000-4b1d0d06f66d"
	uri := coresecrets.NewURI().WithSource(anotherUUID)

	scopeTag := names.NewRelationTag("foo:bar baz:bar")
	mac := jujutesting.MustNewMacaroon("id")

	s.remoteClient = mocks.NewMockCrossModelSecretsClient(ctrl)
	s.remoteClient.EXPECT().Close().Return(nil)

	s.crossModelState.EXPECT().GetToken(names.NewApplicationTag("mariadb")).Return("token", nil)
	s.secretsConsumer.EXPECT().GetSecretConsumer(gomock.Any(), uri, "mariadb/0").Return(nil, consumerErr)
	s.leadership.EXPECT().LeadershipCheck("mariadb", "mariadb/0").Return(s.token)
	s.secretService.EXPECT().ProcessSecretConsumerLabel(gomock.Any(), "mariadb/0", uri, "", gomock.Any()).Return(uri, nil, nil)

	s.remoteClient.EXPECT().GetSecretAccessScope(uri, "token", 0).Return("scope-token", nil)
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

	s.secretsConsumer.EXPECT().SaveSecretConsumer(gomock.Any(), uri, "mariadb/0", &coresecrets.SecretConsumerMetadata{
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

func (s *SecretsManagerSuite) TestWatchConsumedSecretsChanges(c *gc.C) {
	defer s.setup(c).Finish()

	s.secretsConsumer.EXPECT().WatchConsumedSecretsChanges(gomock.Any(), "mariadb/0").Return(
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

func (s *SecretsManagerSuite) TestGetSecretRevisionContentInfo(c *gc.C) {
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

func (s *SecretsManagerSuite) TestWatchObsolete(c *gc.C) {
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

func (s *SecretsManagerSuite) TestWatchSecretsRotationChanges(c *gc.C) {
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

func (s *SecretsManagerSuite) TestSecretsRotated(c *gc.C) {
	defer s.setup(c).Finish()

	uri := coresecrets.NewURI()
	s.leadership.EXPECT().LeadershipCheck("mariadb", "mariadb/0").Return(s.token)
	s.secretTriggers.EXPECT().SecretRotated(gomock.Any(), uri, secretservice.SecretRotatedParams{
		LeaderToken: s.token,
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

func (s *SecretsManagerSuite) TestSecretsRotatedRetry(c *gc.C) {
	defer s.setup(c).Finish()

	uri := coresecrets.NewURI()
	s.leadership.EXPECT().LeadershipCheck("mariadb", "mariadb/0").Return(s.token)
	s.secretTriggers.EXPECT().SecretRotated(gomock.Any(), uri, secretservice.SecretRotatedParams{
		LeaderToken: s.token,
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

func (s *SecretsManagerSuite) TestSecretsRotatedForce(c *gc.C) {
	defer s.setup(c).Finish()

	uri := coresecrets.NewURI()
	s.leadership.EXPECT().LeadershipCheck("mariadb", "mariadb/0").Return(s.token)
	s.secretTriggers.EXPECT().SecretRotated(gomock.Any(), uri, secretservice.SecretRotatedParams{
		LeaderToken: s.token,
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

func (s *SecretsManagerSuite) TestWatchSecretRevisionsExpiryChanges(c *gc.C) {
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

func (s *SecretsManagerSuite) TestSecretsGrant(c *gc.C) {
	defer s.setup(c).Finish()

	uri := coresecrets.NewURI()
	s.secretsConsumer.EXPECT().GrantSecretAccess(gomock.Any(), uri, secretservice.SecretAccessParams{
		LeaderToken: s.token,
		Accessor: secretservice.SecretAccessor{
			Kind: secretservice.UnitAccessor,
			ID:   "mariadb/0",
		},
		Scope:   secretservice.SecretAccessScope{Kind: secretservice.RelationAccessScope, ID: "wordpress:db mysql:server"},
		Subject: secretservice.SecretAccessor{Kind: secretservice.UnitAccessor, ID: "wordpress/0"},
		Role:    coresecrets.RoleView,
	}).Return(errors.New("boom"))
	s.leadership.EXPECT().LeadershipCheck("mariadb", "mariadb/0").Return(s.token)

	subjectTag := names.NewUnitTag("wordpress/0")
	scopeTag := names.NewRelationTag("wordpress:db mysql:server")
	result, err := s.facade.SecretsGrant(context.Background(), params.GrantRevokeSecretArgs{
		Args: []params.GrantRevokeSecretArg{{
			URI:         uri.String(),
			ScopeTag:    scopeTag.String(),
			SubjectTags: []string{subjectTag.String()},
			Role:        "view",
		}, {
			URI:      uri.String(),
			ScopeTag: scopeTag.String(),
			Role:     "bad",
		}},
	})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(result, jc.DeepEquals, params.ErrorResults{
		Results: []params.ErrorResult{
			{
				Error: &params.Error{Code: "", Message: fmt.Sprintf(`cannot change access to %q for "unit-wordpress-0": boom`, uri.String())},
			},
			{
				Error: &params.Error{Code: params.CodeNotValid, Message: `secret role "bad" not valid`},
			},
		},
	})
}

func (s *SecretsManagerSuite) TestSecretsRevoke(c *gc.C) {
	defer s.setup(c).Finish()

	uri := coresecrets.NewURI()
	s.secretsConsumer.EXPECT().RevokeSecretAccess(gomock.Any(), uri, secretservice.SecretAccessParams{
		LeaderToken: s.token,
		Accessor: secretservice.SecretAccessor{
			Kind: secretservice.UnitAccessor,
			ID:   "mariadb/0",
		},
		Scope:   secretservice.SecretAccessScope{Kind: secretservice.RelationAccessScope, ID: "wordpress:db mysql:server"},
		Subject: secretservice.SecretAccessor{Kind: secretservice.UnitAccessor, ID: "wordpress/0"},
		Role:    coresecrets.RoleView,
	}).Return(errors.New("boom"))
	s.leadership.EXPECT().LeadershipCheck("mariadb", "mariadb/0").Return(s.token)

	subjectTag := names.NewUnitTag("wordpress/0")
	scopeTag := names.NewRelationTag("wordpress:db mysql:server")
	result, err := s.facade.SecretsRevoke(context.Background(), params.GrantRevokeSecretArgs{
		Args: []params.GrantRevokeSecretArg{{
			URI:         uri.String(),
			ScopeTag:    scopeTag.String(),
			SubjectTags: []string{subjectTag.String()},
			Role:        "view",
		}, {
			URI:      uri.String(),
			ScopeTag: scopeTag.String(),
			Role:     "bad",
		}},
	})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(result, jc.DeepEquals, params.ErrorResults{
		Results: []params.ErrorResult{
			{
				Error: &params.Error{Code: "", Message: fmt.Sprintf(`cannot change access to %q for "unit-wordpress-0": boom`, uri.String())},
			},
			{
				Error: &params.Error{Code: params.CodeNotValid, Message: `secret role "bad" not valid`},
			},
		},
	})
}

func (s *SecretsManagerSuite) TestUpdateTrackedRevisions(c *gc.C) {
	defer s.setup(c).Finish()

	uri := coresecrets.NewURI()
	s.secretsConsumer.EXPECT().GetConsumedRevision(gomock.Any(), uri, "mariadb/0", true, false, nil).
		Return(668, nil)
	result, err := s.facade.UpdateTrackedRevisions(context.Background(), []string{uri.ID})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(result, jc.DeepEquals, params.ErrorResults{Results: []params.ErrorResult{{}}})
}
