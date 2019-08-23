// Copyright 2019 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package series

import (
	"testing/quick"

	"github.com/juju/collections/set"
	"github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"
)

type SeriesValidateSuite struct {
	testing.IsolationSuite
}

var _ = gc.Suite(&SeriesValidateSuite{})

func (*SeriesValidateSuite) TestValidate(c *gc.C) {
	fn := func(s string) bool {
		if len(s) == 0 {
			// If the string is empty, ensure we don't test that, so just add
			// a default text to test against.
			s = "foo"
		}
		result, err := ValidateSeries(set.NewStrings(s), s, "__bad__")
		if err != nil {
			c.Fatal(err)
		}
		return result == s
	}
	if err := quick.Check(fn, nil); err != nil {
		c.Assert(err, jc.ErrorIsNil)
	}
}

func (*SeriesValidateSuite) TestFallbackValidate(c *gc.C) {
	fn := func(s string) bool {
		if len(s) == 0 {
			// If the string is empty, ensure we don't test that, so just add
			// a default text to test against.
			s = "foo"
		}
		result, err := ValidateSeries(set.NewStrings(s), "", s)
		if err != nil {
			c.Fatal(err)
		}
		return result == s
	}
	if err := quick.Check(fn, nil); err != nil {
		c.Assert(err, jc.ErrorIsNil)
	}
}

func (*SeriesValidateSuite) TestValidateError(c *gc.C) {
	_, err := ValidateSeries(set.NewStrings("bar"), "foo", "faz")
	c.Assert(err, gc.ErrorMatches, "foo not supported")
}

func (*SeriesValidateSuite) TestFallbackValidateError(c *gc.C) {
	_, err := ValidateSeries(set.NewStrings("bar"), "", "faz")
	c.Assert(err, gc.ErrorMatches, "faz not supported")
}
