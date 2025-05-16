// Copyright 2024 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package database

import (
	stdtesting "testing"
	"time"

	"github.com/juju/tc"

	"github.com/juju/juju/internal/testhelpers"
)

type typesSuite struct {
	testhelpers.IsolationSuite
}

func TestTypesSuite(t *stdtesting.T) { tc.Run(t, &typesSuite{}) }
func (s *typesSuite) TestNullDuration(c *tc.C) {
	nd := NullDuration{Duration: 10 * time.Second, Valid: true}
	v, err := nd.Value()
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(v, tc.Equals, int64(10*time.Second))

	nd = NullDuration{Valid: true}
	v, err = nd.Value()
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(v, tc.Equals, int64(0))

	nd = NullDuration{Valid: false}
	v, err = nd.Value()
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(v, tc.IsNil)

	err = nd.Scan("10s")
	c.Assert(err, tc.ErrorMatches, `cannot scan type string into NullDuration`)
	c.Assert(nd.Valid, tc.IsFalse)

	err = nd.Scan(int64(20 * time.Second))
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(nd.Duration, tc.Equals, 20*time.Second)
	c.Assert(nd.Valid, tc.IsTrue)
}
