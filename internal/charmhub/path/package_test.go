// Copyright 2020 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package path

import (
	"net/url"

	"github.com/juju/tc"
)

func MustParseURL(c *tc.C, path string) *url.URL {
	u, err := url.Parse(path)
	c.Assert(err, tc.ErrorIsNil)
	return u
}

func MustMakePath(c *tc.C, path string) Path {
	u := MustParseURL(c, path)
	return MakePath(u)
}
