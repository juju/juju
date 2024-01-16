//go:build dqlite && linux

// Copyright 2024 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package app

import (
	"fmt"

	"github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"
)

type onceErrorSuite struct {
	testing.IsolationSuite
}

var _ = gc.Suite(&onceErrorSuite{})

func (s *onceErrorSuite) TestDoWithNil(c *gc.C) {
	var oe onceError
	err := oe.Do(func() error {
		return nil
	})
	c.Assert(err, gc.IsNil)

	var called bool
	err = oe.Do(func() error {
		called = true
		return nil
	})
	c.Assert(err, gc.IsNil)
	c.Check(called, jc.IsFalse)
}

func (s *onceErrorSuite) TestDoWithError(c *gc.C) {
	var oe onceError
	err := oe.Do(func() error {
		return fmt.Errorf("boom")
	})
	c.Assert(err, gc.ErrorMatches, "boom")

	err = oe.Do(func() error {
		return fmt.Errorf("blah")
	})
	c.Assert(err, gc.ErrorMatches, "boom")
}
