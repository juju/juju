// Copyright 2023 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package bootstrap

import (
	"context"

	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/cloud"
	coreuser "github.com/juju/juju/core/user"
	clouderrors "github.com/juju/juju/domain/cloud/errors"
	"github.com/juju/juju/domain/cloud/state"
	schematesting "github.com/juju/juju/domain/schema/testing"
)

type bootstrapSuite struct {
	schematesting.ControllerSuite
}

var _ = gc.Suite(&bootstrapSuite{})

func (s *bootstrapSuite) TestInsertCloud(c *gc.C) {
	cld := cloud.Cloud{Name: "cirrus", Type: "ec2", AuthTypes: cloud.AuthTypes{cloud.UserPassAuthType}}
	err := InsertCloud(coreuser.AdminUserName, cld)(context.Background(), s.TxnRunner(), s.NoopTxnRunner())
	c.Assert(err, jc.ErrorIsNil)

	var name string
	row := s.DB().QueryRow("SELECT name FROM cloud where cloud_type_id = ?", 5) // 5 = ec2
	c.Assert(row.Scan(&name), jc.ErrorIsNil)
	c.Assert(name, gc.Equals, "cirrus")
}

// TestSetCloudDefaultsNoExist is check that if we try and set cloud defaults
// for a cloud that doesn't exist we get a [clouderrors.NotFound] error back
func (s *bootstrapSuite) TestSetCloudDefaultsNoExist(c *gc.C) {
	set := SetCloudDefaults("noexist", map[string]any{
		"HTTP_PROXY": "[2001:0DB8::1]:80",
	})

	err := set(context.Background(), s.TxnRunner(), s.NoopTxnRunner())
	c.Check(err, jc.ErrorIs, clouderrors.NotFound)

	var count int
	row := s.DB().QueryRow("SELECT count(*) FROM cloud_defaults")
	err = row.Scan(&count)
	c.Check(err, jc.ErrorIsNil)
	c.Check(count, gc.Equals, 0)
}

// TestSetCloudDefaults is testing the happy path for setting cloud defaults.
func (s *bootstrapSuite) TestSetCloudDefaults(c *gc.C) {
	cld := cloud.Cloud{
		Name:      "cirrus",
		Type:      "ec2",
		AuthTypes: cloud.AuthTypes{cloud.UserPassAuthType},
	}
	err := InsertCloud(coreuser.AdminUserName, cld)(context.Background(), s.TxnRunner(), s.NoopTxnRunner())
	c.Check(err, jc.ErrorIsNil)

	set := SetCloudDefaults("cirrus", map[string]any{
		"HTTP_PROXY": "[2001:0DB8::1]:80",
	})

	err = set(context.Background(), s.TxnRunner(), s.NoopTxnRunner())
	c.Check(err, jc.ErrorIsNil)

	st := state.NewState(s.TxnRunnerFactory())
	defaults, err := st.CloudDefaults(context.Background(), "cirrus")
	c.Check(err, jc.ErrorIsNil)
	c.Check(defaults, jc.DeepEquals, map[string]string{
		"HTTP_PROXY": "[2001:0DB8::1]:80",
	})
}

// TestSetCloudDefaultsOverrides is testing that repeated calls to
// [SetCloudDefaults] overrides existing cloud defaults that have been set.
func (s *bootstrapSuite) TestSetCloudDefaultsOverides(c *gc.C) {
	cld := cloud.Cloud{
		Name:      "cirrus",
		Type:      "ec2",
		AuthTypes: cloud.AuthTypes{cloud.UserPassAuthType},
	}
	err := InsertCloud(coreuser.AdminUserName, cld)(context.Background(), s.TxnRunner(), s.NoopTxnRunner())
	c.Check(err, jc.ErrorIsNil)

	set := SetCloudDefaults("cirrus", map[string]any{
		"HTTP_PROXY": "[2001:0DB8::1]:80",
	})

	err = set(context.Background(), s.TxnRunner(), s.NoopTxnRunner())
	c.Check(err, jc.ErrorIsNil)

	st := state.NewState(s.TxnRunnerFactory())
	defaults, err := st.CloudDefaults(context.Background(), "cirrus")
	c.Check(err, jc.ErrorIsNil)
	c.Check(defaults, jc.DeepEquals, map[string]string{
		"HTTP_PROXY": "[2001:0DB8::1]:80",
	})

	// Second time around

	set = SetCloudDefaults("cirrus", map[string]any{
		"foo": "bar",
	})

	err = set(context.Background(), s.TxnRunner(), s.NoopTxnRunner())
	c.Check(err, jc.ErrorIsNil)

	st = state.NewState(s.TxnRunnerFactory())
	defaults, err = st.CloudDefaults(context.Background(), "cirrus")
	c.Check(err, jc.ErrorIsNil)
	c.Check(defaults, jc.DeepEquals, map[string]string{
		"foo": "bar",
	})
}
