// Copyright 2012, 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package commands

import (
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/apiserver/params"
	"github.com/juju/juju/cmd/envcmd"
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
	_, err := testing.RunCommand(c, newAddRelationCommand(), args...)
	return err
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
		err := runAddRelation(c, t.args...)
		if t.err != "" {
			c.Assert(err, gc.ErrorMatches, t.err)
		}
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

func (s *AddRelationSuite) TestAddRelationBothServicesRemote(c *gc.C) {
	err := runAddRelation(c, "local:/u/user/servicename1", "local:/u/user/servicename2")
	c.Assert(err, gc.ErrorMatches, "add-relation between 2 remote services not supported")
}

func (s *AddRelationSuite) getAddRelationAfterRun(c *gc.C, args ...string) addRelationCommand {
	cmd := addRelationCommand{}
	err := envcmd.Wrap(&cmd).Init(args)
	c.Assert(err, jc.ErrorIsNil)
	return cmd
}

func (s *AddRelationSuite) assertHasRemoteServices(c *gc.C, args ...string) {
	cmd := s.getAddRelationAfterRun(c, args...)
	c.Assert(cmd.HasRemoteService, jc.IsTrue)
}

func (s *AddRelationSuite) assertHasNoRemoteServices(c *gc.C, args ...string) {
	cmd := s.getAddRelationAfterRun(c, args...)
	c.Assert(cmd.HasRemoteService, jc.IsFalse)
}

func (s *AddRelationSuite) TestAddRelationHasRemoteServicesFirst(c *gc.C) {
	s.assertHasRemoteServices(c, "servicename", "local:/u/user/servicename2")
}

func (s *AddRelationSuite) TestAddRelationHasRemoteServicesSecond(c *gc.C) {
	s.assertHasRemoteServices(c, "local:/u/user/servicename2", "servicename")
}

func (s *AddRelationSuite) TestAddRelationHasRemoteServicesNone(c *gc.C) {
	s.assertHasNoRemoteServices(c, "servicename2", "servicename")
}

func (s *AddRelationSuite) TestAddRelationNotImplemented(c *gc.C) {
	mockAPI := &mockAddRelationAPI{
		assert: func(...string) {},
		err: &params.Error{
			Message: "AddRelation",
			Code:    params.CodeNotImplemented,
		},
	}
	addRelationCmd := &addRelationCommand{}
	addRelationCmd.newAPIFunc = func() (AddRelationAPI, error) {
		return mockAPI, nil
	}
	_, err := testing.RunCommand(c, envcmd.Wrap(addRelationCmd), "local:/u/user/servicename2", "servicename")
	c.Assert(err, gc.ErrorMatches, ".*not supported by the API server.*")
}

type mockAddRelationAPI struct {
	assert func(...string)
	err    error
}

func (t *mockAddRelationAPI) AddRelation(endpoints ...string) (*params.AddRelationResults, error) {
	t.assert(endpoints...)
	return nil, t.err
}

func (t *mockAddRelationAPI) Close() error {
	return nil
}
