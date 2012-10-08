package jujuc_test

import (
	"fmt"
	. "launchpad.net/gocheck"
	"launchpad.net/juju-core/charm"
	"launchpad.net/juju-core/cmd"
	"launchpad.net/juju-core/juju/testing"
	"launchpad.net/juju-core/state"
	"launchpad.net/juju-core/worker/uniter/jujuc"
)

type RelationIdsSuite struct {
	testing.JujuConnSuite
	ch      *state.Charm
	service *state.Service
	unit    *state.Unit
	relctxs map[int]*jujuc.RelationContext
}

var _ = Suite(&RelationIdsSuite{})

func (s *RelationIdsSuite) SetUpTest(c *C) {
	s.JujuConnSuite.SetUpTest(c)
	s.ch = s.AddTestingCharm(c, "dummy")
	var err error
	s.service, err = s.State.AddService("main", s.ch)
	c.Assert(err, IsNil)
	s.unit, err = s.service.AddUnit()
	c.Assert(err, IsNil)
	s.relctxs = map[int]*jujuc.RelationContext{}
	s.AddRelatedServices(c, "x", 3)
	s.AddRelatedServices(c, "y", 1)
}

func (s *RelationIdsSuite) AddRelatedServices(c *C, relname string, count int) {
	mainep := state.RelationEndpoint{
		"main", "ifce", relname, state.RoleProvider, charm.ScopeGlobal,
	}
	for i := 0; i < count; i++ {
		service, err := s.State.AddService(fmt.Sprintf("%s%d", relname, i), s.ch)
		c.Assert(err, IsNil)
		otherep := state.RelationEndpoint{
			service.Name(), "ifce", relname, state.RoleRequirer, charm.ScopeGlobal,
		}
		rel, err := s.State.AddRelation(mainep, otherep)
		c.Assert(err, IsNil)
		ru, err := rel.Unit(s.unit)
		c.Assert(err, IsNil)
		s.relctxs[rel.Id()] = jujuc.NewRelationContext(ru, nil)
	}
}

func (s *RelationIdsSuite) GetHookContext(c *C, relid int) *jujuc.HookContext {
	if relid != -1 {
		_, found := s.relctxs[relid]
		c.Assert(found, Equals, true)
	}
	return &jujuc.HookContext{
		Service:        s.service,
		Unit_:          s.unit,
		Id:             "TestCtx",
		RelationId_:    relid,
		RemoteUnitName: "",
		Relations:      s.relctxs,
	}
}

var relationIdsTests = []struct {
	summary string
	relid   int
	args    []string
	code    int
	out     string
}{
	{
		summary: "no default, no name",
		relid:   -1,
		code:    2,
		out:     "(.|\n)*error: no relation name specified\n",
	}, {
		summary: "default name",
		relid:   1,
		out:     "x:0\nx:1\nx:2",
	}, {
		summary: "explicit name",
		relid:   -1,
		args:    []string{"x"},
		out:     "x:0\nx:1\nx:2",
	}, {
		summary: "explicit different name",
		relid:   -1,
		args:    []string{"y"},
		out:     "y:3",
	}, {
		summary: "nonexistent name",
		relid:   -1,
		args:    []string{"z"},
	}, {
		summary: "explicit smart formatting 1",
		args:    []string{"--format", "smart", "x"},
		out:     "x:0\nx:1\nx:2",
	}, {
		summary: "explicit smart formatting 2",
		args:    []string{"--format", "smart", "y"},
		out:     "y:3",
	}, {
		summary: "explicit smart formatting 3",
		args:    []string{"--format", "smart", "z"},
	}, {
		summary: "json formatting 1",
		args:    []string{"--format", "json", "x"},
		out:     `["x:0","x:1","x:2"]`,
	}, {
		summary: "json formatting 2",
		args:    []string{"--format", "json", "y"},
		out:     `["y:3"]`,
	}, {
		summary: "json formatting 3",
		args:    []string{"--format", "json", "z"},
		out:     `[]`,
	}, {
		summary: "yaml formatting 1",
		args:    []string{"--format", "yaml", "x"},
		out:     "- x:0\n- x:1\n- x:2",
	}, {
		summary: "yaml formatting 2",
		args:    []string{"--format", "yaml", "y"},
		out:     "- y:3",
	}, {
		summary: "yaml formatting 3",
		args:    []string{"--format", "yaml", "z"},
		out:     "[]",
	},
}

func (s *RelationIdsSuite) TestRelationIds(c *C) {
	for i, t := range relationIdsTests {
		c.Logf("test %d: %s", i, t.summary)
		hctx := s.GetHookContext(c, t.relid)
		com, err := hctx.NewCommand("relation-ids")
		c.Assert(err, IsNil)
		ctx := dummyContext(c)
		code := cmd.Main(com, ctx, t.args)
		c.Assert(code, Equals, t.code)
		if code == 0 {
			c.Assert(bufferString(ctx.Stderr), Equals, "")
			expect := t.out
			if expect != "" {
				expect += "\n"
			}
			c.Assert(bufferString(ctx.Stdout), Equals, expect)
		} else {
			c.Assert(bufferString(ctx.Stdout), Equals, "")
			c.Assert(bufferString(ctx.Stderr), Matches, t.out)
		}
	}
}

func (s *RelationIdsSuite) TestHelp(c *C) {
	template := `
usage: %s
purpose: list all relation ids with the given relation name

options:
--format  (= smart)
    specify output format (json|smart|yaml)
-o, --output (= "")
    specify an output file
%s`[1:]

	for relid, t := range map[int]struct {
		usage, doc string
	}{
		-1: {"relation-ids [options] <name>", ""},
		0:  {"relation-ids [options] [<name>]", "\nCurrent default relation name is \"x\".\n"},
		3:  {"relation-ids [options] [<name>]", "\nCurrent default relation name is \"y\".\n"},
	} {
		c.Logf("relid %d", relid)
		hctx := s.GetHookContext(c, relid)
		com, err := hctx.NewCommand("relation-ids")
		c.Assert(err, IsNil)
		ctx := dummyContext(c)
		code := cmd.Main(com, ctx, []string{"--help"})
		c.Assert(code, Equals, 0)
		c.Assert(bufferString(ctx.Stdout), Equals, "")
		expect := fmt.Sprintf(template, t.usage, t.doc)
		c.Assert(bufferString(ctx.Stderr), Equals, expect)
	}
}
