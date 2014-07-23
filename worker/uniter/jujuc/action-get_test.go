// Copyright 2012, 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package jujuc_test

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/juju/cmd"
	gc "launchpad.net/gocheck"
	"launchpad.net/goyaml"

	"github.com/juju/juju/testing"
	"github.com/juju/juju/worker/uniter/jujuc"
)

type ActionGetSuite struct {
	ContextSuite
}

var _ = gc.Suite(&ActionGetSuite{})

func (s *ActionGetSuite) TestActionGet(c *gc.C) {
	var actionGetTestMaps = []map[string]interface{}{
		map[string]interface{}{
			"outfile": "foo.bz2",
		},

		map[string]interface{}{
			"outfile": map[string]interface{}{
				"filename": "foo.bz2",
				"format":   "bzip",
			},
		},

		map[string]interface{}{
			"outfile": map[string]interface{}{
				"type": map[string]interface{}{
					"1": "raw",
					"2": "gzip",
					"3": "bzip",
				},
			},
		},
	}

	var actionGetTests = []struct {
		args         []string
		actionParams map[string]interface{}
		code         int
		out          interface{}
		errMsg       string
	}{{
		args:         []string{},
		actionParams: nil,
		out:          "",
	}, {
		args:         []string{"foo"},
		actionParams: nil,
		out:          "",
	}, {
		args:         []string{"outfile.type"},
		actionParams: actionGetTestMaps[1],
		out:          "",
	}, {
		args:         []string{"outfile.type.1"},
		actionParams: actionGetTestMaps[1],
		out:          "",
	}, {
		args:         []string{},
		actionParams: actionGetTestMaps[0],
		out:          actionGetTestMaps[0],
	}, {
		args:         []string{},
		actionParams: actionGetTestMaps[2],
		out:          actionGetTestMaps[2],
	}, {
		args:         []string{"outfile.type"},
		actionParams: actionGetTestMaps[2],
		out: map[string]interface{}{
			"1": "raw",
			"2": "gzip",
			"3": "bzip",
		},
	}}

	for i, t := range actionGetTests {
		for j, option := range []string{
			"",
			"--format yaml",
			"--format json",
		} {
			args := t.args
			if option != "" {
				args = append(strings.Split(option, " "), t.args...)
			}
			c.Logf("test %d: args: %#v", i*3+j, args)
			hctx := s.GetHookContext(c, -1, "")
			hctx.actionParams = t.actionParams
			com, err := jujuc.NewCommand(hctx, "action-get")
			c.Assert(err, gc.IsNil)
			ctx := testing.Context(c)
			code := cmd.Main(com, ctx, t.args)
			c.Check(code, gc.Equals, t.code)
			if code == 0 {
				c.Check(bufferString(ctx.Stderr), gc.Equals, "")
				var result []byte

				// If s is an empty string, the Stdout formatter won't
				// format it as expected.
				if s, ok := t.out.(string); ok && s == "" {
					result = []byte("")
				} else if option == "--format json" {
					result, err = json.Marshal(t.out)
					c.Assert(err, gc.IsNil)
				} else {
					result, err = goyaml.Marshal(t.out)
					c.Assert(err, gc.IsNil)
				}

				c.Check(bufferString(ctx.Stdout), gc.Equals, string(result)) // fmt.Sprintf("%v", t.out))
			} else {
				c.Check(bufferString(ctx.Stdout), gc.Equals, "")
				expect := fmt.Sprintf(`(.|\n)*error: %s\n`, t.errMsg)
				c.Check(bufferString(ctx.Stderr), gc.Matches, expect)
			}
		}
	}
}

func (s *ActionGetSuite) TestHelp(c *gc.C) {
	hctx := s.GetHookContext(c, -1, "")
	com, err := jujuc.NewCommand(hctx, "action-get")
	c.Assert(err, gc.IsNil)
	ctx := testing.Context(c)
	code := cmd.Main(com, ctx, []string{"--help"})
	c.Assert(code, gc.Equals, 0)
	c.Assert(bufferString(ctx.Stdout), gc.Equals, `usage: action-get [options] [<key>[.<key>.<key>...]]
purpose: get action parameters

options:
--format  (= smart)
    specify output format (json|smart|yaml)
-o, --output (= "")
    specify an output file

action-get will print the value of the parameter at the given key, serialized
as YAML.  If multiple keys are passed, action-get will recurse into the param
map as needed.
`)
	c.Assert(bufferString(ctx.Stderr), gc.Equals, "")
}

func (s *ActionGetSuite) TestUnknownArg(c *gc.C) {
	hctx := s.GetHookContext(c, -1, "")
	com, err := jujuc.NewCommand(hctx, "action-get")
	c.Assert(err, gc.IsNil)
	testing.TestInit(c, com, []string{"multiple", "keys"}, `unrecognized args: \["keys"\]`)
}
