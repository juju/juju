// Copyright 2018 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package model_test

import (
	"github.com/juju/errors"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/core/model"
	"github.com/juju/juju/testing"
)

type upgradeSeriesSuite struct {
	testing.BaseSuite
}

var _ = gc.Suite(&upgradeSeriesSuite{})

func (*upgradeSeriesSuite) TestValidateUnitUpgradeSeriesStatus(c *gc.C) {
	for _, t := range []struct {
		expected model.UpgradeSeriesStatus
		name     string
		valid    bool
	}{
		{model.UpgradeSeriesPrepareStarted, "prepare started", true},
		{model.UpgradeSeriesNotStarted, "GTFO", false},
	} {
		status, err := model.ValidateUpgradeSeriesStatus(model.UpgradeSeriesStatus(t.name))
		if t.valid {
			c.Check(err, jc.ErrorIsNil)
		} else {
			c.Check(err, jc.Satisfies, errors.IsNotValid)
		}
		c.Check(status, gc.Equals, t.expected)
	}
}

func (*upgradeSeriesSuite) TestUpgradeSeriesStatusOrderComparison(c *gc.C) {
	for status1, i := range model.UpgradeSeriesStatusOrder {
		for status2, j := range model.UpgradeSeriesStatusOrder {
			comp, err := model.CompareUpgradeSeriesStatus(status1, status2)
			c.Check(err, jc.ErrorIsNil)
			if status1 == status2 {
				c.Check(comp, gc.Equals, 0)
			} else {
				sig := i - j
				c.Check(sameSign(comp, sig), jc.IsTrue)
			}
		}
	}
}

func (*upgradeSeriesSuite) TestUpgradeSeriesStatusOrderComparisonVlidatesStatuses(c *gc.C) {
	_, err := model.CompareUpgradeSeriesStatus(model.UpgradeSeriesNotStarted, "bad status")
	c.Check(err.Error(), gc.Equals, "upgrade series status of \"bad status\" is not valid")
}

func sameSign(x, y int) bool {
	return (x >= 0) != (y < 0)
}
