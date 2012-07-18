package cmd_test

import (
	"launchpad.net/gnuflag"
	. "launchpad.net/gocheck"
	"launchpad.net/juju-core/cmd"
)

// OutputCommand is a command that uses the output.go formatters.
type OutputCommand struct {
	out cmd.Output
}

func (c *OutputCommand) Info() *cmd.Info {
	return &cmd.Info{"output", "<something>", "I like to output", "output"}
}

func (c *OutputCommand) Init(f *gnuflag.FlagSet, args []string) error {
	c.out.AddFlags(f, "yaml", cmd.DefaultFormatters)
	if err := f.Parse(true, args); err != nil {
		return err
	}
	return cmd.CheckEmpty(f.Args())
}

func (c *OutputCommand) Run(ctx *cmd.Context) error {
	// use a struct to control field ordering.
	v := struct {
		Juju   int
		Puppet bool
	}{1, false}
	return c.out.Write(ctx, v)
}

var outputTests = []struct {
	options        []string
	result         int
	stdout, stderr string
}{{
	// default
	nil,
	0,
	"juju: 1\npuppet: false\n\n",
	"",
}, {
	[]string{"--format", "yaml"},
	0,
	"juju: 1\npuppet: false\n\n",
	"",
}, {
	[]string{"--format", "json"},
	0,
	"{\"Juju\":1,\"Puppet\":false}\n",
	"",
}, {
	[]string{"--format", "cuneiform"},
	2,
	"",
	`usage(.|\n)*invalid value \"cuneiform\"(.|\n)*`,
}}

func (s *CmdSuite) TestOutputFormat(c *C) {
	for _, t := range outputTests {
		ctx := dummyContext(c)
		c.Logf("Options: %v", t.options)
		result := cmd.Main(&OutputCommand{}, ctx, t.options)
		c.Assert(result, Equals, t.result)
		c.Assert(bufferString(ctx.Stdout), Equals, t.stdout)
		c.Assert(bufferString(ctx.Stderr), Matches, t.stderr)
	}
}
