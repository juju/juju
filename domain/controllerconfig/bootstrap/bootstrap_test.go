// Copyright 2023 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package bootstrap

import (
	stdtesting "testing"

	"github.com/juju/tc"

	"github.com/juju/juju/controller"
	coreerrors "github.com/juju/juju/core/errors"
	coremodel "github.com/juju/juju/core/model"
	jujuversion "github.com/juju/juju/core/version"
	schematesting "github.com/juju/juju/domain/schema/testing"
	"github.com/juju/juju/internal/testing"
)

type bootstrapSuite struct {
	schematesting.ControllerSuite
}

func TestBootstrapSuite(t *stdtesting.T) {
	tc.Run(t, &bootstrapSuite{})
}

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

	var dbUUID, dbModelUUID, dbTargetVersion string
	row = s.DB().QueryRow("SELECT uuid, model_uuid, target_version FROM controller")
	c.Assert(row.Scan(&dbUUID, &dbModelUUID, &dbTargetVersion), tc.ErrorIsNil)

	c.Check(dbModelUUID, tc.Equals, modelUUID.String())
	c.Check(dbUUID, tc.Equals, testing.ControllerTag.Id())
	c.Check(dbTargetVersion, tc.Equals, jujuversion.Current.String())
}

func (s *bootstrapSuite) TestInsertInitialControllerConfigAPIPort(c *tc.C) {
	cfg := controller.Config{
		controller.CACertKey:         testing.CACert,
		controller.ControllerUUIDKey: testing.ControllerTag.Id(),
		controller.APIPort:           "17070",
	}
	modelUUID, err := coremodel.NewUUID()
	c.Assert(err, tc.IsNil)
	err = InsertInitialControllerConfig(cfg, modelUUID)(c.Context(), s.TxnRunner(), s.NoopTxnRunner())
	c.Assert(err, tc.ErrorIsNil)

	var count int
	row := s.DB().QueryRow("SELECT COUNT(*) FROM controller_config where key = ?", controller.APIPort)
	c.Assert(row.Scan(&count), tc.ErrorIsNil)
	c.Check(count, tc.Equals, 0)

	var apiPort string
	row = s.DB().QueryRow("SELECT api_port FROM controller")
	c.Assert(row.Scan(&apiPort), tc.ErrorIsNil)

	c.Check(apiPort, tc.Equals, "17070")
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
