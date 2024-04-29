// Copyright 2022 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package kubernetes_test

import (
	"context"

	"github.com/juju/collections/set"
	"github.com/juju/errors"
	"github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	"go.uber.org/mock/gomock"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/cloud"
	"github.com/juju/juju/core/secrets"
	"github.com/juju/juju/environs"
	"github.com/juju/juju/environs/cloudspec"
	"github.com/juju/juju/environs/config"
	"github.com/juju/juju/internal/secrets/provider"
	_ "github.com/juju/juju/internal/secrets/provider/all"
	"github.com/juju/juju/internal/secrets/provider/kubernetes"
	"github.com/juju/juju/internal/secrets/provider/kubernetes/mocks"
	"github.com/juju/juju/state"
	coretesting "github.com/juju/juju/testing"
)

type providerSuite struct {
	testing.IsolationSuite
}

var _ = gc.Suite(&providerSuite{})

func (s *providerSuite) assertRestrictedConfigWithTag(c *gc.C, isControllerCloud, sameController bool) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	broker := mocks.NewMockBroker(ctrl)

	s.PatchValue(&kubernetes.NewCaas, func(context.Context, environs.OpenParams) (kubernetes.Broker, error) { return broker, nil })
	s.PatchEnvironment("KUBERNETES_SERVICE_HOST", "8.6.8.6")
	s.PatchEnvironment("KUBERNETES_SERVICE_PORT", "8888")

	broker.EXPECT().EnsureSecretAccessToken(
		gomock.Any(), "gitlab/0", []string{"owned-rev-1"}, []string{"read-rev-1", "read-rev-2"}, nil,
	).Return("token", nil)

	p, err := provider.Provider(kubernetes.BackendType)
	c.Assert(err, jc.ErrorIsNil)
	adminCfg := &provider.ModelBackendConfig{
		ControllerUUID: coretesting.ControllerTag.Id(),
		ModelUUID:      coretesting.ModelTag.Id(),
		ModelName:      "fred",
		BackendConfig: provider.BackendConfig{
			BackendType: kubernetes.BackendType,
			Config: map[string]interface{}{
				"ca-certs":            []string{"cert-data"},
				"credential":          `{"auth-type":"access-key","Attributes":{"username":"bar","password":"bar"}}`,
				"endpoint":            "http://nowhere",
				"is-controller-cloud": isControllerCloud,
			},
		},
	}

	accessor := secrets.Accessor{
		Kind: secrets.UnitAccessor,
		ID:   "gitlab/0",
	}
	backendCfg, err := p.RestrictedConfig(context.Background(), adminCfg, sameController, false, accessor,
		provider.SecretRevisions{"owned-a": set.NewStrings("owned-rev-1")},
		provider.SecretRevisions{"read-b": set.NewStrings("read-rev-1", "read-rev-2")},
	)
	c.Assert(err, jc.ErrorIsNil)
	expected := &provider.BackendConfig{
		BackendType: kubernetes.BackendType,
		Config: map[string]interface{}{
			"ca-certs":            []string{"cert-data"},
			"credential":          `{"auth-type":"access-key","Attributes":{"Token":"token","password":"","username":""}}`,
			"endpoint":            "http://nowhere",
			"is-controller-cloud": isControllerCloud,
		},
	}
	if isControllerCloud && sameController {
		expected.Config["endpoint"] = "https://8.6.8.6:8888"
		expected.Config["is-controller-cloud"] = false
	} else {
		expected.Config["is-controller-cloud"] = false
	}
	c.Assert(backendCfg, jc.DeepEquals, expected)
}

func (s *providerSuite) TestRestrictedConfigWithTag(c *gc.C) {
	s.assertRestrictedConfigWithTag(c, false, false)
}

func (s *providerSuite) TestRestrictedConfigWithTagWithControllerCloud(c *gc.C) {
	s.assertRestrictedConfigWithTag(c, true, true)
}

func (s *providerSuite) TestRestrictedConfigWithTagWithControllerCloudDifferentController(c *gc.C) {
	s.assertRestrictedConfigWithTag(c, true, false)
}

func (s *providerSuite) TestCleanupSecrets(c *gc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	broker := mocks.NewMockBroker(ctrl)

	s.PatchValue(&kubernetes.NewCaas, func(context.Context, environs.OpenParams) (kubernetes.Broker, error) { return broker, nil })

	p, err := provider.Provider(kubernetes.BackendType)
	c.Assert(err, jc.ErrorIsNil)
	adminCfg := &provider.ModelBackendConfig{
		ControllerUUID: coretesting.ControllerTag.Id(),
		ModelUUID:      coretesting.ModelTag.Id(),
		ModelName:      "fred",
		BackendConfig: provider.BackendConfig{
			BackendType: kubernetes.BackendType,
			Config: map[string]interface{}{
				"ca-certs":            []string{"cert-data"},
				"credential":          `{"auth-type":"access-key","Attributes":{"username":"bar","password":"bar"}}`,
				"endpoint":            "http://nowhere",
				"is-controller-cloud": true,
			},
		},
	}

	gomock.InOrder(
		broker.EXPECT().EnsureSecretAccessToken(
			gomock.Any(), "gitlab/0", nil, nil, []string{"rev-1", "rev-2"},
		).Return("token", nil),
	)

	err = p.CleanupSecrets(context.Background(), adminCfg, "gitlab/0", provider.SecretRevisions{"removed": set.NewStrings("rev-1", "rev-2")})
	c.Assert(err, jc.ErrorIsNil)
}

func (s *providerSuite) TestNewBackend(c *gc.C) {
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
	_, err = p.NewBackend(&provider.ModelBackendConfig{
		ControllerUUID: coretesting.ControllerTag.Id(),
		ModelUUID:      coretesting.ModelTag.Id(),
		ModelName:      "fred",
		BackendConfig: provider.BackendConfig{
			BackendType: kubernetes.BackendType,
			Config: map[string]interface{}{
				"ca-certs":            []string{"cert-data"},
				"credential":          `{"auth-type":"access-key","Attributes":{"foo":"bar"}}`,
				"endpoint":            "http://nowhere",
				"is-controller-cloud": true,
			},
		},
	})
	c.Assert(err, gc.ErrorMatches, "getting cluster client: boom")
}
