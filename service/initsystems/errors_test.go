// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package initsystems_test

import (
	"github.com/juju/errors"
	"github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/service/initsystems"
)

var _ = gc.Suite(&errorsSuite{})

type errorsSuite struct {
	testing.IsolationSuite
}

func (s *errorsSuite) TestNewUnsupportedField(c *gc.C) {
	err := initsystems.NewUnsupportedField("spam")

	c.Check(err, gc.ErrorMatches, `field "spam" .*`)
	c.Check(err, jc.Satisfies, errors.IsNotSupported)
}

func (s *errorsSuite) TestNewUnsupportedItem(c *gc.C) {
	err := initsystems.NewUnsupportedItem("spam", "eggs")

	c.Check(err, gc.ErrorMatches, `field "spam", item "eggs" .*`)
	c.Check(err, jc.Satisfies, errors.IsNotSupported)
}
