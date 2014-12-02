// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package storage_test

import (
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/storage"
)

type DirectiveSuite struct{}

var _ = gc.Suite(&DirectiveSuite{})

func (s *DirectiveSuite) TestParseDirectiveStorageName(c *gc.C) {
	s.testParse(c, "name=source:1M", &storage.Directive{
		Name: "name", Source: "source", Count: 1, Size: 1,
	})
	s.testParse(c, "n-a-m-e=source:1M", &storage.Directive{
		Name: "n-a-m-e", Source: "source", Count: 1, Size: 1,
	})
}

func (s *DirectiveSuite) TestParseDirectiveCountSize(c *gc.C) {
	s.testParse(c, "n=s:1G", &storage.Directive{
		Name: "n", Source: "s", Count: 1, Size: 1024,
	})
	s.testParse(c, "n=s:1x0.5T", &storage.Directive{
		Name: "n", Source: "s", Count: 1, Size: 1024 * 512,
	})
	s.testParse(c, "n=s:3x0.125P", &storage.Directive{
		Name: "n", Source: "s", Count: 3, Size: 1024 * 1024 * 128,
	})
}

func (s *DirectiveSuite) TestParseDirectiveOptions(c *gc.C) {
	s.testParse(c, "n=s:1M,", &storage.Directive{
		Name: "n", Source: "s", Count: 1, Size: 1,
	})
	s.testParse(c, "n=s:anyoldjunk", &storage.Directive{
		Name: "n", Source: "s", Count: 0, Size: 0,
		Options: "anyoldjunk",
	})
	s.testParse(c, "n=s:1M,whatever options that please me", &storage.Directive{
		Name: "n", Source: "s", Count: 1, Size: 1,
		Options: "whatever options that please me",
	})
}

func (s *DirectiveSuite) TestParseDirectiveStorageNameMissing(c *gc.C) {
	s.testParseError(c, "", "storage name missing")
	s.testParseError(c, ":", "storage name missing")
	s.testParseError(c, "=", "storage name missing")
	s.testParseError(c, "1M", "storage name missing")
	s.testParseError(c, "ebs:1M", "storage name missing")
}

func (s *DirectiveSuite) TestParseDirectiveStorageSourceMissing(c *gc.C) {
	s.testParseError(c, "name=1M", "storage source missing")
}

func (s *DirectiveSuite) TestParseDirectiveCountRange(c *gc.C) {
	s.testParseError(c, "n=s:0x100M", "count must be a positive integer")
	s.testParseError(c, "n=s:-1x100M", "count must be a positive integer")
}

func (s *DirectiveSuite) TestParseDirectiveSizeRange(c *gc.C) {
	s.testParseError(c, "n=s:-100M", `failed to parse size: expected a non-negative number with optional multiplier suffix \(M/G/T/P\), got "-100M"`)
}

func (s *DirectiveSuite) TestParseDirectiveTrailingData(c *gc.C) {
	s.testParseError(c, "name=source:1Msomejunk", `invalid trailing data "somejunk": options must be preceded by ',' when size is specified`)
}

func (*DirectiveSuite) testParse(c *gc.C, s string, expect *storage.Directive) {
	d, err := storage.ParseDirective(s)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(d, gc.DeepEquals, expect)
}

func (*DirectiveSuite) testParseError(c *gc.C, s, expectErr string) {
	d, err := storage.ParseDirective(s)
	c.Check(d, gc.IsNil)
	c.Assert(err, gc.ErrorMatches, expectErr)
}
