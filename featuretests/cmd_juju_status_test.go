// Copyright 2018 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package featuretests

import (
	"github.com/juju/cmd"
	"github.com/juju/cmd/cmdtesting"
	"github.com/juju/loggo"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/cmd/juju/commands"
	jujutesting "github.com/juju/juju/juju/testing"
	"github.com/juju/juju/state"
	"github.com/juju/juju/testing/factory"
)

type StatusSuite struct {
	jujutesting.JujuConnSuite
}

func (s *StatusSuite) SetUpTest(c *gc.C) {
	s.JujuConnSuite.SetUpTest(c)

	// make an application with 2 endpoints
	application1 := s.Factory.MakeApplication(c, &factory.ApplicationParams{
		Charm: s.Factory.MakeCharm(c, &factory.CharmParams{
			Name: "wordpress",
		}),
	})
	endpoint1, err := application1.Endpoint("juju-info")
	c.Assert(err, jc.ErrorIsNil)
	endpoint2, err := application1.Endpoint("logging-dir")
	c.Assert(err, jc.ErrorIsNil)

	// make another application with 2 endpoints
	application2 := s.Factory.MakeApplication(c, &factory.ApplicationParams{
		Charm: s.Factory.MakeCharm(c, &factory.CharmParams{
			Name: "logging",
		}),
	})
	endpoint3, err := application2.Endpoint("info")
	c.Assert(err, jc.ErrorIsNil)
	endpoint4, err := application2.Endpoint("logging-directory")
	c.Assert(err, jc.ErrorIsNil)

	// create relation between a1:e1 and a2:e3
	relation1 := s.Factory.MakeRelation(c, &factory.RelationParams{
		Endpoints: []state.Endpoint{endpoint1, endpoint3},
	})
	c.Assert(relation1, gc.NotNil)

	// create relation between a1:e2 and a2:e4
	relation2 := s.Factory.MakeRelation(c, &factory.RelationParams{
		Endpoints: []state.Endpoint{endpoint2, endpoint4},
	})
	c.Assert(relation2, gc.NotNil)
}

func (s *StatusSuite) run(c *gc.C, args ...string) *cmd.Context {
	context := cmdtesting.Context(c)
	command := commands.NewJujuCommand(context)
	c.Assert(cmdtesting.InitCommand(command, args), jc.ErrorIsNil)
	c.Assert(command.Run(context), jc.ErrorIsNil)
	loggo.RemoveWriter("warning")
	return context
}

func (s *StatusSuite) TestMultipleRelationsInYamlFormat(c *gc.C) {
	context := s.run(c, "status", "--format=yaml")
	out := cmdtesting.Stdout(context)

	// expected relations for 'logging'
	c.Assert(out, jc.Contains, `
    relations:
      info:
      - wordpress
      logging-directory:
      - wordpress
    subordinate-to:
    - wordpress
`)
	// expected relations for 'wordpress'
	c.Assert(out, jc.Contains, `
    relations:
      juju-info:
      - logging
      logging-dir:
      - logging
`)
}

func (s *StatusSuite) TestMultipleRelationsInTabularFormat(c *gc.C) {
	context := s.run(c, "status", "--relations")
	c.Assert(cmdtesting.Stdout(context), jc.Contains, `
Relation provider      Requirer                   Interface  Type         Message
wordpress:juju-info    logging:info               juju-info  subordinate  joining  
wordpress:logging-dir  logging:logging-directory  logging    subordinate  joining  
`[1:])
}
