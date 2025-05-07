// Copyright 2017 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package dummy

import (
	"context"

	"github.com/juju/tc"
	jc "github.com/juju/testing/checkers"

	"github.com/juju/juju/environs"
)

var (
	_ environs.NetworkingEnviron = (*environ)(nil)
)

var _ = tc.Suite(&environWhiteboxSuite{})

type environWhiteboxSuite struct{}

func (s *environWhiteboxSuite) TestSupportsContainerAddresses(c *tc.C) {
	callCtx := context.Background()

	env := new(environ)
	supported, err := env.SupportsContainerAddresses(callCtx)
	c.Check(err, jc.ErrorIsNil)
	c.Check(supported, jc.IsFalse)
	c.Check(environs.SupportsContainerAddresses(callCtx, env), jc.IsFalse)
}
