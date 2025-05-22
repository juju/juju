// Copyright 2017 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package dummy

import (
	"testing"

	"github.com/juju/tc"

	"github.com/juju/juju/environs"
)

var (
	_ environs.NetworkingEnviron = (*environ)(nil)
)

func TestEnvironWhiteboxSuite(t *testing.T) {
	tc.Run(t, &environWhiteboxSuite{})
}

type environWhiteboxSuite struct{}

func (s *environWhiteboxSuite) TestSupportsContainerAddresses(c *tc.C) {
	callCtx := c.Context()

	env := new(environ)
	supported, err := env.SupportsContainerAddresses(callCtx)
	c.Check(err, tc.ErrorIsNil)
	c.Check(supported, tc.IsFalse)
	c.Check(environs.SupportsContainerAddresses(callCtx, env), tc.IsFalse)
}
