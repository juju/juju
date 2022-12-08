// Copyright 2022 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package vault_test

import (
	"fmt"

	"github.com/hashicorp/vault/api"
	"github.com/juju/errors"
	"github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	vault "github.com/mittwald/vaultgo"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/cloud"
	"github.com/juju/juju/environs/config"
	"github.com/juju/juju/feature"
	"github.com/juju/juju/secrets/provider"
	_ "github.com/juju/juju/secrets/provider/all"
	jujuvault "github.com/juju/juju/secrets/provider/vault"
	coretesting "github.com/juju/juju/testing"
)

type vaultSuite struct {
	testing.IsolationSuite
	coretesting.JujuOSEnvSuite
}

var _ = gc.Suite(&vaultSuite{})

func (s *vaultSuite) SetUpTest(c *gc.C) {
	s.SetInitialFeatureFlags(feature.DeveloperMode)
	s.IsolationSuite.SetUpTest(c)
	s.JujuOSEnvSuite.SetUpTest(c)
}

type newVaultClientFunc func(addr string, tlsConf *vault.TLSConfig, opts ...vault.ClientOpts) (*vault.Client, error)

func (s *vaultSuite) newVaultClient(c *gc.C) newVaultClientFunc {
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
		return nil, errors.New("boom")
	}
}

func (s *vaultSuite) TestBackendConfig(c *gc.C) {
	s.PatchValue(&jujuvault.NewVaultClient, s.newVaultClient(c))
	p, err := provider.Provider(jujuvault.Backend)
	c.Assert(err, jc.ErrorIsNil)
	_, err = p.BackendConfig(mockModel{}, nil, nil, nil)
	c.Assert(err, gc.ErrorMatches, "boom")
}

func (s *vaultSuite) TestNewBackend(c *gc.C) {
	s.PatchValue(&jujuvault.NewVaultClient, s.newVaultClient(c))
	p, err := provider.Provider(jujuvault.Backend)
	c.Assert(err, jc.ErrorIsNil)

	cfg := &provider.BackendConfig{
		BackendType: jujuvault.Backend,
		Config: map[string]interface{}{
			"controller-uuid": coretesting.ControllerTag.Id(),
			"model-uuid":      coretesting.ModelTag.Id(),
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

func (mockModel) Config() (*config.Config, error) {
	cert := coretesting.CACert
	return config.New(config.UseDefaults, map[string]interface{}{
		"name":                  "fred",
		"type":                  "lxd",
		"uuid":                  coretesting.ModelTag.Id(),
		"secret-backend":        "vault",
		"secret-backend-config": fmt.Sprintf(`{"endpoint":"http://vault-ip:8200/","token":"vault-token","namespace":"ns","ca-cert":%q,"tls-server-name":"tls-server"}`, cert),
	})
}
