// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package storage_test

import (
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/storage"
)

type ConstraintsSuite struct{}

var _ = gc.Suite(&ConstraintsSuite{})

func (s *ConstraintsSuite) TestParseConstraintsStoragePool(c *gc.C) {
	s.testParse(c, "pool,1M", storage.Constraints{
		Pool: "pool",
		Preferred: storage.ConstraintValues{
			Count: 1,
			Size:  1,
		},
	})
	s.testParse(c, "pool,", storage.Constraints{
		Pool: "pool",
	})
	s.testParse(c, "", storage.Constraints{})
	s.testParse(c, ",", storage.Constraints{})
	s.testParse(c, "1M", storage.Constraints{
		Preferred: storage.ConstraintValues{
			Size:  1,
			Count: 1,
		},
	})
}

func (s *ConstraintsSuite) TestParseConstraintsCountSize(c *gc.C) {
	s.testParse(c, "p,1G", storage.Constraints{
		Pool: "p",
		Preferred: storage.ConstraintValues{
			Count: 1,
			Size:  1024,
		},
	})
	s.testParse(c, "p,1,0.5T", storage.Constraints{
		Pool: "p",
		Preferred: storage.ConstraintValues{
			Count: 1,
			Size:  1024 * 512,
		},
	})
	s.testParse(c, "p,0.125P,3", storage.Constraints{
		Pool: "p",
		Preferred: storage.ConstraintValues{
			Count: 3,
			Size:  1024 * 1024 * 128,
		},
	})
}

func (s *ConstraintsSuite) TestParseConstraintsOptions(c *gc.C) {
	s.testParse(c, "p,1M,", storage.Constraints{
		Pool: "p",
		Preferred: storage.ConstraintValues{
			Count: 1,
			Size:  1,
		},
	})
	s.testParse(c, "p,anyoldjunk", storage.Constraints{
		Pool: "p",
	})
}

func (s *ConstraintsSuite) TestParseConstraintsCountRange(c *gc.C) {
	s.testParseError(c, "p,0,100M", `cannot parse count: count must be greater than zero, got "0"`)
	s.testParseError(c, "p,00,100M", `cannot parse count: count must be greater than zero, got "00"`)
	s.testParseError(c, "p,-1,100M", `cannot parse count: count must be greater than zero, got "-1"`)
}

func (s *ConstraintsSuite) TestParseConstraintsSizeRange(c *gc.C) {
	s.testParseError(c, "p,-100M", `cannot parse size: expected a non-negative number, got "-100M"`)
}

func (*ConstraintsSuite) testParse(c *gc.C, s string, expect storage.Constraints) {
	// ParseConstraints should always return min=preferred. Avoid repetitious
	// tests by setting the expectation here.
	expect.Minimum = expect.Preferred
	cons, err := storage.ParseConstraints(s)
	c.Check(err, jc.ErrorIsNil)
	c.Check(cons, gc.DeepEquals, expect)
}

func (*ConstraintsSuite) testParseError(c *gc.C, s, expectErr string) {
	_, err := storage.ParseConstraints(s)
	c.Check(err, gc.ErrorMatches, expectErr)
}
