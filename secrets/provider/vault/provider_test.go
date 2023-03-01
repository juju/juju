// Copyright 2022 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package vault_test

import (
	"github.com/hashicorp/vault/api"
	"github.com/juju/errors"
	"github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	vault "github.com/mittwald/vaultgo"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/secrets/provider"
	_ "github.com/juju/juju/secrets/provider/all"
	jujuvault "github.com/juju/juju/secrets/provider/vault"
	coretesting "github.com/juju/juju/testing"
)

type providerSuite struct {
	testing.IsolationSuite
	coretesting.JujuOSEnvSuite
}

var _ = gc.Suite(&providerSuite{})

func (s *providerSuite) SetUpTest(c *gc.C) {
	s.IsolationSuite.SetUpTest(c)
	s.JujuOSEnvSuite.SetUpTest(c)
}

type newVaultClientFunc func(addr string, tlsConf *vault.TLSConfig, opts ...vault.ClientOpts) (*vault.Client, error)

func (s *providerSuite) newVaultClient(c *gc.C, returnErr error) newVaultClientFunc {
	return func(addr string, tlsConf *vault.TLSConfig, opts ...vault.ClientOpts) (*vault.Client, error) {
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
		return vault.NewClient(addr, tlsConf, opts...)
	}
}

func (s *providerSuite) TestBackendConfig(c *gc.C) {
	s.PatchValue(&jujuvault.NewVaultClient, s.newVaultClient(c, errors.New("boom")))
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
	_, err = p.RestrictedConfig(adminCfg, nil, nil, nil)
	c.Assert(err, gc.ErrorMatches, "boom")
}

func (s *providerSuite) TestNewBackend(c *gc.C) {
	s.PatchValue(&jujuvault.NewVaultClient, s.newVaultClient(c, nil))
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
