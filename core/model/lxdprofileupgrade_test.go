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

type lxdProfileUpgradeSuite struct {
	testing.BaseSuite
}

var _ = gc.Suite(&lxdProfileUpgradeSuite{})

func (*lxdProfileUpgradeSuite) TestValidateLXDProfileUpgradeStatus(c *gc.C) {
	for _, t := range []struct {
		expected model.LXDProfileUpgradeStatus
		name     string
		valid    bool
	}{
		{model.LXDProfileUpgradeCompleted, "completed", true},
		{model.LXDProfileUpgradeNotStarted, "GTFO", false},
	} {
		status, err := model.ValidateLXDProfileUpgradeStatus(model.LXDProfileUpgradeStatus(t.name))
		if t.valid {
			c.Check(err, jc.ErrorIsNil)
		} else {
			c.Check(err, jc.Satisfies, errors.IsNotValid)
		}
		c.Check(status, gc.Equals, t.expected)
	}
}
