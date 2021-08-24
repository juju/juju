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
		ControllerUUID: coretesting.ControllerTag.Id(),
		ModelUUID:      coretesting.ModelTag.Id(),
		Version:        secrets.Version,
		ProviderLabel:  juju.Provider,
		Type:           "blob",
		Path:           "app.password",
		RotateInterval: time.Hour,
		Params:         map[string]interface{}{"param": 1},
		Data:           map[string]string{"foo": "bar"},
	}
	expectedP := state.CreateSecretParams{
		ControllerUUID: p.ControllerUUID,
		ModelUUID:      p.ModelUUID,
		Version:        p.Version,
		ProviderLabel:  "juju",
		Type:           p.Type,
		Path:           p.Path,
		RotateInterval: time.Hour,
		Params:         p.Params,
		Data:           p.Data,
	}
	URL, _ := coresecrets.ParseURL("secret://v1/app.password")
	s.secretsStore.EXPECT().CreateSecret(expectedP).DoAndReturn(
		func(p state.CreateSecretParams) (*coresecrets.SecretMetadata, error) {
			md := &coresecrets.SecretMetadata{
				URL:  URL,
				Path: "app.password",
			}
			return md, nil
		},
	)

	resultMeta, err := service.CreateSecret(context.Background(), p)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(resultMeta, jc.DeepEquals, &coresecrets.SecretMetadata{
		URL:  URL,
		Path: "app.password",
	})
}

func (s *SecretsManagerSuite) TestUpdateSecret(c *gc.C) {
	defer s.setup(c).Finish()

	service := juju.NewTestService(s.secretsStore)

	p := secrets.UpdateParams{
		RotateInterval: time.Hour,
		Params:         map[string]interface{}{"param": 1},
		Data:           map[string]string{"foo": "bar"},
	}
	expectedP := state.UpdateSecretParams{
		RotateInterval: time.Hour,
		Params:         p.Params,
		Data:           p.Data,
	}
	URL, _ := coresecrets.ParseURL("secret://v1/app.password")
	s.secretsStore.EXPECT().UpdateSecret(URL, expectedP).DoAndReturn(
		func(URL *coresecrets.URL, p state.UpdateSecretParams) (*coresecrets.SecretMetadata, error) {
			md := &coresecrets.SecretMetadata{
				URL:  URL.WithRevision(2),
				Path: "app.password",
			}
			return md, nil
		},
	)

	resultMeta, err := service.UpdateSecret(context.Background(), URL, p)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(resultMeta, jc.DeepEquals, &coresecrets.SecretMetadata{
		URL:  URL.WithRevision(2),
		Path: "app.password",
	})
}

func (s *SecretsManagerSuite) TestGetSecret(c *gc.C) {
	defer s.setup(c).Finish()

	service := juju.NewTestService(s.secretsStore)

	URL, _ := coresecrets.ParseURL("secret://v1/app.password#attr")
	baseURL := URL.WithAttribute("")
	md := &coresecrets.SecretMetadata{
		URL: baseURL,
		Revision: 2,
	}
	s.secretsStore.EXPECT().GetSecret(baseURL).Return(
		md, nil,
	)

	result, err := service.GetSecret(context.Background(), URL)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(result, jc.DeepEquals, md)
}

func (s *SecretsManagerSuite) TestGetSecretValue(c *gc.C) {
	defer s.setup(c).Finish()

	service := juju.NewTestService(s.secretsStore)

	URL, _ := coresecrets.ParseURL("secret://v1/app.password")
	s.secretsStore.EXPECT().GetSecret(URL).Return(
		&coresecrets.SecretMetadata{
			URL: URL,
			Revision: 2,
		}, nil,
	)
	data := map[string]string{"foo": "bar"}
	val := coresecrets.NewSecretValue(data)
	s.secretsStore.EXPECT().GetSecretValue(URL.WithRevision(2)).Return(
		val, nil,
	)

	result, err := service.GetSecretValue(context.Background(), URL)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(result, jc.DeepEquals, val)
}

func (s *SecretsManagerSuite) TestGetSecretValueSpecificRevision(c *gc.C) {
	defer s.setup(c).Finish()

	service := juju.NewTestService(s.secretsStore)

	URL, _ := coresecrets.ParseURL("secret://v1/app.password")
	s.secretsStore.EXPECT().GetSecret(URL).Return(
		&coresecrets.SecretMetadata{
			URL: URL,
			Revision: 2,
		}, nil,
	)
	data := map[string]string{"foo": "bar"}
	val := coresecrets.NewSecretValue(data)
	s.secretsStore.EXPECT().GetSecretValue(URL.WithRevision(2)).Return(
		val, nil,
	)

	result, err := service.GetSecretValue(context.Background(), URL)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(result, jc.DeepEquals, val)
}

func (s *SecretsManagerSuite) TestListSecrets(c *gc.C) {
	defer s.setup(c).Finish()

	service := juju.NewTestService(s.secretsStore)

	metadata := []*coresecrets.SecretMetadata{{ID: 666}}
	s.secretsStore.EXPECT().ListSecrets(state.SecretsFilter{}).Return(
		metadata, nil,
	)

	result, err := service.ListSecrets(context.Background(), secrets.Filter{})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(result, jc.DeepEquals, metadata)
}
