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
	_, err := suite.environ.loadState()
	c.Check(err, FitsTypeOf, environs.NotFoundError{})
}
