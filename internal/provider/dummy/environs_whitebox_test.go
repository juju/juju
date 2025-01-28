// Copyright 2017 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package dummy

import (
	"context"

	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/environs"
	"github.com/juju/juju/environs/envcontext"
)

var (
	_ environs.NetworkingEnviron = (*environ)(nil)
)

var _ = gc.Suite(&environWhiteboxSuite{})

type environWhiteboxSuite struct{}

func (s *environWhiteboxSuite) TestSupportsContainerAddresses(c *gc.C) {
	callCtx := envcontext.WithoutCredentialInvalidator(context.Background())

	env := new(environ)
	supported, err := env.SupportsContainerAddresses(callCtx)
	c.Check(err, jc.ErrorIsNil)
	c.Check(supported, jc.IsFalse)
	c.Check(environs.SupportsContainerAddresses(callCtx, env), jc.IsFalse)
}
