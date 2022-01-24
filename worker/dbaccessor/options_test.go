// Copyright 2022 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package dbaccessor

import (
	gc "gopkg.in/check.v1"
)

type OptionsSuite struct{}

var _ = gc.Suite(&OptionsSuite{})

func (s *OptionsSuite) TestOptions(c *gc.C) {
	options := []Option{
		WithAddress("10.0.0.1:9666"),
		WithCluster([]string{"10.0.0.2:9666"}),
	}

	opts := newOptions()
	for _, option := range options {
		option(opts)
	}

	c.Assert(opts.Address, gc.Equals, "10.0.0.1:9666")
	c.Assert(opts.Cluster, gc.DeepEquals, []string{"10.0.0.2:9666"})
}
