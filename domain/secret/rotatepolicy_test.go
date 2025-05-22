// Copyright 2024 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package secret

import (
	stdtesting "testing"

	"github.com/juju/tc"

	coresecrets "github.com/juju/juju/core/secrets"
	schematesting "github.com/juju/juju/domain/schema/testing"
)

type rotatePolicySuite struct {
	schematesting.ModelSuite
}

func TestRotatePolicySuite(t *stdtesting.T) {
	tc.Run(t, &rotatePolicySuite{})
}

// TestRotatePolicyDBValues ensures there's no skew between what's in the
// database table for rotatepolicy and the typed consts used in the state packages.
func (s *rotatePolicySuite) TestRotatePolicyDBValues(c *tc.C) {
	db := s.DB()
	rows, err := db.Query("SELECT id, policy FROM secret_rotate_policy")
	c.Assert(err, tc.ErrorIsNil)
	defer rows.Close()

	dbValues := make(map[RotatePolicy]string)
	for rows.Next() {
		var (
			id    int
			value string
		)
		err := rows.Scan(&id, &value)
		c.Assert(err, tc.ErrorIsNil)
		dbValues[RotatePolicy(id)] = value
	}
	c.Assert(dbValues, tc.DeepEquals, map[RotatePolicy]string{
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
		c.Assert(coresecrets.RotatePolicy(p).IsValid(), tc.IsTrue)
	}
}
