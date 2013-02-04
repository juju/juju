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
		"control-bucket": "x",
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

func (ConfigSuite) TestParsesMAASServer(c *C) {
	e, err := newConfig(map[string]interface{}{"maas-server": "foo.local"})
	c.Check(err, IsNil)
	c.Check(e.maasServer(), Equals, "foo.local")
}

func (ConfigSuite) TestParsesMAASOAuth(c *C) {
	oauth := []string{"consumer-key", "resource-token", "resource-secret"}
	e, err := newConfig(map[string]interface{}{"maas-oauth": oauth})
	c.Check(err, IsNil)
	c.Check(e.maasOAuth(), DeepEquals, oauth)
}

func (ConfigSuite) TestParsesAdminSecret(c *C) {
	e, err := newConfig(map[string]interface{}{"admin-secret": "sssssht"})
	c.Check(err, IsNil)
	c.Check(e.adminSecret(), Equals, "sssssht")
}

/*
func (ConfigSuite) TestXXX(c *C) {
}

*/
