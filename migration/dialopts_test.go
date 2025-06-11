// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package migration_test

import (
	coremigration "github.com/juju/juju/core/migration"
	"github.com/juju/juju/migration"
	gc "gopkg.in/check.v1"
)

type DialOpsSuite struct{}

var _ = gc.Suite(&DialOpsSuite{})

func (d *DialOpsSuite) TestNewLoginProvider(c *gc.C) {
	targetInfo := coremigration.TargetInfo{
		Token: "test-session",
	}
	sessionTokenloginProvider := migration.NewLoginProvider(targetInfo)
	c.Assert(sessionTokenloginProvider, gc.NotNil)

	targetInfo.Token = ""
	nilProvider := migration.NewLoginProvider(targetInfo)
	c.Assert(nilProvider, gc.IsNil)
}
