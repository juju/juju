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

func AssertDirectoryExists(c *C, path string) {
	fileInfo, err := os.Stat(path)
	c.Assert(err, IsNil)
	c.Assert(fileInfo.IsDir(), IsTrue)
}

func AssertDirectoryDoesNotExist(c *C, path string) {
	_, err := os.Stat(path)
	c.Assert(os.IsNotExist(err), IsTrue)
}
