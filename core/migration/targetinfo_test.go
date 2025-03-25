// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package migration_test

import (
	"github.com/juju/names/v6"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"
	"gopkg.in/macaroon.v2"

	coreerrors "github.com/juju/juju/core/errors"
	"github.com/juju/juju/core/migration"
	coretesting "github.com/juju/juju/internal/testing"
	"github.com/juju/juju/internal/uuid"
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
			info.ControllerTag = names.NewControllerTag("fooo")
		},
		"ControllerTag not valid",
	}, {
		"invalid ControllerTag",
		func(info *migration.TargetInfo) {
			info.ControllerTag = names.NewControllerTag("")
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
		"AuthTag",
		func(info *migration.TargetInfo) {
			info.AuthTag = names.UserTag{}
			info.Macaroons = nil
		},
		"empty AuthTag not valid",
	}, {
		"Success - empty CACert",
		func(info *migration.TargetInfo) {
			info.CACert = ""
		},
		"",
	}, {
		"Password & Macaroons",
		func(info *migration.TargetInfo) {
			info.Password = ""
			info.Macaroons = nil
		},
		"missing Password & Macaroons not valid",
	}, {
		"Success - empty Password",
		func(info *migration.TargetInfo) {
			info.Password = ""
		},
		"",
	}, {
		"Success - empty Macaroons",
		func(info *migration.TargetInfo) {
			info.Macaroons = nil
		},
		"",
	}, {
		"Success - empty AuthTag with macaroons",
		func(info *migration.TargetInfo) {
			info.AuthTag = names.UserTag{}
		},
		"",
	}, {
		"Success - all set",
		func(*migration.TargetInfo) {},
		"",
	}}

	for _, test := range tests {
		c.Logf("%s", test.label)
		info := makeValidTargetInfo(c)
		test.tweakInfo(&info)
		err := info.Validate()
		if test.errorPattern == "" {
			c.Check(err, jc.ErrorIsNil)
		} else {
			c.Check(err, jc.ErrorIs, coreerrors.NotValid)
			c.Check(err, gc.ErrorMatches, test.errorPattern)
		}
	}
}

func makeValidTargetInfo(c *gc.C) migration.TargetInfo {
	mac, err := macaroon.New([]byte("secret"), []byte("id"), "location", macaroon.LatestVersion)
	c.Assert(err, jc.ErrorIsNil)
	return migration.TargetInfo{
		ControllerTag: names.NewControllerTag(uuid.MustNewUUID().String()),
		Addrs:         []string{"1.2.3.4:5555"},
		CACert:        "cert",
		AuthTag:       names.NewUserTag("user"),
		Password:      "password",
		Macaroons:     []macaroon.Slice{{mac}},
	}
}
