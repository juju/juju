// Copyright 2020 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package utils_test

import (
	"testing"

	"github.com/juju/juju/cmd/juju/application/utils"
	"github.com/juju/tc"

	"github.com/juju/juju/core/charm"
)

type typeSuite struct{}

func TestTypeSuite(t *testing.T) {
	tc.Run(t, &typeSuite{})
}

func (*typeSuite) TestSourceMatchSourceInternal(c *tc.C) {
	c.Assert(string(utils.Local), tc.Equals, charm.Local.String())
	c.Assert(string(utils.CharmHub), tc.Equals, charm.CharmHub.String())
}
