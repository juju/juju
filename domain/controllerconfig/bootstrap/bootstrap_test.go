// Copyright 2023 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package bootstrap

import (
	"github.com/juju/tc"

	"github.com/juju/juju/controller"
	coreerrors "github.com/juju/juju/core/errors"
	coremodel "github.com/juju/juju/core/model"
	schematesting "github.com/juju/juju/domain/schema/testing"
	"github.com/juju/juju/internal/testing"
)

type bootstrapSuite struct {
	schematesting.ControllerSuite
}

var _ = tc.Suite(&bootstrapSuite{})

func (s *bootstrapSuite) TestInsertInitialControllerConfig(c *tc.C) {
	cfg := controller.Config{
		controller.CACertKey:         testing.CACert,
		controller.ControllerUUIDKey: testing.ControllerTag.Id(),
	}
	modelUUID, err := coremodel.NewUUID()
	c.Assert(err, tc.IsNil)
	err = InsertInitialControllerConfig(cfg, modelUUID)(c.Context(), s.TxnRunner(), s.NoopTxnRunner())
	c.Assert(err, tc.ErrorIsNil)

	var cert string
	row := s.DB().QueryRow("SELECT value FROM controller_config where key = ?", controller.CACertKey)
	c.Assert(row.Scan(&cert), tc.ErrorIsNil)

	c.Check(cert, tc.Equals, testing.CACert)

	var dbUUID, dbModelUUID string
	row = s.DB().QueryRow("SELECT uuid, model_uuid FROM controller")
	c.Assert(row.Scan(&dbUUID, &dbModelUUID), tc.ErrorIsNil)

	c.Check(dbModelUUID, tc.Equals, modelUUID.String())
	c.Check(dbUUID, tc.Equals, testing.ControllerTag.Id())
}

func (s *bootstrapSuite) TestValidModelUUID(c *tc.C) {
	cfg := controller.Config{controller.CACertKey: testing.CACert}
	err := InsertInitialControllerConfig(cfg, coremodel.UUID("bad-uuid"))(c.Context(), s.TxnRunner(), s.NoopTxnRunner())
	c.Assert(err, tc.ErrorIs, coreerrors.NotValid)
}

func (s *bootstrapSuite) TestInsertMinimalControllerConfig(c *tc.C) {
	cfg := controller.Config{}
	modelUUID, err := coremodel.NewUUID()
	c.Assert(err, tc.IsNil)
	err = InsertInitialControllerConfig(cfg, modelUUID)(c.Context(), s.TxnRunner(), s.NoopTxnRunner())
	c.Assert(err, tc.ErrorMatches, "no controller config values to insert at bootstrap")
}
