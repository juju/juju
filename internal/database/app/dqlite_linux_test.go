//go:build dqlite && linux

// Copyright 2024 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package app

import (
	"fmt"

	"github.com/juju/tc"
	"github.com/juju/testing"
)

type onceErrorSuite struct {
	testing.IsolationSuite
}

var _ = tc.Suite(&onceErrorSuite{})

func (s *onceErrorSuite) TestDoWithNil(c *tc.C) {
	var oe onceError
	err := oe.Do(func() error {
		return nil
	})
	c.Assert(err, tc.IsNil)

	var called bool
	err = oe.Do(func() error {
		called = true
		return nil
	})
	c.Assert(err, tc.IsNil)
	c.Check(called, tc.IsFalse)
}

func (s *onceErrorSuite) TestDoWithError(c *tc.C) {
	var oe onceError
	err := oe.Do(func() error {
		return fmt.Errorf("boom")
	})
	c.Assert(err, tc.ErrorMatches, "boom")

	err = oe.Do(func() error {
		return fmt.Errorf("blah")
	})
	c.Assert(err, tc.ErrorMatches, "boom")
}
