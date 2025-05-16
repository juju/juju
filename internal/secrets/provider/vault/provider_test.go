// Copyright 2022 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package vault_test

import (
	"io"
	"net/http"
	"strings"
	stdtesting "testing"

	"github.com/hashicorp/vault/api"
	"github.com/juju/collections/set"
	"github.com/juju/errors"
	"github.com/juju/tc"
	vault "github.com/mittwald/vaultgo"
	"go.uber.org/mock/gomock"

	"github.com/juju/juju/core/secrets"
	"github.com/juju/juju/internal/secrets/provider"
	_ "github.com/juju/juju/internal/secrets/provider/all"
	jujuvault "github.com/juju/juju/internal/secrets/provider/vault"
	"github.com/juju/juju/internal/secrets/provider/vault/mocks"
	"github.com/juju/juju/internal/testhelpers"
	coretesting "github.com/juju/juju/internal/testing"
)

type providerSuite struct {
	testhelpers.IsolationSuite
	coretesting.JujuOSEnvSuite

	mockRoundTripper *mocks.MockRoundTripper
}

func TestProviderSuite(t *stdtesting.T) { tc.Run(t, &providerSuite{}) }
func (s *providerSuite) SetUpTest(c *tc.C) {
	s.IsolationSuite.SetUpTest(c)
	s.JujuOSEnvSuite.SetUpTest(c)
}

type newVaultClientFunc func(addr string, tlsConf *vault.TLSConfig, opts ...vault.ClientOpts) (*vault.Client, error)

func (s *providerSuite) newVaultClient(c *tc.C, returnErr error) (*gomock.Controller, newVaultClientFunc) {
	ctrl := gomock.NewController(c)
	s.mockRoundTripper = mocks.NewMockRoundTripper(ctrl)

	return ctrl, func(addr string, tlsConf *vault.TLSConfig, opts ...vault.ClientOpts) (*vault.Client, error) {
		c.Assert(addr, tc.Equals, "http://vault-ip:8200/")
		c.Assert(tlsConf, tc.DeepEquals, &vault.TLSConfig{
			TLSConfig: &api.TLSConfig{
				CACertBytes:   []byte(coretesting.CACert),
				TLSServerName: "tls-server",
			},
		})

		client, err := vault.NewClient(addr, tlsConf, opts...)
		c.Assert(err, tc.ErrorIsNil)
		c.Assert(opts, tc.HasLen, 1)
		err = opts[0](client)
		c.Assert(err, tc.ErrorIsNil)
		c.Assert(client.Token(), tc.Equals, "vault-token")
		if returnErr != nil {
			return nil, returnErr
		}

		conf := api.DefaultConfig()
		conf.Address = addr
		if tlsConf != nil {
			err = conf.ConfigureTLS(tlsConf.TLSConfig)
			c.Assert(err, tc.ErrorIsNil)
		}
		conf.HttpClient.Transport = s.mockRoundTripper
		client.Client, err = api.NewClient(conf)
		c.Assert(err, tc.ErrorIsNil)

		return client, nil
	}
}

func (s *providerSuite) TestBackendConfigBadClient(c *tc.C) {
	ctrl, newVaultClient := s.newVaultClient(c, errors.New("boom"))
	defer ctrl.Finish()

	s.PatchValue(&jujuvault.NewVaultClient, newVaultClient)
	p, err := provider.Provider(jujuvault.BackendType)
	c.Assert(err, tc.ErrorIsNil)

	adminCfg := &provider.ModelBackendConfig{
		ControllerUUID: coretesting.ControllerTag.Id(),
		ModelUUID:      coretesting.ModelTag.Id(),
		ModelName:      "fred",
		BackendConfig: provider.BackendConfig{
			BackendType: "vault",
			Config: map[string]interface{}{
				"endpoint":        "http://vault-ip:8200/",
				"namespace":       "ns",
				"token":           "vault-token",
				"ca-cert":         coretesting.CACert,
				"tls-server-name": "tls-server",
			},
		},
	}

	accessor := secrets.Accessor{
		Kind: secrets.UnitAccessor,
		ID:   "gitlab/0",
	}
	_, err = p.RestrictedConfig(c.Context(), adminCfg, true, false, accessor, nil, nil)
	c.Assert(err, tc.ErrorMatches, "boom")
}

func (s *providerSuite) TestBackendConfigAdmin(c *tc.C) {
	ctrl, newVaultClient := s.newVaultClient(c, nil)
	defer ctrl.Finish()

	gomock.InOrder(
		s.mockRoundTripper.EXPECT().RoundTrip(gomock.Any()).DoAndReturn(
			func(req *http.Request) (*http.Response, error) {
				c.Assert(req.URL.String(), tc.Equals, `http://vault-ip:8200/v1/sys/policies/acl/fred-06f00d-read`)
				return &http.Response{
					Request:    req,
					StatusCode: http.StatusOK,
					Body:       io.NopCloser(nil),
				}, nil
			},
		),
		s.mockRoundTripper.EXPECT().RoundTrip(gomock.Any()).DoAndReturn(
			func(req *http.Request) (*http.Response, error) {
				c.Assert(req.URL.String(), tc.Equals, `http://vault-ip:8200/v1/sys/policies/acl/fred-06f00d-create`)
				return &http.Response{
					Request:    req,
					StatusCode: http.StatusOK,
					Body:       io.NopCloser(nil),
				}, nil
			},
		),
		s.mockRoundTripper.EXPECT().RoundTrip(gomock.Any()).DoAndReturn(
			func(req *http.Request) (*http.Response, error) {
				c.Assert(req.URL.String(), tc.Equals, `http://vault-ip:8200/v1/auth/token/create`)
				return &http.Response{
					Request:    req,
					StatusCode: http.StatusOK,
					Body:       io.NopCloser(strings.NewReader(`{"auth": {"client_token": "foo"}}`)),
				}, nil
			},
		),
	)

	s.PatchValue(&jujuvault.NewVaultClient, newVaultClient)
	p, err := provider.Provider(jujuvault.BackendType)
	c.Assert(err, tc.ErrorIsNil)

	adminCfg := &provider.ModelBackendConfig{
		ControllerUUID: coretesting.ControllerTag.Id(),
		ModelUUID:      coretesting.ModelTag.Id(),
		ModelName:      "fred",
		BackendConfig: provider.BackendConfig{
			BackendType: "vault",
			Config: map[string]interface{}{
				"endpoint":        "http://vault-ip:8200/",
				"namespace":       "ns",
				"token":           "vault-token",
				"ca-cert":         coretesting.CACert,
				"tls-server-name": "tls-server",
			},
		},
	}

	accessor := secrets.Accessor{
		Kind: secrets.ModelAccessor,
		ID:   coretesting.ModelTag.Id(),
	}
	cfg, err := p.RestrictedConfig(c.Context(), adminCfg, true, false, accessor, nil, nil)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(cfg.Config["token"], tc.Equals, "foo")
}

func (s *providerSuite) TestBackendConfigNonAdmin(c *tc.C) {
	ctrl, newVaultClient := s.newVaultClient(c, nil)
	defer ctrl.Finish()

	s.mockRoundTripper.EXPECT().RoundTrip(gomock.Any()).DoAndReturn(
		func(req *http.Request) (*http.Response, error) {
			c.Assert(req.URL.String(), tc.Equals, `http://vault-ip:8200/v1/sys/policies/acl/fred-06f00d-create`)
			return &http.Response{
				Request:    req,
				StatusCode: http.StatusOK,
				Body:       io.NopCloser(nil),
			}, nil
		},
	)
	s.mockRoundTripper.EXPECT().RoundTrip(gomock.Any()).DoAndReturn(
		func(req *http.Request) (*http.Response, error) {
			c.Assert(req.URL.String(), tc.Equals, `http://vault-ip:8200/v1/sys/policies/acl/fred-06f00d-owned-1-owner`)
			return &http.Response{
				Request:    req,
				StatusCode: http.StatusOK,
				Body:       io.NopCloser(nil),
			}, nil
		},
	)
	s.mockRoundTripper.EXPECT().RoundTrip(gomock.Any()).DoAndReturn(
		func(req *http.Request) (*http.Response, error) {
			c.Assert(req.URL.String(), tc.Equals, `http://vault-ip:8200/v1/sys/policies/acl/fred-06f00d-read-1-read`)
			return &http.Response{
				Request:    req,
				StatusCode: http.StatusOK,
				Body:       io.NopCloser(nil),
			}, nil
		},
	)
	s.mockRoundTripper.EXPECT().RoundTrip(gomock.Any()).DoAndReturn(
		func(req *http.Request) (*http.Response, error) {
			c.Assert(req.URL.String(), tc.Equals, `http://vault-ip:8200/v1/auth/token/create`)
			return &http.Response{
				Request:    req,
				StatusCode: http.StatusOK,
				Body:       io.NopCloser(strings.NewReader(`{"auth": {"client_token": "foo"}}`)),
			}, nil
		},
	)

	s.PatchValue(&jujuvault.NewVaultClient, newVaultClient)
	p, err := provider.Provider(jujuvault.BackendType)
	c.Assert(err, tc.ErrorIsNil)

	adminCfg := &provider.ModelBackendConfig{
		ControllerUUID: coretesting.ControllerTag.Id(),
		ModelUUID:      coretesting.ModelTag.Id(),
		ModelName:      "fred",
		BackendConfig: provider.BackendConfig{
			BackendType: "vault",
			Config: map[string]interface{}{
				"endpoint":        "http://vault-ip:8200/",
				"namespace":       "ns",
				"token":           "vault-token",
				"ca-cert":         coretesting.CACert,
				"tls-server-name": "tls-server",
			},
		},
	}
	ownedRevs := map[string]set.Strings{
		"owned-1": set.NewStrings("owned-rev-1", "owned-rev-2"),
	}
	readRevs := map[string]set.Strings{
		"read-1": set.NewStrings("read-rev-1"),
	}

	accessor := secrets.Accessor{
		Kind: secrets.UnitAccessor,
		ID:   "ubuntu/0",
	}
	cfg, err := p.RestrictedConfig(c.Context(), adminCfg, true, false, accessor, ownedRevs, readRevs)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(cfg.Config["token"], tc.Equals, "foo")
}

func (s *providerSuite) TestBackendConfigForDrain(c *tc.C) {
	ctrl, newVaultClient := s.newVaultClient(c, nil)
	defer ctrl.Finish()

	s.mockRoundTripper.EXPECT().RoundTrip(gomock.Any()).DoAndReturn(
		func(req *http.Request) (*http.Response, error) {
			c.Assert(req.URL.String(), tc.Equals, `http://vault-ip:8200/v1/sys/policies/acl/fred-06f00d-update`)
			return &http.Response{
				Request:    req,
				StatusCode: http.StatusOK,
				Body:       io.NopCloser(nil),
			}, nil
		},
	)
	s.mockRoundTripper.EXPECT().RoundTrip(gomock.Any()).DoAndReturn(
		func(req *http.Request) (*http.Response, error) {
			c.Assert(req.URL.String(), tc.Equals, `http://vault-ip:8200/v1/sys/policies/acl/fred-06f00d-create`)
			return &http.Response{
				Request:    req,
				StatusCode: http.StatusOK,
				Body:       io.NopCloser(nil),
			}, nil
		},
	)
	s.mockRoundTripper.EXPECT().RoundTrip(gomock.Any()).DoAndReturn(
		func(req *http.Request) (*http.Response, error) {
			c.Assert(req.URL.String(), tc.Equals, `http://vault-ip:8200/v1/sys/policies/acl/fred-06f00d-owned-1-owner`)
			return &http.Response{
				Request:    req,
				StatusCode: http.StatusOK,
				Body:       io.NopCloser(nil),
			}, nil
		},
	)
	s.mockRoundTripper.EXPECT().RoundTrip(gomock.Any()).DoAndReturn(
		func(req *http.Request) (*http.Response, error) {
			c.Assert(req.URL.String(), tc.Equals, `http://vault-ip:8200/v1/sys/policies/acl/fred-06f00d-read-1-read`)
			return &http.Response{
				Request:    req,
				StatusCode: http.StatusOK,
				Body:       io.NopCloser(nil),
			}, nil
		},
	)
	s.mockRoundTripper.EXPECT().RoundTrip(gomock.Any()).DoAndReturn(
		func(req *http.Request) (*http.Response, error) {
			c.Assert(req.URL.String(), tc.Equals, `http://vault-ip:8200/v1/auth/token/create`)
			return &http.Response{
				Request:    req,
				StatusCode: http.StatusOK,
				Body:       io.NopCloser(strings.NewReader(`{"auth": {"client_token": "foo"}}`)),
			}, nil
		},
	)

	s.PatchValue(&jujuvault.NewVaultClient, newVaultClient)
	p, err := provider.Provider(jujuvault.BackendType)
	c.Assert(err, tc.ErrorIsNil)

	adminCfg := &provider.ModelBackendConfig{
		ControllerUUID: coretesting.ControllerTag.Id(),
		ModelUUID:      coretesting.ModelTag.Id(),
		ModelName:      "fred",
		BackendConfig: provider.BackendConfig{
			BackendType: "vault",
			Config: map[string]interface{}{
				"endpoint":        "http://vault-ip:8200/",
				"namespace":       "ns",
				"token":           "vault-token",
				"ca-cert":         coretesting.CACert,
				"tls-server-name": "tls-server",
			},
		},
	}
	ownedRevs := map[string]set.Strings{
		"owned-1": set.NewStrings("owned-rev-1", "owned-rev-2"),
	}
	readRevs := map[string]set.Strings{
		"read-1": set.NewStrings("read-rev-1"),
	}

	accessor := secrets.Accessor{
		Kind: secrets.ModelAccessor,
		ID:   coretesting.ModelTag.Id(),
	}
	cfg, err := p.RestrictedConfig(c.Context(), adminCfg, true, true, accessor, ownedRevs, readRevs)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(cfg.Config["token"], tc.Equals, "foo")
}

func (s *providerSuite) TestNewBackend(c *tc.C) {
	ctrl, newVaultClient := s.newVaultClient(c, nil)
	defer ctrl.Finish()
	s.PatchValue(&jujuvault.NewVaultClient, newVaultClient)
	p, err := provider.Provider(jujuvault.BackendType)
	c.Assert(err, tc.ErrorIsNil)

	cfg := &provider.ModelBackendConfig{
		ModelName: "fred",
		ModelUUID: coretesting.ModelTag.Id(),
		BackendConfig: provider.BackendConfig{
			BackendType: jujuvault.BackendType,
			Config: map[string]interface{}{
				"endpoint":        "http://vault-ip:8200/",
				"namespace":       "ns",
				"token":           "vault-token",
				"ca-cert":         coretesting.CACert,
				"tls-server-name": "tls-server",
			},
		},
	}
	b, err := p.NewBackend(cfg)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(jujuvault.MountPath(b), tc.Equals, "fred-06f00d")
}
