// Copyright 2021 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package lxd

import (
	"github.com/juju/tc"
	"github.com/juju/testing"
	jc "github.com/juju/testing/checkers"

	"github.com/juju/juju/core/semversion"
)

var (
	_ = tc.Suite(&serverSuite{})
)

// serverSuite tests server module functionality from inside the
// lxd package. See server_integration_test.go for tests that use
// only the exported surface of the package.
type serverSuite struct {
	testing.IsolationSuite
}

func (s *serverSuite) TestParseAPIVersion(c *tc.C) {
	ver, err := ParseAPIVersion("5.2")
	c.Check(err, jc.ErrorIsNil)
	c.Check(ver, tc.Equals, semversion.MustParse("5.2.0"))

	_, err = ParseAPIVersion("5")
	c.Check(err, tc.ErrorMatches, `LXD API version "5": expected format <major>.<minor>`)

	_, err = ParseAPIVersion("a.b")
	c.Check(err, tc.ErrorMatches, `major version number  a not valid`)

	_, err = ParseAPIVersion("1.b")
	c.Check(err, tc.ErrorMatches, `minor version number  b not valid`)
}

func (s *serverSuite) TestValidateAPIVersion(c *tc.C) {
	err := ValidateAPIVersion("5.0")
	c.Check(err, jc.ErrorIsNil)

	err = ValidateAPIVersion("4.0")
	c.Check(err, tc.ErrorMatches, `LXD version has to be at least "5.0.0", but current version is only "4.0.0"`)
}
