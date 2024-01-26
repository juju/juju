// Copyright 2022 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package vault_test

import (
	"io"
	"net/http"
	"strings"

	"github.com/hashicorp/vault/api"
	"github.com/juju/collections/set"
	"github.com/juju/errors"
	"github.com/juju/names/v5"
	"github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	vault "github.com/mittwald/vaultgo"
	"go.uber.org/mock/gomock"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/secrets/provider"
	_ "github.com/juju/juju/secrets/provider/all"
	jujuvault "github.com/juju/juju/secrets/provider/vault"
	"github.com/juju/juju/secrets/provider/vault/mocks"
	coretesting "github.com/juju/juju/testing"
)

type providerSuite struct {
	testing.IsolationSuite
	coretesting.JujuOSEnvSuite

	mockRoundTripper *mocks.MockRoundTripper
}

var _ = gc.Suite(&providerSuite{})

func (s *providerSuite) SetUpTest(c *gc.C) {
	s.IsolationSuite.SetUpTest(c)
	s.JujuOSEnvSuite.SetUpTest(c)
}

type newVaultClientFunc func(addr string, tlsConf *vault.TLSConfig, opts ...vault.ClientOpts) (*vault.Client, error)

func (s *providerSuite) newVaultClient(c *gc.C, returnErr error) (*gomock.Controller, newVaultClientFunc) {
	ctrl := gomock.NewController(c)
	s.mockRoundTripper = mocks.NewMockRoundTripper(ctrl)

	return ctrl, func(addr string, tlsConf *vault.TLSConfig, opts ...vault.ClientOpts) (*vault.Client, error) {
		c.Assert(addr, gc.Equals, "http://vault-ip:8200/")
		c.Assert(tlsConf, jc.DeepEquals, &vault.TLSConfig{
			TLSConfig: &api.TLSConfig{
				CACertBytes:   []byte(coretesting.CACert),
				TLSServerName: "tls-server",
			},
		})

		client, err := vault.NewClient(addr, tlsConf, opts...)
		c.Assert(err, jc.ErrorIsNil)
		c.Assert(opts, gc.HasLen, 1)
		err = opts[0](client)
		c.Assert(err, jc.ErrorIsNil)
		c.Assert(client.Token(), gc.Equals, "vault-token")
		if returnErr != nil {
			return nil, returnErr
		}

		conf := api.DefaultConfig()
		conf.Address = addr
		if tlsConf != nil {
			err = conf.ConfigureTLS(tlsConf.TLSConfig)
			c.Assert(err, jc.ErrorIsNil)
		}
		conf.HttpClient.Transport = s.mockRoundTripper
		client.Client, err = api.NewClient(conf)
		c.Assert(err, jc.ErrorIsNil)

		return client, nil
	}
}

func (s *providerSuite) TestBackendConfigBadClient(c *gc.C) {
	ctrl, newVaultClient := s.newVaultClient(c, errors.New("boom"))
	defer ctrl.Finish()

	s.PatchValue(&jujuvault.NewVaultClient, newVaultClient)
	p, err := provider.Provider(jujuvault.BackendType)
	c.Assert(err, jc.ErrorIsNil)

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
	_, err = p.RestrictedConfig(adminCfg, true, false, nil, nil, nil)
	c.Assert(err, gc.ErrorMatches, "boom")
}

func (s *providerSuite) TestBackendConfigAdmin(c *gc.C) {
	ctrl, newVaultClient := s.newVaultClient(c, nil)
	defer ctrl.Finish()

	gomock.InOrder(
		s.mockRoundTripper.EXPECT().RoundTrip(gomock.Any()).DoAndReturn(
			func(req *http.Request) (*http.Response, error) {
				c.Assert(req.URL.String(), gc.Equals, `http://vault-ip:8200/v1/sys/policies/acl/fred-06f00d-read`)
				return &http.Response{
					Request:    req,
					StatusCode: http.StatusOK,
					Body:       io.NopCloser(nil),
				}, nil
			},
		),
		s.mockRoundTripper.EXPECT().RoundTrip(gomock.Any()).DoAndReturn(
			func(req *http.Request) (*http.Response, error) {
				c.Assert(req.URL.String(), gc.Equals, `http://vault-ip:8200/v1/auth/token/create`)
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
	c.Assert(err, jc.ErrorIsNil)

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
	cfg, err := p.RestrictedConfig(adminCfg, true, false, nil, nil, nil)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(cfg.Config["token"], gc.Equals, "foo")
}

func (s *providerSuite) TestBackendConfigNonAdmin(c *gc.C) {
	ctrl, newVaultClient := s.newVaultClient(c, nil)
	defer ctrl.Finish()

	s.mockRoundTripper.EXPECT().RoundTrip(gomock.Any()).DoAndReturn(
		func(req *http.Request) (*http.Response, error) {
			c.Assert(req.URL.String(), gc.Equals, `http://vault-ip:8200/v1/sys/policies/acl/fred-06f00d-create`)
			return &http.Response{
				Request:    req,
				StatusCode: http.StatusOK,
				Body:       io.NopCloser(nil),
			}, nil
		},
	)
	s.mockRoundTripper.EXPECT().RoundTrip(gomock.Any()).DoAndReturn(
		func(req *http.Request) (*http.Response, error) {
			c.Assert(req.URL.String(), gc.Equals, `http://vault-ip:8200/v1/sys/policies/acl/fred-06f00d-owned-1-owner`)
			return &http.Response{
				Request:    req,
				StatusCode: http.StatusOK,
				Body:       io.NopCloser(nil),
			}, nil
		},
	)
	s.mockRoundTripper.EXPECT().RoundTrip(gomock.Any()).DoAndReturn(
		func(req *http.Request) (*http.Response, error) {
			c.Assert(req.URL.String(), gc.Equals, `http://vault-ip:8200/v1/sys/policies/acl/fred-06f00d-read-1-read`)
			return &http.Response{
				Request:    req,
				StatusCode: http.StatusOK,
				Body:       io.NopCloser(nil),
			}, nil
		},
	)
	s.mockRoundTripper.EXPECT().RoundTrip(gomock.Any()).DoAndReturn(
		func(req *http.Request) (*http.Response, error) {
			c.Assert(req.URL.String(), gc.Equals, `http://vault-ip:8200/v1/auth/token/create`)
			return &http.Response{
				Request:    req,
				StatusCode: http.StatusOK,
				Body:       io.NopCloser(strings.NewReader(`{"auth": {"client_token": "foo"}}`)),
			}, nil
		},
	)

	s.PatchValue(&jujuvault.NewVaultClient, newVaultClient)
	p, err := provider.Provider(jujuvault.BackendType)
	c.Assert(err, jc.ErrorIsNil)

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
	cfg, err := p.RestrictedConfig(adminCfg, true, false, names.NewUnitTag("ubuntu/0"), ownedRevs, readRevs)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(cfg.Config["token"], gc.Equals, "foo")
}

func (s *providerSuite) TestBackendConfigForDrain(c *gc.C) {
	ctrl, newVaultClient := s.newVaultClient(c, nil)
	defer ctrl.Finish()

	s.mockRoundTripper.EXPECT().RoundTrip(gomock.Any()).DoAndReturn(
		func(req *http.Request) (*http.Response, error) {
			c.Assert(req.URL.String(), gc.Equals, `http://vault-ip:8200/v1/sys/policies/acl/fred-06f00d-update`)
			return &http.Response{
				Request:    req,
				StatusCode: http.StatusOK,
				Body:       io.NopCloser(nil),
			}, nil
		},
	)
	s.mockRoundTripper.EXPECT().RoundTrip(gomock.Any()).DoAndReturn(
		func(req *http.Request) (*http.Response, error) {
			c.Assert(req.URL.String(), gc.Equals, `http://vault-ip:8200/v1/sys/policies/acl/fred-06f00d-create`)
			return &http.Response{
				Request:    req,
				StatusCode: http.StatusOK,
				Body:       io.NopCloser(nil),
			}, nil
		},
	)
	s.mockRoundTripper.EXPECT().RoundTrip(gomock.Any()).DoAndReturn(
		func(req *http.Request) (*http.Response, error) {
			c.Assert(req.URL.String(), gc.Equals, `http://vault-ip:8200/v1/sys/policies/acl/fred-06f00d-owned-1-owner`)
			return &http.Response{
				Request:    req,
				StatusCode: http.StatusOK,
				Body:       io.NopCloser(nil),
			}, nil
		},
	)
	s.mockRoundTripper.EXPECT().RoundTrip(gomock.Any()).DoAndReturn(
		func(req *http.Request) (*http.Response, error) {
			c.Assert(req.URL.String(), gc.Equals, `http://vault-ip:8200/v1/sys/policies/acl/fred-06f00d-read-1-read`)
			return &http.Response{
				Request:    req,
				StatusCode: http.StatusOK,
				Body:       io.NopCloser(nil),
			}, nil
		},
	)
	s.mockRoundTripper.EXPECT().RoundTrip(gomock.Any()).DoAndReturn(
		func(req *http.Request) (*http.Response, error) {
			c.Assert(req.URL.String(), gc.Equals, `http://vault-ip:8200/v1/auth/token/create`)
			return &http.Response{
				Request:    req,
				StatusCode: http.StatusOK,
				Body:       io.NopCloser(strings.NewReader(`{"auth": {"client_token": "foo"}}`)),
			}, nil
		},
	)

	s.PatchValue(&jujuvault.NewVaultClient, newVaultClient)
	p, err := provider.Provider(jujuvault.BackendType)
	c.Assert(err, jc.ErrorIsNil)

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
	cfg, err := p.RestrictedConfig(adminCfg, true, true, names.NewUnitTag("ubuntu/0"), ownedRevs, readRevs)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(cfg.Config["token"], gc.Equals, "foo")
}

func (s *providerSuite) TestNewBackend(c *gc.C) {
	ctrl, newVaultClient := s.newVaultClient(c, nil)
	defer ctrl.Finish()
	s.PatchValue(&jujuvault.NewVaultClient, newVaultClient)
	p, err := provider.Provider(jujuvault.BackendType)
	c.Assert(err, jc.ErrorIsNil)

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
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(jujuvault.MountPath(b), gc.Equals, "fred-06f00d")
}
