// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package payload_test

import (
	"github.com/juju/testing"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/payload"
)

var _ = gc.Suite(&idSuite{})

type idSuite struct {
	testing.IsolationSuite
}

func (s *idSuite) TestParseIDFull(c *gc.C) {
	name, id := payload.ParseID("a-payload/my-payload")

	c.Check(name, gc.Equals, "a-payload")
	c.Check(id, gc.Equals, "my-payload")
}

func (s *idSuite) TestParseIDNameOnly(c *gc.C) {
	name, id := payload.ParseID("a-payload")

	c.Check(name, gc.Equals, "a-payload")
	c.Check(id, gc.Equals, "")
}

func (s *idSuite) TestParseIDExtras(c *gc.C) {
	name, id := payload.ParseID("somecharm/0/a-payload/my-payload")

	c.Check(name, gc.Equals, "somecharm")
	c.Check(id, gc.Equals, "0/a-payload/my-payload")
}
