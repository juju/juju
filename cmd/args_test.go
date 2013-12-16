// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package cmd_test

import (
	"fmt"

	gc "launchpad.net/gocheck"

	"launchpad.net/juju-core/cmd"
	"launchpad.net/juju-core/testing/testbase"
)

type ArgsSuite struct {
	testbase.LoggingSuite
}

var _ = gc.Suite(&ArgsSuite{})

func (*ArgsSuite) TestNewStringsValue(c *gc.C) {
	for i, test := range []struct {
		message      string
		defaultValue []string
	}{{
		message: "null default",
	}, {
		message:      "empty default",
		defaultValue: []string{},
	}, {
		message:      "single value",
		defaultValue: []string{"foo"},
	}, {
		message:      "multiple values",
		defaultValue: []string{"foo", "bar", "baz"},
	}} {
		c.Log(fmt.Sprintf("%v: %s", i, test.message))
		var underlyingValue []string
		_ = cmd.NewStringsValue(test.defaultValue, &underlyingValue)
		c.Assert(underlyingValue, gc.DeepEquals, test.defaultValue)
	}
}

func (*ArgsSuite) TestSet(c *gc.C) {
	for i, test := range []struct {
		message  string
		arg      string
		expected []string
	}{{
		message: "empty",
	}, {
		message: "just whitespace",
		arg:     "   ",
	}, {
		message: "whitespace and comma",
		arg:     "  ,  ",
	}, {
		message:  "single value",
		arg:      "foo",
		expected: []string{"foo"},
	}, {
		message:  "single value with comma",
		arg:      "foo,",
		expected: []string{"foo"},
	}, {
		message:  "single value with whitespace",
		arg:      " foo ",
		expected: []string{"foo"},
	}, {
		message:  "multiple values",
		arg:      "foo,bar,baz",
		expected: []string{"foo", "bar", "baz"},
	}, {
		message:  "multiple values with spaces",
		arg:      "foo, bar, baz",
		expected: []string{"foo", "bar", "baz"},
	}} {
		c.Log(fmt.Sprintf("%v: %s", i, test.message))
		var result []string
		value := cmd.NewStringsValue(nil, &result)
		error := value.Set(test.arg)
		c.Check(error, gc.IsNil)
		c.Check(result, gc.DeepEquals, test.expected)
	}
}

func (*ArgsSuite) TestString(c *gc.C) {
	for i, test := range []struct {
		message  string
		target   []string
		expected string
	}{{
		message:  "null",
		expected: "",
	}, {
		message:  "empty",
		target:   []string{},
		expected: "",
	}, {
		message:  "single value",
		target:   []string{"foo"},
		expected: "foo",
	}, {
		message:  "multiple values",
		target:   []string{"foo", "bar", "baz"},
		expected: "foo,bar,baz",
	}} {
		c.Log(fmt.Sprintf("%v: %s", i, test.message))
		var temp []string
		value := cmd.NewStringsValue(test.target, &temp)
		c.Assert(value.String(), gc.Equals, test.expected)
	}
}
