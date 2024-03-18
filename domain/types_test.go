// Copyright 2024 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package domain

import (
	"time"

	"github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"
)

type typesSuite struct {
	testing.IsolationSuite
}

var _ = gc.Suite(&typesSuite{})

func (s *typesSuite) TestNullableDuration(c *gc.C) {
	nd := NullableDuration{Duration: 10 * time.Second, Valid: true}
	v, err := nd.Value()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(v, gc.Equals, int64(10*time.Second))

	nd = NullableDuration{Duration: 0, Valid: false}
	v, err = nd.Value()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(v, gc.IsNil)

	err = nd.Scan("10s")
	c.Assert(err, gc.ErrorMatches, `cannot scan type string into NullableDuration`)
	c.Assert(nd.Valid, jc.IsFalse)

	err = nd.Scan(int64(20 * time.Second))
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(nd.Duration, gc.Equals, 20*time.Second)
	c.Assert(nd.Valid, jc.IsTrue)
}
