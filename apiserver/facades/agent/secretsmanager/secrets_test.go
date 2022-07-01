// Copyright 2021 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package secretsmanager_test

import (
	"context"
	"errors"
	"time"

	"github.com/golang/mock/gomock"
	"github.com/juju/names/v4"
	"github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/v3/apiserver/common"
	facademocks "github.com/juju/juju/v3/apiserver/facade/mocks"
	"github.com/juju/juju/v3/apiserver/facades/agent/secretsmanager"
	"github.com/juju/juju/v3/apiserver/facades/agent/secretsmanager/mocks"
	coresecrets "github.com/juju/juju/v3/core/secrets"
	corewatcher "github.com/juju/juju/v3/core/watcher"
	"github.com/juju/juju/v3/rpc/params"
	"github.com/juju/juju/v3/secrets"
	coretesting "github.com/juju/juju/v3/testing"
)

type SecretsManagerSuite struct {
	testing.IsolationSuite

	authorizer *facademocks.MockAuthorizer
	resources  *facademocks.MockResources

	secretsService         *mocks.MockSecretsService
	secretsRotationService *mocks.MockSecretsRotation
	secretsRotationWatcher *mocks.MockSecretsRotationWatcher
	accessSecret           common.GetAuthFunc
	ownerTag               names.Tag

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
	s.secretsRotationService = mocks.NewMockSecretsRotation(ctrl)
	s.secretsRotationWatcher = mocks.NewMockSecretsRotationWatcher(ctrl)
	s.ownerTag = names.NewApplicationTag("mariadb")
	s.accessSecret = func() (common.AuthFunc, error) {
		return func(tag names.Tag) bool {
			return tag.Id() == s.ownerTag.Id()
		}, nil
	}
	s.expectAuthUnitAgent()

	var err error
	s.facade, err = secretsmanager.NewTestAPI(s.authorizer, s.resources, s.secretsService, s.secretsRotationService, s.accessSecret, s.ownerTag)
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
		Owner:          "application-mariadb",
		Path:           "app/mariadb/password",
		RotateInterval: time.Hour,
		Status:         coresecrets.StatusActive,
		Description:    "my secret",
		Tags:           map[string]string{"hello": "world"},
		Params:         map[string]interface{}{"param": 1},
		Data:           map[string]string{"foo": "bar"},
	}
	URL := coresecrets.NewSimpleURL("app/mariadb/password")
	URL.ControllerUUID = coretesting.ControllerTag.Id()
	URL.ModelUUID = coretesting.ModelTag.Id()
	s.secretsService.EXPECT().CreateSecret(gomock.Any(), URL, p).DoAndReturn(
		func(_ context.Context, URL *coresecrets.URL, p secrets.CreateParams) (*coresecrets.SecretMetadata, error) {
			md := &coresecrets.SecretMetadata{
				URL:      URL,
				Path:     "app/mariadb/password",
				Revision: 1,
			}
			return md, nil
		},
	)

	results, err := s.facade.CreateSecrets(params.CreateSecretArgs{
		Args: []params.CreateSecretArg{{
			Type:           "blob",
			Path:           "app/mariadb/password",
			RotateInterval: time.Hour,
			Status:         "active",
			Description:    "my secret",
			Tags:           map[string]string{"hello": "world"},
			Params:         map[string]interface{}{"param": 1},
			Data:           map[string]string{"foo": "bar"},
		}, {
			Status:         "active",
			RotateInterval: -1 * time.Hour,
		}, {
			Status: "active",
			Data:   nil,
		}},
	})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(results, jc.DeepEquals, params.StringResults{
		Results: []params.StringResult{{
			Result: URL.WithRevision(1).ShortString(),
		}, {
			Error: &params.Error{Message: `rotate interval "-1h0m0s" not valid`, Code: params.CodeNotValid},
		}, {
			Error: &params.Error{Message: `empty secret value not valid`, Code: params.CodeNotValid},
		}},
	})
}

func (s *SecretsManagerSuite) TestUpdateSecrets(c *gc.C) {
	defer s.setup(c).Finish()

	rotate := time.Hour
	status := coresecrets.StatusActive
	description := "my secret"
	tags := map[string]string{"hello": "world"}
	p := secrets.UpdateParams{
		RotateInterval: &rotate,
		Status:         &status,
		Description:    &description,
		Tags:           &tags,
		Params:         map[string]interface{}{"param": 1},
		Data:           map[string]string{"foo": "bar"},
	}
	URL := coresecrets.NewSimpleURL("app/password")
	expectURL := *URL
	expectURL.ControllerUUID = coretesting.ControllerTag.Id()
	expectURL.ModelUUID = coretesting.ModelTag.Id()
	s.secretsService.EXPECT().UpdateSecret(gomock.Any(), &expectURL, p).DoAndReturn(
		func(_ context.Context, URL *coresecrets.URL, p secrets.UpdateParams) (*coresecrets.SecretMetadata, error) {
			md := &coresecrets.SecretMetadata{
				URL:      URL,
				Path:     "app/password",
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
	badRotate := -1 * time.Hour
	statusArg := string(status)

	results, err := s.facade.UpdateSecrets(params.UpdateSecretArgs{
		Args: []params.UpdateSecretArg{{
			URL:            URL.String(),
			RotateInterval: &rotate,
			Status:         &statusArg,
			Description:    &description,
			Tags:           &tags,
			Params:         map[string]interface{}{"param": 1},
			Data:           map[string]string{"foo": "bar"},
		}, {
			URL: URL.String(),
		}, {
			URL:            URL.String(),
			RotateInterval: &badRotate,
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
			Result: expectURL.WithRevision(2).ShortString(),
		}, {
			Error: &params.Error{Message: `at least one attribute to update must be specified`},
		}, {
			Error: &params.Error{Code: params.CodeNotValid, Message: `rotate interval -1h0m0s not valid`},
		}, {
			Error: &params.Error{Code: "not supported", Message: `updating a single secret attribute "password" not supported`},
		}, {
			Error: &params.Error{Code: "not supported", Message: `updating secret revision 2 not supported`},
		}, {
			Error: &params.Error{Code: params.CodeNotValid, Message: `secret URL with controller UUID "deadbeef-1bad-500d-9000-4b1d0d061111" not valid`},
		}, {
			Error: &params.Error{Code: params.CodeNotValid, Message: `secret URL with model UUID "deadbeef-1bad-500d-9000-4b1d0d061111" not valid`},
		}},
	})
}

func (s *SecretsManagerSuite) TestGetSecretValues(c *gc.C) {
	defer s.setup(c).Finish()

	data := map[string]string{"foo": "bar"}
	val := coresecrets.NewSecretValue(data)
	URL, _ := coresecrets.ParseURL("secret://app/mariadb/password")
	URL.ControllerUUID = coretesting.ControllerTag.Id()
	URL.ModelUUID = coretesting.ModelTag.Id()
	s.secretsService.EXPECT().GetSecretValue(gomock.Any(), URL).Return(
		val, nil,
	)

	results, err := s.facade.GetSecretValues(params.GetSecretArgs{
		Args: []params.GetSecretArg{{
			URL: URL.String(),
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
	URL, _ := coresecrets.ParseURL("secret://deadbeef-1bad-500d-9000-4b1d0d061111/deadbeef-1bad-500d-9000-4b1d0d062222/app/mariadb/password")
	URL.ControllerUUID = "deadbeef-1bad-500d-9000-4b1d0d061111"
	URL.ModelUUID = "deadbeef-1bad-500d-9000-4b1d0d062222"
	s.secretsService.EXPECT().GetSecretValue(gomock.Any(), URL).Return(
		val, nil,
	)

	results, err := s.facade.GetSecretValues(params.GetSecretArgs{
		Args: []params.GetSecretArg{{
			URL: URL.String(),
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

	s.secretsRotationService.EXPECT().WatchSecretsRotationChanges("application-mariadb").Return(
		s.secretsRotationWatcher,
	)
	s.resources.EXPECT().Register(s.secretsRotationWatcher).Return("1")

	rotateChan := make(chan []corewatcher.SecretRotationChange, 1)
	rotateChan <- []corewatcher.SecretRotationChange{{
		ID:             666,
		URL:            coresecrets.NewSimpleURL("app/mariadb/password"),
		RotateInterval: time.Hour,
		LastRotateTime: time.Time{},
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
	c.Assert(result, jc.DeepEquals, params.SecretRotationWatchResults{
		Results: []params.SecretRotationWatchResult{{
			SecretRotationWatcherId: "1",
			Changes: []params.SecretRotationChange{{
				ID:             666,
				URL:            "secret://app/mariadb/password",
				RotateInterval: time.Hour,
				LastRotateTime: time.Time{},
			}},
		}, {
			Error: &params.Error{Code: "unauthorized access", Message: "permission denied"},
		}},
	})
}

func (s *SecretsManagerSuite) TestSecretsRotated(c *gc.C) {
	defer s.setup(c).Finish()

	URL, _ := coresecrets.ParseURL("secret://app/mariadb/password")
	URL.ControllerUUID = coretesting.ControllerTag.Id()
	URL.ModelUUID = coretesting.ModelTag.Id()
	now := time.Now()
	s.secretsRotationService.EXPECT().SecretRotated(URL, now).Return(errors.New("boom"))

	result, err := s.facade.SecretsRotated(params.SecretRotatedArgs{
		Args: []params.SecretRotatedArg{{
			URL:  "secret://app/mariadb/password",
			When: now,
		}, {
			URL: "bad",
		}},
	})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(result, jc.DeepEquals, params.ErrorResults{
		Results: []params.ErrorResult{
			{
				Error: &params.Error{Code: "", Message: `boom`},
			},
			{
				Error: &params.Error{Code: params.CodeNotValid, Message: `secret URL scheme "" not valid`},
			},
		},
	})
}
