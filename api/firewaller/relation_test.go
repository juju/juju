// Copyright 2017 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package firewaller_test

import (
	"github.com/juju/names/v4"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/api/firewaller"
	"github.com/juju/juju/apiserver/params"
	"github.com/juju/juju/core/life"
)

type relationSuite struct {
	firewallerSuite

	apiRelation *firewaller.Relation
}

var _ = gc.Suite(&relationSuite{})

func (s *relationSuite) SetUpTest(c *gc.C) {
	s.firewallerSuite.SetUpTest(c)

	var err error
	s.apiRelation, err = s.firewaller.Relation(s.relations[0].Tag().(names.RelationTag))
	c.Assert(err, jc.ErrorIsNil)
}

func (s *relationSuite) TearDownTest(c *gc.C) {
	s.firewallerSuite.TearDownTest(c)
}

func (s *relationSuite) TestRelation(c *gc.C) {
	_, err := s.firewaller.Relation(names.NewRelationTag("foo:db bar:db"))
	c.Assert(err, gc.ErrorMatches, `relation "foo:db bar:db" not found`)
	c.Assert(err, jc.Satisfies, params.IsCodeNotFound)

	apiRelation0, err := s.firewaller.Relation(s.relations[0].Tag().(names.RelationTag))
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(apiRelation0, gc.NotNil)
}

func (s *relationSuite) TestTag(c *gc.C) {
	c.Assert(s.apiRelation.Tag(), gc.Equals, names.NewRelationTag(s.relations[0].String()))
}

func (s *relationSuite) TestLife(c *gc.C) {
	c.Assert(s.apiRelation.Life(), gc.Equals, life.Alive)
}
