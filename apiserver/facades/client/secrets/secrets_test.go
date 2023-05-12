// Copyright 2021 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package secrets_test

import (
	"time"

	"github.com/golang/mock/gomock"
	"github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	facademocks "github.com/juju/juju/apiserver/facade/mocks"
	apisecrets "github.com/juju/juju/apiserver/facades/client/secrets"
	"github.com/juju/juju/apiserver/facades/client/secrets/mocks"
	"github.com/juju/juju/core/permission"
	coresecrets "github.com/juju/juju/core/secrets"
	"github.com/juju/juju/rpc/params"
	"github.com/juju/juju/secrets/provider"
	"github.com/juju/juju/state"
	coretesting "github.com/juju/juju/testing"
)

type SecretsSuite struct {
	testing.IsolationSuite

	authorizer     *facademocks.MockAuthorizer
	secretsState   *mocks.MockSecretsState
	secretsBackend *mocks.MockSecretsBackend
}

var _ = gc.Suite(&SecretsSuite{})

func (s *SecretsSuite) SetUpTest(c *gc.C) {
	s.IsolationSuite.SetUpTest(c)
}

func (s *SecretsSuite) setup(c *gc.C) *gomock.Controller {
	ctrl := gomock.NewController(c)

	s.authorizer = facademocks.NewMockAuthorizer(ctrl)
	s.secretsState = mocks.NewMockSecretsState(ctrl)
	s.secretsBackend = mocks.NewMockSecretsBackend(ctrl)

	return ctrl
}

func (s *SecretsSuite) expectAuthClient() {
	s.authorizer.EXPECT().AuthClient().Return(true)
}

func (s *SecretsSuite) TestListSecrets(c *gc.C) {
	s.assertListSecrets(c, false, false)
}

func (s *SecretsSuite) TestListSecretsReveal(c *gc.C) {
	s.assertListSecrets(c, true, false)
}

func (s *SecretsSuite) TestListSecretsRevealFromBackend(c *gc.C) {
	s.assertListSecrets(c, true, true)
}

func ptr[T any](v T) *T {
	return &v
}

func (s *SecretsSuite) assertListSecrets(c *gc.C, reveal, withBackend bool) {
	defer s.setup(c).Finish()

	s.expectAuthClient()
	if reveal {
		s.authorizer.EXPECT().HasPermission(permission.SuperuserAccess, coretesting.ControllerTag).Return(
			true, nil)
	} else {
		s.authorizer.EXPECT().HasPermission(permission.ReadAccess, coretesting.ModelTag).Return(
			true, nil)
	}

	facade, err := apisecrets.NewTestAPI(s.secretsState,
		func() (*provider.ModelBackendConfigInfo, error) {
			return &provider.ModelBackendConfigInfo{
				ActiveID: "backend-id",
				Configs: map[string]provider.ModelBackendConfig{
					"backend-id": {
						ControllerUUID: coretesting.ControllerTag.Id(),
						ModelUUID:      coretesting.ModelTag.Id(),
						ModelName:      "some-model",
						BackendConfig: provider.BackendConfig{
							BackendType: "active-type",
							Config:      map[string]interface{}{"foo": "active-type"},
						},
					},
					"other-backend-id": {
						ControllerUUID: coretesting.ControllerTag.Id(),
						ModelUUID:      coretesting.ModelTag.Id(),
						ModelName:      "some-model",
						BackendConfig: provider.BackendConfig{
							BackendType: "other-type",
							Config:      map[string]interface{}{"foo": "other-type"},
						},
					},
				},
			}, nil
		},
		func(cfg *provider.ModelBackendConfig) (provider.SecretsBackend, error) {
			c.Assert(cfg.Config, jc.DeepEquals, provider.ConfigAttrs{"foo": cfg.BackendType})
			return s.secretsBackend, nil
		}, s.authorizer)
	c.Assert(err, jc.ErrorIsNil)

	now := time.Now()
	uri := coresecrets.NewURI()
	metadata := []*coresecrets.SecretMetadata{{
		URI:              uri,
		Version:          1,
		OwnerTag:         "application-mysql",
		RotatePolicy:     coresecrets.RotateHourly,
		LatestRevision:   2,
		LatestExpireTime: ptr(now),
		NextRotateTime:   ptr(now.Add(time.Hour)),
		Description:      "shhh",
		Label:            "foobar",
		CreateTime:       now,
		UpdateTime:       now.Add(time.Second),
	}}
	revisions := []*coresecrets.SecretRevisionMetadata{{
		Revision:   666,
		CreateTime: now,
		UpdateTime: now.Add(time.Second),
		ExpireTime: ptr(now.Add(time.Hour)),
	}, {
		Revision:    667,
		BackendName: ptr("some backend"),
		CreateTime:  now,
		UpdateTime:  now.Add(2 * time.Second),
		ExpireTime:  ptr(now.Add(2 * time.Hour)),
	}}
	s.secretsState.EXPECT().ListSecrets(state.SecretsFilter{}).Return(
		metadata, nil,
	)
	s.secretsState.EXPECT().ListSecretRevisions(uri).Return(
		revisions, nil,
	)

	var valueResult *params.SecretValueResult
	if reveal {
		valueResult = &params.SecretValueResult{
			Data: map[string]string{"foo": "bar"},
		}
		if withBackend {
			s.secretsState.EXPECT().GetSecretValue(uri, 2).Return(
				nil, &coresecrets.ValueRef{
					BackendID:  "backend-id",
					RevisionID: "rev-id",
				}, nil,
			)
			s.secretsBackend.EXPECT().GetContent(gomock.Any(), "rev-id").Return(
				coresecrets.NewSecretValue(valueResult.Data), nil,
			)
		} else {
			s.secretsState.EXPECT().GetSecretValue(uri, 2).Return(
				coresecrets.NewSecretValue(valueResult.Data), nil, nil,
			)
		}
	}

	results, err := facade.ListSecrets(params.ListSecretsArgs{ShowSecrets: reveal})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(results, jc.DeepEquals, params.ListSecretResults{
		Results: []params.ListSecretResult{{
			URI:              uri.String(),
			Version:          1,
			OwnerTag:         "application-mysql",
			RotatePolicy:     string(coresecrets.RotateHourly),
			LatestExpireTime: ptr(now),
			NextRotateTime:   ptr(now.Add(time.Hour)),
			Description:      "shhh",
			Label:            "foobar",
			LatestRevision:   2,
			CreateTime:       now,
			UpdateTime:       now.Add(time.Second),
			Value:            valueResult,
			Revisions: []params.SecretRevision{{
				Revision:    666,
				BackendName: ptr("internal"),
				CreateTime:  now,
				UpdateTime:  now.Add(time.Second),
				ExpireTime:  ptr(now.Add(time.Hour)),
			}, {
				Revision:    667,
				BackendName: ptr("some backend"),
				CreateTime:  now,
				UpdateTime:  now.Add(2 * time.Second),
				ExpireTime:  ptr(now.Add(2 * time.Hour)),
			}},
		}},
	})
}

func (s *SecretsSuite) TestListSecretsPermissionDenied(c *gc.C) {
	defer s.setup(c).Finish()

	s.expectAuthClient()
	s.authorizer.EXPECT().HasPermission(permission.ReadAccess, coretesting.ModelTag).Return(
		false, nil)

	facade, err := apisecrets.NewTestAPI(s.secretsState, nil, nil, s.authorizer)
	c.Assert(err, jc.ErrorIsNil)

	_, err = facade.ListSecrets(params.ListSecretsArgs{})
	c.Assert(err, gc.ErrorMatches, "permission denied")
}

func (s *SecretsSuite) TestListSecretsPermissionDeniedShow(c *gc.C) {
	defer s.setup(c).Finish()

	s.expectAuthClient()
	s.authorizer.EXPECT().HasPermission(permission.SuperuserAccess, coretesting.ControllerTag).Return(
		false, nil)
	s.authorizer.EXPECT().HasPermission(permission.AdminAccess, coretesting.ModelTag).Return(
		false, nil)

	facade, err := apisecrets.NewTestAPI(s.secretsState, nil, nil, s.authorizer)
	c.Assert(err, jc.ErrorIsNil)

	_, err = facade.ListSecrets(params.ListSecretsArgs{ShowSecrets: true})
	c.Assert(err, gc.ErrorMatches, "permission denied")
}
