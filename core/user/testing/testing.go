// Copyright 2024 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package testing

import (
	"github.com/juju/tc"

	coreuser "github.com/juju/juju/core/user"
)

// GenUserUUID can be used in testing for generating a user uuid that is
// checked for subsequent errors using the test suits go check instance.
func GenUserUUID(c *tc.C) coreuser.UUID {
	uuid, err := coreuser.NewUUID()
	c.Assert(err, tc.ErrorIsNil)
	return uuid
}

// GenNewName returns a new username object. It asserts that the username is
// valid.
func GenNewName(c *tc.C, name string) coreuser.Name {
	un, err := coreuser.NewName(name)
	c.Assert(err, tc.ErrorIsNil)
	return un
}
