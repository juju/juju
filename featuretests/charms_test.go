// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package featuretests

import (
	"github.com/juju/cmd"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/api/charms"
	"github.com/juju/juju/cmd/envcmd"
	cmdcharms "github.com/juju/juju/cmd/juju/charms"
	jujutesting "github.com/juju/juju/juju/testing"
	"github.com/juju/juju/testing"
	"github.com/juju/juju/testing/factory"
)

type apiCharmsSuite struct {
	jujutesting.JujuConnSuite
	charmsClient *charms.Client
}

var _ = gc.Suite(&apiCharmsSuite{})

func (s *apiCharmsSuite) SetUpTest(c *gc.C) {
	s.JujuConnSuite.SetUpTest(c)
	s.charmsClient = charms.NewClient(s.APIState)
	c.Assert(s.charmsClient, gc.NotNil)
}

func (s *apiCharmsSuite) TearDownTest(c *gc.C) {
	s.charmsClient.ClientFacade.Close()
	s.JujuConnSuite.TearDownTest(c)
}

func (s *apiCharmsSuite) TestCharmsListFacadeCall(c *gc.C) {
	s.Factory.MakeCharm(c, &factory.CharmParams{
		Name: "wordpress",
		URL:  "cs:quantal/wordpress-1",
	})

	found, err := s.charmsClient.List([]string{"wordpress"})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(found, gc.HasLen, 1)
	c.Assert(found[0], gc.DeepEquals, "cs:quantal/wordpress-1")
}

func (s *apiCharmsSuite) TestCharmInfoFacadeCall(c *gc.C) {
	s.Factory.MakeCharm(c, &factory.CharmParams{
		Name: "wordpress",
		URL:  "cs:quantal/wordpress-1",
	})

	found, err := s.charmsClient.CharmInfo("cs:quantal/wordpress-1")
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(found.URL, gc.DeepEquals, "cs:quantal/wordpress-1")
}

type cmdCharmsSuite struct {
	jujutesting.RepoSuite
}

var _ = gc.Suite(&cmdCharmsSuite{})

func (s *cmdCharmsSuite) SetUpTest(c *gc.C) {
	s.RepoSuite.SetUpTest(c)
}

func runList(c *gc.C, args []string) *cmd.Context {
	context, err := testing.RunCommand(c, envcmd.Wrap(&cmdcharms.ListCommand{}), args...)
	c.Assert(err, jc.ErrorIsNil)
	return context
}

func (s *cmdCharmsSuite) TestCharmListAllCmdStack(c *gc.C) {
	s.Factory.MakeCharm(c, &factory.CharmParams{
		Name: "wordpress",
		URL:  "cs:quantal/wordpress-1",
	})
	s.Factory.MakeCharm(c, &factory.CharmParams{
		Name: "riak",
		URL:  "cs:quantal/riak-3",
	})

	context := runList(c, []string{})
	obtained := testing.Stdout(context)
	expected := "- cs:quantal/wordpress-1\n- cs:quantal/riak-3\n"
	c.Assert(obtained, gc.Equals, expected)
}

func (s *cmdCharmsSuite) TestCharmListNamesCmdStack(c *gc.C) {
	s.Factory.MakeCharm(c, &factory.CharmParams{
		Name: "wordpress",
		URL:  "cs:quantal/wordpress-1",
	})
	s.Factory.MakeCharm(c, &factory.CharmParams{Name: "riak"})

	context := runList(c, []string{"wordpress"})
	obtained := testing.Stdout(context)
	expected := "- cs:quantal/wordpress-1\n"
	c.Assert(obtained, gc.Equals, expected)
}
