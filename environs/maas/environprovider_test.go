package maas

import (
	. "launchpad.net/gocheck"
	"launchpad.net/juju-core/environs/config"
)

type EnvironProviderSuite struct {
	ProviderSuite
}

var _ = Suite(new(EnvironProviderSuite))

func (suite *EnvironProviderSuite) TestSecretAttrsReturnsSensitiveMAASAttributes(c *C) {
	const oauth = "aa:bb:cc"
	attrs := map[string]interface{}{
		"maas-oauth":  oauth,
		"maas-server": "http://maas.example.com/maas/api/1.0/",
		"name":        "wheee",
		"type":        "maas",
	}
	config, err := config.New(attrs)
	c.Assert(err, IsNil)

	secretAttrs, err := suite.environ.Provider().SecretAttrs(config)
	c.Assert(err, IsNil)

	expectedAttrs := map[string]interface{}{"maas-oauth": oauth}
	c.Check(secretAttrs, DeepEquals, expectedAttrs)
}
