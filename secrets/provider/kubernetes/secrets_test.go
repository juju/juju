// Copyright 2022 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package kubernetes_test

import (
	"context"

	"github.com/juju/errors"
	"github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/caas"
	"github.com/juju/juju/cloud"
	"github.com/juju/juju/environs"
	"github.com/juju/juju/environs/cloudspec"
	"github.com/juju/juju/environs/config"
	"github.com/juju/juju/secrets/provider"
	_ "github.com/juju/juju/secrets/provider/all"
	"github.com/juju/juju/secrets/provider/kubernetes"
	coretesting "github.com/juju/juju/testing"
)

type kubernetesSuite struct {
	testing.IsolationSuite
}

var _ = gc.Suite(&kubernetesSuite{})

func (*kubernetesSuite) TestStoreConfig(c *gc.C) {
	p, err := provider.Provider(kubernetes.Store)
	c.Assert(err, jc.ErrorIsNil)
	cfg, err := p.StoreConfig(mockModel{}, false, nil, nil)
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

func (s *kubernetesSuite) TestNewStore(c *gc.C) {
	model := mockModel{}
	s.PatchValue(&kubernetes.NewCaas, func(ctx context.Context, args environs.OpenParams) (caas.Broker, error) {
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
	cfg, err := p.StoreConfig(model, false, nil, nil)
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
