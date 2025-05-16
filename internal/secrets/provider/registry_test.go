// Copyright 2022 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package provider_test

import (
	stdtesting "testing"

	"github.com/juju/errors"
	"github.com/juju/tc"

	"github.com/juju/juju/internal/secrets/provider"
	_ "github.com/juju/juju/internal/secrets/provider/all"
	"github.com/juju/juju/internal/testhelpers"
)

type registrySuite struct {
	testhelpers.IsolationSuite
}

func TestRegistrySuite(t *stdtesting.T) { tc.Run(t, &registrySuite{}) }
func (*registrySuite) TestProvider(c *tc.C) {
	_, err := provider.Provider("bad")
	c.Assert(err, tc.ErrorIs, errors.NotFound)

	_, err = provider.Provider("controller")
	c.Assert(err, tc.ErrorIsNil)
}
