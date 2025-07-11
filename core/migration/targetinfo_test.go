// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package migration_test

import (
	"testing"

	"github.com/juju/tc"
	"gopkg.in/macaroon.v2"

	coreerrors "github.com/juju/juju/core/errors"
	"github.com/juju/juju/core/migration"
	coretesting "github.com/juju/juju/internal/testing"
	"github.com/juju/juju/internal/uuid"
)

type TargetInfoSuite struct {
	coretesting.BaseSuite
}

func TestTargetInfoSuite(t *testing.T) {
	tc.Run(t, new(TargetInfoSuite))
}

func (s *TargetInfoSuite) TestValidation(c *tc.C) {
	tests := []struct {
		label        string
		tweakInfo    func(*migration.TargetInfo)
		errorPattern string
	}{{
		"empty ControllerTag",
		func(info *migration.TargetInfo) {
			info.ControllerUUID = "fooo"
		},
		"ControllerTag not valid",
	}, {
		"invalid ControllerTag",
		func(info *migration.TargetInfo) {
			info.ControllerUUID = ""
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
			info.User = ""
			info.Macaroons = nil
		},
		"empty User not valid",
	}, {
		"Success - empty CACert",
		func(info *migration.TargetInfo) {
			info.CACert = ""
		},
		"",
	}, {
		"Empty Password, Macaroons & Token",
		func(info *migration.TargetInfo) {
			info.Password = ""
			info.Macaroons = nil
			info.Token = ""
		},
		"missing Password, Macaroons or Token not valid",
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
		"Success - empty Macaroons and Token",
		func(info *migration.TargetInfo) {
			info.Macaroons = nil
			info.Token = ""
		},
		"",
	}, {
		"Success - empty Macaroons and Password",
		func(info *migration.TargetInfo) {
			info.Macaroons = nil
			info.Password = ""
		},
		"",
	}, {
		"Success - empty Password and Token",
		func(info *migration.TargetInfo) {
			info.Password = ""
			info.Token = ""
		},
		"",
	}, {
		"Success - empty AuthTag with macaroons",
		func(info *migration.TargetInfo) {
			info.User = ""
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
			c.Check(err, tc.ErrorIsNil)
		} else {
			c.Check(err, tc.ErrorIs, coreerrors.NotValid)
			c.Check(err, tc.ErrorMatches, test.errorPattern)
		}
	}
}

func makeValidTargetInfo(c *tc.C) migration.TargetInfo {
	mac, err := macaroon.New([]byte("secret"), []byte("id"), "location", macaroon.LatestVersion)
	c.Assert(err, tc.ErrorIsNil)
	return migration.TargetInfo{
		ControllerUUID: uuid.MustNewUUID().String(),
		Addrs:          []string{"1.2.3.4:5555"},
		CACert:         "cert",
		User:           "user",
		Password:       "password",
		Macaroons:      []macaroon.Slice{{mac}},
		Token:          "token",
	}
}
