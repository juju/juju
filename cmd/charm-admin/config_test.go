// Copyright 2012, 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package main_test

import (
	"fmt"
	"os"

	gc "launchpad.net/gocheck"

	"launchpad.net/juju-core/cmd"
	"launchpad.net/juju-core/testing"
	"launchpad.net/juju-core/testing/testbase"
)

type ConfigSuite struct {
	testbase.LoggingSuite
}

var _ = gc.Suite(&ConfigSuite{})

const testConfig = `
mongo-url: localhost:23456
foo: 1
bar: false
`

func (s *ConfigSuite) SetUpSuite(c *gc.C) {
	s.LoggingSuite.SetUpSuite(c)
}

func (s *ConfigSuite) TearDownSuite(c *gc.C) {
	s.LoggingSuite.TearDownSuite(c)
}

type SomeConfigCommand struct {
	ConfigCommand
}

func (c *SomeConfigCommand) Info() *cmd.Info {
	return &cmd.Info{
		Name:    "some-cmd",
		Purpose: "something in particular that requires configuration",
	}
}

func (s *ConfigSuite) TestReadConfig(c *gc.C) {
	confDir := c.MkDir()
	f, err := os.Create(path.Join(confDir, "charmd.conf"))
	c.Assert(err, gc.IsNil)
	cfgPath := f.Name()
	{
		defer f.Close()
		fmt.Fprint(f, testConfig)
	}

	config := &SomeConfigCommand{}
	err = testing.InitCommand(config, []string{"--config", cfgPath})
	c.Assert(err, gc.IsNil)

	dstr, err := config.ReadConfig()
	c.Assert(err, gc.IsNil)
	c.Assert(dstr.MongoUrl, gc.Equals, "localhost:23456")
}
