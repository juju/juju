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

	"github.com/juju/juju/cloud"
	"github.com/juju/juju/core/secrets"
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
	s.PatchValue(&jujuvault.NewVaultClient, func(addr string, tlsConf *vault.TLSConfig, opts ...vault.ClientOpts) (*vault.Client, error) {
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
		return nil, errors.New("boom")
	})
}

func (s *providerSuite) TestBackendConfig(c *gc.C) {
	p, err := provider.Provider(jujuvault.BackendType)
	c.Assert(err, jc.ErrorIsNil)
	_, err = p.BackendConfig(mockModel{}, nil, nil, nil)
	c.Assert(err, gc.ErrorMatches, "boom")
}

func (s *providerSuite) TestNewBackend(c *gc.C) {
	p, err := provider.Provider(jujuvault.BackendType)
	c.Assert(err, jc.ErrorIsNil)

	cfg := &provider.BackendConfig{
		BackendType: jujuvault.BackendType,
		Config: map[string]interface{}{
			"endpoint":        "http://vault-ip:8200/",
			"namespace":       "ns",
			"token":           "vault-token",
			"ca-cert":         coretesting.CACert,
			"tls-server-name": "tls-server",
		},
	}
	_, err = p.NewBackend(cfg)
	c.Assert(err, gc.ErrorMatches, "boom")
}

type mockModel struct{}

func (mockModel) ControllerUUID() string {
	return coretesting.ControllerTag.Id()
}

func (mockModel) Name() string {
	return "fred"
}

func (mockModel) UUID() string {
	return coretesting.ModelTag.Id()
}

func (mockModel) Cloud() (cloud.Cloud, error) {
	return cloud.Cloud{
		Name:              "test",
		Type:              "lxd",
		Endpoint:          "http://nowhere",
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
		BackendType: "vault",
		Config: map[string]interface{}{
			"endpoint":        "http://vault-ip:8200/",
			"namespace":       "ns",
			"token":           "vault-token",
			"ca-cert":         coretesting.CACert,
			"tls-server-name": "tls-server",
		},
	}, nil
}
