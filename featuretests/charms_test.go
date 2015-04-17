// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package featuretests

import (
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/api/charms"
	jujutesting "github.com/juju/juju/juju/testing"
	"github.com/juju/juju/testing/factory"
)

type apiCharmsSuite struct {
	jujutesting.JujuConnSuite
	charmsClient *charms.Client
}

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
