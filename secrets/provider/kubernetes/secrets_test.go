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
	"github.com/juju/juju/environs"
	"github.com/juju/juju/environs/cloudspec"
	"github.com/juju/juju/environs/config"
	"github.com/juju/juju/secrets/provider"
	_ "github.com/juju/juju/secrets/provider/all"
	"github.com/juju/juju/secrets/provider/kubernetes"
	"github.com/juju/juju/secrets/provider/kubernetes/mocks"
	coretesting "github.com/juju/juju/testing"
)

type kubernetesSuite struct {
	testing.IsolationSuite
}

var _ = gc.Suite(&kubernetesSuite{})

func (*kubernetesSuite) TestStoreConfig(c *gc.C) {
	p, err := provider.Provider(kubernetes.Store)
	c.Assert(err, jc.ErrorIsNil)
	cfg, err := p.StoreConfig(mockModel{}, nil, nil, nil)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(cfg, jc.DeepEquals, &provider.StoreConfig{
		StoreType: kubernetes.Store,
		Params: map[string]interface{}{
			"ca-certs":            []string{"cert-data"},
			"controller-uuid":     coretesting.ControllerTag.Id(),
			"credential":          `{"auth-type":"access-key","Attributes":{"foo":"bar"}}`,
			"endpoint":            "http://nowhere",
			"is-controller-cloud": true,
			"model-name":          "fred",
			"model-type":          "kubernetes",
			"model-uuid":          coretesting.ModelTag.Id(),
		},
	})
}

func (s *kubernetesSuite) assertStoreConfigWithTag(c *gc.C, isControllerCloud bool) {
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
	cfg, err := config.New(config.UseDefaults, map[string]interface{}{
		"name": "fred",
		"type": "kubernetes",
		"uuid": coretesting.ModelTag.Id(),
	})
	c.Assert(err, jc.ErrorIsNil)

	gomock.InOrder(
		model.EXPECT().Cloud().Return(cld, nil),
		model.EXPECT().CloudCredential().Return(&cred, nil),
		model.EXPECT().Config().Return(cfg, nil),
		model.EXPECT().ControllerUUID().Return(coretesting.ControllerTag.Id()),

		broker.EXPECT().EnsureSecretAccessToken(
			tag, []string{"owned-a-1"}, []string{"read-b-1", "read-b-2"}, nil,
		).Return("token", nil),
	)

	p, err := provider.Provider(kubernetes.Store)
	c.Assert(err, jc.ErrorIsNil)
	storeCfg, err := p.StoreConfig(model, tag,
		provider.NameMetaSlice{"owned-a": set.NewInts(1)},
		provider.NameMetaSlice{"read-b": set.NewInts(1, 2)},
	)
	c.Assert(err, jc.ErrorIsNil)
	expected := &provider.StoreConfig{
		StoreType: kubernetes.Store,
		Params: map[string]interface{}{
			"ca-certs":            []string{"cert-data"},
			"controller-uuid":     coretesting.ControllerTag.Id(),
			"credential":          `{"auth-type":"access-key","Attributes":{"Token":"token","password":"","username":""}}`,
			"endpoint":            "http://nowhere",
			"is-controller-cloud": isControllerCloud,
			"model-name":          "fred",
			"model-type":          "kubernetes",
			"model-uuid":          coretesting.ModelTag.Id(),
		},
	}
	if isControllerCloud {
		expected.Params["endpoint"] = "https://8.6.8.6:8888"
		expected.Params["is-controller-cloud"] = false
	}
	c.Assert(storeCfg, jc.DeepEquals, expected)
}

func (s *kubernetesSuite) TestStoreConfigWithTag(c *gc.C) {
	s.assertStoreConfigWithTag(c, false)
}

func (s *kubernetesSuite) TestStoreConfigWithTagWithControllerCloud(c *gc.C) {
	s.assertStoreConfigWithTag(c, true)
}

func (s *kubernetesSuite) TestCleanupSecrets(c *gc.C) {
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
	cfg, err := config.New(config.UseDefaults, map[string]interface{}{
		"name": "fred",
		"type": "kubernetes",
		"uuid": coretesting.ModelTag.Id(),
	})
	c.Assert(err, jc.ErrorIsNil)

	gomock.InOrder(
		model.EXPECT().Cloud().Return(cld, nil),
		model.EXPECT().CloudCredential().Return(&cred, nil),
		model.EXPECT().Config().Return(cfg, nil),
		model.EXPECT().ControllerUUID().Return(coretesting.ControllerTag.Id()),

		broker.EXPECT().EnsureSecretAccessToken(
			tag, nil, nil, []string{"removed-1", "removed-2"},
		).Return("token", nil),
	)

	p, err := provider.Provider(kubernetes.Store)
	c.Assert(err, jc.ErrorIsNil)
	err = p.CleanupSecrets(model, tag, provider.NameMetaSlice{"removed": set.NewInts(1, 2)})
	c.Assert(err, jc.ErrorIsNil)
}

func (s *kubernetesSuite) TestNewStore(c *gc.C) {
	model := mockModel{}
	s.PatchValue(&kubernetes.NewCaas, func(ctx context.Context, args environs.OpenParams) (kubernetes.Broker, error) {
		cred := cloud.NewCredential(cloud.AccessKeyAuthType, map[string]string{"foo": "bar"})
		cfg, err := model.Config()
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
			Config: cfg,
		})
		return nil, errors.New("boom")
	})
	p, err := provider.Provider(kubernetes.Store)
	c.Assert(err, jc.ErrorIsNil)
	cfg, err := p.StoreConfig(model, nil, nil, nil)
	c.Assert(err, jc.ErrorIsNil)
	_, err = p.NewStore(cfg)
	c.Assert(err, gc.ErrorMatches, "boom")
}

type mockModel struct{}

func (mockModel) ControllerUUID() string {
	return coretesting.ControllerTag.Id()
}

func (mockModel) UUID() string {
	return coretesting.ModelTag.Id()
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

func (mockModel) Config() (*config.Config, error) {
	return config.New(config.UseDefaults, map[string]interface{}{
		"name": "fred",
		"type": "kubernetes",
		"uuid": coretesting.ModelTag.Id(),
	})
}
