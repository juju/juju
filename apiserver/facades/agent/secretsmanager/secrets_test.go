// Copyright 2021 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package secretsmanager_test

import (
	"context"
	"time"

	"github.com/golang/mock/gomock"
	"github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	facademocks "github.com/juju/juju/apiserver/facade/mocks"
	"github.com/juju/juju/apiserver/facades/agent/secretsmanager"
	"github.com/juju/juju/apiserver/facades/agent/secretsmanager/mocks"
	"github.com/juju/juju/apiserver/params"
	coresecrets "github.com/juju/juju/core/secrets"
	"github.com/juju/juju/secrets"
	coretesting "github.com/juju/juju/testing"
)

type SecretsManagerSuite struct {
	testing.IsolationSuite

	authorizer     *facademocks.MockAuthorizer
	secretsService *mocks.MockSecretsService
}

var _ = gc.Suite(&SecretsManagerSuite{})

func (s *SecretsManagerSuite) SetUpTest(c *gc.C) {
	s.IsolationSuite.SetUpTest(c)
}

func (s *SecretsManagerSuite) setup(c *gc.C) *gomock.Controller {
	ctrl := gomock.NewController(c)

	s.authorizer = facademocks.NewMockAuthorizer(ctrl)
	s.secretsService = mocks.NewMockSecretsService(ctrl)

	return ctrl
}

func (s *SecretsManagerSuite) expectAuthUnitAgent() {
	s.authorizer.EXPECT().AuthUnitAgent().Return(true)
}

func (s *SecretsManagerSuite) TestCreateSecrets(c *gc.C) {
	defer s.setup(c).Finish()

	s.expectAuthUnitAgent()
	facade, err := secretsmanager.NewTestAPI(s.secretsService, s.authorizer)
	c.Assert(err, jc.ErrorIsNil)

	p := secrets.CreateParams{
		ControllerUUID: coretesting.ControllerTag.Id(),
		ModelUUID:      coretesting.ModelTag.Id(),
		Version:        secrets.Version,
		Type:           "blob",
		Path:           "app.password",
		RotateDuration: time.Hour,
		Params:         map[string]interface{}{"param": 1},
		Data:           map[string]string{"foo": "bar"},
	}
	URL, _ := coresecrets.ParseURL("secret://v1/app.password")
	s.secretsService.EXPECT().CreateSecret(gomock.Any(), p).DoAndReturn(
		func(_ context.Context, p secrets.CreateParams) (*coresecrets.SecretMetadata, error) {
			md := &coresecrets.SecretMetadata{
				URL:  URL,
				Path: "app.password",
			}
			return md, nil
		},
	)

	results, err := facade.CreateSecrets(params.CreateSecretArgs{
		Args: []params.CreateSecretArg{{
			Type:           "blob",
			Path:           "app.password",
			RotateDuration: time.Hour,
			Params:         map[string]interface{}{"param": 1},
			Data:           map[string]string{"foo": "bar"},
		}},
	})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(results, jc.DeepEquals, params.StringResults{
		Results: []params.StringResult{{
			Result: "secret://v1/app.password",
		}},
	})
}

func (s *SecretsManagerSuite) TestGetSecretValues(c *gc.C) {
	defer s.setup(c).Finish()

	s.expectAuthUnitAgent()
	facade, err := secretsmanager.NewTestAPI(s.secretsService, s.authorizer)
	c.Assert(err, jc.ErrorIsNil)

	data := map[string]string{"foo": "bar"}
	val := coresecrets.NewSecretValue(data)
	URL, _ := coresecrets.ParseURL("secret://v1/app.password")
	URL.ControllerUUID = coretesting.ControllerTag.Id()
	URL.ModelUUID = coretesting.ModelTag.Id()
	s.secretsService.EXPECT().GetSecretValue(gomock.Any(), URL).Return(
		val, nil,
	)

	results, err := facade.GetSecretValues(params.GetSecretArgs{
		Args: []params.GetSecretArg{{
			ID: URL.String(),
		}},
	})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(results, jc.DeepEquals, params.SecretValueResults{
		Results: []params.SecretValueResult{{
			Data: data,
		}},
	})
}

func (s *SecretsManagerSuite) TestGetSecretValuesExplicitUUIDs(c *gc.C) {
	defer s.setup(c).Finish()

	s.expectAuthUnitAgent()
	facade, err := secretsmanager.NewTestAPI(s.secretsService, s.authorizer)
	c.Assert(err, jc.ErrorIsNil)

	data := map[string]string{"foo": "bar"}
	val := coresecrets.NewSecretValue(data)
	URL, _ := coresecrets.ParseURL("secret://v1/deadbeef-1bad-500d-9000-4b1d0d061111/deadbeef-1bad-500d-9000-4b1d0d062222/app.password")
	URL.ControllerUUID = "deadbeef-1bad-500d-9000-4b1d0d061111"
	URL.ModelUUID = "deadbeef-1bad-500d-9000-4b1d0d062222"
	s.secretsService.EXPECT().GetSecretValue(gomock.Any(), URL).Return(
		val, nil,
	)

	results, err := facade.GetSecretValues(params.GetSecretArgs{
		Args: []params.GetSecretArg{{
			ID: URL.String(),
		}},
	})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(results, jc.DeepEquals, params.SecretValueResults{
		Results: []params.SecretValueResult{{
			Data: data,
		}},
	})
}
