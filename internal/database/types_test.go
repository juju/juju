// Copyright 2024 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package database

import (
	"time"

	"github.com/juju/tc"
	"github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
)

type typesSuite struct {
	testing.IsolationSuite
}

var _ = tc.Suite(&typesSuite{})

func (s *typesSuite) TestNullDuration(c *tc.C) {
	nd := NullDuration{Duration: 10 * time.Second, Valid: true}
	v, err := nd.Value()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(v, tc.Equals, int64(10*time.Second))

	nd = NullDuration{Valid: true}
	v, err = nd.Value()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(v, tc.Equals, int64(0))

	nd = NullDuration{Valid: false}
	v, err = nd.Value()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(v, tc.IsNil)

	err = nd.Scan("10s")
	c.Assert(err, tc.ErrorMatches, `cannot scan type string into NullDuration`)
	c.Assert(nd.Valid, jc.IsFalse)

	err = nd.Scan(int64(20 * time.Second))
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(nd.Duration, tc.Equals, 20*time.Second)
	c.Assert(nd.Valid, jc.IsTrue)
}
