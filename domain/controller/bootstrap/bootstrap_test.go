// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package bootstrap

import (
	stdtesting "testing"

	"github.com/juju/tc"

	"github.com/juju/juju/controller"
	coremodel "github.com/juju/juju/core/model"
	controllerconfigbootstrap "github.com/juju/juju/domain/controllerconfig/bootstrap"
	schematesting "github.com/juju/juju/domain/schema/testing"
	"github.com/juju/juju/internal/testing"
)

type bootstrapSuite struct {
	schematesting.ControllerSuite
}

func TestBootstrapSuite(t *stdtesting.T) {
	tc.Run(t, &bootstrapSuite{})
}

func (s *bootstrapSuite) TestInsertInitialController(c *tc.C) {
	cfg := controller.Config{
		controller.CACertKey:         testing.CACert,
		controller.ControllerUUIDKey: testing.ControllerTag.Id(),
	}
	modelUUID, err := coremodel.NewUUID()
	c.Assert(err, tc.IsNil)
	err = controllerconfigbootstrap.InsertInitialControllerConfig(cfg, modelUUID)(c.Context(), s.TxnRunner(), s.NoopTxnRunner())

	err = InsertInitialController("a", "b", "c", "d")(c.Context(), s.TxnRunner(), s.NoopTxnRunner())
	c.Assert(err, tc.ErrorIsNil)

	var cert, pk, caPK, si string
	row := s.DB().QueryRow("SELECT cert, private_key, ca_private_key, system_identity FROM controller", controller.CACertKey)
	c.Assert(row.Scan(&cert, &pk, &caPK, &si), tc.ErrorIsNil)

	c.Check(cert, tc.Equals, "a")
	c.Check(pk, tc.Equals, "b")
	c.Check(caPK, tc.Equals, "c")
	c.Check(si, tc.Equals, "d")
}
