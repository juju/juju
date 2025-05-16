// Copyright 2015 Canonical Ltd.
// Licensed under the LGPLv3, see LICENSE file for details.

package cmd_test

import (
	_ "fmt"
	"os"
	"path/filepath"
	stdtesting "testing"

	"github.com/juju/tc"

	"github.com/juju/juju/internal/cmd"
	"github.com/juju/juju/internal/testhelpers"
)

type ParseAliasFileSuite struct {
	testhelpers.LoggingSuite
}

func TestParseAliasFileSuite(t *stdtesting.T) { tc.Run(t, &ParseAliasFileSuite{}) }
func (*ParseAliasFileSuite) TestMissing(c *tc.C) {
	dir := c.MkDir()
	filename := filepath.Join(dir, "missing")
	aliases := cmd.ParseAliasFile(c.Context(), filename)
	c.Assert(aliases, tc.NotNil)
	c.Assert(aliases, tc.HasLen, 0)
}

func (*ParseAliasFileSuite) TestParse(c *tc.C) {
	dir := c.MkDir()
	filename := filepath.Join(dir, "missing")
	content := `
# comments skipped, as are the blank lines, such as the line
# at the start of this file
   foo =  trailing-space    
repeat = first
flags = flags  --with   flag

# if the same alias name is used more than once, last one wins
repeat = second

# badly formated values are logged, but skipped
no equals sign
=
key = 
= value
`
	err := os.WriteFile(filename, []byte(content), 0644)
	c.Assert(err, tc.IsNil)
	aliases := cmd.ParseAliasFile(c.Context(), filename)
	c.Assert(aliases, tc.DeepEquals, map[string][]string{
		"foo":    {"trailing-space"},
		"repeat": {"second"},
		"flags":  {"flags", "--with", "flag"},
	})
}
