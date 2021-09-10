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
	"github.com/juju/juju/apiserver/params"
	"github.com/juju/juju/core/permission"
	coresecrets "github.com/juju/juju/core/secrets"
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

func (s *SecretsSuite) TestListSecretsShow(c *gc.C) {
	s.assertListSecrets(c, true)
}

func (s *SecretsSuite) assertListSecrets(c *gc.C, show bool) {
	defer s.setup(c).Finish()

	s.expectAuthClient()
	if show {
		s.authorizer.EXPECT().HasPermission(permission.SuperuserAccess, coretesting.ControllerTag).Return(
			true, nil)
	} else {
		s.authorizer.EXPECT().HasPermission(permission.ReadAccess, coretesting.ModelTag).Return(
			true, nil)
	}

	facade, err := apisecrets.NewTestAPI(s.secretsService, s.authorizer)
	c.Assert(err, jc.ErrorIsNil)

	now := time.Now()
	URL := &coresecrets.URL{
		ControllerUUID: coretesting.ControllerTag.Id(),
		ModelUUID:      coretesting.ModelTag.Id(),
		Path:           "app/password",
	}
	metadata := []*coresecrets.SecretMetadata{{
		URL:            URL,
		Path:           "app/password",
		RotateInterval: time.Hour,
		Version:        1,
		Status:         coresecrets.StatusActive,
		Description:    "shhh",
		Tags:           map[string]string{"foo": "bar"},
		ID:             666,
		Provider:       "juju",
		ProviderID:     "abcd",
		Revision:       2,
		CreateTime:     now,
		UpdateTime:     now.Add(time.Second),
	}}
	s.secretsService.EXPECT().ListSecrets(gomock.Any(), secrets.Filter{}).Return(
		metadata, nil,
	)

	var valueResult *params.SecretValueResult
	if show {
		valueResult = &params.SecretValueResult{
			Data: map[string]string{"foo": "bar"},
		}
		s.secretsService.EXPECT().GetSecretValue(gomock.Any(), URL).Return(
			coresecrets.NewSecretValue(valueResult.Data), nil,
		)
	}

	results, err := facade.ListSecrets(params.ListSecretsArgs{ShowSecrets: show})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(results, jc.DeepEquals, params.ListSecretResults{
		Results: []params.ListSecretResult{{
			URL:            URL.String(),
			Path:           "app/password",
			RotateInterval: time.Hour,
			Version:        1,
			Status:         "active",
			Description:    "shhh",
			Tags:           map[string]string{"foo": "bar"},
			ID:             666,
			Provider:       "juju",
			ProviderID:     "abcd",
			Revision:       2,
			CreateTime:     now,
			UpdateTime:     now.Add(time.Second),
			Value:          valueResult,
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
