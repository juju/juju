package maas

import (
	. "launchpad.net/gocheck"
	"launchpad.net/juju-core/environs"
)

type StateSuite struct {
	ProviderSuite
}

var _ = Suite(new(StateSuite))

func (suite *StateSuite) TestLoadStateReturnsNotFoundForMissingFile(c *C) {
	serverURL := suite.testMAASObject.URL().String()
	config := getTestConfig("loadState-test", serverURL, "a:b:c", "foo")
	env, err := NewEnviron(config)
	c.Assert(err, IsNil)

	_, err = env.loadState()

	c.Check(err, FitsTypeOf, environs.NotFoundError{})
}
