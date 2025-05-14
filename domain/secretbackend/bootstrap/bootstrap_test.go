// Copyright 2024 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package bootstrap

import (
	"github.com/juju/tc"

	coremodel "github.com/juju/juju/core/model"
	schematesting "github.com/juju/juju/domain/schema/testing"
)

type bootstrapSuite struct {
	schematesting.ControllerSuite
}

var _ = tc.Suite(&bootstrapSuite{})

func (s *bootstrapSuite) TestCreateDefaultBackendsIAAS(c *tc.C) {
	err := CreateDefaultBackends(coremodel.IAAS)(c.Context(), s.TxnRunner(), s.NoopTxnRunner())
	c.Assert(err, tc.ErrorIsNil)

	var (
		name   string
		typeID int
	)
	row := s.DB().QueryRow("SELECT name, backend_type_id FROM secret_backend where backend_type_id = ?", 0) // 0 = internal
	c.Assert(row.Scan(&name, &typeID), tc.ErrorIsNil)
	c.Assert(name, tc.Equals, "internal")
	c.Assert(typeID, tc.Equals, 0)
	row = s.DB().QueryRow("SELECT name, backend_type_id FROM secret_backend where backend_type_id = ?", 1) // 1 = kubernetes
	c.Assert(row.Scan(&name, &typeID), tc.ErrorIsNil)
	c.Assert(name, tc.Equals, "kubernetes")
	c.Assert(typeID, tc.Equals, 1)
}

func (s *bootstrapSuite) TestCreateDefaultBackendsCAAS(c *tc.C) {
	err := CreateDefaultBackends(coremodel.CAAS)(c.Context(), s.TxnRunner(), s.NoopTxnRunner())
	c.Assert(err, tc.ErrorIsNil)

	var (
		name   string
		typeID int
	)
	row := s.DB().QueryRow("SELECT name, backend_type_id FROM secret_backend where backend_type_id = ?", 0) // 0 = internal
	c.Assert(row.Scan(&name, &typeID), tc.ErrorIsNil)
	c.Assert(name, tc.Equals, "internal")
	c.Assert(typeID, tc.Equals, 0)
	row = s.DB().QueryRow("SELECT name, backend_type_id FROM secret_backend where backend_type_id = ?", 1) // 1 = kubernetes
	c.Assert(row.Scan(&name, &typeID), tc.ErrorIsNil)
	c.Assert(name, tc.Equals, "kubernetes")
	c.Assert(typeID, tc.Equals, 1)
}
