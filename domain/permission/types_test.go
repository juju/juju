// Copyright 2024 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package permission

import (
	"github.com/juju/errors"
	"github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/core/permission"
)

type typesSuite struct{}

var _ = gc.Suite(&typesSuite{})

func (s *typesSuite) TestUpsertPermissionArgsValidationFail(c *gc.C) {
	argsToTest := []UpsertPermissionArgs{
		{}, { // Missing Subject
			ApiUser: "admin",
		}, { // Missing Target
			ApiUser: "admin",
			Subject: "testme",
		}, { // Target and Access don't mesh
			Access:  permission.AddModelAccess,
			ApiUser: "admin",
			Subject: "testme",
			Target: permission.ID{
				ObjectType: permission.Cloud,
				Key:        "aws",
			},
		}, { // Invalid Change
			Access:  permission.AddModelAccess,
			ApiUser: "admin",
			Change:  "testing",
			Subject: "testme",
			Target: permission.ID{
				ObjectType: permission.Model,
				Key:        "aws",
			},
		}}
	for i, args := range argsToTest {
		c.Logf("Test %d", i)
		c.Check(args.Validate(), checkers.ErrorIs, errors.NotValid)
	}
}
