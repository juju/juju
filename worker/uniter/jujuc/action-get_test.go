// Copyright 2012, 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package jujuc_test

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/juju/cmd"
	jc "github.com/juju/testing/checkers"
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
	var actionGetTestMaps = []map[interface{}]interface{}{
		map[interface{}]interface{}{
			"outfile": "foo.bz2",
		},

		map[interface{}]interface{}{
			"outfile": map[interface{}]interface{}{
				"filename": "foo.bz2",
				"format":   "bzip",
			},
		},

		map[interface{}]interface{}{
			"outfile": map[interface{}]interface{}{
				"type": map[interface{}]interface{}{
					"1": "raw",
					"2": "gzip",
					"3": "bzip",
				},
			},
		},
	}

	var actionGetTests = []struct {
		args         []string
		actionParams map[interface{}]interface{}
		code         int
		out          interface{}
		errMsg       string
	}{{
		args:         []string{},
		actionParams: nil,
	}, {
		args:         []string{"foo"},
		actionParams: nil,
	}, {
		args:         []string{"outfile.type"},
		actionParams: actionGetTestMaps[1],
	}, {
		args:         []string{"outfile.type.1"},
		actionParams: actionGetTestMaps[1],
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
		out: map[interface{}]interface{}{
			"1": "raw",
			"2": "gzip",
			"3": "bzip",
		},
	}}

	for i, t := range actionGetTests {
		for j, option := range []string{
			"",
			"--format=yaml",
			"--format=json",
		} {
			args := t.args
			if option != "" {
				args = append(strings.Split(option, " "), t.args...)
			}
			c.Logf("test %d: args: %#v", i*3+j, args)
			hctx := s.GetHookContext(c, -1, "")
			// This is necessary because Action params should be
			// map[string]interface, but YAML returns m[i{}]i{}.
			// The alternative is to recursively coerce all inner
			// maps to have string keys.
			coercedParams := make(map[string]interface{})
			for key, value := range t.actionParams {
				if stringKey, ok := key.(string); ok {
					coercedParams[stringKey] = value
				} else {
					c.Logf("There was a bad key: %#v", key)
					c.Fail()
				}
			}
			hctx.actionParams = coercedParams

			com, err := jujuc.NewCommand(hctx, "action-get")
			c.Assert(err, gc.IsNil)
			ctx := testing.Context(c)
			code := cmd.Main(com, ctx, t.args)
			c.Check(code, gc.Equals, t.code)
			if code == 0 {
				c.Check(bufferString(ctx.Stderr), gc.Equals, "")

				var result interface{}
				if option == "--format=json" {
					// if nil, don't worry about unmarshaling
					if t.out == nil {
						c.Check(bufferBytes(ctx.Stderr), jc.DeepEquals, []byte{})
					} else {
						err = json.Unmarshal(bufferBytes(ctx.Stdout), &result)
						if err != nil {
							c.Logf("Unexpected JSON error: %q", bufferString(ctx.Stdout))
						}
						c.Assert(err, gc.IsNil)
						switch tResult := result.(type) {
						case map[string]interface{}:
							if tExpect, ok := t.out.(map[interface{}]interface{}); ok {
								expect, err := coerceKeysToStrings(tExpect)
								c.Check(err, gc.IsNil)
								c.Check(tResult, jc.DeepEquals, expect)
							} else {
								c.Logf("Unexpected type %T", t.out)
								c.Fail()
							}
						default:
							c.Check(result, jc.DeepEquals, t.out)
						}
					}
				} else {
					// Otherwise, it was YAML.
					err = goyaml.Unmarshal(bufferBytes(ctx.Stdout), &result)
					c.Assert(err, gc.IsNil)
					if t.out == nil {
						switch result.(type) {
						case map[interface{}]interface{}:
							c.Check(result, jc.DeepEquals, map[interface{}]interface{}{})
						default:
							c.Check(result, jc.DeepEquals, t.out)
						}
					} else {
						c.Check(result, jc.DeepEquals, t.out)
					}
				}
			} else {
				c.Check(bufferString(ctx.Stdout), gc.Equals, "")
				expect := fmt.Sprintf(`(.|\n)*error: %s\n`, t.errMsg)
				c.Check(bufferString(ctx.Stderr), gc.Matches, expect)
			}
		}
	}
}

func coerceKeysToStrings(in map[interface{}]interface{}) (map[string]interface{}, error) {
	ans := make(map[string]interface{})
	for k, v := range in {
		if tK, ok := k.(string); ok {
			ans[tK] = v
		} else {
			return nil, fmt.Errorf("Key was not a string")
		}
	}
	return ans, nil
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
