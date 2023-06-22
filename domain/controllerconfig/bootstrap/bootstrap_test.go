// Copyright 2023 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package bootstrap

import (
	"context"

	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/controller"
	dbtesting "github.com/juju/juju/database/testing"
	"github.com/juju/juju/testing"
)

type bootstrapSuite struct {
	dbtesting.ControllerSuite
}

var _ = gc.Suite(&bootstrapSuite{})

func (s *bootstrapSuite) TestInsertInitialControllerConfig(c *gc.C) {
	cfg := controller.Config{controller.CACertKey: testing.CACert}
	err := InsertInitialControllerConfig(cfg)(context.Background(), s.TxnRunner())
	c.Assert(err, jc.ErrorIsNil)

	var cert string
	row := s.DB().QueryRow("SELECT value FROM CONTROLLER_CONFIG where key = ?", controller.CACertKey)
	c.Assert(row.Scan(&cert), jc.ErrorIsNil)

	c.Check(cert, gc.Equals, testing.CACert)
}
