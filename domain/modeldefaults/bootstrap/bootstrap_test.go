// Copyright 2023 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package bootstrap

import (
	"context"
	"database/sql"
	"testing"

	"github.com/juju/tc"

	"github.com/juju/juju/cloud"
	corecloud "github.com/juju/juju/core/cloud"
	coreuser "github.com/juju/juju/core/user"
	cloudbootstrap "github.com/juju/juju/domain/cloud/bootstrap"
	clouderrors "github.com/juju/juju/domain/cloud/errors"
	"github.com/juju/juju/domain/modeldefaults/state"
	schematesting "github.com/juju/juju/domain/schema/testing"
	_ "github.com/juju/juju/internal/provider/dummy"
)

type bootstrapSuite struct {
	schematesting.ControllerSuite
}

func TestBootstrapSuite(t *testing.T) {
	tc.Run(t, &bootstrapSuite{})
}

func (*bootstrapSuite) TestBootstrapModelDefaults(c *tc.C) {
	provider := ModelDefaultsProvider(
		map[string]any{
			"foo":        "controller",
			"controller": "some value",
		},
		map[string]any{
			"foo":    "region",
			"region": "some value",
		},
		"dummy",
	)

	defaults, err := provider.ModelDefaults(c.Context())
	c.Check(err, tc.ErrorIsNil)
	c.Check(defaults["foo"].Region, tc.Equals, "region")
	c.Check(defaults["controller"].Controller, tc.Equals, "some value")
	c.Check(defaults["region"].Region, tc.Equals, "some value")

	configDefaults := state.ConfigDefaults(c.Context())
	for k, v := range configDefaults {
		c.Check(defaults[k].Default, tc.Equals, v)
	}
}

// TestSetCloudDefaultsNoExist asserts that if we try and set cloud defaults
// for a cloud that doesn't exist we get a [clouderrors.NotFound] error back.
func (s *bootstrapSuite) TestSetCloudDefaultsNoExist(c *tc.C) {
	set := SetCloudDefaults("noexist", map[string]any{
		"HTTP_PROXY": "[2001:0DB8::1]:80",
	})

	err := set(c.Context(), s.TxnRunner(), s.NoopTxnRunner())
	c.Check(err, tc.ErrorIs, clouderrors.NotFound)

	var count int
	row := s.DB().QueryRow("SELECT count(*) FROM cloud_defaults")
	err = row.Scan(&count)
	c.Check(err, tc.ErrorIsNil)
	c.Check(count, tc.Equals, 0)
}

// TestSetCloudDefaults is testing the happy path for setting cloud defaults. We
// expect no errors to be returned in this test and at the end of setting the
// clouds defaults for the same values to be reported back.
func (s *bootstrapSuite) TestSetCloudDefaults(c *tc.C) {
	cld := cloud.Cloud{
		Name:      "cirrus",
		Type:      "ec2",
		AuthTypes: cloud.AuthTypes{cloud.UserPassAuthType},
	}

	err := cloudbootstrap.InsertCloud(
		coreuser.AdminUserName, cld)(c.Context(), s.TxnRunner(), s.NoopTxnRunner())
	c.Check(err, tc.ErrorIsNil)

	set := SetCloudDefaults("cirrus", map[string]any{
		"HTTP_PROXY": "[2001:0DB8::1]:80",
	})

	err = set(c.Context(), s.TxnRunner(), s.NoopTxnRunner())
	c.Check(err, tc.ErrorIsNil)

	var cloudUUID string
	err = s.TxnRunner().StdTxn(c.Context(), func(ctx context.Context, tx *sql.Tx) error {
		return tx.QueryRowContext(ctx, "SELECT uuid FROM cloud WHERE name = ?", "cirrus").Scan(&cloudUUID)
	})
	c.Check(err, tc.ErrorIsNil)

	st := state.NewState(s.TxnRunnerFactory())
	defaults, err := st.CloudDefaults(c.Context(), corecloud.UUID(cloudUUID))
	c.Check(err, tc.ErrorIsNil)
	c.Check(defaults, tc.DeepEquals, map[string]string{
		"HTTP_PROXY": "[2001:0DB8::1]:80",
	})
}

// TestSetCloudDefaultsOverrides is testing that repeated calls to
// [SetCloudDefaults] overrides existing cloud defaults that have been set.
func (s *bootstrapSuite) TestSetCloudDefaultsOverides(c *tc.C) {
	cld := cloud.Cloud{
		Name:      "cirrus",
		Type:      "ec2",
		AuthTypes: cloud.AuthTypes{cloud.UserPassAuthType},
	}
	err := cloudbootstrap.InsertCloud(
		coreuser.AdminUserName,
		cld,
	)(c.Context(), s.TxnRunner(), s.NoopTxnRunner())
	c.Check(err, tc.ErrorIsNil)

	set := SetCloudDefaults("cirrus", map[string]any{
		"HTTP_PROXY": "[2001:0DB8::1]:80",
	})

	err = set(c.Context(), s.TxnRunner(), s.NoopTxnRunner())
	c.Check(err, tc.ErrorIsNil)

	var cloudUUID string
	err = s.TxnRunner().StdTxn(c.Context(), func(ctx context.Context, tx *sql.Tx) error {
		return tx.QueryRowContext(ctx, "SELECT uuid FROM cloud WHERE name = ?", "cirrus").Scan(&cloudUUID)
	})
	c.Check(err, tc.ErrorIsNil)

	st := state.NewState(s.TxnRunnerFactory())
	defaults, err := st.CloudDefaults(c.Context(), corecloud.UUID(cloudUUID))
	c.Check(err, tc.ErrorIsNil)
	c.Check(defaults, tc.DeepEquals, map[string]string{
		"HTTP_PROXY": "[2001:0DB8::1]:80",
	})

	// Second time around

	set = SetCloudDefaults("cirrus", map[string]any{
		"foo": "bar",
	})

	err = set(c.Context(), s.TxnRunner(), s.NoopTxnRunner())
	c.Check(err, tc.ErrorIsNil)

	defaults, err = st.CloudDefaults(c.Context(), corecloud.UUID(cloudUUID))
	c.Check(err, tc.ErrorIsNil)
	c.Check(defaults, tc.DeepEquals, map[string]string{
		"foo": "bar",
	})
}
