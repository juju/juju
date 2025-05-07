// Copyright 2022 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package provider_test

import (
	"github.com/juju/errors"
	"github.com/juju/tc"
	"github.com/juju/testing"
	jc "github.com/juju/testing/checkers"

	"github.com/juju/juju/internal/secrets/provider"
	_ "github.com/juju/juju/internal/secrets/provider/all"
)

type registrySuite struct {
	testing.IsolationSuite
}

var _ = tc.Suite(&registrySuite{})

func (*registrySuite) TestProvider(c *tc.C) {
	_, err := provider.Provider("bad")
	c.Assert(err, jc.ErrorIs, errors.NotFound)

	_, err = provider.Provider("controller")
	c.Assert(err, jc.ErrorIsNil)
}
