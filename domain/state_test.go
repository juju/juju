// Copyright 2033 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package domain

import (
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/database/testing"
)

type dbFactorySuite struct {
	testing.ControllerSuite
}

var _ = gc.Suite(&dbFactorySuite{})

func (s *dbFactorySuite) TestDBFactory(c *gc.C) {
	f := testing.TxnRunnerFactory(s.TxnRunner())
	db, err := f()
	c.Assert(err, gc.IsNil)
	c.Assert(db, gc.NotNil)
}

func (s *dbFactorySuite) TestStateBaseGetDB(c *gc.C) {
	f := testing.TxnRunnerFactory(s.TxnRunner())
	base := NewStateBase(f)
	db, err := base.DB()
	c.Assert(err, gc.IsNil)
	c.Assert(db, gc.NotNil)
}

func (s *dbFactorySuite) TestStateBaseGetDBNilFactory(c *gc.C) {
	base := NewStateBase(nil)
	_, err := base.DB()
	c.Assert(err, gc.ErrorMatches, `nil getDB`)

}

func (s *dbFactorySuite) TestStateBaseGetDBNilDB(c *gc.C) {
	f := testing.TxnRunnerFactory(nil)
	base := NewStateBase(f)
	_, err := base.DB()
	c.Assert(err, gc.ErrorMatches, `invoking getDB: nil db`)
}
