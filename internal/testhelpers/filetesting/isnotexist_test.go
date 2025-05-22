// Copyright 2014 Canonical Ltd.
// Licensed under the LGPLv3, see LICENCE file for details.

package filetesting

import (
	"io/ioutil"
	"os"
	"path/filepath"
	stdtesting "testing"

	"github.com/juju/tc"
)

type isNotExistSuite struct{}

func TestIsNotExistSuite(t *stdtesting.T) {
	tc.Run(t, &isNotExistSuite{})
}

func (*isNotExistSuite) TestIsNotExist(c *tc.C) {
	dir := c.MkDir()
	path := func(s string) string { return filepath.Join(dir, s) }
	err := ioutil.WriteFile(path("file"), []byte("blah"), 0644)
	c.Assert(err, tc.IsNil)

	_, err = os.Lstat(path("noexist"))
	c.Assert(err, tc.Satisfies, isNotExist)

	_, err = os.Lstat(path("file/parent-not-a-dir"))
	c.Assert(err, tc.Satisfies, isNotExist)
}
