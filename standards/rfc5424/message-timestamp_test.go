// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package rfc5424_test

import (
	"time"

	"github.com/juju/testing"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/standards/rfc5424"
)

type TimestampSuite struct {
	testing.IsolationSuite
}

var _ = gc.Suite(&TimestampSuite{})

func (s *TimestampSuite) TestStringOkay(c *gc.C) {
	ts := rfc5424.Timestamp{time.Unix(54321, 123).UTC()}

	str := ts.String()

	c.Check(str, gc.Equals, "1970-01-01T15:05:21.000000123Z")
}

func (s *TimestampSuite) TestStringNoNano(c *gc.C) {
	ts := rfc5424.Timestamp{time.Unix(54321, 0).UTC()}

	str := ts.String()

	c.Check(str, gc.Equals, "1970-01-01T15:05:21Z")
}

func (s *TimestampSuite) TestStringTimezone(c *gc.C) {
	ts := rfc5424.Timestamp{time.Unix(54321, 123).In(time.FixedZone("MST", -7*60*60))}

	str := ts.String()

	c.Check(str, gc.Equals, "1970-01-01T08:05:21.000000123-07:00")
}

func (s *TimestampSuite) TestStringZeroValue(c *gc.C) {
	var ts rfc5424.Timestamp

	str := ts.String()

	c.Check(str, gc.Equals, "-")
}
