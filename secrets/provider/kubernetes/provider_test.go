// Copyright 2022 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package kubernetes_test

import (
	"context"

	"github.com/golang/mock/gomock"
	"github.com/juju/collections/set"
	"github.com/juju/errors"
	"github.com/juju/names/v4"
	"github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/cloud"
	"github.com/juju/juju/core/secrets"
	"github.com/juju/juju/environs"
	"github.com/juju/juju/environs/cloudspec"
	"github.com/juju/juju/environs/config"
	"github.com/juju/juju/secrets/provider"
	_ "github.com/juju/juju/secrets/provider/all"
	"github.com/juju/juju/secrets/provider/kubernetes"
	"github.com/juju/juju/secrets/provider/kubernetes/mocks"
	"github.com/juju/juju/state"
	coretesting "github.com/juju/juju/testing"
)

type providerSuite struct {
	testing.IsolationSuite
}

var _ = gc.Suite(&providerSuite{})

func (*providerSuite) TestBackendConfig(c *gc.C) {
	p, err := provider.Provider(kubernetes.BackendType)
	c.Assert(err, jc.ErrorIsNil)
	cfg, err := p.BackendConfig(mockModel{}, nil, nil, nil)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(cfg, jc.DeepEquals, &provider.BackendConfig{
		ControllerUUID: coretesting.ControllerTag.Id(),
		ModelUUID:      coretesting.ModelTag.Id(),
		ModelName:      "fred",
		BackendType:    kubernetes.BackendType,
		Config: map[string]interface{}{
			"ca-certs":            []string{"cert-data"},
			"credential":          `{"auth-type":"access-key","Attributes":{"foo":"bar"}}`,
			"endpoint":            "http://nowhere",
			"is-controller-cloud": true,
		},
	})
}

func (s *providerSuite) assertBackendConfigWithTag(c *gc.C, isControllerCloud bool) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	tag := names.NewUnitTag("gitlab/0")
	model := mocks.NewMockModel(ctrl)
	broker := mocks.NewMockBroker(ctrl)

	s.PatchValue(&kubernetes.NewCaas, func(context.Context, environs.OpenParams) (kubernetes.Broker, error) { return broker, nil })
	s.PatchEnvironment("KUBERNETES_SERVICE_HOST", "8.6.8.6")
	s.PatchEnvironment("KUBERNETES_SERVICE_PORT", "8888")

	cld := cloud.Cloud{
		Name:              "test",
		Type:              "kubernetes",
		Endpoint:          "http://nowhere",
		CACertificates:    []string{"cert-data"},
		IsControllerCloud: isControllerCloud,
	}
	cred := cloud.NewCredential(cloud.AccessKeyAuthType, map[string]string{"username": "bar", "password": "bar"})

	model.EXPECT().ControllerUUID().Return(coretesting.ControllerTag.Id()).AnyTimes()
	model.EXPECT().UUID().Return(coretesting.ModelTag.Id()).AnyTimes()
	model.EXPECT().Name().Return("fred").AnyTimes()
	gomock.InOrder(
		model.EXPECT().Cloud().Return(cld, nil),
		model.EXPECT().CloudCredential().Return(&cred, nil),

		broker.EXPECT().EnsureSecretAccessToken(
			tag, []string{"owned-a-1"}, []string{"read-b-1", "read-b-2"}, nil,
		).Return("token", nil),
	)

	p, err := provider.Provider(kubernetes.BackendType)
	c.Assert(err, jc.ErrorIsNil)
	backendCfg, err := p.BackendConfig(model, tag,
		provider.SecretRevisions{"owned-a": set.NewInts(1)},
		provider.SecretRevisions{"read-b": set.NewInts(1, 2)},
	)
	c.Assert(err, jc.ErrorIsNil)
	expected := &provider.BackendConfig{
		ControllerUUID: coretesting.ControllerTag.Id(),
		ModelUUID:      coretesting.ModelTag.Id(),
		ModelName:      "fred",
		BackendType:    kubernetes.BackendType,
		Config: map[string]interface{}{
			"ca-certs":            []string{"cert-data"},
			"credential":          `{"auth-type":"access-key","Attributes":{"Token":"token","password":"","username":""}}`,
			"endpoint":            "http://nowhere",
			"is-controller-cloud": isControllerCloud,
		},
	}
	if isControllerCloud {
		expected.Config["endpoint"] = "https://8.6.8.6:8888"
		expected.Config["is-controller-cloud"] = false
	}
	c.Assert(backendCfg, jc.DeepEquals, expected)
}

func (s *providerSuite) TestBackendConfigWithTag(c *gc.C) {
	s.assertBackendConfigWithTag(c, false)
}

func (s *providerSuite) TestBackendConfigWithTagWithControllerCloud(c *gc.C) {
	s.assertBackendConfigWithTag(c, true)
}

func (s *providerSuite) TestCleanupSecrets(c *gc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	tag := names.NewUnitTag("gitlab/0")
	model := mocks.NewMockModel(ctrl)
	broker := mocks.NewMockBroker(ctrl)

	s.PatchValue(&kubernetes.NewCaas, func(context.Context, environs.OpenParams) (kubernetes.Broker, error) { return broker, nil })

	cld := cloud.Cloud{
		Name:              "test",
		Type:              "kubernetes",
		Endpoint:          "http://nowhere",
		CACertificates:    []string{"cert-data"},
		IsControllerCloud: true,
	}
	cred := cloud.NewCredential(cloud.AccessKeyAuthType, map[string]string{"username": "bar", "password": "bar"})

	gomock.InOrder(
		model.EXPECT().Cloud().Return(cld, nil),
		model.EXPECT().CloudCredential().Return(&cred, nil),
		model.EXPECT().ControllerUUID().Return(coretesting.ControllerTag.Id()),
		model.EXPECT().UUID().Return(coretesting.ModelTag.Id()),
		model.EXPECT().Name().Return("fred"),

		broker.EXPECT().EnsureSecretAccessToken(
			tag, nil, nil, []string{"removed-1", "removed-2"},
		).Return("token", nil),
	)

	p, err := provider.Provider(kubernetes.BackendType)
	c.Assert(err, jc.ErrorIsNil)
	err = p.CleanupSecrets(model, tag, provider.SecretRevisions{"removed": set.NewInts(1, 2)})
	c.Assert(err, jc.ErrorIsNil)
}

func (s *providerSuite) TestNewBackend(c *gc.C) {
	model := mockModel{}
	s.PatchValue(&kubernetes.NewCaas, func(ctx context.Context, args environs.OpenParams) (kubernetes.Broker, error) {
		cred := cloud.NewCredential(cloud.AccessKeyAuthType, map[string]string{"foo": "bar"})
		modelCfg, err := config.New(config.UseDefaults, map[string]interface{}{
			config.TypeKey: state.ModelTypeCAAS,
			config.NameKey: "fred",
			config.UUIDKey: coretesting.ModelTag.Id(),
		})
		c.Assert(err, jc.ErrorIsNil)
		c.Assert(args, jc.DeepEquals, environs.OpenParams{
			ControllerUUID: coretesting.ControllerTag.Id(),
			Cloud: cloudspec.CloudSpec{
				Type:              "kubernetes",
				Name:              "secret-access",
				Endpoint:          "http://nowhere",
				Credential:        &cred,
				CACertificates:    []string{"cert-data"},
				IsControllerCloud: true,
			},
			Config: modelCfg,
		})
		return nil, errors.New("boom")
	})
	p, err := provider.Provider(kubernetes.BackendType)
	c.Assert(err, jc.ErrorIsNil)
	cfg, err := p.BackendConfig(model, nil, nil, nil)
	c.Assert(err, jc.ErrorIsNil)
	_, err = p.NewBackend(cfg)
	c.Assert(err, gc.ErrorMatches, "boom")
}

type mockModel struct{}

func (mockModel) ControllerUUID() string {
	return coretesting.ControllerTag.Id()
}

func (mockModel) UUID() string {
	return coretesting.ModelTag.Id()
}

func (mockModel) Name() string {
	return "fred"
}

func (mockModel) Cloud() (cloud.Cloud, error) {
	return cloud.Cloud{
		Name:              "test",
		Type:              "kubernetes",
		Endpoint:          "http://nowhere",
		CACertificates:    []string{"cert-data"},
		IsControllerCloud: true,
	}, nil
}

func (mockModel) CloudCredential() (*cloud.Credential, error) {
	cred := cloud.NewCredential(cloud.AccessKeyAuthType, map[string]string{"foo": "bar"})
	return &cred, nil
}

func (mockModel) GetSecretBackend() (*secrets.SecretBackend, error) {
	return &secrets.SecretBackend{
		Name:        "myk8s",
		BackendType: "kubernetes",
	}, nil
}
