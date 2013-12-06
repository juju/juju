// Copyright 2012, 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package main_test

import (
	gc "launchpad.net/gocheck"

	"launchpad.net/juju-core/testing"
	"launchpad.net/juju-core/testing/testbase"
)

type DeleteCharmSuite struct {
	testbase.LoggingSuite
}

var _ = gc.Suite(&DeleteCharmSuite{})

const testDeleteCharm = `
mongo-url: localhost:23456
`

func (s *DeleteCharmSuite) SetUpSuite(c *gc.C) {
	s.LoggingSuite.SetUpSuite(c)
}

func (s *DeleteCharmSuite) TearDownSuite(c *gc.C) {
	s.LoggingSuite.TearDownSuite(c)
}

func (s *DeleteCharmSuite) TestInit(c *gc.C) {
	config := &DeleteCharmCommand{}
	err := testing.InitCommand(config, []string{"--config", "/etc/charmd.conf", "--url", "cs:go"})
	c.Assert(err, gc.IsNil)
	c.Assert(config.ConfigPath, gc.Equals, "/etc/charmd.conf")
	c.Assert(config.Url, gc.Equals, "cs:go")
}
