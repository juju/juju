// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package utils_test

import (
	"os"

	gc "launchpad.net/gocheck"

	"launchpad.net/juju-core/testing/testbase"
	"launchpad.net/juju-core/utils"
)

type sudoSuite struct {
	testbase.LoggingSuite
}

func (s *sudoSuite) TestSudoCallerIds(c *gc.C) {
	s.PatchEnvironment("SUDO_UID", "0")
	s.PatchEnvironment("SUDO_GID", "0")
	for _, test := range []struct {
		uid         string
		gid         string
		errString   string
		expectedUid int
		expectedGid int
	}{{
		uid: "",
		gid: "",
	}, {
		uid:         "1001",
		gid:         "1002",
		expectedUid: 1001,
		expectedGid: 1002,
	}, {
		uid:       "1001",
		gid:       "foo",
		errString: `invalid value "foo" for SUDO_GID`,
	}, {
		uid:       "foo",
		gid:       "bar",
		errString: `invalid value "foo" for SUDO_UID`,
	}} {
		os.Setenv("SUDO_UID", test.uid)
		os.Setenv("SUDO_GID", test.gid)
		uid, gid, err := utils.SudoCallerIds()
		if test.errString == "" {
			c.Assert(err, gc.IsNil)
			c.Assert(uid, gc.Equals, test.expectedUid)
			c.Assert(gid, gc.Equals, test.expectedGid)
		} else {
			c.Assert(err, gc.ErrorMatches, test.errString)
			c.Assert(uid, gc.Equals, 0)
			c.Assert(gid, gc.Equals, 0)
		}
	}
}
