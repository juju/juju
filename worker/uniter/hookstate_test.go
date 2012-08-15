package uniter_test

import (
	"io/ioutil"
	. "launchpad.net/gocheck"
	"launchpad.net/juju-core/worker/uniter"
	"path/filepath"
)

type HookStateSuite struct{}

var _ = Suite(&HookStateSuite{})

func (s *HookStateSuite) TestHookState(c *C) {
	path := filepath.Join(c.MkDir(), "hook")
	f := uniter.NewHookStateFile(path)
	_, _, err := f.Read()
	c.Assert(err, Equals, uniter.ErrNoHookState)

	err = ioutil.WriteFile(path, []byte("roflcopter"), 0644)
	c.Assert(err, IsNil)
	_, _, err = f.Read()
	c.Assert(err, ErrorMatches, "invalid hook state at "+path)

	bad := func() { f.Write(uniter.HookInfo{}, uniter.HookStatus("nonsense")) }
	c.Assert(bad, PanicMatches, `unknown hook status "nonsense"`)

	bad = func() { f.Write(uniter.HookInfo{}, uniter.StatusStarted) }
	c.Assert(bad, PanicMatches, `empty HookKind!`)

	hi := uniter.HookInfo{
		RelationId:    123,
		HookKind:      "changed",
		RemoteUnit:    "abc/999",
		ChangeVersion: 321,
		Members: map[string]map[string]interface{}{
			"abc/999":  {"foo": 1, "bar": 2},
			"abc/1000": {"baz": 3, "qux": 4},
		},
	}
	err = f.Write(hi, uniter.StatusStarted)
	c.Assert(err, IsNil)

	rhi, rst, err := f.Read()
	c.Assert(err, IsNil)
	c.Assert(rst, Equals, uniter.StatusStarted)
	c.Assert(rhi, DeepEquals, uniter.HookInfo{
		RelationId:    123,
		HookKind:      "changed",
		RemoteUnit:    "abc/999",
		ChangeVersion: 321,
		Members: map[string]map[string]interface{}{
			"abc/999":  nil,
			"abc/1000": nil,
		},
	})
}
