// Copyright 2033 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package domain

import (
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/core/database"
	"github.com/juju/juju/database/testing"
)

type dbFactorySuite struct {
	testing.ControllerSuite
}

var _ = gc.Suite(&dbFactorySuite{})

func (s *dbFactorySuite) TestDBFactoryNilGetter(c *gc.C) {
	factory := NamespaceDBFactory(nil, "foo")
	_, err := factory()
	c.Assert(err, gc.ErrorMatches, `nil getter`)
}

func (s *dbFactorySuite) TestStateBaseNilFactory(c *gc.C) {
	state := NewStateBase(nil)
	_, err := state.DB()
	c.Assert(err, gc.ErrorMatches, `nil getDB`)
}

func (s *dbFactorySuite) TestStateBase(c *gc.C) {
	count := &counter{}
	factory := NamespaceDBFactory(nsGetter(s.TrackedDB(), count), "foo")

	state := NewStateBase(factory)
	db, err := state.DB()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(db, gc.Equals, s.TrackedDB())
	c.Assert(count.count, gc.Equals, 1)
}

func (s *dbFactorySuite) TestStateBaseIsCached(c *gc.C) {
	count := &counter{}
	factory := NamespaceDBFactory(nsGetter(s.TrackedDB(), count), "foo")

	state := NewStateBase(factory)
	db, err := state.DB()
	c.Assert(err, jc.ErrorIsNil)

	c.Assert(db, gc.Equals, s.TrackedDB())
	c.Assert(count.count, gc.Equals, 1)

	c.Assert(db, gc.Equals, s.TrackedDB())
	c.Assert(count.count, gc.Equals, 1)
}

func nsGetter(db database.TrackedDB, c *counter) func(ns string) (database.TrackedDB, error) {
	return func(ns string) (database.TrackedDB, error) {
		c.Inc()
		return db, nil
	}
}

type counter struct {
	count int
}

func (c *counter) Inc() {
	c.count++
}
