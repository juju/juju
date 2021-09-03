// Copyright 2021 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package secretsmanager_test

import (
	"context"
	"time"

	"github.com/golang/mock/gomock"
	"github.com/juju/names/v4"
	"github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/apiserver/common"
	facademocks "github.com/juju/juju/apiserver/facade/mocks"
	"github.com/juju/juju/apiserver/facades/agent/secretsmanager"
	"github.com/juju/juju/apiserver/facades/agent/secretsmanager/mocks"
	"github.com/juju/juju/apiserver/params"
	coresecrets "github.com/juju/juju/core/secrets"
	corewatcher "github.com/juju/juju/core/watcher"
	"github.com/juju/juju/secrets"
	coretesting "github.com/juju/juju/testing"
)

type SecretsManagerSuite struct {
	testing.IsolationSuite

	authorizer *facademocks.MockAuthorizer
	resources  *facademocks.MockResources

	secretsService         *mocks.MockSecretsService
	secretsWatcherService  *mocks.MockSecretsWatcher
	secretsRotationWatcher *mocks.MockSecretsRotationWatcher
	accessSecret           common.GetAuthFunc

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

	s.secretsService = mocks.NewMockSecretsService(ctrl)
	s.secretsWatcherService = mocks.NewMockSecretsWatcher(ctrl)
	s.secretsRotationWatcher = mocks.NewMockSecretsRotationWatcher(ctrl)
	s.accessSecret = func() (common.AuthFunc, error) {
		return func(tag names.Tag) bool {
			return tag.Id() == "app"
		}, nil
	}
	s.expectAuthUnitAgent()

	var err error
	s.facade, err = secretsmanager.NewTestAPI(s.authorizer, s.resources, s.secretsService, s.secretsWatcherService, s.accessSecret)
	c.Assert(err, jc.ErrorIsNil)

	return ctrl
}

func (s *SecretsManagerSuite) expectAuthUnitAgent() {
	s.authorizer.EXPECT().AuthUnitAgent().Return(true)
}

func (s *SecretsManagerSuite) TestCreateSecrets(c *gc.C) {
	defer s.setup(c).Finish()

	p := secrets.CreateParams{
		Version:        secrets.Version,
		Type:           "blob",
		Path:           "app.password",
		RotateInterval: time.Hour,
		Params:         map[string]interface{}{"param": 1},
		Data:           map[string]string{"foo": "bar"},
	}
	URL := coresecrets.NewSimpleURL(1, "app.password")
	URL.ControllerUUID = coretesting.ControllerTag.Id()
	URL.ModelUUID = coretesting.ModelTag.Id()
	s.secretsService.EXPECT().CreateSecret(gomock.Any(), URL, p).DoAndReturn(
		func(_ context.Context, URL *coresecrets.URL, p secrets.CreateParams) (*coresecrets.SecretMetadata, error) {
			md := &coresecrets.SecretMetadata{
				URL:  URL,
				Path: "app.password",
			}
			return md, nil
		},
	)

	results, err := s.facade.CreateSecrets(params.CreateSecretArgs{
		Args: []params.CreateSecretArg{{
			Type:           "blob",
			Path:           "app.password",
			RotateInterval: time.Hour,
			Params:         map[string]interface{}{"param": 1},
			Data:           map[string]string{"foo": "bar"},
		}, {
			RotateInterval: -1 * time.Hour,
		}, {
			Data: nil,
		}},
	})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(results, jc.DeepEquals, params.StringResults{
		Results: []params.StringResult{{
			Result: URL.String(),
		}, {
			Error: &params.Error{Message: `rotate interval "-1h0m0s" not valid`},
		}, {
			Error: &params.Error{Message: `empty secret value not valid`},
		}},
	})
}

func (s *SecretsManagerSuite) TestUpdateSecrets(c *gc.C) {
	defer s.setup(c).Finish()

	p := secrets.UpdateParams{
		RotateInterval: time.Hour,
		Params:         map[string]interface{}{"param": 1},
		Data:           map[string]string{"foo": "bar"},
	}
	URL := coresecrets.NewSimpleURL(1, "app.password")
	expectURL := *URL
	expectURL.ControllerUUID = coretesting.ControllerTag.Id()
	expectURL.ModelUUID = coretesting.ModelTag.Id()
	s.secretsService.EXPECT().UpdateSecret(gomock.Any(), &expectURL, p).DoAndReturn(
		func(_ context.Context, URL *coresecrets.URL, p secrets.UpdateParams) (*coresecrets.SecretMetadata, error) {
			md := &coresecrets.SecretMetadata{
				URL:      URL,
				Path:     "app.password",
				Revision: 2,
			}
			return md, nil
		},
	)
	URL1 := *URL
	URL1.ControllerUUID = "deadbeef-1bad-500d-9000-4b1d0d061111"
	URL2 := *URL
	URL2.ControllerUUID = coretesting.ControllerTag.Id()
	URL2.ModelUUID = "deadbeef-1bad-500d-9000-4b1d0d061111"

	results, err := s.facade.UpdateSecrets(params.UpdateSecretArgs{
		Args: []params.UpdateSecretArg{{
			URL:            URL.String(),
			RotateInterval: time.Hour,
			Params:         map[string]interface{}{"param": 1},
			Data:           map[string]string{"foo": "bar"},
		}, {
			URL:            URL.String(),
			RotateInterval: -1 * time.Hour,
		}, {
			URL: URL.WithAttribute("password").String(),
		}, {
			URL: URL.WithRevision(2).String(),
		}, {
			URL: URL1.String(),
		}, {
			URL: URL2.String(),
		}},
	})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(results, jc.DeepEquals, params.StringResults{
		Results: []params.StringResult{{
			Result: expectURL.WithRevision(2).String(),
		}, {
			Error: &params.Error{Message: `either rotate interval or data must be specified`},
		}, {
			Error: &params.Error{Code: "not supported", Message: `updating a single secret attribute "password" not supported`},
		}, {
			Error: &params.Error{Code: "not supported", Message: `updating secret revision 2 not supported`},
		}, {
			Error: &params.Error{Code: "", Message: `secret URL with controller UUID "deadbeef-1bad-500d-9000-4b1d0d061111" not valid`},
		}, {
			Error: &params.Error{Code: "", Message: `secret URL with model UUID "deadbeef-1bad-500d-9000-4b1d0d061111" not valid`},
		}},
	})
}

func (s *SecretsManagerSuite) TestGetSecretValues(c *gc.C) {
	defer s.setup(c).Finish()

	data := map[string]string{"foo": "bar"}
	val := coresecrets.NewSecretValue(data)
	URL, _ := coresecrets.ParseURL("secret://v1/app.password")
	URL.ControllerUUID = coretesting.ControllerTag.Id()
	URL.ModelUUID = coretesting.ModelTag.Id()
	s.secretsService.EXPECT().GetSecretValue(gomock.Any(), URL).Return(
		val, nil,
	)

	results, err := s.facade.GetSecretValues(params.GetSecretArgs{
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

	data := map[string]string{"foo": "bar"}
	val := coresecrets.NewSecretValue(data)
	URL, _ := coresecrets.ParseURL("secret://v1/deadbeef-1bad-500d-9000-4b1d0d061111/deadbeef-1bad-500d-9000-4b1d0d062222/app.password")
	URL.ControllerUUID = "deadbeef-1bad-500d-9000-4b1d0d061111"
	URL.ModelUUID = "deadbeef-1bad-500d-9000-4b1d0d062222"
	s.secretsService.EXPECT().GetSecretValue(gomock.Any(), URL).Return(
		val, nil,
	)

	results, err := s.facade.GetSecretValues(params.GetSecretArgs{
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

func (s *SecretsManagerSuite) TestWatchSecretsRotationChanges(c *gc.C) {
	defer s.setup(c).Finish()

	s.secretsWatcherService.EXPECT().WatchSecretsRotationChanges("application-app").Return(
		s.secretsRotationWatcher,
	)
	s.resources.EXPECT().Register(s.secretsRotationWatcher).Return("1")

	rotateChan := make(chan []corewatcher.SecretRotationChange, 1)
	rotateChan <- []corewatcher.SecretRotationChange{{
		ID:             666,
		URL:            coresecrets.NewSimpleURL(1, "app.password"),
		RotateInterval: time.Hour,
		LastRotateTime: time.Time{},
	}}
	s.secretsRotationWatcher.EXPECT().Changes().Return(rotateChan)

	result, err := s.facade.WatchSecretsRotationChanges(params.Entities{
		Entities: []params.Entity{{
			Tag: "application-app",
		}, {
			Tag: "application-foo",
		}},
	})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(result, jc.DeepEquals, params.SecretRotationWatchResults{
		Results: []params.SecretRotationWatchResult{{
			SecretRotationWatcherId: "1",
			Changes: []params.SecretRotationChange{{
				ID:             666,
				URL:            "secret://v1/app.password",
				RotateInterval: time.Hour,
				LastRotateTime: time.Time{},
			}},
		}, {
			Error: &params.Error{Code: "unauthorized access", Message: "permission denied"},
		}},
	})
}
