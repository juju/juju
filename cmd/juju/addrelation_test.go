package main

import (
	"bytes"
	. "launchpad.net/gocheck"
	"launchpad.net/juju-core/cmd"
	"launchpad.net/juju-core/testing"
)

type AddRelationSuite struct {
	repoSuite
}

var _ = Suite(&AddRelationSuite{})

func runAddRelation(c *C, args ...string) error {
	com := &AddRelationCommand{}
	err := com.Init(newFlagSet(), args)
	c.Assert(err, IsNil)
	return com.Run(&cmd.Context{c.MkDir(), &bytes.Buffer{}, &bytes.Buffer{}, &bytes.Buffer{}})
}

func (s *AddRelationSuite) TestAddRelation(c *C) {
	testing.Charms.BundlePath(s.seriesPath, "series", "wordpress")
	err := runDeploy(c, "local:wordpress", "wp")
	c.Assert(err, IsNil)
	testing.Charms.BundlePath(s.seriesPath, "series", "mysql")
	err = runDeploy(c, "local:mysql", "ms")
	c.Assert(err, IsNil)
	testing.Charms.BundlePath(s.seriesPath, "series", "riak")
	err = runDeploy(c, "local:riak", "rk")
	c.Assert(err, IsNil)
	testing.Charms.BundlePath(s.seriesPath, "series", "logging")
	err = runDeploy(c, "local:logging", "lg")
	c.Assert(err, IsNil)

	err = runAddRelation(c, "rk", "ms")
	c.Assert(err, ErrorMatches, "no relations found")

	err = runAddRelation(c, "rk")
	c.Assert(err, ErrorMatches, `cannot add relation "rk:ring": relation already exists`)
	err = runAddRelation(c, "rk:ring")
	c.Assert(err, ErrorMatches, `cannot add relation "rk:ring": relation already exists`)

	err = runAddRelation(c, "ms", "wp")
	c.Assert(err, IsNil)
	alreadyExists := `cannot add relation "ms:server wp:db": relation already exists`
	err = runAddRelation(c, "ms", "wp")
	c.Assert(err, ErrorMatches, alreadyExists)
	err = runAddRelation(c, "ms", "wp:db")
	c.Assert(err, ErrorMatches, alreadyExists)
	err = runAddRelation(c, "ms:server", "wp")
	c.Assert(err, ErrorMatches, alreadyExists)
	err = runAddRelation(c, "ms:server", "wp:db")
	c.Assert(err, ErrorMatches, alreadyExists)

	err = runAddRelation(c, "ms", "lg")
	c.Assert(err, IsNil)
	err = runAddRelation(c, "ms", "lg")
	c.Assert(err, ErrorMatches, `cannot add relation "lg:info ms:juju-info": relation already exists`)

	err = runAddRelation(c, "wp", "lg")
	c.Assert(err, IsNil)
	err = runAddRelation(c, "wp", "lg")
	c.Assert(err, ErrorMatches, `cannot add relation "lg:logging-directory wp:logging-dir": relation already exists`)

	err = runAddRelation(c, "wp:juju-info", "lg")
	c.Assert(err, IsNil)
	err = runAddRelation(c, "wp:juju-info", "lg")
	c.Assert(err, ErrorMatches, `cannot add relation "lg:info wp:juju-info": relation already exists`)
}
