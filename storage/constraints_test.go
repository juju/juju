// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package storage_test

import (
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/storage"
	"github.com/juju/juju/testing"
)

type ConstraintsSuite struct {
	testing.BaseSuite
}

var _ = gc.Suite(&ConstraintsSuite{})

func (s *ConstraintsSuite) TestParseConstraintsStoragePool(c *gc.C) {
	s.testParse(c, "pool,1M", storage.Constraints{
		Pool:  "pool",
		Count: 1,
		Size:  1,
	})
	s.testParse(c, "pool,", storage.Constraints{
		Pool:  "pool",
		Count: 1,
	})
	s.testParse(c, "1M", storage.Constraints{
		Size:  1,
		Count: 1,
	})
}

func (s *ConstraintsSuite) TestParseConstraintsCountSize(c *gc.C) {
	s.testParse(c, "p,1G", storage.Constraints{
		Pool:  "p",
		Count: 1,
		Size:  1024,
	})
	s.testParse(c, "p,1,0.5T", storage.Constraints{
		Pool:  "p",
		Count: 1,
		Size:  1024 * 512,
	})
	s.testParse(c, "p,0.125P,3", storage.Constraints{
		Pool:  "p",
		Count: 3,
		Size:  1024 * 1024 * 128,
	})
}

func (s *ConstraintsSuite) TestParseConstraintsOptions(c *gc.C) {
	s.testParse(c, "p,1M,", storage.Constraints{
		Pool:  "p",
		Count: 1,
		Size:  1,
	})
	s.testParse(c, "p,anyoldjunk", storage.Constraints{
		Pool:  "p",
		Count: 1,
	})
}

func (s *ConstraintsSuite) TestParseConstraintsCountRange(c *gc.C) {
	s.testParseError(c, "p,0,100M", `cannot parse count: count must be greater than zero, got "0"`)
	s.testParseError(c, "p,00,100M", `cannot parse count: count must be greater than zero, got "00"`)
	s.testParseError(c, "p,-1,100M", `cannot parse count: count must be greater than zero, got "-1"`)
	s.testParseError(c, "", `storage constraints require at least one field to be specified`)
	s.testParseError(c, ",", `storage constraints require at least one field to be specified`)
}

func (s *ConstraintsSuite) TestParseConstraintsSizeRange(c *gc.C) {
	s.testParseError(c, "p,-100M", `cannot parse size: expected a non-negative number, got "-100M"`)
}

func (*ConstraintsSuite) testParse(c *gc.C, s string, expect storage.Constraints) {
	cons, err := storage.ParseConstraints(s)
	c.Check(err, jc.ErrorIsNil)
	c.Check(cons, gc.DeepEquals, expect)
}

func (*ConstraintsSuite) testParseError(c *gc.C, s, expectErr string) {
	_, err := storage.ParseConstraints(s)
	c.Check(err, gc.ErrorMatches, expectErr)
}

func (s *ConstraintsSuite) TestValidPoolName(c *gc.C) {
	c.Assert(storage.IsValidPoolName("pool"), jc.IsTrue)
	c.Assert(storage.IsValidPoolName("p-ool"), jc.IsTrue)
	c.Assert(storage.IsValidPoolName("p-00l"), jc.IsTrue)
	c.Assert(storage.IsValidPoolName("p?00l"), jc.IsTrue)
	c.Assert(storage.IsValidPoolName("p-?00l"), jc.IsTrue)
	c.Assert(storage.IsValidPoolName("p"), jc.IsTrue)
	c.Assert(storage.IsValidPoolName("P"), jc.IsTrue)
	c.Assert(storage.IsValidPoolName("p?0?l"), jc.IsTrue)
}

func (s *ConstraintsSuite) TestInvalidPoolName(c *gc.C) {
	c.Assert(storage.IsValidPoolName("7ool"), jc.IsFalse)
	c.Assert(storage.IsValidPoolName("/ool"), jc.IsFalse)
	c.Assert(storage.IsValidPoolName("-00l"), jc.IsFalse)
	c.Assert(storage.IsValidPoolName("*00l"), jc.IsFalse)
}

func (s *ConstraintsSuite) TestParseStorageConstraints(c *gc.C) {
	s.testParseStorageConstraints(c,
		[]string{"data=p,1M,"}, true,
		map[string]storage.Constraints{"data": storage.Constraints{
			Pool:  "p",
			Count: 1,
			Size:  1,
		}})
	s.testParseStorageConstraints(c,
		[]string{"data"}, false,
		map[string]storage.Constraints{"data": storage.Constraints{
			Count: 1,
		}})
	s.testParseStorageConstraints(c,
		[]string{"data=3", "cache"}, false,
		map[string]storage.Constraints{
			"data": storage.Constraints{
				Count: 3,
			},
			"cache": storage.Constraints{
				Count: 1,
			},
		})
}

func (s *ConstraintsSuite) TestParseStorageConstraintsErrors(c *gc.C) {
	s.testStorageConstraintsError(c,
		[]string{"data"}, true,
		`.*where "constraints" must be specified.*`)
	s.testStorageConstraintsError(c,
		[]string{"data=p,=1M,"}, false,
		`.*expected "name=constraints" or "name", got .*`)
	s.testStorageConstraintsError(c,
		[]string{"data", "data"}, false,
		`storage "data" specified more than once`)
	s.testStorageConstraintsError(c,
		[]string{"data=-1"}, false,
		`.*cannot parse constraints for storage "data".*`)
	s.testStorageConstraintsError(c,
		[]string{"data="}, false,
		`.*cannot parse constraints for storage "data".*`)
}

func (*ConstraintsSuite) testParseStorageConstraints(c *gc.C,
	s []string,
	mustHave bool,
	expect map[string]storage.Constraints,
) {
	cons, err := storage.ParseConstraintsMap(s, mustHave)
	c.Check(err, jc.ErrorIsNil)
	c.Assert(len(cons), gc.Equals, len(expect))
	for k, v := range expect {
		c.Check(cons[k], gc.DeepEquals, v)
	}
}

func (*ConstraintsSuite) testStorageConstraintsError(c *gc.C, s []string, mustHave bool, e string) {
	_, err := storage.ParseConstraintsMap(s, mustHave)
	c.Check(err, gc.ErrorMatches, e)
}
