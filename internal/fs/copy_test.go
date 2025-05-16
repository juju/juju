// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package fs

import (
	"path/filepath"
	stdtesting "testing"

	"github.com/juju/tc"

	"github.com/juju/juju/internal/testhelpers/filetesting"
)

type copySuite struct{}

func TestCopySuite(t *stdtesting.T) { tc.Run(t, &copySuite{}) }

var copyTests = []struct {
	about string
	src   filetesting.Entries
	dst   filetesting.Entries
	err   string
}{{
	about: "one file",
	src: []filetesting.Entry{
		filetesting.File{Path: "file", Data: "data", Perm: 0756},
	},
}, {
	about: "one directory",
	src: []filetesting.Entry{
		filetesting.Dir{Path: "dir", Perm: 0777},
	},
}, {
	about: "one symlink",
	src: []filetesting.Entry{
		filetesting.Symlink{Path: "link", Link: "/foo"},
	},
}, {
	about: "several entries",
	src: []filetesting.Entry{
		filetesting.Dir{Path: "top", Perm: 0755},
		filetesting.File{Path: "top/foo", Data: "foodata", Perm: 0644},
		filetesting.File{Path: "top/bar", Data: "bardata", Perm: 0633},
		filetesting.Dir{Path: "top/next", Perm: 0721},
		filetesting.Symlink{Path: "top/next/link", Link: "../foo"},
		filetesting.File{Path: "top/next/another", Data: "anotherdata", Perm: 0644},
	},
}, {
	about: "destination already exists",
	src: []filetesting.Entry{
		filetesting.Dir{Path: "dir", Perm: 0777},
	},
	dst: []filetesting.Entry{
		filetesting.Dir{Path: "dir", Perm: 0777},
	},
	err: `will not overwrite ".+dir"`,
}, {
	about: "source with unwritable directory",
	src: []filetesting.Entry{
		filetesting.Dir{Path: "dir", Perm: 0555},
	},
}}

func (*copySuite) TestCopy(c *tc.C) {
	for i, test := range copyTests {
		c.Logf("test %d: %v", i, test.about)
		src, dst := c.MkDir(), c.MkDir()
		test.src.Create(c, src)
		test.dst.Create(c, dst)
		path := test.src[0].GetPath()
		err := Copy(
			filepath.Join(src, path),
			filepath.Join(dst, path),
		)
		if test.err != "" {
			c.Check(err, tc.ErrorMatches, test.err)
		} else {
			c.Assert(err, tc.IsNil)
			test.src.Check(c, dst)
		}
	}
}
