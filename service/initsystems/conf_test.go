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

var _ = gc.Suite(&confSuite{})

type confSuite struct {
	testing.IsolationSuite
}

func (s *confSuite) TestIsUnsupportedField(c *gc.C) {
	err := initsystems.NewUnsupportedField("spam")
	result := initsystems.IsUnsupported(err)

	c.Check(result, jc.IsTrue)
}

func (s *confSuite) TestIsUnsupportedItem(c *gc.C) {
	err := initsystems.NewUnsupportedItem("spam", "eggs")
	result := initsystems.IsUnsupported(err)

	c.Check(result, jc.IsTrue)
}

func (s *confSuite) TestIsUnsupportedFalse(c *gc.C) {
	err := errors.New("<unknown>")
	result := initsystems.IsUnsupported(err)

	c.Check(result, jc.IsFalse)
}

func (s *confSuite) TestNewUnsupportedField(c *gc.C) {
	err := initsystems.NewUnsupportedField("spam")

	c.Check(err, gc.ErrorMatches, `field "spam" .*`)
	c.Check(err, jc.Satisfies, errors.IsNotFound)
}

func (s *confSuite) TestNewUnsupportedItem(c *gc.C) {
	err := initsystems.NewUnsupportedItem("spam", "eggs")

	c.Check(err, gc.ErrorMatches, `field "spam", item "eggs" .*`)
	c.Check(err, jc.Satisfies, errors.IsNotFound)
}
