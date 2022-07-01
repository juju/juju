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

	coresecrets "github.com/juju/juju/v3/core/secrets"
	"github.com/juju/juju/v3/secrets"
	"github.com/juju/juju/v3/secrets/provider/juju"
	"github.com/juju/juju/v3/secrets/provider/juju/mocks"
	"github.com/juju/juju/v3/state"
	coretesting "github.com/juju/juju/v3/testing"
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
		Type:           "blob",
		Owner:          "application-app",
		Path:           "app/mariadb/password",
		RotateInterval: time.Hour,
		Status:         coresecrets.StatusActive,
		Description:    "my secret",
		Tags:           map[string]string{"hello": "world"},
		Params:         map[string]interface{}{"param": 1},
		Data:           map[string]string{"foo": "bar"},
	}
	expectedP := state.CreateSecretParams{
		Version:        p.Version,
		ProviderLabel:  "juju",
		Type:           p.Type,
		Owner:          "application-app",
		Path:           p.Path,
		RotateInterval: time.Hour,
		Status:         coresecrets.StatusActive,
		Description:    "my secret",
		Tags:           map[string]string{"hello": "world"},
		Params:         p.Params,
		Data:           p.Data,
	}
	URL := coresecrets.NewSimpleURL("app/mariadb/password")
	URL.ControllerUUID = coretesting.ControllerTag.Id()
	URL.ModelUUID = coretesting.ModelTag.Id()
	s.secretsStore.EXPECT().CreateSecret(URL, expectedP).DoAndReturn(
		func(URL *coresecrets.URL, p state.CreateSecretParams) (*coresecrets.SecretMetadata, error) {
			md := &coresecrets.SecretMetadata{
				URL:  URL,
				Path: "app/mariadb/password",
			}
			return md, nil
		},
	)

	resultMeta, err := service.CreateSecret(context.Background(), URL, p)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(resultMeta, jc.DeepEquals, &coresecrets.SecretMetadata{
		URL:  URL,
		Path: "app/mariadb/password",
	})
}

func (s *SecretsManagerSuite) TestUpdateSecret(c *gc.C) {
	defer s.setup(c).Finish()

	service := juju.NewTestService(s.secretsStore)

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
	expectedP := state.UpdateSecretParams{
		RotateInterval: &rotate,
		Status:         &status,
		Description:    &description,
		Tags:           &tags,
		Params:         p.Params,
		Data:           p.Data,
	}
	URL, _ := coresecrets.ParseURL("secret://app/mariadb/password")
	s.secretsStore.EXPECT().UpdateSecret(URL, expectedP).DoAndReturn(
		func(URL *coresecrets.URL, p state.UpdateSecretParams) (*coresecrets.SecretMetadata, error) {
			md := &coresecrets.SecretMetadata{
				URL:  URL.WithRevision(2),
				Path: "app/mariadb/password",
			}
			return md, nil
		},
	)

	resultMeta, err := service.UpdateSecret(context.Background(), URL, p)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(resultMeta, jc.DeepEquals, &coresecrets.SecretMetadata{
		URL:  URL.WithRevision(2),
		Path: "app/mariadb/password",
	})
}

func (s *SecretsManagerSuite) TestGetSecret(c *gc.C) {
	defer s.setup(c).Finish()

	service := juju.NewTestService(s.secretsStore)

	URL, _ := coresecrets.ParseURL("secret://app/mariadb/password#attr")
	baseURL := URL.WithAttribute("")
	md := &coresecrets.SecretMetadata{
		URL:      baseURL,
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

	URL, _ := coresecrets.ParseURL("secret://app/mariadb/password")
	s.secretsStore.EXPECT().GetSecret(URL).Return(
		&coresecrets.SecretMetadata{
			URL:      URL,
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

	URL, _ := coresecrets.ParseURL("secret://app/mariadb/password")
	s.secretsStore.EXPECT().GetSecret(URL).Return(
		&coresecrets.SecretMetadata{
			URL:      URL,
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
