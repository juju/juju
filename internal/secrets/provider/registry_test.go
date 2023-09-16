// Copyright 2022 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package provider_test

import (
	"github.com/juju/errors"
	"github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/internal/secrets/provider"
	_ "github.com/juju/juju/internal/secrets/provider/all"
)

type registrySuite struct {
	testing.IsolationSuite
}

var _ = gc.Suite(&registrySuite{})

func (*registrySuite) TestProvider(c *gc.C) {
	_, err := provider.Provider("bad")
	c.Assert(err, jc.ErrorIs, errors.NotFound)

	_, err = provider.Provider("controller")
	c.Assert(err, jc.ErrorIsNil)
}
