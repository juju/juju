package maas

import (
	. "launchpad.net/gocheck"
)

type StateSuite struct {
	ProviderSuite
}

var _ = Suite(new(StateSuite))

func (suite *StateSuite) TestLoadStateReturnsNotFoundForMissingFile(c *C) {
	_, err := suite.loadState()
	c.Check(err, FitsTypeOf, environs.NotFoundError{})
}
