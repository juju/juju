// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package testing

import (
	"os"

	. "launchpad.net/gocheck"
	. "launchpad.net/juju-core/testing/checkers"
)

func AssertNonEmptyFileExists(c *C, filename string) {
	fileInfo, err := os.Stat(filename)
	c.Assert(err, IsNil)
	c.Assert(fileInfo.Size(), GreaterThan, 0)
}

func AssertDirectoryExists(c *C, filename string) {
	fileInfo, err := os.Stat(filename)
	c.Assert(err, IsNil)
	c.Assert(fileInfo.IsDir(), IsTrue)
}
