// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package modelmigration_test

import (
	"github.com/juju/errors"
	"github.com/juju/names"
	jc "github.com/juju/testing/checkers"
	"github.com/juju/utils"
	gc "gopkg.in/check.v1"

	migration "github.com/juju/juju/core/modelmigration"
	coretesting "github.com/juju/juju/testing"
)

type TargetInfoSuite struct {
	coretesting.BaseSuite
}

var _ = gc.Suite(new(TargetInfoSuite))

func (s *TargetInfoSuite) TestValidation(c *gc.C) {
	tests := []struct {
		label        string
		tweakInfo    func(*migration.TargetInfo)
		errorPattern string
	}{{
		"empty ControllerTag",
		func(info *migration.TargetInfo) {
			info.ControllerTag = names.NewModelTag("fooo")
		},
		"ControllerTag not valid",
	}, {
		"invalid ControllerTag",
		func(info *migration.TargetInfo) {
			info.ControllerTag = names.NewModelTag("")
		},
		"ControllerTag not valid",
	}, {
		"empty Addrs",
		func(info *migration.TargetInfo) {
			info.Addrs = []string{}
		},
		"empty Addrs not valid",
	}, {
		"invalid Addrs",
		func(info *migration.TargetInfo) {
			info.Addrs = []string{"1.2.3.4:555", "abc"}
		},
		`"abc" in Addrs not valid`,
	}, {
		"CACert",
		func(info *migration.TargetInfo) {
			info.CACert = ""
		},
		"empty CACert not valid",
	}, {
		"EntityTag",
		func(info *migration.TargetInfo) {
			info.EntityTag = names.NewMachineTag("")
		},
		"empty EntityTag not valid",
	}, {
		"Password",
		func(info *migration.TargetInfo) {
			info.Password = ""
		},
		"empty Password not valid",
	}, {
		"Success",
		func(*migration.TargetInfo) {},
		"",
	}}

	modelTag := names.NewModelTag(utils.MustNewUUID().String())
	for _, test := range tests {
		c.Logf("---- %s -----------", test.label)

		info := migration.TargetInfo{
			ControllerTag: modelTag,
			Addrs:         []string{"1.2.3.4:5555", "4.3.2.1:6666"},
			CACert:        "cert",
			EntityTag:     names.NewUserTag("user"),
			Password:      "password",
		}
		test.tweakInfo(&info)

		err := info.Validate()
		if test.errorPattern == "" {
			c.Check(err, jc.ErrorIsNil)
		} else {
			c.Check(errors.IsNotValid(err), jc.IsTrue)
			c.Check(err, gc.ErrorMatches, test.errorPattern)
		}
	}
}
