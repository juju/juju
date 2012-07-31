package cmd_test

import (
	"launchpad.net/gnuflag"
	. "launchpad.net/gocheck"
	"launchpad.net/juju-core/cmd"
)

// OutputCommand is a command that uses the output.go formatters.
type OutputCommand struct {
	out   cmd.Output
	value interface{}
}

func (c *OutputCommand) Info() *cmd.Info {
	return &cmd.Info{"output", "<something>", "I like to output", "output"}
}

func (c *OutputCommand) Init(f *gnuflag.FlagSet, args []string) error {
	c.out.AddFlags(f, "smart", cmd.DefaultFormatters)
	if err := f.Parse(true, args); err != nil {
		return err
	}
	return cmd.CheckEmpty(f.Args())
}

func (c *OutputCommand) Run(ctx *cmd.Context) error {
	return c.out.Write(ctx, c.value)
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
		{1, "1\n"},
		{-1, "-1\n"},
		{1.1, "1.1\n"},
		{true, "true\n"},
		{false, "false\n"},
		{"hello", "hello\n"},
		{"\n\n\n", "\n\n\n\n"},
		{"foo: bar", "foo: bar\n"},
		{[]string{"blam", "dink"}, "blam\ndink\n"},
		{map[interface{}]interface{}{"foo": "bar"}, "foo: bar\n"},
	},
	"smart": {
		{1, "1\n"},
		{-1, "-1\n"},
		{1.1, "1.1\n"},
		{true, "true\n"},
		{false, "false\n"},
		{"hello", "hello\n"},
		{"\n\n\n", "\n\n\n\n"},
		{"foo: bar", "foo: bar\n"},
		{[]string{"blam", "dink"}, "blam\ndink\n"},
		{map[interface{}]interface{}{"foo": "bar"}, "foo: bar\n"},
	},
	"json": {
		{1, "1\n"},
		{-1, "-1\n"},
		{1.1, "1.1\n"},
		{true, "true\n"},
		{false, "false\n"},
		{"hello", `"hello"` + "\n"},
		{"\n\n\n", `"\n\n\n"` + "\n"},
		{"foo: bar", `"foo: bar"` + "\n"},
		{[]string{"blam", "dink"}, `["blam","dink"]` + "\n"},
		{defaultValue, `{"Juju":1,"Puppet":false}` + "\n"},
	},
	"yaml": {
		{1, "1\n"},
		{-1, "-1\n"},
		{1.1, "1.1\n"},
		{true, "true\n"},
		{false, "false\n"},
		{"hello", "hello\n"},
		{"\n\n\n", "'\n\n\n\n'\n"},
		{"foo: bar", "'foo: bar'\n"},
		{[]string{"blam", "dink"}, "- blam\n- dink\n"},
		{defaultValue, "juju: 1\npuppet: false\n"},
	},
}

func (s *CmdSuite) TestOutputFormat(c *C) {
	for format, tests := range outputTests {
		c.Logf("format %s", format)
		args := []string{}
		if format != "" {
			args = []string{"--format", format}
		}
		for i, t := range tests {
			c.Logf("  test %d", i)
			ctx := dummyContext(c)
			result := cmd.Main(&OutputCommand{value: t.value}, ctx, args)
			c.Assert(result, Equals, 0)
			c.Assert(bufferString(ctx.Stdout), Equals, t.output)
			c.Assert(bufferString(ctx.Stderr), Equals, "")
		}
	}

	ctx := dummyContext(c)
	result := cmd.Main(&OutputCommand{}, ctx, []string{"--format", "cuneiform"})
	c.Assert(result, Equals, 2)
	c.Assert(bufferString(ctx.Stdout), Equals, "")
	c.Assert(bufferString(ctx.Stderr), Matches, "usage(.|\n)*: unknown format \"cuneiform\"\n")
}
