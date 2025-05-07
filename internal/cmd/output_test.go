// Copyright 2012, 2013 Canonical Ltd.
// Licensed under the LGPLv3, see LICENSE file for details.

package cmd_test

import (
	"github.com/juju/gnuflag"
	"github.com/juju/loggo/v2"
	"github.com/juju/tc"

	"github.com/juju/juju/internal/cmd"
	"github.com/juju/juju/internal/cmd/cmdtesting"
	"github.com/juju/juju/internal/testhelpers"
)

// OutputCommand is a command that uses the output.go formatters.
type OutputCommand struct {
	cmd.CommandBase
	out   cmd.Output
	value interface{}
}

func (c *OutputCommand) Info() *cmd.Info {
	return &cmd.Info{
		Name:    "output",
		Args:    "<something>",
		Purpose: "I like to output",
		Doc:     "output",
	}
}

func (c *OutputCommand) SetFlags(f *gnuflag.FlagSet) {
	formatters := make(map[string]cmd.Formatter, len(cmd.DefaultFormatters))
	for k, v := range cmd.DefaultFormatters {
		formatters[k] = v.Formatter
	}
	c.out.AddFlags(f, "smart", formatters)
}

func (c *OutputCommand) Init(args []string) error {
	return cmd.CheckEmpty(args)
}

func (c *OutputCommand) Run(ctx *cmd.Context) error {
	if value, ok := c.value.(overrideFormatter); ok {
		return c.out.WriteFormatter(ctx, value.formatter, value.value)
	}
	return c.out.Write(ctx, c.value)
}

type overrideFormatter struct {
	formatter cmd.Formatter
	value     interface{}
}

// use a struct to control field ordering.
var defaultValue = struct {
	Juju   int
	Puppet bool
}{1, false}

var outputTests = map[string][]struct {
	value  interface{}
	output string
}{
	"": {
		{nil, ""},
		{"", ""},
		{1, "1\n"},
		{-1, "-1\n"},
		{1.1, "1.1\n"},
		{10000000, "10000000\n"},
		{true, "True\n"},
		{false, "False\n"},
		{"hello", "hello\n"},
		{"\n\n\n", "\n\n\n\n"},
		{"foo: bar", "foo: bar\n"},
		{[]string{}, ""},
		{[]string{"blam", "dink"}, "blam\ndink\n"},
		{map[interface{}]interface{}{"foo": "bar"}, "foo: bar\n"},
		{overrideFormatter{cmd.FormatSmart, "abc\ndef"}, "abc\ndef\n"},
	},
	"smart": {
		{nil, ""},
		{"", ""},
		{1, "1\n"},
		{-1, "-1\n"},
		{1.1, "1.1\n"},
		{10000000, "10000000\n"},
		{true, "True\n"},
		{false, "False\n"},
		{"hello", "hello\n"},
		{"\n\n\n", "\n\n\n\n"},
		{"foo: bar", "foo: bar\n"},
		{[]string{}, ""},
		{[]string{"blam", "dink"}, "blam\ndink\n"},
		{map[interface{}]interface{}{"foo": "bar"}, "foo: bar\n"},
		{overrideFormatter{cmd.FormatSmart, "abc\ndef"}, "abc\ndef\n"},
	},
	"json": {
		{nil, "null\n"},
		{"", `""` + "\n"},
		{1, "1\n"},
		{-1, "-1\n"},
		{1.1, "1.1\n"},
		{10000000, "10000000\n"},
		{true, "true\n"},
		{false, "false\n"},
		{"hello", `"hello"` + "\n"},
		{"\n\n\n", `"\n\n\n"` + "\n"},
		{"foo: bar", `"foo: bar"` + "\n"},
		{[]string{}, `[]` + "\n"},
		{[]string{"blam", "dink"}, `["blam","dink"]` + "\n"},
		{defaultValue, `{"Juju":1,"Puppet":false}` + "\n"},
		{overrideFormatter{cmd.FormatSmart, "abc\ndef"}, "abc\ndef\n"},
		{overrideFormatter{cmd.FormatJson, struct{}{}}, "{}\n"},
	},
	"yaml": {
		{nil, ""},
		{"", `""` + "\n"},
		{1, "1\n"},
		{-1, "-1\n"},
		{1.1, "1.1\n"},
		{10000000, "10000000\n"},
		{true, "true\n"},
		{false, "false\n"},
		{"hello", "hello\n"},
		{"\n\n\n", "|2+\n"},
		{"foo: bar", "'foo: bar'\n"},
		{[]string{}, "[]\n"},
		{[]string{"blam", "dink"}, "- blam\n- dink\n"},
		{defaultValue, "juju: 1\npuppet: false\n"},
		{overrideFormatter{cmd.FormatSmart, "abc\ndef"}, "abc\ndef\n"},
		{overrideFormatter{cmd.FormatYaml, struct{}{}}, "{}\n"},
	},
}

type OutputSuite struct {
	testhelpers.LoggingCleanupSuite

	ctx *cmd.Context
}

var _ = tc.Suite(&OutputSuite{})

func (s *OutputSuite) SetUpTest(c *tc.C) {
	s.LoggingCleanupSuite.SetUpTest(c)
	s.ctx = cmdtesting.Context(c)
	loggo.ReplaceDefaultWriter(cmd.NewWarningWriter(s.ctx.Stderr))
}

func (s *OutputSuite) TestOutputFormat(c *tc.C) {
	s.testOutputFormat(c, "")
}
func (s *OutputSuite) TestOutputFormatSmart(c *tc.C) {
	s.testOutputFormat(c, "smart")
}
func (s *OutputSuite) TestOutputFormatJson(c *tc.C) {
	s.testOutputFormat(c, "json")
}
func (s *OutputSuite) TestOutputFormatYaml(c *tc.C) {
	s.testOutputFormat(c, "yaml")
}

func (s *OutputSuite) testOutputFormat(c *tc.C, format string) {
	tests := outputTests[format]
	var args []string
	if format != "" {
		args = []string{"--format", format}
	}
	for i, t := range tests {
		c.Logf("  test %d", i)
		s.SetUpTest(c)
		result := cmd.Main(&OutputCommand{value: t.value}, s.ctx, args)
		c.Check(result, tc.Equals, 0)
		c.Check(bufferString(s.ctx.Stdout), tc.Equals, t.output)
		c.Check(bufferString(s.ctx.Stderr), tc.Equals, "")
		s.TearDownTest(c)
	}
}

func (s *OutputSuite) TestUnknownOutputFormat(c *tc.C) {
	result := cmd.Main(&OutputCommand{}, s.ctx, []string{"--format", "cuneiform"})
	c.Check(result, tc.Equals, 2)
	c.Check(bufferString(s.ctx.Stdout), tc.Equals, "")
	c.Check(bufferString(s.ctx.Stderr), tc.Matches, ".*: unknown format \"cuneiform\"\n")
}

// Py juju allowed both --format json and --format=json. This test verifies that juju is
// being built against a version of the gnuflag library (rev 14 or above) that supports
// this argument format.
// LP #1059921
func (s *OutputSuite) TestFormatAlternativeSyntax(c *tc.C) {
	result := cmd.Main(&OutputCommand{}, s.ctx, []string{"--format=json"})
	c.Assert(result, tc.Equals, 0)
	c.Assert(bufferString(s.ctx.Stdout), tc.Equals, "null\n")
}

func (s *OutputSuite) TestFormatters(c *tc.C) {
	typeFormatters := cmd.DefaultFormatters
	formatters := typeFormatters.Formatters()

	c.Assert(len(typeFormatters), tc.Equals, len(formatters))
	for k := range typeFormatters {
		_, ok := formatters[k]
		c.Assert(ok, tc.Equals, true)
	}
}
