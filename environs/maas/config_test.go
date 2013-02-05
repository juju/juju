package maas

import (
	. "launchpad.net/gocheck"
	"launchpad.net/juju-core/environs"
	"launchpad.net/juju-core/testing"
)

type ConfigSuite struct{}

var _ = Suite(&ConfigSuite{})

// newConfig creates a MAAS environment config from attributes.
func newConfig(values map[string]interface{}) (*maasEnvironConfig, error) {
	cfg := map[string]interface{}{
		"name": "testenv",
		"type": "maas",
		"ca-cert": testing.CACert,
		"ca-private-key": testing.CAKey,
	}
	for k, v := range values {
		cfg[k] = v
	}
	env, err := environs.NewFromAttrs(cfg)
	if err != nil {
		return nil, err
	}
	return env.(*maasEnviron).ecfg(), nil
}

func (ConfigSuite) TestParsesMAASSettings(c *C) {
	server := "maas.example.com"
	oauth := "consumer-key:resource-token:resource-secret"
	secret := "ssssssht"
	ecfg, err := newConfig(map[string]interface{}{
		"maas-server": server,
		"maas-oauth": oauth,
		"admin-secret": secret,
	})
	c.Check(err, IsNil)
	c.Check(ecfg.maasServer(), Equals, server)
	c.Check(ecfg.maasOAuth(), DeepEquals, oauth)
	c.Check(ecfg.adminSecret(), Equals, secret)
}

func (ConfigSuite) TestRequiresMaasServer(c *C) {
	oauth := "consumer-key:resource-token:resource-secret"
	_, err := newConfig(map[string]interface{}{
		"maas-oauth": oauth,
		"admin-secret": "secret",
	})
	c.Check(err, NotNil)
}

func (ConfigSuite) TestRequiresOAuth(c *C) {
	_, err := newConfig(map[string]interface{}{
		"maas-server": "maas.example.com",
		"admin-secret": "secret",
	})
	c.Check(err, NotNil)
}

func (ConfigSuite) TestChecksWellFormedOAuth(c *C) {
	_, err := newConfig(map[string]interface{}{
		"maas-server": "maas.example.com",
		"maas-oauth": "This should have been a 3-part token.",
		"admin-secret": "secret",
	})
	c.Check(err, NotNil)
}

func (ConfigSuite) TestRequiresAdminSecret(c *C) {
	oauth := "consumer-key:resource-token:resource-secret"
	_, err := newConfig(map[string]interface{}{
		"maas-server": "maas.example.com",
		"maas-oauth": oauth,
	})
	c.Check(err, NotNil)
}
