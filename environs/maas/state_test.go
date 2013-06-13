// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package maas

import (
	. "launchpad.net/gocheck"
	"launchpad.net/juju-core/errors"
)

type StateSuite struct {
	ProviderSuite
}

var _ = Suite(new(StateSuite))

func (suite *StateSuite) TestLoadStateReturnsNotFoundPointerForMissingFile(c *C) {
	serverURL := suite.testMAASObject.URL().String()
	config := getTestConfig("loadState-test", serverURL, "a:b:c", "foo")
	env, err := NewEnviron(config)
	c.Assert(err, IsNil)

	_, err = env.loadState()

	c.Check(err, FitsTypeOf, &errors.NotFoundError{})
}
