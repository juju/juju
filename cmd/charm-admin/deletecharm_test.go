// Copyright 2012, 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package main

import (
	"fmt"
	"os"
	"path"

	"github.com/juju/charm"
	charmtesting "github.com/juju/charm/testing"
	"github.com/juju/charmstore"
	gitjujutesting "github.com/juju/testing"
	gc "launchpad.net/gocheck"

	"github.com/juju/juju/testing"
)

type DeleteCharmSuite struct {
	testing.BaseSuite
}

var _ = gc.Suite(&DeleteCharmSuite{})

const testDeleteCharm = `
mongo-url: localhost:23456
`

func (s *DeleteCharmSuite) SetUpSuite(c *gc.C) {
	s.BaseSuite.SetUpSuite(c)
}

func (s *DeleteCharmSuite) TearDownSuite(c *gc.C) {
	s.BaseSuite.TearDownSuite(c)
}

func (s *DeleteCharmSuite) TestInit(c *gc.C) {
	config := &DeleteCharmCommand{}
	err := testing.InitCommand(config, []string{"--config", "/etc/charmd.conf", "--url", "cs:go"})
	c.Assert(err, gc.IsNil)
	c.Assert(config.ConfigPath, gc.Equals, "/etc/charmd.conf")
	c.Assert(config.Url, gc.Equals, "cs:go")
}

func (s *DeleteCharmSuite) TestRun(c *gc.C) {
	// Derive config file from test mongo port
	confDir := c.MkDir()
	f, err := os.Create(path.Join(confDir, "charmd.conf"))
	c.Assert(err, gc.IsNil)
	configPath := f.Name()
	{
		defer f.Close()
		fmt.Fprintf(f, "mongo-url: %s\n", gitjujutesting.MgoServer.Addr())
	}
	// Delete charm that does not exist, not found error.
	config := &DeleteCharmCommand{}
	out, err := testing.RunCommand(c, config, "--config", configPath, "--url", "cs:unreleased/foo")
	fmt.Println(out)
	c.Assert(err, gc.NotNil)
	// Publish that charm now
	url := charm.MustParseURL("cs:unreleased/foo")
	{
		s, err := charmstore.Open(gitjujutesting.MgoServer.Addr())
		defer s.Close()
		c.Assert(err, gc.IsNil)
		pub, err := s.CharmPublisher([]*charm.URL{url}, "such-digest-much-unique")
		c.Assert(err, gc.IsNil)
		err = pub.Publish(charmtesting.Charms.ClonedDir(c.MkDir(), "dummy"))
		c.Assert(err, gc.IsNil)
	}
	// Delete charm, should now succeed
	_, err = testing.RunCommand(c, config, "--config", configPath, "--url", "cs:unreleased/foo")
	c.Assert(err, gc.IsNil)
	c.Assert(config.Config, gc.NotNil)
	// Confirm that the charm is gone
	{
		s, err := charmstore.Open(gitjujutesting.MgoServer.Addr())
		defer s.Close()
		c.Assert(err, gc.IsNil)
		_, err = s.CharmInfo(url)
		c.Assert(err, gc.NotNil)
	}
}
