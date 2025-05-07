// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package storage_test

import (
	"github.com/juju/tc"
	jc "github.com/juju/testing/checkers"

	"github.com/juju/juju/internal/storage"
	"github.com/juju/juju/internal/testing"
)

type DirectiveSuite struct {
	testing.BaseSuite
}

var _ = tc.Suite(&DirectiveSuite{})

func (s *DirectiveSuite) TestParseConstraintsStoragePool(c *tc.C) {
	s.testParse(c, "pool,1M", storage.Directive{
		Pool:  "pool",
		Count: 1,
		Size:  1,
	})
	s.testParse(c, "pool,", storage.Directive{
		Pool:  "pool",
		Count: 1,
	})
	s.testParse(c, "1M", storage.Directive{
		Size:  1,
		Count: 1,
	})
}

func (s *DirectiveSuite) TestParseConstraintsCountSize(c *tc.C) {
	s.testParse(c, "p,1G", storage.Directive{
		Pool:  "p",
		Count: 1,
		Size:  1024,
	})
	s.testParse(c, "p,1,0.5T", storage.Directive{
		Pool:  "p",
		Count: 1,
		Size:  1024 * 512,
	})
	s.testParse(c, "p,0.125P,3", storage.Directive{
		Pool:  "p",
		Count: 3,
		Size:  1024 * 1024 * 128,
	})
	s.testParse(c, "3,p,0.125P", storage.Directive{
		Pool:  "p",
		Count: 3,
		Size:  1024 * 1024 * 128,
	})
}

func (s *DirectiveSuite) TestParseConstraintsOptions(c *tc.C) {
	s.testParse(c, "p,1M,", storage.Directive{
		Pool:  "p",
		Count: 1,
		Size:  1,
	})
}

func (s *DirectiveSuite) TestParseConstraintsCountRange(c *tc.C) {
	s.testParseError(c, "p,0,100M", `cannot parse count: count must be greater than zero, got "0"`)
	s.testParseError(c, "p,00,100M", `cannot parse count: count must be greater than zero, got "00"`)
	s.testParseError(c, "p,-1,100M", `cannot parse count: count must be greater than zero, got "-1"`)
	s.testParseError(c, "", `storage directives require at least one field to be specified`)
	s.testParseError(c, ",", `storage directives require at least one field to be specified`)
}

func (s *DirectiveSuite) TestParseConstraintsSizeRange(c *tc.C) {
	s.testParseError(c, "p,-100M", `cannot parse size: expected a non-negative number, got "-100M"`)
}

func (s *DirectiveSuite) TestParseMultiplePoolNames(c *tc.C) {
	s.testParseError(c, "pool1,anyoldjunk", `pool name is already set to "pool1", new value "anyoldjunk" not valid`)
	s.testParseError(c, "pool1,pool2", `pool name is already set to "pool1", new value "pool2" not valid`)
	s.testParseError(c, "pool1,pool2,pool3", `pool name is already set to "pool1", new value "pool2" not valid`)
}

func (s *DirectiveSuite) TestParseMultipleCounts(c *tc.C) {
	s.testParseError(c, "pool1,10,20", `storage instance count is already set to 10, new value 20 not valid`)
}

func (s *DirectiveSuite) TestParseMultipleStorageSize(c *tc.C) {
	s.testParseError(c, "pool1,10M,20M", `storage size is already set to 10, new value 20 not valid`)
}

func (s *DirectiveSuite) TestParseConstraintsUnknown(c *tc.C) {
	// Regression test for #1855181
	s.testParseError(c, "p,100M database-b", `unrecognized storage directive "100M database-b" not valid`)
	s.testParseError(c, "p,$1234", `unrecognized storage directive "\$1234" not valid`)
}

func (*DirectiveSuite) testParse(c *tc.C, s string, expect storage.Directive) {
	cons, err := storage.ParseDirective(s)
	c.Check(err, jc.ErrorIsNil)
	c.Check(cons, tc.DeepEquals, expect)
}

func (*DirectiveSuite) testParseError(c *tc.C, s, expectErr string) {
	_, err := storage.ParseDirective(s)
	c.Check(err, tc.ErrorMatches, expectErr)
}

func (s *DirectiveSuite) TestValidPoolName(c *tc.C) {
	c.Assert(storage.IsValidPoolName("pool"), jc.IsTrue)
	c.Assert(storage.IsValidPoolName("p-ool"), jc.IsTrue)
	c.Assert(storage.IsValidPoolName("p-00l"), jc.IsTrue)
	c.Assert(storage.IsValidPoolName("p?00l"), jc.IsTrue)
	c.Assert(storage.IsValidPoolName("p-?00l"), jc.IsTrue)
	c.Assert(storage.IsValidPoolName("p"), jc.IsTrue)
	c.Assert(storage.IsValidPoolName("P"), jc.IsTrue)
	c.Assert(storage.IsValidPoolName("p?0?l"), jc.IsTrue)
}

func (s *DirectiveSuite) TestInvalidPoolName(c *tc.C) {
	c.Assert(storage.IsValidPoolName("7ool"), jc.IsFalse)
	c.Assert(storage.IsValidPoolName("/ool"), jc.IsFalse)
	c.Assert(storage.IsValidPoolName("-00l"), jc.IsFalse)
	c.Assert(storage.IsValidPoolName("*00l"), jc.IsFalse)
}

func (s *DirectiveSuite) TestParseStorageDirectives(c *tc.C) {
	s.testParseStorageDirectives(c,
		[]string{"data=p,1M,"}, true,
		map[string]storage.Directive{"data": {
			Pool:  "p",
			Count: 1,
			Size:  1,
		}})
	s.testParseStorageDirectives(c,
		[]string{"data"}, false,
		map[string]storage.Directive{"data": {
			Count: 1,
		}})
	s.testParseStorageDirectives(c,
		[]string{"data=3", "cache"}, false,
		map[string]storage.Directive{
			"data": {
				Count: 3,
			},
			"cache": {
				Count: 1,
			},
		})
}

func (s *DirectiveSuite) TestParseStorageDirectivesErrors(c *tc.C) {
	s.testStorageDirectivesError(c,
		[]string{"data"}, true,
		`.*where "directive" must be specified.*`)
	s.testStorageDirectivesError(c,
		[]string{"data=p,=1M,"}, false,
		`.*expected "name=directive" or "name", got .*`)
	s.testStorageDirectivesError(c,
		[]string{"data", "data"}, false,
		`storage "data" specified more than once`)
	s.testStorageDirectivesError(c,
		[]string{"data=-1"}, false,
		`.*cannot parse directive for storage "data".*`)
	s.testStorageDirectivesError(c,
		[]string{"data="}, false,
		`.*cannot parse directive for storage "data".*`)
}

func (*DirectiveSuite) testParseStorageDirectives(c *tc.C,
	s []string,
	mustHave bool,
	expect map[string]storage.Directive,
) {
	cons, err := storage.ParseDirectivesMap(s, mustHave)
	c.Check(err, jc.ErrorIsNil)
	c.Assert(len(cons), tc.Equals, len(expect))
	for k, v := range expect {
		c.Check(cons[k], tc.DeepEquals, v)
	}
}

func (*DirectiveSuite) testStorageDirectivesError(c *tc.C, s []string, mustHave bool, e string) {
	_, err := storage.ParseDirectivesMap(s, mustHave)
	c.Check(err, tc.ErrorMatches, e)
}

func (s *DirectiveSuite) TestToString(c *tc.C) {
	_, err := storage.ToString(storage.Directive{})
	c.Assert(err, tc.ErrorMatches, "must provide one of pool or size or count")

	for _, t := range []struct {
		pool     string
		count    uint64
		size     uint64
		expected string
	}{
		{"loop", 0, 0, "loop"},
		{"loop", 1, 0, "loop,1"},
		{"loop", 0, 1024, "loop,1024M"},
		{"loop", 1, 1024, "loop,1,1024M"},
		{"", 0, 1024, "1024M"},
		{"", 1, 0, "1"},
		{"", 1, 1024, "1,1024M"},
	} {
		str, err := storage.ToString(storage.Directive{
			Pool:  t.pool,
			Size:  t.size,
			Count: t.count,
		})
		c.Assert(err, jc.ErrorIsNil)
		c.Assert(str, tc.Equals, t.expected)

		// Test roundtrip, count defaults to 1.
		if t.count == 0 {
			t.count = 1
		}
		s.testParse(c, str, storage.Directive{
			Pool:  t.pool,
			Size:  t.size,
			Count: t.count,
		})
	}
}
