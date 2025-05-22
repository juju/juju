// Copyright 2024 Canonical Ltd.
// Licensed under the LGPLv3, see LICENCE file for details.

package cmd_test

import (
	"bytes"
	"errors"
	"os"
	"testing"

	"github.com/juju/tc"

	"github.com/juju/juju/internal/cmd"
)

type markdownSuite struct{}

func TestMarkdownSuite(t *testing.T) {
	tc.Run(t, &markdownSuite{})
}

// TestWriteError ensures that the cmd.PrintMarkdown function surfaces errors
// returned by the writer.
func (*markdownSuite) TestWriteError(c *tc.C) {
	expectedErr := errors.New("foo")
	writer := errorWriter{err: expectedErr}
	command := &docTestCommand{
		info: &cmd.Info{},
	}
	err := cmd.PrintMarkdown(writer, command, cmd.MarkdownOptions{})
	c.Assert(err, tc.NotNil)
	c.Check(err, tc.ErrorMatches, ".*foo")
}

// errorWriter is an io.Writer that returns an error whenever the Write method
// is called.
type errorWriter struct {
	err error
}

func (e errorWriter) Write([]byte) (n int, err error) {
	return 0, e.err
}

// TestOutput tests that the output of the PrintMarkdown function is
// fundamentally correct.
func (*markdownSuite) TestOutput(c *tc.C) {
	seeAlso := []string{"clouds", "update-cloud", "remove-cloud", "update-credential"}
	subcommands := map[string]string{
		"foo": "foo the bar baz",
		"bar": "bar the baz foo",
		"baz": "baz the foo bar",
	}

	command := &docTestCommand{
		info: &cmd.Info{
			Name:        "add-cloud",
			Args:        "<cloud name> [<cloud definition file>]",
			Purpose:     "Add a cloud definition to Juju.",
			Doc:         "details for add-cloud...",
			Examples:    "examples for add-cloud...",
			SeeAlso:     seeAlso,
			Aliases:     []string{"new-cloud", "cloud-add"},
			Subcommands: subcommands,
		},
		flags: []testFlag{{
			name: "force",
		}, {
			name:  "file",
			short: "f",
		}, {
			name:  "credential",
			short: "c",
		}},
	}

	// These functions verify the provided argument is in the expected set.
	linkForCommand := func(s string) string {
		for _, cmd := range seeAlso {
			if cmd == s {
				return "https://docs.com/" + cmd
			}
		}
		c.Fatalf("linkForCommand called with unexpected command %q", s)
		return ""
	}

	linkForSubcommand := func(s string) string {
		_, ok := subcommands[s]
		if !ok {
			c.Fatalf("linkForSubcommand called with unexpected subcommand %q", s)
		}
		return "https://docs.com/add-cloud/" + s
	}

	expected, err := os.ReadFile("testdata/add-cloud.md")
	c.Assert(err, tc.ErrorIsNil)

	var buf bytes.Buffer
	err = cmd.PrintMarkdown(&buf, command, cmd.MarkdownOptions{
		Title:             `Command "juju add-cloud"`,
		UsagePrefix:       "juju ",
		LinkForCommand:    linkForCommand,
		LinkForSubcommand: linkForSubcommand,
	})
	c.Assert(err, tc.ErrorIsNil)
	c.Check(buf.String(), tc.Equals, string(expected))
}
