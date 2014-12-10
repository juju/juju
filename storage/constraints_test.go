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

func (s *ConstraintsSuite) TestParseConstraintsStorageSource(c *gc.C) {
	s.testParse(c, "provider:1M", storage.Constraints{
		Source: "provider",
		Preferred: storage.ConstraintValues{
			Count: 1,
			Size:  1,
		},
	})
	s.testParse(c, "provider0123:", storage.Constraints{
		Source: "provider0123",
	})
	s.testParse(c, ":", storage.Constraints{})
	s.testParse(c, "1M", storage.Constraints{
		Preferred: storage.ConstraintValues{
			Size:  1,
			Count: 1,
		},
	})
}

func (s *ConstraintsSuite) TestParseConstraintsCountSize(c *gc.C) {
	s.testParse(c, "s:1G", storage.Constraints{
		Source: "s",
		Preferred: storage.ConstraintValues{
			Count: 1,
			Size:  1024,
		},
	})
	s.testParse(c, "s:1x0.5T", storage.Constraints{
		Source: "s",
		Preferred: storage.ConstraintValues{
			Count: 1,
			Size:  1024 * 512,
		},
	})
	s.testParse(c, "s:3x0.125P", storage.Constraints{
		Source: "s",
		Preferred: storage.ConstraintValues{
			Count: 3,
			Size:  1024 * 1024 * 128,
		},
	})
}

func (s *ConstraintsSuite) TestParseConstraintsOptions(c *gc.C) {
	s.testParse(c, "s:1M,", storage.Constraints{
		Source: "s",
		Preferred: storage.ConstraintValues{
			Count: 1,
			Size:  1,
		},
	})
	s.testParse(c, "s:anyoldjunk", storage.Constraints{
		Source: "s",
	})
	s.testParse(c, "s:persistent", storage.Constraints{
		Source: "s",
		Preferred: storage.ConstraintValues{
			Persistent: true,
		},
	})
	s.testParse(c, "s:persistent,iops:10000", storage.Constraints{
		Source: "s",
		Preferred: storage.ConstraintValues{
			Persistent: true,
			IOPS:       10000,
		},
	})
	s.testParse(c, "s:iops:10000,persistent", storage.Constraints{
		Source: "s",
		Preferred: storage.ConstraintValues{
			Persistent: true,
			IOPS:       10000,
		},
	})
}

func (s *ConstraintsSuite) TestParseConstraintsCountRange(c *gc.C) {
	s.testParseError(c, "s:0x100M", `count must be greater than zero, got "0"`)
	s.testParseError(c, "s:00x100M", `count must be greater than zero, got "00"`)
	s.testParseError(c, "s:-1x100M", `count must be greater than zero, got "-1"`)
}

func (s *ConstraintsSuite) TestParseConstraintsSizeRange(c *gc.C) {
	s.testParseError(c, "s:-100M", `cannot parse size: expected a non-negative number, got "-100M"`)
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
