// Copyright 2024 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package access

import (
	"testing"

	"github.com/juju/tc"

	coreerrors "github.com/juju/juju/core/errors"
	"github.com/juju/juju/core/permission"
	coreuser "github.com/juju/juju/core/user"
)

type typesSuite struct{}

func TestTypesSuite(t *testing.T) {
	tc.Run(t, &typesSuite{})
}

func (s *typesSuite) TestUpsertPermissionArgsValidationFail(c *tc.C) {
	argsToTest := []UpdatePermissionArgs{
		{}, { // Missing Subject
		}, {  // Missing Target
			Subject: coreuser.GenName(c, "testme"),
		}, { // Target and Access don't mesh
			AccessSpec: permission.AccessSpec{
				Access: permission.AddModelAccess,
				Target: permission.ID{
					ObjectType: permission.Cloud,
					Key:        "aws",
				},
			},
			Subject: coreuser.GenName(c, "testme"),
		}, { // Invalid Change
			AccessSpec: permission.AccessSpec{
				Access: permission.AddModelAccess,
				Target: permission.ID{
					ObjectType: permission.Model,
					Key:        "aws",
				},
			},
			Change:  "testing",
			Subject: coreuser.GenName(c, "testme"),
		}}
	for i, args := range argsToTest {
		c.Logf("Test %d", i)
		c.Check(args.Validate(), tc.ErrorIs, coreerrors.NotValid)
	}
}
