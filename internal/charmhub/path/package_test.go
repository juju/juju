// Copyright 2020 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package path

import (
	"net/url"
	"testing"

	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"
)

func Test(t *testing.T) {
	gc.TestingT(t)
}

func MustParseURL(c *gc.C, path string) *url.URL {
	u, err := url.Parse(path)
	c.Assert(err, jc.ErrorIsNil)
	return u
}

func MustMakePath(c *gc.C, path string) Path {
	u := MustParseURL(c, path)
	return MakePath(u)
}
