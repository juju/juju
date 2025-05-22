// Copyright 2023 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package bootstrap

import (
	"testing"

	"github.com/juju/tc"

	"github.com/juju/juju/cloud"
	coreuser "github.com/juju/juju/core/user"
	schematesting "github.com/juju/juju/domain/schema/testing"
)

type bootstrapSuite struct {
	schematesting.ControllerSuite
}

func TestBootstrapSuite(t *testing.T) {
	tc.Run(t, &bootstrapSuite{})
}

func (s *bootstrapSuite) TestInsertCloud(c *tc.C) {
	cld := cloud.Cloud{Name: "cirrus", Type: "ec2", AuthTypes: cloud.AuthTypes{cloud.UserPassAuthType}}
	err := InsertCloud(coreuser.AdminUserName, cld)(c.Context(), s.TxnRunner(), s.NoopTxnRunner())
	c.Assert(err, tc.ErrorIsNil)

	var name string
	row := s.DB().QueryRow("SELECT name FROM cloud where cloud_type_id = ?", 5) // 5 = ec2
	c.Assert(row.Scan(&name), tc.ErrorIsNil)
	c.Assert(name, tc.Equals, "cirrus")
}
