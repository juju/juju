// Copyright 2012-2022 Canonical Ltd.
// Licensed under the LGPLv3, see LICENSE file for details.

package cmd_test

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/juju/gnuflag"
	"github.com/juju/tc"
	jc "github.com/juju/testing/checkers"

	"github.com/juju/juju/internal/cmd"
)

type documentationSuite struct{}

var _ = tc.Suite(&documentationSuite{})

func (s *documentationSuite) TestFormatCommand(c *tc.C) {
	tests := []struct {
		command  cmd.Command
		title    bool
		expected string
	}{{
		// "smoke test" - just a regular command
		command: &docTestCommand{
			info: &cmd.Info{
				Name:     "add-cloud",
				Args:     "<cloud name> [<cloud definition file>]",
				Purpose:  "summary for add-cloud...",
				Doc:      "details for add-cloud...",
				Examples: "examples for add-cloud...",
				SeeAlso:  []string{"clouds", "update-cloud", "remove-cloud", "update-credential"},
				Aliases:  []string{"cloud-add", "import-cloud"},
			},
			flags: []testFlag{{name: "force", short: "f"}, {name: "format" /* no short flag */}, {name: "output", short: "o"}},
		},
		title: false,
		expected: (`
> See also: [clouds](#clouds), [update-cloud](#update-cloud), [remove-cloud](#remove-cloud), [update-credential](#update-credential)

**Aliases:** cloud-add, import-cloud

## Summary
summary for add-cloud...

## Usage
` + "```" + `juju add-cloud [options] <cloud name> [<cloud definition file>]` + "```" + `

### Options
| Flag | Default | Usage |
| --- | --- | --- |
| ` + "`" + `-f` + "`" + `, ` + "`" + `--force` + "`" + ` | default value for "force" flag | description for "force" flag |
| ` + "`" + `--format` + "`" + ` | default value for "format" flag | description for "format" flag |
| ` + "`" + `-o` + "`" + `, ` + "`" + `--output` + "`" + ` | default value for "output" flag | description for "output" flag |

## Examples
examples for add-cloud...

## Details
details for add-cloud...

`)[1:],
	}, {
		// no flags - don't print "Options" table
		command: &docTestCommand{
			info: &cmd.Info{
				Name:     "foo",
				Args:     "<args>",
				Purpose:  "insert summary here...",
				Doc:      "insert details here...",
				Examples: "insert examples here...",
			},
			flags: []testFlag{},
		},
		title: false,
		expected: (`
## Summary
insert summary here...

## Usage
` + "```" + `juju foo [options] <args>` + "```" + `

## Examples
insert examples here...

## Details
insert details here...

`)[1:],
	}}

	for _, t := range tests {
		output, err := cmd.FormatCommand(
			t.command,
			&cmd.SuperCommand{Name: "juju"},
			t.title,
			[]string{"juju", t.command.Info().Name},
		)
		c.Check(output, tc.Equals, t.expected)
		c.Check(err, jc.ErrorIsNil)
	}
}

// docTestCommand is a fake implementation of cmd.Command, used for testing
// documentation output.
type docTestCommand struct {
	info  *cmd.Info
	flags []testFlag
}

// testFlag is a definition for flag in test. It allows to provide an optional short name for the flag
type testFlag struct {
	name  string
	short string // optional
}

func (c *docTestCommand) Info() *cmd.Info {
	return c.info
}

func (c *docTestCommand) SetFlags(f *gnuflag.FlagSet) {
	for _, flag := range c.flags {
		var fakeValue string
		defaultValue := fmt.Sprintf("default value for %q flag", flag.name)
		description := fmt.Sprintf("description for %q flag", flag.name)
		f.StringVar(&fakeValue, flag.name,
			defaultValue,
			description)
		if flag.short != "" {
			f.StringVar(&fakeValue, flag.short,
				defaultValue,
				description)
		}
	}
}

func (c *docTestCommand) IsSuperCommand() bool         { return false }
func (c *docTestCommand) Init(args []string) error     { return nil }
func (c *docTestCommand) Run(ctx *cmd.Context) error   { return nil }
func (c *docTestCommand) AllowInterspersedFlags() bool { return false }

func (*documentationSuite) TestEscapeMarkdown(c *tc.C) {
	tests := []struct {
		input, output string
	}{{
		input: `
Juju needs to know how to connect to clouds. A cloud definition 
describes a cloud's endpoints and authentication requirements. Each
definition is stored and accessed later as <cloud name>.

If you are accessing a public cloud, running add-cloud is unlikely to be 
necessary.  Juju already contains definitions for the public cloud 
providers it supports.

add-cloud operates in two modes:

    juju add-cloud
    juju add-cloud <cloud name> <cloud definition file>
`,
		output: `
Juju needs to know how to connect to clouds. A cloud definition 
describes a cloud's endpoints and authentication requirements. Each
definition is stored and accessed later as &lt;cloud name&gt;.

If you are accessing a public cloud, running add-cloud is unlikely to be 
necessary.  Juju already contains definitions for the public cloud 
providers it supports.

add-cloud operates in two modes:

    juju add-cloud
    juju add-cloud <cloud name> <cloud definition file>
`,
	}, {
		input:  "Specify output format (default|json|tabular|yaml)",
		output: "Specify output format (default&#x7c;json&#x7c;tabular&#x7c;yaml)",
	}, {
		input:  "Model to operate in. Accepts [<controller name>:]<model name>|<model UUID>",
		output: "Model to operate in. Accepts [&lt;controller name&gt;:]&lt;model name&gt;&#x7c;&lt;model UUID&gt;",
	}, {
		input:  "The following characters are inside a code span, so they shouldn't be escaped: `< > | &`",
		output: "The following characters are inside a code span, so they shouldn't be escaped: `< > | &`",
	}, {
		input: `
The juju add-credential command operates in two modes.

When called with only the <cloud name> argument, ` + "`" + `juju add-credential` + "`" + ` will 
take you through an interactive prompt to add a credential specific to 
the cloud provider.

Providing the ` + "`" + `-f <credentials.yaml>` + "`" + ` option switches to the 
non-interactive mode. <credentials.yaml> must be a path to a correctly 
formatted YAML-formatted file.
`,
		output: `
The juju add-credential command operates in two modes.

When called with only the &lt;cloud name&gt; argument, ` + "`" + `juju add-credential` + "`" + ` will 
take you through an interactive prompt to add a credential specific to 
the cloud provider.

Providing the ` + "`" + `-f <credentials.yaml>` + "`" + ` option switches to the 
non-interactive mode. &lt;credentials.yaml&gt; must be a path to a correctly 
formatted YAML-formatted file.
`,
	}}

	for _, t := range tests {
		c.Check(cmd.EscapeMarkdown(t.input), tc.Equals, t.output)
	}
}

// TestWriteIndex checks that the index file is successfully written.
func (*documentationSuite) TestWriteIndex(c *tc.C) {
	// Make temp dir to hold docs
	docsDir := c.MkDir()

	// Create a supercommand and run the documentation command
	superCmd := cmd.NewSuperCommand(cmd.SuperCommandParams{})
	superCmd.SetFlags(&gnuflag.FlagSet{})
	err := superCmd.Init([]string{"documentation", "--split", "--out", docsDir})
	c.Assert(err, tc.IsNil)
	err = superCmd.Run(&cmd.Context{})
	c.Assert(err, tc.IsNil)

	// Check the index file
	indexPath := filepath.Join(docsDir, "index.md")
	indexContents, err := os.ReadFile(indexPath)
	c.Assert(err, tc.IsNil)
	// Index should be non-empty
	c.Assert(string(indexContents), tc.Matches, "(?m).*Index.*")
}
