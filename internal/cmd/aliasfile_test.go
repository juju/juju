// Copyright 2015 Canonical Ltd.
// Licensed under the LGPLv3, see LICENSE file for details.

package cmd_test

import (
	_ "fmt"
	"io/ioutil"
	"path/filepath"

	"github.com/juju/testing"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/internal/cmd"
)

type ParseAliasFileSuite struct {
	testing.LoggingSuite
}

var _ = gc.Suite(&ParseAliasFileSuite{})

func (*ParseAliasFileSuite) TestMissing(c *gc.C) {
	dir := c.MkDir()
	filename := filepath.Join(dir, "missing")
	aliases := cmd.ParseAliasFile(filename)
	c.Assert(aliases, gc.NotNil)
	c.Assert(aliases, gc.HasLen, 0)
}

func (*ParseAliasFileSuite) TestParse(c *gc.C) {
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
	err := ioutil.WriteFile(filename, []byte(content), 0644)
	c.Assert(err, gc.IsNil)
	aliases := cmd.ParseAliasFile(filename)
	c.Assert(aliases, gc.DeepEquals, map[string][]string{
		"foo":    []string{"trailing-space"},
		"repeat": []string{"second"},
		"flags":  []string{"flags", "--with", "flag"},
	})
}
