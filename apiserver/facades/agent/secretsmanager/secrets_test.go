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

	"github.com/juju/juju/apiserver/common"
	facademocks "github.com/juju/juju/apiserver/facade/mocks"
	"github.com/juju/juju/apiserver/facades/agent/secretsmanager"
	"github.com/juju/juju/apiserver/facades/agent/secretsmanager/mocks"
	coresecrets "github.com/juju/juju/core/secrets"
	corewatcher "github.com/juju/juju/core/watcher"
	"github.com/juju/juju/rpc/params"
	"github.com/juju/juju/secrets"
	coretesting "github.com/juju/juju/testing"
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
		Owner:          "application-mariadb",
		RotateInterval: time.Hour,
		Description:    "my secret",
		Tags:           map[string]string{"hello": "world"},
		Params:         map[string]interface{}{"param": 1},
		Data:           map[string]string{"foo": "bar"},
	}
	var gotURI *coresecrets.URI
	s.secretsService.EXPECT().CreateSecret(gomock.Any(), gomock.Any(), p).DoAndReturn(
		func(_ context.Context, uri *coresecrets.URI, p secrets.CreateParams) (*coresecrets.SecretMetadata, error) {
			gotURI = uri
			md := &coresecrets.SecretMetadata{
				URI:      uri,
				Revision: 1,
			}
			return md, nil
		},
	)

	results, err := s.facade.CreateSecrets(params.CreateSecretArgs{
		Args: []params.CreateSecretArg{{
			RotateInterval: time.Hour,
			Description:    "my secret",
			Tags:           map[string]string{"hello": "world"},
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
			Result: gotURI.String(),
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
	description := "my secret"
	tags := map[string]string{"hello": "world"}
	p := secrets.UpdateParams{
		RotateInterval: &rotate,
		Description:    &description,
		Tags:           &tags,
		Params:         map[string]interface{}{"param": 1},
		Data:           map[string]string{"foo": "bar"},
	}
	uri := coresecrets.NewURI()
	expectURI := *uri
	expectURI.ControllerUUID = coretesting.ControllerTag.Id()
	s.secretsService.EXPECT().UpdateSecret(gomock.Any(), &expectURI, p).DoAndReturn(
		func(_ context.Context, uri *coresecrets.URI, p secrets.UpdateParams) (*coresecrets.SecretMetadata, error) {
			md := &coresecrets.SecretMetadata{
				URI:      uri,
				Revision: 2,
			}
			return md, nil
		},
	)
	uri1 := *uri
	uri1.ControllerUUID = "deadbeef-1bad-500d-9000-4b1d0d061111"
	badRotate := -1 * time.Hour

	results, err := s.facade.UpdateSecrets(params.UpdateSecretArgs{
		Args: []params.UpdateSecretArg{{
			URI:            uri.ShortString(),
			RotateInterval: &rotate,
			Description:    &description,
			Tags:           &tags,
			Params:         map[string]interface{}{"param": 1},
			Data:           map[string]string{"foo": "bar"},
		}, {
			URI: uri.String(),
		}, {
			URI:            uri.ShortString(),
			RotateInterval: &badRotate,
		}, {
			URI: uri1.String(),
		}},
	})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(results, jc.DeepEquals, params.ErrorResults{
		Results: []params.ErrorResult{{}, {
			Error: &params.Error{Message: `at least one attribute to update must be specified`},
		}, {
			Error: &params.Error{Code: params.CodeNotValid, Message: `rotate interval -1h0m0s not valid`},
		}, {
			Error: &params.Error{Code: params.CodeNotValid, Message: `secret URL with controller UUID "deadbeef-1bad-500d-9000-4b1d0d061111" not valid`},
		}},
	})
}

func (s *SecretsManagerSuite) TestGetSecretValues(c *gc.C) {
	defer s.setup(c).Finish()

	data := map[string]string{"foo": "bar"}
	val := coresecrets.NewSecretValue(data)
	uri := coresecrets.NewURI()
	uri.ControllerUUID = coretesting.ControllerTag.Id()
	s.secretsService.EXPECT().GetSecretValue(gomock.Any(), uri, 1).Return(
		val, nil,
	)

	results, err := s.facade.GetSecretValues(params.GetSecretArgs{
		Args: []params.GetSecretArg{{
			URI: uri.ShortString(),
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
	uri := coresecrets.NewURI()
	uri.ControllerUUID = "deadbeef-1bad-500d-9000-4b1d0d061111"
	s.secretsService.EXPECT().GetSecretValue(gomock.Any(), uri, 1).Return(
		val, nil,
	)

	results, err := s.facade.GetSecretValues(params.GetSecretArgs{
		Args: []params.GetSecretArg{{
			URI: uri.String(),
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

	uri := coresecrets.NewURI()
	rotateChan := make(chan []corewatcher.SecretRotationChange, 1)
	rotateChan <- []corewatcher.SecretRotationChange{{
		URI:            uri,
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
				URI:            uri.String(),
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

	uri := coresecrets.NewURI()
	uri.ControllerUUID = coretesting.ControllerTag.Id()
	now := time.Now()
	s.secretsRotationService.EXPECT().SecretRotated(uri, now).Return(errors.New("boom"))
	s.secretsService.EXPECT().GetSecret(gomock.Any(), uri).Return(&coresecrets.SecretMetadata{
		OwnerTag: "application-mariadb",
	}, nil)

	result, err := s.facade.SecretsRotated(params.SecretRotatedArgs{
		Args: []params.SecretRotatedArg{{
			URI:  uri.ShortString(),
			When: now,
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
