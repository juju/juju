// Copyright 2024 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package access

import (
	"github.com/juju/errors"
	"github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/core/permission"
)

type typesSuite struct{}

var _ = gc.Suite(&typesSuite{})

func (s *typesSuite) TestUpsertPermissionArgsValidationFail(c *gc.C) {
	argsToTest := []UpdatePermissionArgs{
		{}, { // Missing Subject
			ApiUser: "admin",
		}, { // Missing Target
			ApiUser: "admin",
			Subject: "testme",
		}, { // Target and Access don't mesh
			AccessSpec: permission.AccessSpec{
				Access: permission.AddModelAccess,
				Target: permission.ID{
					ObjectType: permission.Cloud,
					Key:        "aws",
				},
			},
			ApiUser: "admin",
			Subject: "testme",
		}, { // Invalid Change
			AccessSpec: permission.AccessSpec{
				Access: permission.AddModelAccess,
				Target: permission.ID{
					ObjectType: permission.Model,
					Key:        "aws",
				},
			},
			ApiUser: "admin",
			Change:  "testing",
			Subject: "testme",
		}}
	for i, args := range argsToTest {
		c.Logf("Test %d", i)
		c.Check(args.Validate(), checkers.ErrorIs, errors.NotValid)
	}
}
