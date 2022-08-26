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
	"github.com/juju/juju/apiserver/facades/agent/secretsmanager/mocks"
	apisecrets "github.com/juju/juju/apiserver/facades/client/secrets"
	"github.com/juju/juju/core/permission"
	coresecrets "github.com/juju/juju/core/secrets"
	"github.com/juju/juju/rpc/params"
	"github.com/juju/juju/secrets"
	coretesting "github.com/juju/juju/testing"
)

type SecretsSuite struct {
	testing.IsolationSuite

	authorizer     *facademocks.MockAuthorizer
	secretsService *mocks.MockSecretsService
}

var _ = gc.Suite(&SecretsSuite{})

func (s *SecretsSuite) SetUpTest(c *gc.C) {
	s.IsolationSuite.SetUpTest(c)
}

func (s *SecretsSuite) setup(c *gc.C) *gomock.Controller {
	ctrl := gomock.NewController(c)

	s.authorizer = facademocks.NewMockAuthorizer(ctrl)
	s.secretsService = mocks.NewMockSecretsService(ctrl)

	return ctrl
}

func (s *SecretsSuite) expectAuthClient() {
	s.authorizer.EXPECT().AuthClient().Return(true)
}

func (s *SecretsSuite) TestListSecrets(c *gc.C) {
	s.assertListSecrets(c, false)
}

func (s *SecretsSuite) TestListSecretsReveal(c *gc.C) {
	s.assertListSecrets(c, true)
}

func ptr[T any](v T) *T {
	return &v
}

func (s *SecretsSuite) assertListSecrets(c *gc.C, reveal bool) {
	defer s.setup(c).Finish()

	s.expectAuthClient()
	if reveal {
		s.authorizer.EXPECT().HasPermission(permission.SuperuserAccess, coretesting.ControllerTag).Return(
			true, nil)
	} else {
		s.authorizer.EXPECT().HasPermission(permission.ReadAccess, coretesting.ModelTag).Return(
			true, nil)
	}

	facade, err := apisecrets.NewTestAPI(s.secretsService, s.authorizer)
	c.Assert(err, jc.ErrorIsNil)

	now := time.Now()
	uri := coresecrets.NewURI()
	uri.ControllerUUID = coretesting.ControllerTag.Id()
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
		ProviderID:       "abcd",
		CreateTime:       now,
		UpdateTime:       now.Add(time.Second),
	}}
	revisions := map[string][]*coresecrets.SecretRevisionMetadata{
		uri.ID: {{
			Revision:   666,
			CreateTime: now,
			UpdateTime: now.Add(time.Second),
			ExpireTime: ptr(now.Add(time.Hour)),
		}, {
			Revision:   667,
			CreateTime: now,
			UpdateTime: now.Add(2 * time.Second),
			ExpireTime: ptr(now.Add(2 * time.Hour)),
		}},
	}
	s.secretsService.EXPECT().ListSecrets(gomock.Any(), secrets.Filter{}).Return(
		metadata, revisions, nil,
	)

	var valueResult *params.SecretValueResult
	if reveal {
		valueResult = &params.SecretValueResult{
			Data: map[string]string{"foo": "bar"},
		}
		s.secretsService.EXPECT().GetSecretValue(gomock.Any(), uri, 2).Return(
			coresecrets.NewSecretValue(valueResult.Data), nil,
		)
	}

	results, err := facade.ListSecrets(params.ListSecretsArgs{ShowSecrets: reveal})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(results, jc.DeepEquals, params.ListSecretResults{
		Results: []params.ListSecretResult{{
			URI:              uri.String(),
			Version:          1,
			OwnerTag:         "application-mysql",
			ProviderID:       "abcd",
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
				Revision:   666,
				CreateTime: now,
				UpdateTime: now.Add(time.Second),
				ExpireTime: ptr(now.Add(time.Hour)),
			}, {
				Revision:   667,
				CreateTime: now,
				UpdateTime: now.Add(2 * time.Second),
				ExpireTime: ptr(now.Add(2 * time.Hour)),
			}},
		}},
	})
}

func (s *SecretsSuite) TestListSecretsPermissionDenied(c *gc.C) {
	defer s.setup(c).Finish()

	s.expectAuthClient()
	s.authorizer.EXPECT().HasPermission(permission.ReadAccess, coretesting.ModelTag).Return(
		false, nil)

	facade, err := apisecrets.NewTestAPI(s.secretsService, s.authorizer)
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

	facade, err := apisecrets.NewTestAPI(s.secretsService, s.authorizer)
	c.Assert(err, jc.ErrorIsNil)

	_, err = facade.ListSecrets(params.ListSecretsArgs{ShowSecrets: true})
	c.Assert(err, gc.ErrorMatches, "permission denied")
}
