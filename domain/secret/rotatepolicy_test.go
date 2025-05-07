// Copyright 2024 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package secret

import (
	"github.com/juju/tc"
	jc "github.com/juju/testing/checkers"

	coresecrets "github.com/juju/juju/core/secrets"
	schematesting "github.com/juju/juju/domain/schema/testing"
)

type rotatePolicySuite struct {
	schematesting.ModelSuite
}

var _ = tc.Suite(&rotatePolicySuite{})

// TestRotatePolicyDBValues ensures there's no skew between what's in the
// database table for rotatepolicy and the typed consts used in the state packages.
func (s *rotatePolicySuite) TestRotatePolicyDBValues(c *tc.C) {
	db := s.DB()
	rows, err := db.Query("SELECT id, policy FROM secret_rotate_policy")
	c.Assert(err, jc.ErrorIsNil)
	defer rows.Close()

	dbValues := make(map[RotatePolicy]string)
	for rows.Next() {
		var (
			id    int
			value string
		)
		err := rows.Scan(&id, &value)
		c.Assert(err, jc.ErrorIsNil)
		dbValues[RotatePolicy(id)] = value
	}
	c.Assert(dbValues, jc.DeepEquals, map[RotatePolicy]string{
		RotateNever:     "never",
		RotateHourly:    "hourly",
		RotateDaily:     "daily",
		RotateWeekly:    "weekly",
		RotateMonthly:   "monthly",
		RotateQuarterly: "quarterly",
		RotateYearly:    "yearly",
	})
	// Also check the core secret enums match.
	for _, p := range dbValues {
		c.Assert(coresecrets.RotatePolicy(p).IsValid(), jc.IsTrue)
	}
}
