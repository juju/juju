// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package charms_test

import (
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	basetesting "github.com/juju/juju/api/base/testing"
	"github.com/juju/juju/api/charms"
	"github.com/juju/juju/apiserver/params"
	jujutesting "github.com/juju/juju/juju/testing"
	coretesting "github.com/juju/juju/testing"
	"github.com/juju/juju/testing/factory"
)

type charmsMockSuite struct {
	coretesting.BaseSuite
	charmsClient *charms.Client
}

var _ = gc.Suite(&charmsMockSuite{})

func (s *charmsMockSuite) TestCharmInfo(c *gc.C) {
	var called bool
	curl := "local:quantal/dummy-1"

	apiCaller := basetesting.APICallerFunc(
		func(objType string,
			version int,
			id, request string,
			a, result interface{},
		) error {
			called = true
			c.Check(objType, gc.Equals, "Charms")
			c.Check(id, gc.Equals, "")
			c.Check(request, gc.Equals, "CharmInfo")

			args, ok := a.(params.CharmInfo)
			c.Assert(ok, jc.IsTrue)

			c.Assert(args.CharmURL, gc.DeepEquals, curl)
			return nil
		})
	charmsClient := charms.NewClient(apiCaller)
	_, err := charmsClient.CharmInfo(curl)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(called, jc.IsTrue)
}

func (s *charmsMockSuite) TestList(c *gc.C) {
	var called bool
	curl := "local:quantal/dummy-1"

	apiCaller := basetesting.APICallerFunc(
		func(objType string,
			version int,
			id, request string,
			a, result interface{},
		) error {
			called = true
			c.Check(objType, gc.Equals, "Charms")
			c.Check(id, gc.Equals, "")
			c.Check(request, gc.Equals, "List")

			args, ok := a.(params.CharmsList)
			c.Assert(ok, jc.IsTrue)

			c.Assert(args.Names, gc.HasLen, 1)
			c.Assert(args.Names[0], gc.DeepEquals, curl)
			return nil
		})
	charmsClient := charms.NewClient(apiCaller)
	_, err := charmsClient.List([]string{curl})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(called, jc.IsTrue)
}

type charmsSuite struct {
	jujutesting.JujuConnSuite
	charmsClient *charms.Client
}

var _ = gc.Suite(&charmsSuite{})

func (s *charmsSuite) SetUpTest(c *gc.C) {
	s.JujuConnSuite.SetUpTest(c)
	s.charmsClient = charms.NewClient(s.APIState)
	c.Assert(s.charmsClient, gc.NotNil)
}

func (s *charmsSuite) TearDownTest(c *gc.C) {
	s.charmsClient.ClientFacade.Close()
	s.JujuConnSuite.TearDownTest(c)
}

func (s *charmsSuite) TestCharmsFacadeCall(c *gc.C) {
	s.Factory.MakeCharm(c, &factory.CharmParams{Name: "wordpress"})

	found, err := s.charmsClient.List([]string{"wordpress"})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(found, gc.HasLen, 1)
	c.Assert(found[0], gc.DeepEquals, "cs:quantal/wordpress-1")
}
