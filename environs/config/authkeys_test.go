// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package config_test

import (
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/environs/config"
	"github.com/juju/testing"
)

type AuthKeysSuite struct {
	testing.IsolationSuite
}

var _ = gc.Suite(&AuthKeysSuite{})

func (s *AuthKeysSuite) TestConcatAuthKeys(c *gc.C) {
	for _, test := range []struct{ a, b, result string }{
		{"a", "", "a"},
		{"", "b", "b"},
		{"a", "b", "a\nb"},
		{"a\n", "b", "a\nb"},
	} {
		c.Check(config.ConcatAuthKeys(test.a, test.b), gc.Equals, test.result)
	}
}
