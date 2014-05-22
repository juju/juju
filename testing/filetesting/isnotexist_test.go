// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package filetesting

import (
	"io/ioutil"
	"os"
	"path/filepath"

	jc "github.com/juju/testing/checkers"
	gc "launchpad.net/gocheck"
)

type isNotExistSuite struct{}

var _ = gc.Suite(&isNotExistSuite{})

func (*isNotExistSuite) TestIsNotExist(c *gc.C) {
	dir := c.MkDir()
	path := func(s string) string { return filepath.Join(dir, s) }
	err := ioutil.WriteFile(path("file"), []byte("blah"), 0644)
	c.Assert(err, gc.IsNil)

	_, err = os.Lstat(path("noexist"))
	c.Assert(err, jc.Satisfies, isNotExist)

	_, err = os.Lstat(path("file/parent-not-a-dir"))
	c.Assert(err, jc.Satisfies, isNotExist)
}
