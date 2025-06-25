// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package migration_test

import (
	stdtesting "testing"

	"github.com/juju/tc"

	coremigration "github.com/juju/juju/core/migration"
	"github.com/juju/juju/internal/migration"
)

type DialOpsSuite struct{}

func TestDialOpsSuite(t *stdtesting.T) {
	tc.Run(t, &DialOpsSuite{})
}

func (d *DialOpsSuite) TestNewLoginProvider(c *tc.C) {
	targetInfo := coremigration.TargetInfo{
		Token: "test-session",
	}
	sessionTokenloginProvider := migration.NewLoginProvider(targetInfo)
	c.Assert(sessionTokenloginProvider, tc.NotNil)

	targetInfo.Token = ""
	nilProvider := migration.NewLoginProvider(targetInfo)
	c.Assert(nilProvider, tc.IsNil)
}
