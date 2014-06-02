// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package cmd_test

import (
	"fmt"
	"io/ioutil"

	"github.com/juju/testing"
	"launchpad.net/gnuflag"
	gc "launchpad.net/gocheck"

	"launchpad.net/juju-core/cmd"
)

type ArgsSuite struct {
	testing.LoggingSuite
}

var _ = gc.Suite(&ArgsSuite{})

func (*ArgsSuite) TestFlagsUsage(c *gc.C) {
	for i, test := range []struct {
		message       string
		defaultValue  []string
		args          []string
		expectedValue []string
	}{{
		message: "nil default and no arg",
	}, {
		message:       "default value and not set by args",
		defaultValue:  []string{"foo", "bar"},
		expectedValue: []string{"foo", "bar"},
	}, {
		message:       "no value set by args",
		args:          []string{"--value", "foo,bar"},
		expectedValue: []string{"foo", "bar"},
	}, {
		message:       "default value and set by args",
		defaultValue:  []string{"omg"},
		args:          []string{"--value", "foo,bar"},
		expectedValue: []string{"foo", "bar"},
	}} {
		c.Log(fmt.Sprintf("%v: %s", i, test.message))
		f := gnuflag.NewFlagSet("test", gnuflag.ContinueOnError)
		f.SetOutput(ioutil.Discard)
		var value []string
		f.Var(cmd.NewStringsValue(test.defaultValue, &value), "value", "help")
		err := f.Parse(false, test.args)
		c.Check(err, gc.IsNil)
		c.Check(value, gc.DeepEquals, test.expectedValue)
	}
}

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
		message:  "empty",
		expected: []string{""},
	}, {
		message:  "just whitespace",
		arg:      "   ",
		expected: []string{"   "},
	}, {
		message:  "whitespace and comma",
		arg:      "  ,  ",
		expected: []string{"  ", "  "},
	}, {
		message:  "single value",
		arg:      "foo",
		expected: []string{"foo"},
	}, {
		message:  "single value with comma",
		arg:      "foo,",
		expected: []string{"foo", ""},
	}, {
		message:  "single value with whitespace",
		arg:      " foo ",
		expected: []string{" foo "},
	}, {
		message:  "multiple values",
		arg:      "foo,bar,baz",
		expected: []string{"foo", "bar", "baz"},
	}, {
		message:  "multiple values with spaces",
		arg:      "foo, bar, baz",
		expected: []string{"foo", " bar", " baz"},
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

func (*ArgsSuite) TestAppendStringsUsage(c *gc.C) {
	for i, test := range []struct {
		message       string
		args          []string
		expectedValue []string
	}{{
		message: "no args",
	}, {
		message:       "value set by args",
		args:          []string{"--value", "foo", "--value=bar"},
		expectedValue: []string{"foo", "bar"},
	}} {
		c.Log(fmt.Sprintf("%v: %s", i, test.message))
		f := gnuflag.NewFlagSet("test", gnuflag.ContinueOnError)
		f.SetOutput(ioutil.Discard)
		var value []string
		f.Var(cmd.NewAppendStringsValue(&value), "value", "help")
		err := f.Parse(false, test.args)
		c.Check(err, gc.IsNil)
		c.Check(value, gc.DeepEquals, test.expectedValue)
	}
}
