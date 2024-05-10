// Copyright 2023 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package bootstrap

import (
	"context"

	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/controller"
	schematesting "github.com/juju/juju/domain/schema/testing"
	"github.com/juju/juju/testing"
)

type bootstrapSuite struct {
	schematesting.ControllerSuite
}

var _ = gc.Suite(&bootstrapSuite{})

func (s *bootstrapSuite) TestInsertInitialControllerConfig(c *gc.C) {
	cfg := controller.Config{controller.CACertKey: testing.CACert}
	err := InsertInitialControllerConfig(cfg)(context.Background(), s.TxnRunner(), s.NoopTxnRunner())
	c.Assert(err, jc.ErrorIsNil)

	var cert string
	row := s.DB().QueryRow("SELECT value FROM controller_config where key = ?", controller.CACertKey)
	c.Assert(row.Scan(&cert), jc.ErrorIsNil)

	c.Check(cert, gc.Equals, testing.CACert)
}
