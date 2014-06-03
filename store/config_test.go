// Copyright 2012, 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package store_test

import (
	"fmt"
	"os"
	"path"

	gc "launchpad.net/gocheck"

	"github.com/juju/juju/store"
	"github.com/juju/juju/testing"
)

type ConfigSuite struct {
	testing.BaseSuite
}

var _ = gc.Suite(&ConfigSuite{})

const testConfig = `
mongo-url: localhost:23456
foo: 1
bar: false
`

func (s *ConfigSuite) SetUpSuite(c *gc.C) {
	s.BaseSuite.SetUpSuite(c)
}

func (s *ConfigSuite) TearDownSuite(c *gc.C) {
	s.BaseSuite.TearDownSuite(c)
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

	dstr, err := store.ReadConfig(cfgPath)
	c.Assert(err, gc.IsNil)
	c.Assert(dstr.MongoURL, gc.Equals, "localhost:23456")
}
