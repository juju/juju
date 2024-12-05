// Copyright 2023 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package bootstrap

import (
	"context"

	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/controller"
	coreerrors "github.com/juju/juju/core/errors"
	coremodel "github.com/juju/juju/core/model"
	schematesting "github.com/juju/juju/domain/schema/testing"
	"github.com/juju/juju/internal/testing"
)

type bootstrapSuite struct {
	schematesting.ControllerSuite
}

var _ = gc.Suite(&bootstrapSuite{})

func (s *bootstrapSuite) TestInsertInitialControllerConfig(c *gc.C) {
	cfg := controller.Config{
		controller.CACertKey:         testing.CACert,
		controller.ControllerUUIDKey: testing.ControllerTag.Id(),
	}
	modelUUID, err := coremodel.NewUUID()
	c.Assert(err, gc.IsNil)
	err = InsertInitialControllerConfig(cfg, modelUUID)(context.Background(), s.TxnRunner(), s.NoopTxnRunner())
	c.Assert(err, jc.ErrorIsNil)

	var cert string
	row := s.DB().QueryRow("SELECT value FROM controller_config where key = ?", controller.CACertKey)
	c.Assert(row.Scan(&cert), jc.ErrorIsNil)

	c.Check(cert, gc.Equals, testing.CACert)

	var dbUUID, dbModelUUID string
	row = s.DB().QueryRow("SELECT uuid, model_uuid FROM controller")
	c.Assert(row.Scan(&dbUUID, &dbModelUUID), jc.ErrorIsNil)

	c.Check(dbModelUUID, gc.Equals, modelUUID.String())
	c.Check(dbUUID, gc.Equals, testing.ControllerTag.Id())
}

func (s *bootstrapSuite) TestValidModelUUID(c *gc.C) {
	cfg := controller.Config{controller.CACertKey: testing.CACert}
	err := InsertInitialControllerConfig(cfg, coremodel.UUID("bad-uuid"))(context.Background(), s.TxnRunner(), s.NoopTxnRunner())
	c.Assert(err, jc.ErrorIs, coreerrors.NotValid)
}

func (s *bootstrapSuite) TestInsertMinimalControllerConfig(c *gc.C) {
	cfg := controller.Config{}
	modelUUID, err := coremodel.NewUUID()
	c.Assert(err, gc.IsNil)
	err = InsertInitialControllerConfig(cfg, modelUUID)(context.Background(), s.TxnRunner(), s.NoopTxnRunner())
	c.Assert(err, gc.ErrorMatches, "no controller config values to insert at bootstrap")
}
