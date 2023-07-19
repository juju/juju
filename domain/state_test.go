// Copyright 2033 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package domain

import (
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/database/testing"
	schematesting "github.com/juju/juju/domain/schema/testing"
)

type stateSuite struct {
	schematesting.ControllerSuite
}

var _ = gc.Suite(&stateSuite{})

func (s *stateSuite) TestStateBaseGetDB(c *gc.C) {
	f := testing.TxnRunnerFactory(s.TxnRunner())
	base := NewStateBase(f)
	db, err := base.DB()
	c.Assert(err, gc.IsNil)
	c.Assert(db, gc.NotNil)
}

func (s *stateSuite) TestStateBaseGetDBNilFactory(c *gc.C) {
	base := NewStateBase(nil)
	_, err := base.DB()
	c.Assert(err, gc.ErrorMatches, `nil getDB`)
}

func (s *stateSuite) TestStateBaseGetDBNilDB(c *gc.C) {
	f := testing.TxnRunnerFactory(nil)
	base := NewStateBase(f)
	_, err := base.DB()
	c.Assert(err, gc.ErrorMatches, `invoking getDB: nil db`)
}
