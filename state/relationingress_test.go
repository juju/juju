// Copyright 2017 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state_test

import (
	"fmt"
	"regexp"

	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"
	"gopkg.in/mgo.v2/bson"

	"github.com/juju/juju/state"
)

type relationIngressSuite struct {
	ConnSuite
	relationIngressNetworks state.RelationIngressNetworks
}

var _ = gc.Suite(&relationIngressSuite{})

func (s *relationIngressSuite) SetUpTest(c *gc.C) {
	s.ConnSuite.SetUpTest(c)
	s.relationIngressNetworks = state.NewRelationIngressNetworks(s.State)
	wordpress := s.AddTestingService(c, "wordpress", s.AddTestingCharm(c, "wordpress"))
	wordpressEP, err := wordpress.Endpoint("db")
	c.Assert(err, jc.ErrorIsNil)
	mysql := s.AddTestingService(c, "mysql", s.AddTestingCharm(c, "mysql"))
	mysqlEP, err := mysql.Endpoint("server")
	c.Assert(err, jc.ErrorIsNil)
	_, err = s.State.AddRelation(wordpressEP, mysqlEP)
	c.Assert(err, jc.ErrorIsNil)
}

func (s *relationIngressSuite) TestSaveMissingRelation(c *gc.C) {
	_, err := s.relationIngressNetworks.Save("wordpress:db something:database", []string{"192.168.1.0/32"})
	c.Assert(err, gc.ErrorMatches, ".*"+regexp.QuoteMeta(`"wordpress:db something:database" not found`))
}

func (s *relationIngressSuite) TestSaveInvalidAddress(c *gc.C) {
	_, err := s.relationIngressNetworks.Save("wordpress:db mysql:server", []string{"192.168.1"})
	c.Assert(err, gc.ErrorMatches, regexp.QuoteMeta(`CIDR "192.168.1" not valid`))
}

func (s *relationIngressSuite) assertSavedIngressInfo(c *gc.C, relationKey string, expectedCIDRS ...string) {
	coll, closer := state.GetCollection(s.State, "relationIngress")
	defer closer()

	var raw bson.M
	err := coll.FindId(relationKey).One(&raw)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(raw["_id"], gc.Equals, fmt.Sprintf("%v:%v", s.State.ModelUUID(), relationKey))
	var cidrs []string
	for _, m := range raw["cidrs"].([]interface{}) {
		cidrs = append(cidrs, m.(string))
	}
	c.Assert(cidrs, jc.SameContents, expectedCIDRS)
}

func (s *relationIngressSuite) TestSave(c *gc.C) {
	rin, err := s.relationIngressNetworks.Save("wordpress:db mysql:server", []string{"192.168.1.0/16"})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(rin.RelationKey(), gc.Equals, "wordpress:db mysql:server")
	c.Assert(rin.CIDRS(), jc.DeepEquals, []string{"192.168.1.0/16"})
	s.assertSavedIngressInfo(c, "wordpress:db mysql:server", "192.168.1.0/16")
}

func (s *relationIngressSuite) TestSaveIdempotent(c *gc.C) {
	rin, err := s.relationIngressNetworks.Save("wordpress:db mysql:server", []string{"192.168.1.0/16"})
	c.Assert(err, jc.ErrorIsNil)
	rin, err = s.relationIngressNetworks.Save("wordpress:db mysql:server", []string{"192.168.1.0/16"})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(rin.RelationKey(), gc.Equals, "wordpress:db mysql:server")
	c.Assert(rin.CIDRS(), jc.DeepEquals, []string{"192.168.1.0/16"})
	s.assertSavedIngressInfo(c, "wordpress:db mysql:server", "192.168.1.0/16")
}

func (s *relationIngressSuite) TestUpdateCIDRs(c *gc.C) {
	_, err := s.relationIngressNetworks.Save("wordpress:db mysql:server", []string{"192.168.1.0/16"})
	c.Assert(err, jc.ErrorIsNil)
	_, err = s.relationIngressNetworks.Save("wordpress:db mysql:server", []string{"10.0.0.1/16"})
	c.Assert(err, jc.ErrorIsNil)
	s.assertSavedIngressInfo(c, "wordpress:db mysql:server", "10.0.0.1/16")
}
