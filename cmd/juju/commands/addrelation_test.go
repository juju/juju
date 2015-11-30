// Copyright 2012, 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package commands

import (
	"github.com/juju/cmd"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	jujutesting "github.com/juju/juju/juju/testing"
	"github.com/juju/juju/testcharms"
	"github.com/juju/juju/testing"
)

type AddRelationSuite struct {
	jujutesting.RepoSuite
	CmdBlockHelper
}

func (s *AddRelationSuite) SetUpTest(c *gc.C) {
	s.RepoSuite.SetUpTest(c)
	s.CmdBlockHelper = NewCmdBlockHelper(s.APIState)
	c.Assert(s.CmdBlockHelper, gc.NotNil)
	s.AddCleanup(func(*gc.C) { s.CmdBlockHelper.Close() })
}

var _ = gc.Suite(&AddRelationSuite{})

func runAddRelation(c *gc.C, args ...string) error {
	_, err := getContextFromAddRelationRun(c, args...)
	return err
}

func getContextFromAddRelationRun(c *gc.C, args ...string) (*cmd.Context, error) {
	return testing.RunCommand(c, newAddRelationCommand(), args...)
}

var msWpAlreadyExists = `cannot add relation "wp:db ms:server": relation already exists`
var msLgAlreadyExists = `cannot add relation "lg:info ms:juju-info": relation already exists`
var wpLgAlreadyExists = `cannot add relation "lg:logging-directory wp:logging-dir": relation already exists`
var wpLgAlreadyExistsJuju = `cannot add relation "lg:info wp:juju-info": relation already exists`

var addRelationTests = []struct {
	args   []string
	err    string
	output string
}{
	{
		args: []string{"rk", "ms"},
		err:  "no relations found",
	}, {
		err: "a relation must involve two service endpoints",
	}, {
		args: []string{"rk"},
		err:  "a relation must involve two service endpoints",
	}, {
		args: []string{"rk:ring"},
		err:  "a relation must involve two service endpoints",
	}, {
		args: []string{"ping:pong", "tic:tac", "icki:wacki"},
		err:  "a relation must involve two service endpoints",
	},

	// Add a real relation, and check various ways of failing to re-add it.
	{
		args: []string{"ms", "wp"},
		output: `
endpoints:
  ms:
    name: server
    role: provider
    interface: mysql
    optional: false
    limit: 0
    scope: global
  wp:
    name: db
    role: requirer
    interface: mysql
    optional: false
    limit: 1
    scope: global
`[1:],
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
		output: `
endpoints:
  lg:
    name: info
    role: requirer
    interface: juju-info
    optional: false
    limit: 1
    scope: container
  ms:
    name: juju-info
    role: provider
    interface: juju-info
    optional: false
    limit: 0
    scope: container
`[1:],
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
		output: `
endpoints:
  lg:
    name: logging-directory
    role: requirer
    interface: logging
    optional: false
    limit: 1
    scope: container
  wp:
    name: logging-dir
    role: provider
    interface: logging
    optional: false
    limit: 0
    scope: container
`[1:],
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
		output: `
endpoints:
  lg:
    name: info
    role: requirer
    interface: juju-info
    optional: false
    limit: 1
    scope: container
  wp:
    name: juju-info
    role: provider
    interface: juju-info
    optional: false
    limit: 0
    scope: container
`[1:],
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

func (s *AddRelationSuite) TestAddRelation(c *gc.C) {
	testcharms.Repo.CharmArchivePath(s.SeriesPath, "wordpress")
	err := runDeploy(c, "local:wordpress", "wp")
	c.Assert(err, jc.ErrorIsNil)
	testcharms.Repo.CharmArchivePath(s.SeriesPath, "mysql")
	err = runDeploy(c, "local:mysql", "ms")
	c.Assert(err, jc.ErrorIsNil)
	testcharms.Repo.CharmArchivePath(s.SeriesPath, "riak")
	err = runDeploy(c, "local:riak", "rk")
	c.Assert(err, jc.ErrorIsNil)
	testcharms.Repo.CharmArchivePath(s.SeriesPath, "logging")
	err = runDeploy(c, "local:logging", "lg")
	c.Assert(err, jc.ErrorIsNil)

	for i, t := range addRelationTests {
		c.Logf("test %d: %v", i, t.args)
		context, err := getContextFromAddRelationRun(c, t.args...)
		if t.err != "" {
			c.Assert(err, gc.ErrorMatches, t.err)
		}
		obtained := testing.Stdout(context)
		c.Assert(obtained, gc.Matches, t.output)
	}
}

func (s *AddRelationSuite) TestBlockAddRelation(c *gc.C) {
	testcharms.Repo.CharmArchivePath(s.SeriesPath, "wordpress")
	err := runDeploy(c, "local:wordpress", "wp")
	c.Assert(err, jc.ErrorIsNil)
	testcharms.Repo.CharmArchivePath(s.SeriesPath, "mysql")
	err = runDeploy(c, "local:mysql", "ms")
	c.Assert(err, jc.ErrorIsNil)
	testcharms.Repo.CharmArchivePath(s.SeriesPath, "riak")
	err = runDeploy(c, "local:riak", "rk")
	c.Assert(err, jc.ErrorIsNil)
	testcharms.Repo.CharmArchivePath(s.SeriesPath, "logging")
	err = runDeploy(c, "local:logging", "lg")
	c.Assert(err, jc.ErrorIsNil)

	// Block operation
	s.BlockAllChanges(c, "TestBlockAddRelation")

	for i, t := range addRelationTests {
		c.Logf("test %d: %v", i, t.args)
		err := runAddRelation(c, t.args...)
		if len(t.args) == 2 {
			// Only worry about Run being blocked.
			// For len(t.args) != 2, an Init will fail
			s.AssertBlocked(c, err, ".*TestBlockAddRelation.*")
		}
	}
}
