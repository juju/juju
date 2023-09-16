// Copyright 2017 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state_test

import (
	"fmt"
	"regexp"

	"github.com/juju/errors"
	"github.com/juju/mgo/v3/bson"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/state"
)

type relationNetworksSuite struct {
	ConnSuite
	relationNetworks state.RelationNetworker
	direction        string
	relation         *state.Relation
}

type relationIngressNetworksSuite struct {
	relationNetworksSuite
}

type relationEgressNetworksSuite struct {
	relationNetworksSuite
}

var _ = gc.Suite(&relationIngressNetworksSuite{})
var _ = gc.Suite(&relationEgressNetworksSuite{})

func (s *relationNetworksSuite) SetUpTest(c *gc.C) {
	s.ConnSuite.SetUpTest(c)
	wordpress := s.AddTestingApplication(c, "wordpress", s.AddTestingCharm(c, "wordpress"))
	wordpressEP, err := wordpress.Endpoint("db")
	c.Assert(err, jc.ErrorIsNil)
	mysql := s.AddTestingApplication(c, "mysql", s.AddTestingCharm(c, "mysql"))
	mysqlEP, err := mysql.Endpoint("server")
	c.Assert(err, jc.ErrorIsNil)
	s.relation, err = s.State.AddRelation(wordpressEP, mysqlEP)
	c.Assert(err, jc.ErrorIsNil)
}

func (s *relationIngressNetworksSuite) SetUpTest(c *gc.C) {
	s.relationNetworksSuite.SetUpTest(c)
	s.direction = "ingress"
	s.relationNetworks = state.NewRelationIngressNetworks(s.State)
}

func (s *relationEgressNetworksSuite) SetUpTest(c *gc.C) {
	s.relationNetworksSuite.SetUpTest(c)
	s.direction = "egress"
	s.relationNetworks = state.NewRelationEgressNetworks(s.State)
}

func (s *relationNetworksSuite) TestSaveMissingRelation(c *gc.C) {
	_, err := s.relationNetworks.Save("wordpress:db something:database", false, []string{"192.168.1.0/32"})
	c.Assert(err, gc.ErrorMatches, ".*"+regexp.QuoteMeta(`"wordpress:db something:database" not found`))
}

func (s *relationNetworksSuite) TestSaveInvalidAddress(c *gc.C) {
	_, err := s.relationNetworks.Save("wordpress:db mysql:server", false, []string{"192.168.1"})
	c.Assert(err, gc.ErrorMatches, regexp.QuoteMeta(`CIDR "192.168.1" not valid`))
}

func (s *relationNetworksSuite) assertSavedIngressInfo(c *gc.C, relationKey string, expectedCIDRS ...string) {
	coll, closer := state.GetCollection(s.State, "relationNetworks")
	defer closer()

	var raw bson.M
	err := coll.FindId(fmt.Sprintf("%v:%v:default", relationKey, s.direction)).One(&raw)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(raw["_id"], gc.Equals, fmt.Sprintf("%v:%v:%v:default", s.State.ModelUUID(), relationKey, s.direction))
	var cidrs []string
	for _, m := range raw["cidrs"].([]interface{}) {
		cidrs = append(cidrs, m.(string))
	}
	c.Assert(cidrs, jc.SameContents, expectedCIDRS)
}

func (s *relationNetworksSuite) TestSave(c *gc.C) {
	rin, err := s.relationNetworks.Save("wordpress:db mysql:server", false, []string{"192.168.1.0/16"})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(rin.RelationKey(), gc.Equals, "wordpress:db mysql:server")
	c.Assert(rin.CIDRS(), jc.DeepEquals, []string{"192.168.1.0/16"})
	s.assertSavedIngressInfo(c, "wordpress:db mysql:server", "192.168.1.0/16")
}

func (s *relationNetworksSuite) TestSaveAdminOverrides(c *gc.C) {
	_, err := s.relationNetworks.Save("wordpress:db mysql:server", false, []string{"192.168.1.0/16"})
	c.Assert(err, jc.ErrorIsNil)
	rin, err := s.relationNetworks.Save("wordpress:db mysql:server", true, []string{"10.2.0.0/16"})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(rin.RelationKey(), gc.Equals, "wordpress:db mysql:server")
	c.Assert(rin.CIDRS(), jc.DeepEquals, []string{"10.2.0.0/16"})
	result, err := s.relationNetworks.Networks("wordpress:db mysql:server")
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(result.CIDRS(), jc.DeepEquals, []string{"10.2.0.0/16"})
}

func (s *relationNetworksSuite) TestSaveIdempotent(c *gc.C) {
	_, err := s.relationNetworks.Save("wordpress:db mysql:server", false, []string{"192.168.1.0/16"})
	c.Assert(err, jc.ErrorIsNil)
	rin, err := s.relationNetworks.Save("wordpress:db mysql:server", false, []string{"192.168.1.0/16"})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(rin.RelationKey(), gc.Equals, "wordpress:db mysql:server")
	c.Assert(rin.CIDRS(), jc.DeepEquals, []string{"192.168.1.0/16"})
	s.assertSavedIngressInfo(c, "wordpress:db mysql:server", "192.168.1.0/16")
}

func (s *relationNetworksSuite) TestNetworks(c *gc.C) {
	_, err := s.relationNetworks.Save("wordpress:db mysql:server", false, []string{"192.168.1.0/16"})
	c.Assert(err, jc.ErrorIsNil)
	result, err := s.relationNetworks.Networks("wordpress:db mysql:server")
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(result.CIDRS(), gc.DeepEquals, []string{"192.168.1.0/16"})
	_, err = s.relationNetworks.Networks("mediawiki:db mysql:server")
	c.Assert(err, jc.ErrorIs, errors.NotFound)
}

func (s *relationNetworksSuite) TestUpdateCIDRs(c *gc.C) {
	_, err := s.relationNetworks.Save("wordpress:db mysql:server", false, []string{"192.168.1.0/16"})
	c.Assert(err, jc.ErrorIsNil)
	_, err = s.relationNetworks.Save("wordpress:db mysql:server", false, []string{"10.0.0.1/16"})
	c.Assert(err, jc.ErrorIsNil)
	s.assertSavedIngressInfo(c, "wordpress:db mysql:server", "10.0.0.1/16")
}

func (s *relationIngressNetworksSuite) TestCrossContanination(c *gc.C) {
	_, err := s.relationNetworks.Save("wordpress:db mysql:server", false, []string{"192.168.1.0/16"})
	c.Assert(err, jc.ErrorIsNil)
	egress := state.NewRelationEgressNetworks(s.State)
	_, err = egress.Networks(s.relation.Tag().Id())
	c.Assert(err, jc.ErrorIs, errors.NotFound)
}

func (s *relationEgressNetworksSuite) TestCrossContanination(c *gc.C) {
	_, err := s.relationNetworks.Save("wordpress:db mysql:server", false, []string{"192.168.1.0/16"})
	c.Assert(err, jc.ErrorIsNil)
	ingress := state.NewRelationIngressNetworks(s.State)
	_, err = ingress.Networks(s.relation.Tag().Id())
	c.Assert(err, jc.ErrorIs, errors.NotFound)
}

type relationRootNetworksSuite struct {
	relationNetworksSuite
}

var _ = gc.Suite(&relationRootNetworksSuite{})

func (s *relationRootNetworksSuite) SetUpTest(c *gc.C) {
	s.ConnSuite.SetUpTest(c)
	wordpress := s.AddTestingApplication(c, "wordpress", s.AddTestingCharm(c, "wordpress"))
	wordpressEP, err := wordpress.Endpoint("db")
	c.Assert(err, jc.ErrorIsNil)
	mysql := s.AddTestingApplication(c, "mysql", s.AddTestingCharm(c, "mysql"))
	mysqlEP, err := mysql.Endpoint("server")
	c.Assert(err, jc.ErrorIsNil)
	s.relation, err = s.State.AddRelation(wordpressEP, mysqlEP)
	c.Assert(err, jc.ErrorIsNil)

	s.direction = "ingress"
	s.relationNetworks = state.NewRelationIngressNetworks(s.State)
}

func (s *relationRootNetworksSuite) TestAllRelationNetworks(c *gc.C) {
	s.relationNetworks.Save("wordpress:db mysql:server", false, []string{"192.168.1.0/16"})

	relationNetworks := state.NewRelationNetworks(s.State)
	relations, err := relationNetworks.AllRelationNetworks()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(relations, gc.HasLen, 1)
	s.assertSavedIngressInfo(c, "wordpress:db mysql:server", "192.168.1.0/16")
}
