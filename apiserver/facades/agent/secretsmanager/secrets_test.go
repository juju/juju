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
	"github.com/juju/juju/secrets/provider"
	"github.com/juju/juju/state"
)

type SecretsManagerSuite struct {
	testing.IsolationSuite

	authorizer *facademocks.MockAuthorizer
	resources  *facademocks.MockResources

	provider              *mocks.MockSecretStoreProvider
	leadership            *mocks.MockChecker
	token                 *mocks.MockToken
	secretsBackend        *mocks.MockSecretsBackend
	secretsConsumer       *mocks.MockSecretsConsumer
	secretsWatcher        *mocks.MockStringsWatcher
	secretTriggers        *mocks.MockSecretTriggers
	secretsTriggerWatcher *mocks.MockSecretsTriggerWatcher
	authTag               names.Tag
	clock                 clock.Clock

	facade *secretsmanager.SecretsManagerAPI
}

var _ = gc.Suite(&SecretsManagerSuite{})

func (s *SecretsManagerSuite) SetUpTest(c *gc.C) {
	s.IsolationSuite.SetUpTest(c)
}

type mockModel struct {
	provider.Model
}

func (s *SecretsManagerSuite) setup(c *gc.C) *gomock.Controller {
	ctrl := gomock.NewController(c)

	s.authorizer = facademocks.NewMockAuthorizer(ctrl)
	s.resources = facademocks.NewMockResources(ctrl)

	s.provider = mocks.NewMockSecretStoreProvider(ctrl)
	s.leadership = mocks.NewMockChecker(ctrl)
	s.token = mocks.NewMockToken(ctrl)
	s.secretsBackend = mocks.NewMockSecretsBackend(ctrl)
	s.secretsConsumer = mocks.NewMockSecretsConsumer(ctrl)
	s.secretsWatcher = mocks.NewMockStringsWatcher(ctrl)
	s.secretTriggers = mocks.NewMockSecretTriggers(ctrl)
	s.secretsTriggerWatcher = mocks.NewMockSecretsTriggerWatcher(ctrl)
	s.authTag = names.NewUnitTag("mariadb/0")
	s.expectAuthUnitAgent()

	s.clock = testclock.NewClock(time.Now())

	storeConfigGetter := func() (*provider.StoreConfig, error) {
		return &provider.StoreConfig{
			StoreType: "juju",
			Params:    map[string]interface{}{"foo": "bar"},
		}, nil
	}
	providerGetter := func() (provider.SecretStoreProvider, provider.Model, error) {
		return s.provider, mockModel{}, nil
	}
	var err error
	s.facade, err = secretsmanager.NewTestAPI(
		s.authorizer, s.resources, s.leadership, s.secretsBackend, s.secretsConsumer, s.secretTriggers,
		storeConfigGetter, providerGetter, s.authTag, s.clock)
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

func (s *SecretsManagerSuite) TestGetSecretStoreConfig(c *gc.C) {
	defer s.setup(c).Finish()

	result, err := s.facade.GetSecretStoreConfig()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(result, jc.DeepEquals, params.SecretStoreConfig{
		StoreType: "juju",
		Params:    map[string]interface{}{"foo": "bar"},
	})
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
		Owner:   names.NewApplicationTag("mariadb"),
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
	s.secretsBackend.EXPECT().CreateSecret(gomock.Any(), p).DoAndReturn(
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
		Owner:   names.NewApplicationTag("mariadb"),
		UpdateSecretParams: state.UpdateSecretParams{
			LeaderToken: s.token,
			Label:       ptr("foobar"),
			Data:        map[string]string{"foo": "bar"},
		},
	}
	s.leadership.EXPECT().LeadershipCheck("mariadb", "mariadb/0").Return(s.token)
	s.token.EXPECT().Check(0, nil).Return(nil)
	s.secretsBackend.EXPECT().CreateSecret(gomock.Any(), p).Return(
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
	pWithProviderId := p
	p.ProviderId = ptr("provider-id")
	p.Data = nil
	uri := coresecrets.NewURI()
	expectURI := *uri
	s.secretsBackend.EXPECT().GetSecret(&expectURI).Return(&coresecrets.SecretMetadata{}, nil).Times(2)
	s.secretsBackend.EXPECT().UpdateSecret(&expectURI, p).DoAndReturn(
		func(uri *coresecrets.URI, p state.UpdateSecretParams) (*coresecrets.SecretMetadata, error) {
			md := &coresecrets.SecretMetadata{
				URI:            uri,
				LatestRevision: 2,
			}
			return md, nil
		},
	)
	s.secretsBackend.EXPECT().UpdateSecret(&expectURI, pWithProviderId).DoAndReturn(
		func(uri *coresecrets.URI, p state.UpdateSecretParams) (*coresecrets.SecretMetadata, error) {
			md := &coresecrets.SecretMetadata{
				URI:            uri,
				LatestRevision: 3,
			}
			return md, nil
		},
	)
	s.leadership.EXPECT().LeadershipCheck("mariadb", "mariadb/0").Return(s.token).Times(2)
	s.token.EXPECT().Check(0, nil).Return(nil).Times(2)
	s.expectSecretAccessQuery(2)

	results, err := s.facade.UpdateSecrets(params.UpdateSecretArgs{
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
				Content:      params.SecretContentParams{ProviderId: ptr("provider-id")},
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

func (s *SecretsManagerSuite) TestUpdateSecretDuplicateLabel(c *gc.C) {
	defer s.setup(c).Finish()

	p := state.UpdateSecretParams{
		LeaderToken: s.token,
		Label:       ptr("foobar"),
	}
	uri := coresecrets.NewURI()
	expectURI := *uri
	s.secretsBackend.EXPECT().GetSecret(&expectURI).Return(&coresecrets.SecretMetadata{}, nil)
	s.secretsBackend.EXPECT().UpdateSecret(&expectURI, p).Return(
		nil, fmt.Errorf("dup label %w", state.LabelExists),
	)
	s.leadership.EXPECT().LeadershipCheck("mariadb", "mariadb/0").Return(s.token)
	s.token.EXPECT().Check(0, nil).Return(nil)
	s.expectSecretAccessQuery(1)

	results, err := s.facade.UpdateSecrets(params.UpdateSecretArgs{
		Args: []params.UpdateSecretArg{{
			URI: uri.String(),
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
	s.secretsBackend.EXPECT().DeleteSecret(&expectURI, []int{666}).Return(true, nil)
	s.leadership.EXPECT().LeadershipCheck("mariadb", "mariadb/0").Return(s.token)
	s.token.EXPECT().Check(0, nil).Return(nil)
	s.expectSecretAccessQuery(1)
	s.provider.EXPECT().CleanupSecrets(mockModel{}, []*coresecrets.URI{uri}).Return(nil)

	results, err := s.facade.RemoveSecrets(params.DeleteSecretArgs{
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

func (s *SecretsManagerSuite) TestRemoveSecretRevision(c *gc.C) {
	defer s.setup(c).Finish()

	uri := coresecrets.NewURI()
	expectURI := *uri
	s.secretsBackend.EXPECT().DeleteSecret(&expectURI, []int{666}).Return(false, nil)
	s.leadership.EXPECT().LeadershipCheck("mariadb", "mariadb/0").Return(s.token)
	s.token.EXPECT().Check(0, nil).Return(nil)
	s.expectSecretAccessQuery(1)

	results, err := s.facade.RemoveSecrets(params.DeleteSecretArgs{
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

func (s *SecretsManagerSuite) TestGetConsumerSecretsRevisionInfo(c *gc.C) {
	defer s.setup(c).Finish()

	s.expectSecretAccessQuery(1)
	uri := coresecrets.NewURI()
	s.secretsConsumer.EXPECT().GetSecretConsumer(uri, names.NewApplicationTag("mariadb")).Return(
		&coresecrets.SecretConsumerMetadata{
			LatestRevision: 666,
			Label:          "label",
		}, nil)

	results, err := s.facade.GetConsumerSecretsRevisionInfo(params.GetSecretConsumerInfoArgs{
		ConsumerTag: "application-mariadb",
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

func (s *SecretsManagerSuite) TestGetSecretMetadata(c *gc.C) {
	defer s.setup(c).Finish()

	s.leadership.EXPECT().LeadershipCheck("mariadb", "mariadb/0").Return(s.token)
	s.token.EXPECT().Check(0, nil).Return(nil)

	now := time.Now()
	uri := coresecrets.NewURI()
	s.secretsBackend.EXPECT().ListSecrets(
		state.SecretsFilter{
			OwnerTags: []names.Tag{names.NewUnitTag("mariadb/0"), names.NewApplicationTag("mariadb")},
		}).Return([]*coresecrets.SecretMetadata{{
		URI:              uri,
		OwnerTag:         "application-mariadb",
		Description:      "description",
		Label:            "label",
		RotatePolicy:     coresecrets.RotateHourly,
		LatestRevision:   666,
		LatestExpireTime: &now,
		NextRotateTime:   &now,
	}}, nil)
	s.secretsBackend.EXPECT().ListSecretRevisions(uri).Return([]*coresecrets.SecretRevisionMetadata{{
		Revision:   666,
		ProviderId: ptr("provider-id"),
	}, {
		Revision: 667,
	}}, nil)

	results, err := s.facade.GetSecretMetadata()
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
				Revision:   666,
				ProviderId: ptr("provider-id"),
			}, {
				Revision: 667,
			}},
		}},
	})
}

func (s *SecretsManagerSuite) TestGetSecretContent(c *gc.C) {
	defer s.setup(c).Finish()

	// Secret 1 has been consumed before.
	data := map[string]string{"foo": "bar"}
	val := coresecrets.NewSecretValue(data)
	uri := coresecrets.NewURI()
	s.secretsConsumer.EXPECT().GetSecretConsumer(uri, names.NewUnitTag("mariadb/0")).Return(
		&coresecrets.SecretConsumerMetadata{CurrentRevision: 666}, nil)

	// Secret 2 has not been consumed before.
	data2 := map[string]string{"foo": "bar2"}
	val2 := coresecrets.NewSecretValue(data2)
	uri2 := coresecrets.NewURI()
	s.secretsConsumer.EXPECT().GetSecretConsumer(uri2, names.NewUnitTag("mariadb/0")).Return(
		nil, errors.NotFoundf("secret"))
	s.expectSecretAccessQuery(2)
	s.secretsBackend.EXPECT().GetSecret(uri2).Return(&coresecrets.SecretMetadata{LatestRevision: 668}, nil)
	s.secretsConsumer.EXPECT().SaveSecretConsumer(
		uri2, names.NewUnitTag("mariadb/0"), &coresecrets.SecretConsumerMetadata{
			CurrentRevision: 668,
			LatestRevision:  668,
		}).Return(nil)
	s.secretsBackend.EXPECT().GetSecretValue(uri, 666).Return(
		val, nil, nil,
	)
	s.secretsBackend.EXPECT().GetSecretValue(uri2, 668).Return(
		val2, nil, nil,
	)

	results, err := s.facade.GetSecretContentInfo(params.GetSecretContentArgs{
		Args: []params.GetSecretContentArg{{
			URI: uri.String(),
		}, {
			URI: uri2.String(),
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

func (s *SecretsManagerSuite) TestWatchConsumedSecretsChanges(c *gc.C) {
	defer s.setup(c).Finish()

	s.secretsConsumer.EXPECT().WatchConsumedSecretsChanges(names.NewUnitTag("mariadb/0")).Return(
		s.secretsWatcher, nil,
	)
	s.resources.EXPECT().Register(s.secretsWatcher).Return("1")

	uri := coresecrets.NewURI()
	watchChan := make(chan []string, 1)
	watchChan <- []string{uri.String()}
	s.secretsWatcher.EXPECT().Changes().Return(watchChan)

	result, err := s.facade.WatchConsumedSecretsChanges(params.Entities{
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

func (s *SecretsManagerSuite) TestWatchObsolete(c *gc.C) {
	defer s.setup(c).Finish()

	s.leadership.EXPECT().LeadershipCheck("mariadb", "mariadb/0").Return(s.token)
	s.token.EXPECT().Check(0, nil).Return(nil)
	s.secretsBackend.EXPECT().WatchObsolete(
		[]names.Tag{names.NewUnitTag("mariadb/0"), names.NewApplicationTag("mariadb")}).Return(
		s.secretsWatcher, nil,
	)
	s.resources.EXPECT().Register(s.secretsWatcher).Return("1")

	uri := coresecrets.NewURI()
	watchChan := make(chan []string, 1)
	watchChan <- []string{uri.String()}
	s.secretsWatcher.EXPECT().Changes().Return(watchChan)

	result, err := s.facade.WatchObsolete(params.Entities{
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
	s.token.EXPECT().Check(0, nil).Return(nil)
	s.secretTriggers.EXPECT().WatchSecretsRotationChanges(
		[]names.Tag{names.NewUnitTag("mariadb/0"), names.NewApplicationTag("mariadb")}).Return(
		s.secretsTriggerWatcher, nil,
	)
	s.resources.EXPECT().Register(s.secretsTriggerWatcher).Return("1")

	next := time.Now().Add(time.Hour)
	uri := coresecrets.NewURI()
	rotateChan := make(chan []corewatcher.SecretTriggerChange, 1)
	rotateChan <- []corewatcher.SecretTriggerChange{{
		URI:             uri,
		NextTriggerTime: next,
	}}
	s.secretsTriggerWatcher.EXPECT().Changes().Return(rotateChan)

	result, err := s.facade.WatchSecretsRotationChanges(params.Entities{
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
			URI:             uri.String(),
			NextTriggerTime: next,
		}},
	})
}

func (s *SecretsManagerSuite) TestSecretsRotated(c *gc.C) {
	defer s.setup(c).Finish()

	uri := coresecrets.NewURI()
	nextRotateTime := s.clock.Now().Add(time.Hour)
	s.secretTriggers.EXPECT().SecretRotated(uri, nextRotateTime).Return(errors.New("boom"))
	s.secretsBackend.EXPECT().GetSecret(uri).Return(&coresecrets.SecretMetadata{
		OwnerTag:       "application-mariadb",
		RotatePolicy:   coresecrets.RotateHourly,
		LatestRevision: 667,
	}, nil)

	result, err := s.facade.SecretsRotated(params.SecretRotatedArgs{
		Args: []params.SecretRotatedArg{{
			URI:              uri.String(),
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
	nextRotateTime := s.clock.Now().Add(coresecrets.RotateRetryDelay)
	s.secretTriggers.EXPECT().SecretRotated(uri, nextRotateTime).Return(errors.New("boom"))
	s.secretsBackend.EXPECT().GetSecret(uri).Return(&coresecrets.SecretMetadata{
		OwnerTag:       "application-mariadb",
		RotatePolicy:   coresecrets.RotateHourly,
		LatestRevision: 666,
	}, nil)

	result, err := s.facade.SecretsRotated(params.SecretRotatedArgs{
		Args: []params.SecretRotatedArg{{
			URI:              uri.String(),
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
	nextRotateTime := s.clock.Now().Add(coresecrets.RotateRetryDelay)
	s.secretTriggers.EXPECT().SecretRotated(uri, nextRotateTime).Return(errors.New("boom"))
	s.secretsBackend.EXPECT().GetSecret(uri).Return(&coresecrets.SecretMetadata{
		OwnerTag:         "application-mariadb",
		RotatePolicy:     coresecrets.RotateHourly,
		LatestExpireTime: ptr(s.clock.Now().Add(50 * time.Minute)),
		LatestRevision:   667,
	}, nil)

	result, err := s.facade.SecretsRotated(params.SecretRotatedArgs{
		Args: []params.SecretRotatedArg{{
			URI:              uri.String(),
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
	s.secretsBackend.EXPECT().GetSecret(uri).Return(&coresecrets.SecretMetadata{
		OwnerTag:       "application-mariadb",
		RotatePolicy:   coresecrets.RotateNever,
		LatestRevision: 667,
	}, nil)

	result, err := s.facade.SecretsRotated(params.SecretRotatedArgs{
		Args: []params.SecretRotatedArg{{
			URI:              uri.String(),
			OriginalRevision: 666,
		}},
	})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(result, jc.DeepEquals, params.ErrorResults{
		Results: []params.ErrorResult{{}},
	})
}

func (s *SecretsManagerSuite) TestWatchSecretRevisionsExpiryChanges(c *gc.C) {
	defer s.setup(c).Finish()

	s.leadership.EXPECT().LeadershipCheck("mariadb", "mariadb/0").Return(s.token)
	s.token.EXPECT().Check(0, nil).Return(nil)
	s.secretTriggers.EXPECT().WatchSecretRevisionsExpiryChanges(
		[]names.Tag{names.NewUnitTag("mariadb/0"), names.NewApplicationTag("mariadb")}).Return(
		s.secretsTriggerWatcher, nil,
	)
	s.resources.EXPECT().Register(s.secretsTriggerWatcher).Return("1")

	next := time.Now().Add(time.Hour)
	uri := coresecrets.NewURI()
	expiryChan := make(chan []corewatcher.SecretTriggerChange, 1)
	expiryChan <- []corewatcher.SecretTriggerChange{{
		URI:             uri,
		Revision:        666,
		NextTriggerTime: next,
	}}
	s.secretsTriggerWatcher.EXPECT().Changes().Return(expiryChan)

	result, err := s.facade.WatchSecretRevisionsExpiryChanges(params.Entities{
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
			URI:             uri.String(),
			Revision:        666,
			NextTriggerTime: next,
		}},
	})
}

func (s *SecretsManagerSuite) TestSecretsGrant(c *gc.C) {
	defer s.setup(c).Finish()

	s.expectSecretAccessQuery(1)
	uri := coresecrets.NewURI()
	subjectTag := names.NewUnitTag("wordpress/0")
	scopeTag := names.NewRelationTag("wordpress:db mysql:server")
	s.secretsBackend.EXPECT().GetSecret(uri).Return(&coresecrets.SecretMetadata{
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

	s.expectSecretAccessQuery(1)
	uri := coresecrets.NewURI()
	subjectTag := names.NewUnitTag("wordpress/0")
	scopeTag := names.NewRelationTag("wordpress:db mysql:server")
	s.secretsBackend.EXPECT().GetSecret(uri).Return(&coresecrets.SecretMetadata{
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
