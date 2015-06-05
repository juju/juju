// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package process_test

import (
	"fmt"

	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/process"
	"github.com/juju/juju/testing"
)

type statusSuite struct {
	testing.BaseSuite
}

var _ = gc.Suite(&statusSuite{})

func (*statusSuite) TestIsKnownTrue(c *gc.C) {
	status := process.StatusActive
	isUnknown := status.IsUnknown()

	c.Check(isUnknown, jc.IsFalse)
}

func (*statusSuite) TestIsKnownFalse(c *gc.C) {
	status := process.StatusActive
	isUnknown := status.IsUnknown()

	c.Check(isUnknown, jc.IsFalse)
}

func (*statusSuite) TestStringKnown(c *gc.C) {
	expected := map[process.Status]string{
		process.StatusPending: "pending",
		process.StatusActive:  "active",
		process.StatusFailed:  "failed",
		process.StatusStopped: "stopped",
	}
	c.Assert(expected, gc.HasLen, len(process.KnownStatuses))

	for _, status := range process.KnownStatuses {
		str := fmt.Sprintf("%s", status)

		c.Check(str, gc.Equals, expected[status])
	}
}

func (*statusSuite) TestStringUnknown(c *gc.C) {
	status := process.Status(-1)
	str := fmt.Sprintf("%s", status)

	c.Check(str, gc.Equals, "unknown")
}

func (*statusSuite) TestGoString(c *gc.C) {
	str := fmt.Sprintf("%#v", process.StatusPending)

	c.Check(str, gc.Equals, `<process.Status "pending">`)
}
