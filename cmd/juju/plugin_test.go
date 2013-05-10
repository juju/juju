package main

import (
	. "launchpad.net/gocheck"
)

type PluginSuite struct {
}

var _ = Suite(&PluginSuite{})

func (*PluginSuite) TestFindPlugins(c *C) {
	plugins := findPlugins()
	c.Assert(plugins, DeepEquals, []string{})
}
