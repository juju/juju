// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package jujuc_test

import (
	"fmt"

	"github.com/juju/cmd"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/apiserver/params"
	"github.com/juju/juju/testing"
	"github.com/juju/juju/worker/uniter/runner/jujuc"
	jujuctesting "github.com/juju/juju/worker/uniter/runner/jujuc/testing"
)

type NetworkGetSuite struct {
	relationSuite
}

var _ = gc.Suite(&NetworkGetSuite{})

func (s *NetworkGetSuite) newHookContext(relid int) (jujuc.Context, *relationInfo) {
	netConfig := []params.NetworkConfig{
		{Address: "8.8.8.8"},
		{Address: "10.0.0.1"},
	}

	hctx, info := s.relationSuite.newHookContext(relid, "remote")
	info.rels[0].Units["u/0"]["private-address"] = "foo: bar\n"
	info.rels[1].SetRelated("m/0", jujuctesting.Settings{"pew": "pew\npew\n"}, netConfig)
	info.rels[1].SetRelated("u/1", jujuctesting.Settings{"value": "12345"}, netConfig)
	return hctx, info
}

func (s *NetworkGetSuite) TestNetworkGet(c *gc.C) {
	for i, t := range []struct {
		summary  string
		relid    int
		args     []string
		code     int
		out      string
		checkctx func(*gc.C, *cmd.Context)
	}{{
		summary: "no default relation",
		relid:   -1,
		code:    2,
		out:     `no relation id specified`,
	}, {
		summary: "explicit relation, not known",
		relid:   -1,
		code:    2,
		args:    []string{"-r", "burble:123"},
		out:     `invalid value "burble:123" for flag -r: relation not found`,
	}, {
		summary: "default relation, no --primary-address given",
		relid:   1,
		code:    2,
		out:     `--primary-address is currently required`,
	}, {
		summary: "explicit relation, no --primary-address given",
		relid:   -1,
		code:    2,
		args:    []string{"-r", "burble:1"},
		out:     `--primary-address is currently required`,
	}, {
		summary: "explicit relation with --primary-address",
		relid:   1,
		args:    []string{"-r", "burble:1", "--primary-address"},
		out:     "8.8.8.8",
	}, {
		summary: "default relation with --primary-address",
		relid:   1,
		args:    []string{"--primary-address"},
		out:     "8.8.8.8",
	}} {
		c.Logf("test %d: %s", i, t.summary)
		hctx, _ := s.newHookContext(t.relid)
		com, err := jujuc.NewCommand(hctx, cmdString("network-get"))
		c.Assert(err, jc.ErrorIsNil)
		ctx := testing.Context(c)
		code := cmd.Main(com, ctx, t.args)
		c.Check(code, gc.Equals, t.code)
		if code == 0 {
			c.Check(bufferString(ctx.Stderr), gc.Equals, "")
			expect := t.out
			if expect != "" {
				expect = expect + "\n"
			}
			c.Check(bufferString(ctx.Stdout), gc.Equals, expect)
		} else {
			c.Check(bufferString(ctx.Stdout), gc.Equals, "")
			expect := fmt.Sprintf(`(.|\n)*error: %s\n`, t.out)
			c.Check(bufferString(ctx.Stderr), gc.Matches, expect)
		}
	}
}

func (s *NetworkGetSuite) TestHelp(c *gc.C) {

	var helpTemplate = `
usage: network-get [options] --primary-address
purpose: get network config

options:
--format  (= smart)
    specify output format (json|smart|yaml)
-o, --output (= "")
    specify an output file
--primary-address  (= false)
    get the primary address for the relation
-r, --relation  (= %s)
    specify a relation by id

network-get returns the network config for a relation. The only supported
flag for now is --primary-address, which is required and returns the IP
address the local unit should advertise as its endpoint to its peers.
`[1:]

	for i, t := range []struct {
		summary string
		relid   int
		usage   string
		rel     string
	}{{
		summary: "no default relation",
		relid:   -1,
	}, {
		summary: "default relation",
		relid:   1,
		rel:     "peer1:1",
	}} {
		c.Logf("test %d", i)
		hctx, _ := s.newHookContext(t.relid)
		com, err := jujuc.NewCommand(hctx, cmdString("network-get"))
		c.Check(err, jc.ErrorIsNil)
		ctx := testing.Context(c)
		code := cmd.Main(com, ctx, []string{"--help"})
		c.Check(code, gc.Equals, 0)

		expect := fmt.Sprintf(helpTemplate, t.rel)
		c.Check(bufferString(ctx.Stdout), gc.Equals, expect)
		c.Check(bufferString(ctx.Stderr), gc.Equals, "")
	}
}
