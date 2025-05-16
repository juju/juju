// Copyright 2024 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package access

import (
	stdtesting "testing"

	"github.com/juju/tc"

	coreerrors "github.com/juju/juju/core/errors"
	"github.com/juju/juju/core/permission"
	usertesting "github.com/juju/juju/core/user/testing"
)

type typesSuite struct{}

func TestTypesSuite(t *stdtesting.T) { tc.Run(t, &typesSuite{}) }
func (s *typesSuite) TestUpsertPermissionArgsValidationFail(c *tc.C) {
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
		c.Check(args.Validate(), tc.ErrorIs, coreerrors.NotValid)
	}
}
