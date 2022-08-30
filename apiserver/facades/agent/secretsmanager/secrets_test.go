// Copyright 2021 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package secretsmanager_test

import (
	"fmt"
	"time"

	"github.com/golang/mock/gomock"
	"github.com/juju/clock"
	"github.com/juju/clock/testclock"
	"github.com/juju/errors"
	"github.com/juju/names/v4"
	"github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	facademocks "github.com/juju/juju/apiserver/facade/mocks"
	"github.com/juju/juju/apiserver/facades/agent/secretsmanager"
	"github.com/juju/juju/apiserver/facades/agent/secretsmanager/mocks"
	coresecrets "github.com/juju/juju/core/secrets"
	corewatcher "github.com/juju/juju/core/watcher"
	"github.com/juju/juju/rpc/params"
	"github.com/juju/juju/secrets"
	"github.com/juju/juju/state"
	coretesting "github.com/juju/juju/testing"
)

type SecretsManagerSuite struct {
	testing.IsolationSuite

	authorizer *facademocks.MockAuthorizer
	resources  *facademocks.MockResources

	leadership             *mocks.MockChecker
	token                  *mocks.MockToken
	secretsStore           *mocks.MockSecretsStore
	secretsConsumer        *mocks.MockSecretsConsumer
	secretsWatcher         *mocks.MockStringsWatcher
	secretsRotationService *mocks.MockSecretsRotation
	secretsRotationWatcher *mocks.MockSecretsRotationWatcher
	authTag                names.Tag
	clock                  clock.Clock

	facade *secretsmanager.SecretsManagerAPI
}

var _ = gc.Suite(&SecretsManagerSuite{})

func (s *SecretsManagerSuite) SetUpTest(c *gc.C) {
	s.IsolationSuite.SetUpTest(c)
}

func (s *SecretsManagerSuite) setup(c *gc.C) *gomock.Controller {
	ctrl := gomock.NewController(c)

	s.authorizer = facademocks.NewMockAuthorizer(ctrl)
	s.resources = facademocks.NewMockResources(ctrl)

	s.leadership = mocks.NewMockChecker(ctrl)
	s.token = mocks.NewMockToken(ctrl)
	s.secretsStore = mocks.NewMockSecretsStore(ctrl)
	s.secretsConsumer = mocks.NewMockSecretsConsumer(ctrl)
	s.secretsWatcher = mocks.NewMockStringsWatcher(ctrl)
	s.secretsRotationService = mocks.NewMockSecretsRotation(ctrl)
	s.secretsRotationWatcher = mocks.NewMockSecretsRotationWatcher(ctrl)
	s.authTag = names.NewUnitTag("mariadb/0")
	s.expectAuthUnitAgent()

	s.clock = testclock.NewClock(time.Now())
	var err error
	s.facade, err = secretsmanager.NewTestAPI(
		s.authorizer, s.resources, s.leadership, s.secretsStore, s.secretsConsumer, s.secretsRotationService,
		s.authTag, s.clock)
	c.Assert(err, jc.ErrorIsNil)

	return ctrl
}

func (s *SecretsManagerSuite) expectAuthUnitAgent() {
	s.authorizer.EXPECT().AuthUnitAgent().Return(true)
}

func (s *SecretsManagerSuite) expectSecretAccessQuery(n int) {
	s.secretsConsumer.EXPECT().SecretAccess(gomock.Any(), gomock.Any()).DoAndReturn(
		func(uri *coresecrets.URI, entity names.Tag) (coresecrets.SecretRole, error) {
			if entity.String() == s.authTag.String() {
				return coresecrets.RoleView, nil
			}
			if s.authTag.Kind() == names.UnitTagKind {
				appName, _ := names.UnitApplication(s.authTag.Id())
				if entity.Id() == appName {
					return coresecrets.RoleManage, nil
				}
			}
			return coresecrets.RoleNone, errors.NotFoundf("role")
		},
	).Times(n)
}

func ptr[T any](v T) *T {
	return &v
}

func (s *SecretsManagerSuite) TestCreateSecretURIs(c *gc.C) {
	defer s.setup(c).Finish()

	results, err := s.facade.CreateSecretURIs(params.CreateSecretURIsArg{
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

	p := state.CreateSecretParams{
		Version: secrets.Version,
		Owner:   "application-mariadb",
		UpdateSecretParams: state.UpdateSecretParams{
			LeaderToken:    s.token,
			RotatePolicy:   ptr(coresecrets.RotateDaily),
			NextRotateTime: ptr(s.clock.Now().AddDate(0, 0, 1)),
			ExpireTime:     ptr(s.clock.Now()),
			Description:    ptr("my secret"),
			Label:          ptr("foobar"),
			Params:         map[string]interface{}{"param": 1},
			Data:           map[string]string{"foo": "bar"},
		},
	}
	var gotURI *coresecrets.URI
	s.leadership.EXPECT().LeadershipCheck("mariadb", "mariadb/0").Return(s.token)
	s.token.EXPECT().Check(0, nil).Return(nil)
	s.secretsStore.EXPECT().CreateSecret(gomock.Any(), p).DoAndReturn(
		func(uri *coresecrets.URI, p state.CreateSecretParams) (*coresecrets.SecretMetadata, error) {
			ownerTag := names.NewApplicationTag("mariadb")
			s.secretsConsumer.EXPECT().GrantSecretAccess(uri, state.SecretAccessParams{
				LeaderToken: s.token,
				Scope:       ownerTag,
				Subject:     ownerTag,
				Role:        coresecrets.RoleManage,
			}).Return(nil)
			gotURI = uri
			md := &coresecrets.SecretMetadata{
				URI:            uri,
				LatestRevision: 1,
			}
			return md, nil
		},
	)

	results, err := s.facade.CreateSecrets(params.CreateSecretArgs{
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

	p := state.CreateSecretParams{
		Version: secrets.Version,
		Owner:   "application-mariadb",
		UpdateSecretParams: state.UpdateSecretParams{
			LeaderToken: s.token,
			Label:       ptr("foobar"),
			Data:        map[string]string{"foo": "bar"},
		},
	}
	s.leadership.EXPECT().LeadershipCheck("mariadb", "mariadb/0").Return(s.token)
	s.token.EXPECT().Check(0, nil).Return(nil)
	s.secretsStore.EXPECT().CreateSecret(gomock.Any(), p).Return(
		nil, fmt.Errorf("dup label %w", state.LabelExists),
	)

	results, err := s.facade.CreateSecrets(params.CreateSecretArgs{
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

	p := state.UpdateSecretParams{
		LeaderToken:    s.token,
		RotatePolicy:   ptr(coresecrets.RotateDaily),
		NextRotateTime: ptr(s.clock.Now().AddDate(0, 0, 1)),
		ExpireTime:     ptr(s.clock.Now()),
		Description:    ptr("my secret"),
		Label:          ptr("foobar"),
		Params:         map[string]interface{}{"param": 1},
		Data:           map[string]string{"foo": "bar"},
	}
	uri := coresecrets.NewURI()
	expectURI := *uri
	expectURI.ControllerUUID = coretesting.ControllerTag.Id()
	s.secretsStore.EXPECT().GetSecret(&expectURI).Return(&coresecrets.SecretMetadata{}, nil)
	s.secretsStore.EXPECT().UpdateSecret(&expectURI, p).DoAndReturn(
		func(uri *coresecrets.URI, p state.UpdateSecretParams) (*coresecrets.SecretMetadata, error) {
			md := &coresecrets.SecretMetadata{
				URI:            uri,
				LatestRevision: 2,
			}
			return md, nil
		},
	)
	s.leadership.EXPECT().LeadershipCheck("mariadb", "mariadb/0").Return(s.token)
	s.token.EXPECT().Check(0, nil).Return(nil)
	s.expectSecretAccessQuery(1)
	uri1 := *uri
	uri1.ControllerUUID = "deadbeef-1bad-500d-9000-4b1d0d061111"

	results, err := s.facade.UpdateSecrets(params.UpdateSecretArgs{
		Args: []params.UpdateSecretArg{{
			URI: uri.ShortString(),
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
		}, {
			URI: uri1.String(),
		}},
	})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(results, jc.DeepEquals, params.ErrorResults{
		Results: []params.ErrorResult{{}, {
			Error: &params.Error{Message: `at least one attribute to update must be specified`},
		}, {
			Error: &params.Error{Code: params.CodeNotValid, Message: `secret URI with controller UUID "deadbeef-1bad-500d-9000-4b1d0d061111" not valid`},
		}},
	})
}

func (s *SecretsManagerSuite) TestUpdateSecretDuplicateLabel(c *gc.C) {
	defer s.setup(c).Finish()

	p := state.UpdateSecretParams{
		LeaderToken: s.token,
		Label:       ptr("foobar"),
	}
	uri := coresecrets.NewURI()
	expectURI := *uri
	expectURI.ControllerUUID = coretesting.ControllerTag.Id()
	s.secretsStore.EXPECT().GetSecret(&expectURI).Return(&coresecrets.SecretMetadata{}, nil)
	s.secretsStore.EXPECT().UpdateSecret(&expectURI, p).Return(
		nil, fmt.Errorf("dup label %w", state.LabelExists),
	)
	s.leadership.EXPECT().LeadershipCheck("mariadb", "mariadb/0").Return(s.token)
	s.token.EXPECT().Check(0, nil).Return(nil)
	s.expectSecretAccessQuery(1)
	uri1 := *uri
	uri1.ControllerUUID = "deadbeef-1bad-500d-9000-4b1d0d061111"

	results, err := s.facade.UpdateSecrets(params.UpdateSecretArgs{
		Args: []params.UpdateSecretArg{{
			URI: uri.ShortString(),
			UpsertSecretArg: params.UpsertSecretArg{
				Label: ptr("foobar"),
			},
		}},
	})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(results, jc.DeepEquals, params.ErrorResults{
		Results: []params.ErrorResult{{
			Error: &params.Error{Message: `secret with label "foobar" already exists`, Code: params.CodeAlreadyExists},
		}},
	})
}

func (s *SecretsManagerSuite) TestRemoveSecrets(c *gc.C) {
	defer s.setup(c).Finish()

	uri := coresecrets.NewURI()
	expectURI := *uri
	expectURI.ControllerUUID = coretesting.ControllerTag.Id()
	s.secretsStore.EXPECT().DeleteSecret(&expectURI).Return(nil)
	s.leadership.EXPECT().LeadershipCheck("mariadb", "mariadb/0").Return(s.token)
	s.token.EXPECT().Check(0, nil).Return(nil)
	s.expectSecretAccessQuery(1)
	uri1 := *uri
	uri1.ControllerUUID = "deadbeef-1bad-500d-9000-4b1d0d061111"

	results, err := s.facade.RemoveSecrets(params.SecretURIArgs{
		Args: []params.SecretURIArg{{
			URI: expectURI.ShortString(),
		}, {
			URI: uri1.String(),
		}},
	})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(results, jc.DeepEquals, params.ErrorResults{
		Results: []params.ErrorResult{{}, {
			Error: &params.Error{Code: params.CodeNotValid, Message: `secret URI with controller UUID "deadbeef-1bad-500d-9000-4b1d0d061111" not valid`},
		}},
	})
}

func (s *SecretsManagerSuite) TestGetConsumerSecretsRevisionInfo(c *gc.C) {
	defer s.setup(c).Finish()

	s.expectSecretAccessQuery(1)
	uri := coresecrets.NewURI()
	uri.ControllerUUID = coretesting.ControllerTag.Id()
	s.secretsConsumer.EXPECT().GetSecretConsumer(uri, "application-mariadb").Return(
		&coresecrets.SecretConsumerMetadata{
			LatestRevision: 666,
			Label:          "label",
		}, nil)

	results, err := s.facade.GetConsumerSecretsRevisionInfo(params.GetSecretConsumerInfoArgs{
		ConsumerTag: "application-mariadb",
		URIs:        []string{uri.ShortString()},
	})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(results, jc.DeepEquals, params.SecretConsumerInfoResults{
		Results: []params.SecretConsumerInfoResult{{
			Label:    "label",
			Revision: 666,
		}},
	})
}

func (s *SecretsManagerSuite) TestGetSecretMetadata(c *gc.C) {
	defer s.setup(c).Finish()

	now := time.Now()
	uri := coresecrets.NewURI()
	uri.ControllerUUID = coretesting.ControllerTag.Id()
	s.secretsStore.EXPECT().ListSecrets(
		state.SecretsFilter{
			OwnerTag: ptr("application-mariadb"),
		}).Return([]*coresecrets.SecretMetadata{{
		URI:              uri,
		Description:      "description",
		Label:            "label",
		RotatePolicy:     coresecrets.RotateHourly,
		LatestRevision:   666,
		LatestExpireTime: &now,
		NextRotateTime:   &now,
	}}, nil)

	results, err := s.facade.GetSecretMetadata()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(results, jc.DeepEquals, params.ListSecretResults{
		Results: []params.ListSecretResult{{
			URI:              uri.ShortString(),
			Description:      "description",
			Label:            "label",
			RotatePolicy:     coresecrets.RotateHourly.String(),
			LatestRevision:   666,
			LatestExpireTime: &now,
			NextRotateTime:   &now,
		}},
	})
}

func (s *SecretsManagerSuite) TestGetSecretContent(c *gc.C) {
	defer s.setup(c).Finish()

	// Secret 1 has been consumed before.
	data := map[string]string{"foo": "bar"}
	val := coresecrets.NewSecretValue(data)
	uri := coresecrets.NewURI()
	uri.ControllerUUID = coretesting.ControllerTag.Id()
	s.secretsConsumer.EXPECT().GetSecretConsumer(uri, "unit-mariadb-0").Return(
		&coresecrets.SecretConsumerMetadata{CurrentRevision: 666}, nil)

	// Secret 2 has not been consumed before.
	data2 := map[string]string{"foo": "bar2"}
	val2 := coresecrets.NewSecretValue(data2)
	uri2 := coresecrets.NewURI()
	uri2.ControllerUUID = coretesting.ControllerTag.Id()
	s.secretsConsumer.EXPECT().GetSecretConsumer(uri2, "unit-mariadb-0").Return(
		nil, errors.NotFoundf("secret"))
	s.expectSecretAccessQuery(2)
	s.secretsStore.EXPECT().GetSecret(uri2).Return(&coresecrets.SecretMetadata{LatestRevision: 668}, nil)
	s.secretsConsumer.EXPECT().SaveSecretConsumer(
		uri2, "unit-mariadb-0", &coresecrets.SecretConsumerMetadata{
			CurrentRevision: 668,
			LatestRevision:  668,
		}).Return(nil)
	s.secretsStore.EXPECT().GetSecretValue(uri, 666).Return(
		val, nil,
	)
	s.secretsStore.EXPECT().GetSecretValue(uri2, 668).Return(
		val2, nil,
	)

	results, err := s.facade.GetSecretContentInfo(params.GetSecretContentArgs{
		Args: []params.GetSecretContentArg{{
			URI: uri.ShortString(),
		}, {
			URI: uri2.ShortString(),
		}},
	})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(results, jc.DeepEquals, params.SecretContentResults{
		Results: []params.SecretContentResult{{
			Content: params.SecretContentParams{Data: data},
		}, {
			Content: params.SecretContentParams{Data: data2},
		}},
	})
}

func (s *SecretsManagerSuite) TestGetSecretContentExplicitUUIDs(c *gc.C) {
	defer s.setup(c).Finish()

	data := map[string]string{"foo": "bar"}
	val := coresecrets.NewSecretValue(data)
	uri := coresecrets.NewURI()
	uri.ControllerUUID = "deadbeef-1bad-500d-9000-4b1d0d061111"
	s.secretsConsumer.EXPECT().GetSecretConsumer(uri, "unit-mariadb-0").Return(
		&coresecrets.SecretConsumerMetadata{CurrentRevision: 666}, nil)
	s.expectSecretAccessQuery(1)
	s.secretsStore.EXPECT().GetSecretValue(uri, 666).Return(
		val, nil,
	)

	results, err := s.facade.GetSecretContentInfo(params.GetSecretContentArgs{
		Args: []params.GetSecretContentArg{{
			URI: uri.String(),
		}},
	})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(results, jc.DeepEquals, params.SecretContentResults{
		Results: []params.SecretContentResult{{
			Content: params.SecretContentParams{Data: data},
		}},
	})
}

func (s *SecretsManagerSuite) TestWatchSecretsChanges(c *gc.C) {
	defer s.setup(c).Finish()

	s.secretsConsumer.EXPECT().WatchConsumedSecretsChanges("unit-mariadb-0").Return(
		s.secretsWatcher,
	)
	s.resources.EXPECT().Register(s.secretsWatcher).Return("1")

	uri := coresecrets.NewURI()
	watchChan := make(chan []string, 1)
	watchChan <- []string{uri.String()}
	s.secretsWatcher.EXPECT().Changes().Return(watchChan)

	result, err := s.facade.WatchSecretsChanges(params.Entities{
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

func (s *SecretsManagerSuite) TestWatchSecretsRotationChanges(c *gc.C) {
	defer s.setup(c).Finish()

	s.secretsRotationService.EXPECT().WatchSecretsRotationChanges("application-mariadb").Return(
		s.secretsRotationWatcher,
	)
	s.resources.EXPECT().Register(s.secretsRotationWatcher).Return("1")

	next := time.Now().Add(time.Hour)
	uri := coresecrets.NewURI()
	rotateChan := make(chan []corewatcher.SecretTriggerChange, 1)
	rotateChan <- []corewatcher.SecretTriggerChange{{
		URI:             uri,
		NextTriggerTime: next,
	}}
	s.secretsRotationWatcher.EXPECT().Changes().Return(rotateChan)

	result, err := s.facade.WatchSecretsRotationChanges(params.Entities{
		Entities: []params.Entity{{
			Tag: "application-mariadb",
		}, {
			Tag: "application-foo",
		}},
	})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(result, jc.DeepEquals, params.SecretTriggerWatchResults{
		Results: []params.SecretTriggerWatchResult{{
			WatcherId: "1",
			Changes: []params.SecretTriggerChange{{
				URI:             uri.String(),
				NextTriggerTime: next,
			}},
		}, {
			Error: &params.Error{Code: "unauthorized access", Message: "permission denied"},
		}},
	})
}

func (s *SecretsManagerSuite) TestSecretsRotated(c *gc.C) {
	defer s.setup(c).Finish()

	uri := coresecrets.NewURI()
	uri.ControllerUUID = coretesting.ControllerTag.Id()
	nextRotateTime := s.clock.Now().Add(time.Hour)
	s.secretsRotationService.EXPECT().SecretRotated(uri, nextRotateTime).Return(errors.New("boom"))
	s.secretsStore.EXPECT().GetSecret(uri).Return(&coresecrets.SecretMetadata{
		OwnerTag:       "application-mariadb",
		RotatePolicy:   coresecrets.RotateHourly,
		LatestRevision: 667,
	}, nil)

	result, err := s.facade.SecretsRotated(params.SecretRotatedArgs{
		Args: []params.SecretRotatedArg{{
			URI:              uri.ShortString(),
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
	uri.ControllerUUID = coretesting.ControllerTag.Id()
	nextRotateTime := s.clock.Now().Add(coresecrets.RotateRetryDelay)
	s.secretsRotationService.EXPECT().SecretRotated(uri, nextRotateTime).Return(errors.New("boom"))
	s.secretsStore.EXPECT().GetSecret(uri).Return(&coresecrets.SecretMetadata{
		OwnerTag:       "application-mariadb",
		RotatePolicy:   coresecrets.RotateHourly,
		LatestRevision: 666,
	}, nil)

	result, err := s.facade.SecretsRotated(params.SecretRotatedArgs{
		Args: []params.SecretRotatedArg{{
			URI:              uri.ShortString(),
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

func (s *SecretsManagerSuite) TestSecretsRotatedThenNever(c *gc.C) {
	defer s.setup(c).Finish()

	uri := coresecrets.NewURI()
	uri.ControllerUUID = coretesting.ControllerTag.Id()
	s.secretsStore.EXPECT().GetSecret(uri).Return(&coresecrets.SecretMetadata{
		OwnerTag:       "application-mariadb",
		RotatePolicy:   coresecrets.RotateNever,
		LatestRevision: 667,
	}, nil)

	result, err := s.facade.SecretsRotated(params.SecretRotatedArgs{
		Args: []params.SecretRotatedArg{{
			URI:              uri.ShortString(),
			OriginalRevision: 666,
		}},
	})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(result, jc.DeepEquals, params.ErrorResults{
		Results: []params.ErrorResult{{}},
	})
}

func (s *SecretsManagerSuite) TestSecretsGrant(c *gc.C) {
	defer s.setup(c).Finish()

	s.expectSecretAccessQuery(2)
	uri := coresecrets.NewURI()
	uri.ControllerUUID = coretesting.ControllerTag.Id()
	subjectTag := names.NewUnitTag("wordpress/0")
	scopeTag := names.NewRelationTag("wordpress:db mysql:server")
	s.secretsStore.EXPECT().GetSecret(uri).Return(&coresecrets.SecretMetadata{
		OwnerTag: "application-mariadb",
	}, nil).AnyTimes()
	s.secretsConsumer.EXPECT().GrantSecretAccess(uri, state.SecretAccessParams{
		LeaderToken: s.token,
		Scope:       scopeTag,
		Subject:     subjectTag,
		Role:        coresecrets.RoleView,
	}).Return(errors.New("boom"))
	s.leadership.EXPECT().LeadershipCheck("mariadb", "mariadb/0").Return(s.token)
	s.token.EXPECT().Check(0, nil).Return(nil)

	result, err := s.facade.SecretsGrant(params.GrantRevokeSecretArgs{
		Args: []params.GrantRevokeSecretArg{{
			URI:         uri.ShortString(),
			ScopeTag:    scopeTag.String(),
			SubjectTags: []string{subjectTag.String()},
			Role:        "view",
		}, {
			URI:      uri.ShortString(),
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

	s.expectSecretAccessQuery(2)
	uri := coresecrets.NewURI()
	uri.ControllerUUID = coretesting.ControllerTag.Id()
	subjectTag := names.NewUnitTag("wordpress/0")
	scopeTag := names.NewRelationTag("wordpress:db mysql:server")
	s.secretsStore.EXPECT().GetSecret(uri).Return(&coresecrets.SecretMetadata{
		OwnerTag: "application-mariadb",
	}, nil).AnyTimes()
	s.secretsConsumer.EXPECT().RevokeSecretAccess(uri, state.SecretAccessParams{
		LeaderToken: s.token,
		Scope:       scopeTag,
		Subject:     subjectTag,
		Role:        coresecrets.RoleView,
	}).Return(errors.New("boom"))
	s.leadership.EXPECT().LeadershipCheck("mariadb", "mariadb/0").Return(s.token)
	s.token.EXPECT().Check(0, nil).Return(nil)

	result, err := s.facade.SecretsRevoke(params.GrantRevokeSecretArgs{
		Args: []params.GrantRevokeSecretArg{{
			URI:         uri.ShortString(),
			ScopeTag:    scopeTag.String(),
			SubjectTags: []string{subjectTag.String()},
			Role:        "view",
		}, {
			URI:      uri.ShortString(),
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
