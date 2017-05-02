// Copyright 2017 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package oracle_test

import (
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/environs"
	"github.com/juju/juju/provider/oracle"
	"github.com/juju/juju/testing"
)

type networkingSuite struct{}

var _ = gc.Suite(&networkingSuite{})

func (n networkingSuite) TestDeleteMachineVnicSet(c *gc.C) {
	environ, err := oracle.NewOracleEnviron(
		oracle.DefaultProvider,
		environs.OpenParams{
			Config: testing.ModelConfig(c),
		},
		DefaultEnvironAPI,
		&advancingClock,
	)

	c.Assert(err, gc.IsNil)
	c.Assert(environ, gc.NotNil)

	err = environ.DeleteMachineVnicSet("0")
	c.Assert(err, gc.IsNil)
}
