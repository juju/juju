package server_test

import (
	"io/ioutil"
	. "launchpad.net/gocheck"
	"launchpad.net/juju-core/cmd"
	"path/filepath"
)

type RelationGetSuite struct {
	HookContextSuite
}

var _ = Suite(&RelationGetSuite{})

var relationGetTests = []struct {
	relid int
	unit  string
	args  []string
	code  int
	out   string
}{
	{1, "foo/1", nil, 1, ""},
}

func (s *RelationGetSuite) SetUpTest(c *C) {
	s.unit1 = s.AddUnit(c)
	s.unit2 = s.AddUnit(c)
}

func (s *RelationGetSuite) Reset(c *C) {

}

func (s *RelationGetSuite) TestRelationGet(c *C) {
	for _, t := range relationGetTests {
		hctx := s.GetHookContext(c, t.relid, t.unit)
		com, err := hctx.NewCommand("relation-get")
		c.Assert(err, IsNil)
		ctx := dummyContext(c)
		code := cmd.Main(com, ctx, t.args)
		c.Assert(code, Equals, t.code)
		if t.code == 0 {
			c.Assert(bufferString(ctx.Stdout), Matches, t.out)
			c.Assert(bufferString(ctx.Stderr), Equals, "")
		} else {
			c.Assert(bufferString(ctx.Stdout), Equals, "")
			c.Assert(bufferString(ctx.Stderr), Matches, t.out)
		}
	}
}

func (s *RelationGetSuite) TestTestMode(c *C) {
	c.Fatalf("write me")
}

func (s *RelationGetSuite) TestHelp(c *C) {
	c.Fatalf("write me")
}

func (s *RelationGetSuite) TestOutputPath(c *C) {
	c.Fatalf("write me")
	hctx := s.GetHookContext(c, -1, "")
	com, err := hctx.NewCommand("unit-get")
	c.Assert(err, IsNil)
	ctx := dummyContext(c)
	code := cmd.Main(com, ctx, []string{"--output", "some-file", "private-address"})
	c.Assert(code, Equals, 0)
	c.Assert(bufferString(ctx.Stderr), Equals, "")
	c.Assert(bufferString(ctx.Stdout), Equals, "")
	content, err := ioutil.ReadFile(filepath.Join(ctx.Dir, "some-file"))
	c.Assert(err, IsNil)
	c.Assert(string(content), Equals, "192.168.0.99\n\n")
}

func (s *RelationGetSuite) TestUnknownSetting(c *C) {
	c.Fatalf("write me")
	hctx := s.GetHookContext(c, -1, "")
	com, err := hctx.NewCommand("unit-get")
	c.Assert(err, IsNil)
	err = com.Init(dummyFlagSet(), []string{"protected-address"})
	c.Assert(err, ErrorMatches, `unknown setting "protected-address"`)
}

func (s *RelationGetSuite) TestUnknownArg(c *C) {
	c.Fatalf("write me")
	hctx := s.GetHookContext(c, -1, "")
	com, err := hctx.NewCommand("unit-get")
	c.Assert(err, IsNil)
	err = com.Init(dummyFlagSet(), []string{"private-address", "blah"})
	c.Assert(err, ErrorMatches, `unrecognized args: \["blah"\]`)
}
