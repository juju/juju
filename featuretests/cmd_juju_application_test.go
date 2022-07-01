// Copyright 2019 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package featuretests

import (
	"github.com/juju/cmd/v3/cmdtesting"
	"github.com/juju/juju/v2/cmd/juju/application"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	jujutesting "github.com/juju/juju/v2/juju/testing"
	"github.com/juju/juju/v2/state"
	"github.com/juju/juju/v2/testing/factory"
)

type cmdApplicationSuite struct {
	jujutesting.JujuConnSuite
}

func (s *cmdApplicationSuite) setupApplications(c *gc.C) {
	s.Factory.MakeSpace(c, &factory.SpaceParams{Name: "vlan2"})
	// make an application with 2 endpoints
	application1 := s.Factory.MakeApplication(c, &factory.ApplicationParams{
		Charm: s.Factory.MakeCharm(c, &factory.CharmParams{
			Name:     "wordpress",
			Revision: "23",
		}),
	})
	endpoint1, err := application1.Endpoint("juju-info")
	c.Assert(err, jc.ErrorIsNil)
	endpoint2, err := application1.Endpoint("logging-dir")
	c.Assert(err, jc.ErrorIsNil)

	// make another application with 2 endpoints
	application2 := s.Factory.MakeApplication(c, &factory.ApplicationParams{
		Charm: s.Factory.MakeCharm(c, &factory.CharmParams{
			Name:     "logging",
			Revision: "43",
		}),
		CharmConfig:      map[string]interface{}{"foo": "bar"},
		EndpointBindings: map[string]string{"info": "vlan2"},
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

func (s *cmdApplicationSuite) TestShowApplicationOne(c *gc.C) {
	s.setupApplications(c)

	ctx, err := cmdtesting.RunCommand(c, application.NewShowApplicationCommand(), "wordpress")
	c.Assert(err, jc.ErrorIsNil)
	output := cmdtesting.Stdout(ctx)
	c.Assert(output, gc.Equals, `
wordpress:
  charm: wordpress
  series: quantal
  constraints:
    arch: amd64
  principal: true
  exposed: false
  remote: false
  life: alive
  endpoint-bindings:
    "": alpha
    admin-api: alpha
    cache: alpha
    db: alpha
    db-client: alpha
    foo-bar: alpha
    logging-dir: alpha
    monitoring-port: alpha
    url: alpha
`[1:])
}

func (s *cmdApplicationSuite) TestShowApplicationManyYaml(c *gc.C) {
	s.setupApplications(c)

	ctx, err := cmdtesting.RunCommand(c, application.NewShowApplicationCommand(), "wordpress", "logging", "--format", "yaml")
	c.Assert(err, jc.ErrorIsNil)
	output := cmdtesting.Stdout(ctx)
	c.Assert(output, gc.Equals, `
logging:
  charm: logging
  series: quantal
  principal: false
  exposed: false
  remote: false
  life: alive
  endpoint-bindings:
    "": alpha
    info: vlan2
    logging-client: alpha
    logging-directory: alpha
wordpress:
  charm: wordpress
  series: quantal
  constraints:
    arch: amd64
  principal: true
  exposed: false
  remote: false
  life: alive
  endpoint-bindings:
    "": alpha
    admin-api: alpha
    cache: alpha
    db: alpha
    db-client: alpha
    foo-bar: alpha
    logging-dir: alpha
    monitoring-port: alpha
    url: alpha
`[1:])
}

func (s *cmdApplicationSuite) TestRemoveApplication(c *gc.C) {
	s.setupApplications(c)

	ctx, err := cmdtesting.RunCommand(c, application.NewRemoveApplicationCommand(), "wordpress")
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(cmdtesting.Stdout(ctx), gc.Equals, "")
	c.Assert(cmdtesting.Stderr(ctx), gc.Equals, "removing application wordpress\n")
}
