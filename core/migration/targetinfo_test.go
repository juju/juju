// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package migration_test

import (
	"github.com/juju/errors"
	jc "github.com/juju/testing/checkers"
	"github.com/juju/utils"
	gc "gopkg.in/check.v1"
	"gopkg.in/juju/names.v2"
	"gopkg.in/macaroon.v1"

	"github.com/juju/juju/core/migration"
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
		"AuthTag",
		func(info *migration.TargetInfo) {
			info.AuthTag = names.UserTag{}
		},
		"empty AuthTag not valid",
	}, {
		"Password & Macaroon",
		func(info *migration.TargetInfo) {
			info.Password = ""
			info.Macaroon = nil
		},
		"missing Password & Macaroon not valid",
	}, {
		"Success - empty Password",
		func(info *migration.TargetInfo) {
			info.Password = ""
		},
		"",
	}, {
		"Success - empty Macaroon",
		func(info *migration.TargetInfo) {
			info.Macaroon = nil
		},
		"",
	}, {
		"Success - all set",
		func(*migration.TargetInfo) {},
		"",
	}}

	for _, test := range tests {
		c.Logf("---- %s -----------", test.label)
		info := makeValidTargetInfo(c)
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

func makeValidTargetInfo(c *gc.C) migration.TargetInfo {
	modelTag := names.NewModelTag(utils.MustNewUUID().String())
	mac, err := macaroon.New([]byte("secret"), "id", "location")
	c.Assert(err, jc.ErrorIsNil)
	return migration.TargetInfo{
		ControllerTag: modelTag,
		Addrs:         []string{"1.2.3.4:5555"},
		CACert:        "cert",
		AuthTag:       names.NewUserTag("user"),
		Password:      "password",
		Macaroon:      mac,
	}
}
