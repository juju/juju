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

func ptr[T any](v T) *T {
	return &v
}

func (s *SecretsManagerSuite) TestCreateSecret(c *gc.C) {
	defer s.setup(c).Finish()

	service := juju.NewTestService(s.secretsStore)

	now := time.Now()
	p := secrets.CreateParams{
		Version:       secrets.Version,
		ProviderLabel: juju.Provider,
		Owner:         "application-app",
		UpsertParams: secrets.UpsertParams{
			RotatePolicy:   ptr(coresecrets.RotateDaily),
			NextRotateTime: ptr(now.Add(time.Minute)),
			ExpireTime:     ptr(now.Add(time.Hour)),
			Description:    ptr("my secret"),
			Label:          ptr("foobar"),
			Params:         map[string]interface{}{"param": 1},
			Data:           map[string]string{"foo": "bar"},
		},
	}
	expectedP := state.CreateSecretParams{
		Version:       p.Version,
		ProviderLabel: "juju",
		Owner:         "application-app",
		UpdateSecretParams: state.UpdateSecretParams{
			RotatePolicy:   ptr(coresecrets.RotateDaily),
			NextRotateTime: ptr(now.Add(time.Minute)),
			ExpireTime:     ptr(now.Add(time.Hour)),
			Description:    ptr("my secret"),
			Label:          ptr("foobar"),
			Params:         map[string]interface{}{"param": 1},
			Data:           map[string]string{"foo": "bar"},
		},
	}
	uri := coresecrets.NewURI()
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

func (s *SecretsManagerSuite) TestCreateSecretMissingNextRotateTime(c *gc.C) {
	defer s.setup(c).Finish()

	service := juju.NewTestService(s.secretsStore)

	p := secrets.CreateParams{
		Version:       secrets.Version,
		ProviderLabel: juju.Provider,
		Owner:         "application-app",
		UpsertParams: secrets.UpsertParams{
			RotatePolicy: ptr(coresecrets.RotateDaily),
			Data:         map[string]string{"foo": "bar"},
		},
	}
	uri := coresecrets.NewURI()
	uri.ControllerUUID = coretesting.ControllerTag.Id()

	_, err := service.CreateSecret(context.Background(), uri, p)
	c.Assert(err, gc.ErrorMatches, "cannot specify a secret rotate policy without a next rotate time")
}

func (s *SecretsManagerSuite) TestUpdateSecret(c *gc.C) {
	defer s.setup(c).Finish()

	service := juju.NewTestService(s.secretsStore)

	now := time.Now()
	p := secrets.UpsertParams{
		RotatePolicy:   ptr(coresecrets.RotateDaily),
		NextRotateTime: ptr(now.Add(time.Minute)),
		ExpireTime:     ptr(now.Add(time.Hour)),
		Description:    ptr("my secret"),
		Label:          ptr("foobar"),
		Params:         map[string]interface{}{"param": 1},
		Data:           map[string]string{"foo": "bar"},
	}
	expectedP := state.UpdateSecretParams{
		RotatePolicy:   ptr(coresecrets.RotateDaily),
		NextRotateTime: ptr(now.Add(time.Minute)),
		ExpireTime:     ptr(now.Add(time.Hour)),
		Description:    ptr("my secret"),
		Label:          ptr("foobar"),
		Params:         map[string]interface{}{"param": 1},
		Data:           map[string]string{"foo": "bar"},
	}
	uri, _ := coresecrets.ParseURI("secret:9m4e2mr0ui3e8a215n4g")
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

func (s *SecretsManagerSuite) TestUpdateSecretMissingNextRotateTime(c *gc.C) {
	defer s.setup(c).Finish()

	service := juju.NewTestService(s.secretsStore)

	p := secrets.UpsertParams{
		RotatePolicy: ptr(coresecrets.RotateDaily),
		Data:         map[string]string{"foo": "bar"},
	}
	uri := coresecrets.NewURI()
	uri.ControllerUUID = coretesting.ControllerTag.Id()

	_, err := service.UpdateSecret(context.Background(), uri, p)
	c.Assert(err, gc.ErrorMatches, "cannot specify a secret rotate policy without a next rotate time")
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
