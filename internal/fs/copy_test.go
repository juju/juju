// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package fs

import (
	"path/filepath"

	"github.com/juju/tc"
	ft "github.com/juju/testing/filetesting"
)

type copySuite struct{}

var _ = tc.Suite(&copySuite{})

var copyTests = []struct {
	about string
	src   ft.Entries
	dst   ft.Entries
	err   string
}{{
	about: "one file",
	src: []ft.Entry{
		ft.File{Path: "file", Data: "data", Perm: 0756},
	},
}, {
	about: "one directory",
	src: []ft.Entry{
		ft.Dir{Path: "dir", Perm: 0777},
	},
}, {
	about: "one symlink",
	src: []ft.Entry{
		ft.Symlink{Path: "link", Link: "/foo"},
	},
}, {
	about: "several entries",
	src: []ft.Entry{
		ft.Dir{Path: "top", Perm: 0755},
		ft.File{Path: "top/foo", Data: "foodata", Perm: 0644},
		ft.File{Path: "top/bar", Data: "bardata", Perm: 0633},
		ft.Dir{Path: "top/next", Perm: 0721},
		ft.Symlink{Path: "top/next/link", Link: "../foo"},
		ft.File{Path: "top/next/another", Data: "anotherdata", Perm: 0644},
	},
}, {
	about: "destination already exists",
	src: []ft.Entry{
		ft.Dir{Path: "dir", Perm: 0777},
	},
	dst: []ft.Entry{
		ft.Dir{Path: "dir", Perm: 0777},
	},
	err: `will not overwrite ".+dir"`,
}, {
	about: "source with unwritable directory",
	src: []ft.Entry{
		ft.Dir{Path: "dir", Perm: 0555},
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
