// Copyright 2017 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package dummy

import (
	// stdtesting "testing"
	"github.com/juju/errors"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/environs"
)

var (
	_ environs.NetworkingEnviron = (*environ)(nil)
)

var _ = gc.Suite(&environWhiteboxSuite{})

type environWhiteboxSuite struct{}

func (s *environWhiteboxSuite) TestSupportsContainerAddresses(c *gc.C) {
	// For now this is a static method so we can use a nil environ
	var env *environ
	supported, err := env.SupportsContainerAddresses()
	c.Check(err, jc.Satisfies, errors.IsNotSupported)
	c.Check(supported, jc.IsFalse)
	c.Check(env, gc.Not(jc.Satisfies), environs.SupportsContainerAddresses)
}
