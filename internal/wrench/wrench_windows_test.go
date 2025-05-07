// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package wrench_test

import "github.com/juju/tc"

const fileNotFound = "GetFileAttributesEx.*: The system cannot find the (file|path) specified."

// Patch out the os.Stat call used by wrench so that a particular file
// appears to be owned by a UID that isn't Juju's UID.
func (s *wrenchSuite) tweakOwner(c *tc.C, targetPath string) {
	c.Skip("this test only supports POSIX systems")
}
