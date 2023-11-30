// Copyright 2023 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package secrets_test

import (
	"time"

	"github.com/juju/errors"
	"github.com/juju/names/v4"
	"github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	"go.uber.org/mock/gomock"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/apiserver/common/secrets"
	"github.com/juju/juju/apiserver/common/secrets/mocks"
	facademocks "github.com/juju/juju/apiserver/facade/mocks"
	coresecrets "github.com/juju/juju/core/secrets"
	"github.com/juju/juju/environs/config"
	"github.com/juju/juju/rpc/params"
	"github.com/juju/juju/secrets/provider"
	"github.com/juju/juju/state"
	coretesting "github.com/juju/juju/testing"
)

type secretsDrainSuite struct {
	testing.IsolationSuite

	authorizer *facademocks.MockAuthorizer
	resources  *facademocks.MockResources

	provider                  *mocks.MockSecretBackendProvider
	leadership                *mocks.MockChecker
	token                     *mocks.MockToken
	secretsMetaState          *mocks.MockSecretsMetaState
	model                     *mocks.MockModel
	secretsConsumer           *mocks.MockSecretsConsumer
	modelConfigChangesWatcher *mocks.MockNotifyWatcher

	authTag names.Tag

	facade *secrets.SecretsDrainAPI
}

var _ = gc.Suite(&secretsDrainSuite{})

func (s *secretsDrainSuite) SetUpTest(c *gc.C) {
	s.IsolationSuite.SetUpTest(c)

	s.authTag = names.NewUnitTag("mariadb/0")
}

func (s *secretsDrainSuite) setup(c *gc.C) *gomock.Controller {
	ctrl := gomock.NewController(c)

	s.authorizer = facademocks.NewMockAuthorizer(ctrl)
	s.resources = facademocks.NewMockResources(ctrl)

	s.provider = mocks.NewMockSecretBackendProvider(ctrl)
	s.leadership = mocks.NewMockChecker(ctrl)
	s.token = mocks.NewMockToken(ctrl)
	s.secretsMetaState = mocks.NewMockSecretsMetaState(ctrl)
	s.model = mocks.NewMockModel(ctrl)
	s.secretsConsumer = mocks.NewMockSecretsConsumer(ctrl)
	s.modelConfigChangesWatcher = mocks.NewMockNotifyWatcher(ctrl)
	s.expectAuthUnitAgent()

	s.PatchValue(&secrets.GetProvider, func(string) (provider.SecretBackendProvider, error) { return s.provider, nil })

	var err error
	s.facade, err = secrets.NewSecretsDrainAPI(
		s.authTag,
		s.authorizer,
		s.resources,
		s.leadership,
		s.model,
		s.secretsMetaState,
		s.secretsConsumer,
	)
	c.Assert(err, jc.ErrorIsNil)
	return ctrl
}

func (s *secretsDrainSuite) expectAuthUnitAgent() {
	s.authorizer.EXPECT().AuthUnitAgent().Return(true)
}

func (s *secretsDrainSuite) expectSecretAccessQuery(n int) {
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

func (s *secretsDrainSuite) assertGetSecretsToDrain(
	c *gc.C, modelType state.ModelType, secretBackend string,
	expectedRevions ...params.SecretRevision,
) {
	defer s.setup(c).Finish()

	s.leadership.EXPECT().LeadershipCheck("mariadb", "mariadb/0").Return(s.token)
	s.token.EXPECT().Check().Return(nil)

	configAttrs := map[string]interface{}{
		"name":           "some-name",
		"type":           "some-type",
		"uuid":           coretesting.ModelTag.Id(),
		"secret-backend": secretBackend,
	}
	cfg, err := config.New(config.NoDefaults, configAttrs)
	c.Assert(err, jc.ErrorIsNil)
	s.model.EXPECT().ModelConfig().Return(cfg, nil)
	s.model.EXPECT().Type().Return(modelType)

	s.model.EXPECT().ControllerUUID().Return(coretesting.ControllerTag.Id())
	s.model.EXPECT().UUID().Return(coretesting.ModelTag.Id())

	now := time.Now()
	uri := coresecrets.NewURI()
	s.secretsMetaState.EXPECT().ListSecrets(
		state.SecretsFilter{
			OwnerTags: []names.Tag{names.NewUnitTag("mariadb/0"), names.NewApplicationTag("mariadb")},
		}).Return([]*coresecrets.SecretMetadata{{
		URI:              uri,
		OwnerTag:         "application-mariadb",
		Label:            "label",
		RotatePolicy:     coresecrets.RotateHourly,
		LatestRevision:   666,
		LatestExpireTime: &now,
		NextRotateTime:   &now,
	}}, nil)
	s.secretsMetaState.EXPECT().SecretGrants(uri, coresecrets.RoleView).Return([]coresecrets.AccessInfo{}, nil)
	s.secretsMetaState.EXPECT().ListSecretRevisions(uri).Return([]*coresecrets.SecretRevisionMetadata{
		{
			// External backend.
			Revision: 666,
			ValueRef: &coresecrets.ValueRef{
				BackendID:  "backend-id",
				RevisionID: "rev-666",
			},
		}, {
			// Internal backend.
			Revision: 667,
		},
		{
			// k8s backend.
			Revision: 668,
			ValueRef: &coresecrets.ValueRef{
				BackendID:  coretesting.ModelTag.Id(),
				RevisionID: "rev-668",
			},
		},
	}, nil)

	results, err := s.facade.GetSecretsToDrain()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(results, jc.DeepEquals, params.ListSecretResults{
		Results: []params.ListSecretResult{{
			URI:              uri.String(),
			OwnerTag:         "application-mariadb",
			Label:            "label",
			RotatePolicy:     coresecrets.RotateHourly.String(),
			LatestRevision:   666,
			LatestExpireTime: &now,
			NextRotateTime:   &now,
			Revisions:        expectedRevions,
		}},
	})
}

func (s *secretsDrainSuite) TestGetSecretsToDrainAutoIAAS(c *gc.C) {
	s.assertGetSecretsToDrain(c, state.ModelTypeIAAS, "auto",
		// External backend.
		params.SecretRevision{
			Revision: 666,
			ValueRef: &params.SecretValueRef{
				BackendID:  "backend-id",
				RevisionID: "rev-666",
			},
		},
		// k8s backend.
		params.SecretRevision{
			Revision: 668,
			ValueRef: &params.SecretValueRef{
				BackendID:  coretesting.ModelTag.Id(),
				RevisionID: "rev-668",
			},
		},
	)
}

func (s *secretsDrainSuite) TestGetSecretsToDrainAutoCAAS(c *gc.C) {
	s.assertGetSecretsToDrain(c, state.ModelTypeCAAS, "auto",
		// External backend.
		params.SecretRevision{
			Revision: 666,
			ValueRef: &params.SecretValueRef{
				BackendID:  "backend-id",
				RevisionID: "rev-666",
			},
		},
		// Internal backend.
		params.SecretRevision{
			Revision: 667,
		},
	)
}

func (s *secretsDrainSuite) TestGetSecretsToDrainInternal(c *gc.C) {
	s.assertGetSecretsToDrain(c, state.ModelTypeIAAS, provider.Internal,
		// External backend.
		params.SecretRevision{
			Revision: 666,
			ValueRef: &params.SecretValueRef{
				BackendID:  "backend-id",
				RevisionID: "rev-666",
			},
		},
		// k8s backend.
		params.SecretRevision{
			Revision: 668,
			ValueRef: &params.SecretValueRef{
				BackendID:  coretesting.ModelTag.Id(),
				RevisionID: "rev-668",
			},
		},
	)
}

func (s *secretsDrainSuite) TestGetSecretsToDrainExternalIAAS(c *gc.C) {
	s.assertGetSecretsToDrain(c, state.ModelTypeIAAS, "backend-id",
		// Internal backend.
		params.SecretRevision{
			Revision: 667,
		},
		// k8s backend.
		params.SecretRevision{
			Revision: 668,
			ValueRef: &params.SecretValueRef{
				BackendID:  coretesting.ModelTag.Id(),
				RevisionID: "rev-668",
			},
		},
	)
}

func (s *secretsDrainSuite) TestGetSecretsToDrainExternalCAAS(c *gc.C) {
	s.assertGetSecretsToDrain(c, state.ModelTypeIAAS, "backend-id",
		// Internal backend.
		params.SecretRevision{
			Revision: 667,
		},
		// k8s backend.
		params.SecretRevision{
			Revision: 668,
			ValueRef: &params.SecretValueRef{
				BackendID:  coretesting.ModelTag.Id(),
				RevisionID: "rev-668",
			},
		},
	)
}

func (s *secretsDrainSuite) TestChangeSecretBackend(c *gc.C) {
	defer s.setup(c).Finish()

	s.expectSecretAccessQuery(4)
	uri1 := coresecrets.NewURI()
	uri2 := coresecrets.NewURI()
	s.secretsMetaState.EXPECT().ChangeSecretBackend(
		state.ChangeSecretBackendParams{
			Token:    s.token,
			URI:      uri1,
			Revision: 666,
			ValueRef: &coresecrets.ValueRef{
				BackendID:  "backend-id",
				RevisionID: "rev-666",
			},
		},
	).Return(nil)
	s.secretsMetaState.EXPECT().ChangeSecretBackend(
		state.ChangeSecretBackendParams{
			Token:    s.token,
			URI:      uri2,
			Revision: 888,
			Data:     map[string]string{"foo": "bar"},
		},
	).Return(nil)
	s.leadership.EXPECT().LeadershipCheck("mariadb", "mariadb/0").Return(s.token).Times(2)
	s.token.EXPECT().Check().Return(nil).Times(2)

	result, err := s.facade.ChangeSecretBackend(params.ChangeSecretBackendArgs{
		Args: []params.ChangeSecretBackendArg{
			{
				URI:      uri1.String(),
				Revision: 666,
				Content: params.SecretContentParams{
					// Change to external backend.
					ValueRef: &params.SecretValueRef{
						BackendID:  "backend-id",
						RevisionID: "rev-666",
					},
				},
			},
			{
				URI:      uri2.String(),
				Revision: 888,
				Content: params.SecretContentParams{
					// Change to internal backend.
					Data: map[string]string{"foo": "bar"},
				},
			},
		},
	})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(result, jc.DeepEquals, params.ErrorResults{
		Results: []params.ErrorResult{{Error: nil}, {Error: nil}},
	})
}

func (s *secretsDrainSuite) TestWatchSecretBackendChanged(c *gc.C) {
	defer s.setup(c).Finish()

	done := make(chan struct{})
	changeChan := make(chan struct{}, 1)
	changeChan <- struct{}{}
	s.modelConfigChangesWatcher.EXPECT().Wait().DoAndReturn(func() error {
		close(done)
		return nil
	})
	s.modelConfigChangesWatcher.EXPECT().Changes().Return(changeChan).AnyTimes()

	s.model.EXPECT().WatchForModelConfigChanges().Return(s.modelConfigChangesWatcher)
	configAttrs := map[string]interface{}{
		"name":           "some-name",
		"type":           "some-type",
		"uuid":           coretesting.ModelTag.Id(),
		"secret-backend": "backend-id",
	}
	cfg, err := config.New(config.NoDefaults, configAttrs)
	c.Assert(err, jc.ErrorIsNil)
	s.model.EXPECT().ModelConfig().Return(cfg, nil).Times(2)

	s.resources.EXPECT().Register(gomock.Any()).Return("11")

	result, err := s.facade.WatchSecretBackendChanged()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(result, jc.DeepEquals, params.NotifyWatchResult{
		NotifyWatcherId: "11",
	})

	select {
	case <-done:
		// We need to wait for the watcher to fully start to ensure that all expected methods are called.
	case <-time.After(coretesting.LongWait):
		c.Fatalf("timed out waiting for 2nd loop")
	}
}
