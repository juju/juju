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
	if err := com.Init(newFlagSet(), args); err != nil {
		return err
	}
	return com.Run(&cmd.Context{c.MkDir(), &bytes.Buffer{}, &bytes.Buffer{}, &bytes.Buffer{}})
}

var msWpAlreadyExists = `cannot add relation "wp:db ms:server": relation already exists`
var msLgAlreadyExists = `cannot add relation "lg:info ms:juju-info": relation already exists`
var wpLgAlreadyExists = `cannot add relation "lg:logging-directory wp:logging-dir": relation already exists`
var wpLgAlreadyExistsJuju = `cannot add relation "lg:info wp:juju-info": relation already exists`

var addRelationTests = []struct {
	args []string
	err  string
}{
	{
		args: []string{"rk", "ms"},
		err:  "no relations found",
	}, {
		err: "a relation must involve two services",
	}, {
		args: []string{"rk"},
		err:  "a relation must involve two services",
	}, {
		args: []string{"rk:ring"},
		err:  "a relation must involve two services",
	}, {
		args: []string{"ping:pong", "tic:tac", "icki:wacki"},
		err:  "a relation must involve two services",
	},

	// Add a real relation, and check various ways of failing to re-add it.
	{
		args: []string{"ms", "wp"},
	}, {
		args: []string{"ms", "wp"},
		err:  msWpAlreadyExists,
	}, {
		args: []string{"wp", "ms"},
		err:  msWpAlreadyExists,
	}, {
		args: []string{"ms", "wp:db"},
		err:  msWpAlreadyExists,
	}, {
		args: []string{"ms:server", "wp"},
		err:  msWpAlreadyExists,
	}, {
		args: []string{"ms:server", "wp:db"},
		err:  msWpAlreadyExists,
	},

	// Add a real relation using an implicit endpoint.
	{
		args: []string{"ms", "lg"},
	}, {
		args: []string{"ms", "lg"},
		err:  msLgAlreadyExists,
	}, {
		args: []string{"lg", "ms"},
		err:  msLgAlreadyExists,
	}, {
		args: []string{"ms:juju-info", "lg"},
		err:  msLgAlreadyExists,
	}, {
		args: []string{"ms", "lg:info"},
		err:  msLgAlreadyExists,
	}, {
		args: []string{"ms:juju-info", "lg:info"},
		err:  msLgAlreadyExists,
	},

	// Add a real relation using an explicit endpoint, avoiding the potential implicit one.
	{
		args: []string{"wp", "lg"},
	}, {
		args: []string{"wp", "lg"},
		err:  wpLgAlreadyExists,
	}, {
		args: []string{"lg", "wp"},
		err:  wpLgAlreadyExists,
	}, {
		args: []string{"wp:logging-dir", "lg"},
		err:  wpLgAlreadyExists,
	}, {
		args: []string{"wp", "lg:logging-directory"},
		err:  wpLgAlreadyExists,
	}, {
		args: []string{"wp:logging-dir", "lg:logging-directory"},
		err:  wpLgAlreadyExists,
	},

	// Check we can still use the implicit endpoint if specified explicitly.
	{
		args: []string{"wp:juju-info", "lg"},
	}, {
		args: []string{"wp:juju-info", "lg"},
		err:  wpLgAlreadyExistsJuju,
	}, {
		args: []string{"lg", "wp:juju-info"},
		err:  wpLgAlreadyExistsJuju,
	}, {
		args: []string{"wp:juju-info", "lg"},
		err:  wpLgAlreadyExistsJuju,
	}, {
		args: []string{"wp", "lg:info"},
		err:  wpLgAlreadyExistsJuju,
	}, {
		args: []string{"wp:juju-info", "lg:info"},
		err:  wpLgAlreadyExistsJuju,
	},
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

	for i, t := range addRelationTests {
		c.Logf("test %d: %v", i, t.args)
		err := runAddRelation(c, t.args...)
		if t.err != "" {
			c.Assert(err, ErrorMatches, t.err)
		}
	}
}
