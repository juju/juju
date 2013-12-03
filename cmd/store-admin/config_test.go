// Copyright 2012, 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package main

import (
	"fmt"
	"io/ioutil"
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
	f, err := ioutil.TempFile("", "")
	c.Assert(err, gc.IsNil)
	var cfgPath string
	{
		defer f.Close()
		fmt.Fprint(f, testConfig)
		cfgPath = f.Name()
	}
	defer os.Remove(cfgPath)

	config := &SomeConfigCommand{}
	err = testing.InitCommand(config, []string{"--config", cfgPath})
	c.Assert(err, gc.IsNil)

	dmap := make(map[string]interface{})
	config.ReadConfig(&dmap)
	{
		v, has := dmap["mongo-url"]
		c.Assert(has, gc.Equals, true)
		c.Assert(v, gc.FitsTypeOf, "s")
		c.Assert(v.(string), gc.Equals, "localhost:23456")
	}
	{
		v, has := dmap["foo"]
		c.Assert(has, gc.Equals, true)
		c.Assert(v, gc.FitsTypeOf, 1)
		c.Assert(v.(int), gc.Equals, 1)
	}
	{
		v, has := dmap["bar"]
		c.Assert(has, gc.Equals, true)
		c.Assert(v, gc.FitsTypeOf, true)
		c.Assert(v.(bool), gc.Equals, false)
	}
	{
		_, has := dmap["nope"]
		c.Assert(has, gc.Equals, false)
	}

	// store-admin might want to reuse charmd/charmload config files.
	// Let's see if extra keys pose a problem.
	dstr := struct {
		MongoUrl string `yaml:"mongo-url"`
	}{}
	config.ReadConfig(&dstr)
	c.Assert(dstr.MongoUrl, gc.Equals, "localhost:23456")
}
