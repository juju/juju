// Copyright 2024 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package access

import (
	"github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	coreerrors "github.com/juju/juju/core/errors"
	"github.com/juju/juju/core/permission"
	usertesting "github.com/juju/juju/core/user/testing"
)

type typesSuite struct{}

var _ = gc.Suite(&typesSuite{})

func (s *typesSuite) TestUpsertPermissionArgsValidationFail(c *gc.C) {
	argsToTest := []UpdatePermissionArgs{
		{}, { // Missing Subject
		}, {  // Missing Target
			Subject: usertesting.GenNewName(c, "testme"),
		}, { // Target and Access don't mesh
			AccessSpec: permission.AccessSpec{
				Access: permission.AddModelAccess,
				Target: permission.ID{
					ObjectType: permission.Cloud,
					Key:        "aws",
				},
			},
			Subject: usertesting.GenNewName(c, "testme"),
		}, { // Invalid Change
			AccessSpec: permission.AccessSpec{
				Access: permission.AddModelAccess,
				Target: permission.ID{
					ObjectType: permission.Model,
					Key:        "aws",
				},
			},
			Change:  "testing",
			Subject: usertesting.GenNewName(c, "testme"),
		}}
	for i, args := range argsToTest {
		c.Logf("Test %d", i)
		c.Check(args.Validate(), checkers.ErrorIs, coreerrors.NotValid)
	}
}
