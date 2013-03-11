package maas

import (
	. "launchpad.net/gocheck"
	"launchpad.net/juju-core/environs/config"
	"sort"
)

type EnvironProviderSuite struct {
	ProviderSuite
}

var _ = Suite(new(EnvironProviderSuite))

// Return (in lexicographical order) the keys in a map of the given type.
func getMapKeys(original map[string]interface{}) []string {
	keys := make([]string, 0)
	for k, _ := range original {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}

func (suite *EnvironProviderSuite) TestSecretAttrsReturnsSensitiveMAASAttributes(c *C) {
	attrs := map[string]interface{}{
		"maas-oauth":  "a:b:c",
		"maas-server": "http://maas.example.com/maas/api/1.0/",
		"name":        "wheee",
		"type":        "maas",
	}
	config, err := config.New(attrs)
	c.Assert(err, IsNil)

	secretAttrs, err := suite.environ.Provider().SecretAttrs(config)
	c.Assert(err, IsNil)

	c.Check(getMapKeys(secretAttrs), DeepEquals, []string{"maas-oauth"})
	c.Check(secretAttrs["maas-oauth"], Equals, attrs["maas-oauth"])
}
