// Copyright 2021 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package juju_test

import (
	"context"
	"time"

	"github.com/golang/mock/gomock"
	"github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	coresecrets "github.com/juju/juju/core/secrets"
	"github.com/juju/juju/secrets"
	"github.com/juju/juju/secrets/provider/juju"
	"github.com/juju/juju/secrets/provider/juju/mocks"
	"github.com/juju/juju/state"
	coretesting "github.com/juju/juju/testing"
)

type SecretsManagerSuite struct {
	testing.IsolationSuite
	secretsStore *mocks.MockSecretsStore
}

var _ = gc.Suite(&SecretsManagerSuite{})

func (s *SecretsManagerSuite) SetUpTest(c *gc.C) {
	s.IsolationSuite.SetUpTest(c)
}

func (s *SecretsManagerSuite) setup(c *gc.C) *gomock.Controller {
	ctrl := gomock.NewController(c)
	s.secretsStore = mocks.NewMockSecretsStore(ctrl)
	return ctrl
}

func (s *SecretsManagerSuite) TestNewService(c *gc.C) {
	cfg := secrets.ProviderConfig{
		"juju-backend": &state.State{},
	}
	p, err := juju.NewSecretService(cfg)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(p, gc.NotNil)
}

func (s *SecretsManagerSuite) TestNewServiceInvalidBackend(c *gc.C) {
	cfg := secrets.ProviderConfig{
		"juju-backend": struct{}{},
	}
	_, err := juju.NewSecretService(cfg)
	c.Assert(err, gc.ErrorMatches, `Juju secret store config missing state backend`)
}

func (s *SecretsManagerSuite) TestCreateSecret(c *gc.C) {
	defer s.setup(c).Finish()

	service := juju.NewTestService(s.secretsStore)

	p := secrets.CreateParams{
		Version:        secrets.Version,
		ProviderLabel:  juju.Provider,
		Owner:          "application-app",
		RotateInterval: time.Hour,
		Description:    "my secret",
		Tags:           map[string]string{"hello": "world"},
		Params:         map[string]interface{}{"param": 1},
		Data:           map[string]string{"foo": "bar"},
	}
	expectedP := state.CreateSecretParams{
		Version:        p.Version,
		ProviderLabel:  "juju",
		Owner:          "application-app",
		RotateInterval: time.Hour,
		Description:    "my secret",
		Tags:           map[string]string{"hello": "world"},
		Params:         p.Params,
		Data:           p.Data,
	}
	uri := coresecrets.NewURI()
	now := time.Now()
	uri.ControllerUUID = coretesting.ControllerTag.Id()
	s.secretsStore.EXPECT().CreateSecret(uri, expectedP).DoAndReturn(
		func(uri *coresecrets.URI, p state.CreateSecretParams) (*coresecrets.SecretMetadata, error) {
			md := &coresecrets.SecretMetadata{
				URI:        uri,
				CreateTime: now,
			}
			return md, nil
		},
	)

	resultMeta, err := service.CreateSecret(context.Background(), uri, p)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(resultMeta, jc.DeepEquals, &coresecrets.SecretMetadata{
		URI:        uri,
		CreateTime: now,
	})
}

func (s *SecretsManagerSuite) TestUpdateSecret(c *gc.C) {
	defer s.setup(c).Finish()

	service := juju.NewTestService(s.secretsStore)

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
	expectedP := state.UpdateSecretParams{
		RotateInterval: &rotate,
		Description:    &description,
		Tags:           &tags,
		Params:         p.Params,
		Data:           p.Data,
	}
	uri, _ := coresecrets.ParseURI("secret:9m4e2mr0ui3e8a215n4g")
	now := time.Now()
	s.secretsStore.EXPECT().UpdateSecret(uri, expectedP).DoAndReturn(
		func(uri *coresecrets.URI, p state.UpdateSecretParams) (*coresecrets.SecretMetadata, error) {
			md := &coresecrets.SecretMetadata{
				URI:        uri,
				UpdateTime: now,
			}
			return md, nil
		},
	)

	resultMeta, err := service.UpdateSecret(context.Background(), uri, p)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(resultMeta, jc.DeepEquals, &coresecrets.SecretMetadata{
		URI:        uri,
		UpdateTime: now,
	})
}

func (s *SecretsManagerSuite) TestGetSecret(c *gc.C) {
	defer s.setup(c).Finish()

	service := juju.NewTestService(s.secretsStore)

	uri, _ := coresecrets.ParseURI("secret:9m4e2mr0ui3e8a215n4g")
	md := &coresecrets.SecretMetadata{
		URI:      uri,
		Revision: 2,
	}
	s.secretsStore.EXPECT().GetSecret(uri).Return(
		md, nil,
	)

	result, err := service.GetSecret(context.Background(), uri)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(result, jc.DeepEquals, md)
}

func (s *SecretsManagerSuite) TestGetSecretValue(c *gc.C) {
	defer s.setup(c).Finish()

	service := juju.NewTestService(s.secretsStore)

	uri, _ := coresecrets.ParseURI("secret:9m4e2mr0ui3e8a215n4g")
	data := map[string]string{"foo": "bar"}
	val := coresecrets.NewSecretValue(data)
	s.secretsStore.EXPECT().GetSecretValue(uri, 666).Return(
		val, nil,
	)

	result, err := service.GetSecretValue(context.Background(), uri, 666)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(result, jc.DeepEquals, val)
}

func (s *SecretsManagerSuite) TestListSecrets(c *gc.C) {
	defer s.setup(c).Finish()

	service := juju.NewTestService(s.secretsStore)

	uri, _ := coresecrets.ParseURI("secret:9m4e2mr0ui3e8a215n4g")
	metadata := []*coresecrets.SecretMetadata{{
		URI:      uri,
		Revision: 2,
	}}
	s.secretsStore.EXPECT().ListSecrets(state.SecretsFilter{}).Return(
		metadata, nil,
	)

	result, err := service.ListSecrets(context.Background(), secrets.Filter{})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(result, jc.DeepEquals, metadata)
}
