// Copyright 2023 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package bootstrap

import (
	"context"

	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/domain/modeldefaults/state"
	_ "github.com/juju/juju/internal/provider/dummy"
)

type bootstrapSuite struct{}

var _ = gc.Suite(&bootstrapSuite{})

func (*bootstrapSuite) TestBootstrapModelDefaults(c *gc.C) {
	provider := ModelDefaultsProvider(
		map[string]any{
			"foo":        "controller",
			"controller": "some value",
		},
		map[string]any{
			"foo":    "region",
			"region": "some value",
		},
		"dummy",
	)

	defaults, err := provider.ModelDefaults(context.Background())
	c.Check(err, jc.ErrorIsNil)
	c.Check(defaults["foo"].Region, gc.Equals, "region")
	c.Check(defaults["controller"].Controller, gc.Equals, "some value")
	c.Check(defaults["region"].Region, gc.Equals, "some value")

	configDefaults := state.ConfigDefaults(context.Background())
	for k, v := range configDefaults {
		c.Check(defaults[k].Default, gc.Equals, v)
	}
}
