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
	hs := uniter.NewHookState(path)
	_, _, err := hs.Get()
	c.Assert(err, Equals, uniter.ErrNoHook)

	err = ioutil.WriteFile(path, []byte("roflcopter"), 0644)
	c.Assert(err, IsNil)
	_, _, err = hs.Get()
	c.Assert(err, ErrorMatches, "invalid hook state at "+path)

	f := func() { hs.Set(uniter.HookInfo{}, uniter.HookStatus("nonsense")) }
	c.Assert(f, PanicMatches, `unknown HookStatus "nonsense"!`)

	f = func() { hs.Set(uniter.HookInfo{}, uniter.StatusStarted) }
	c.Assert(f, PanicMatches, `empty HookKind!`)

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
	err = hs.Set(hi, uniter.StatusStarted)
	c.Assert(err, IsNil)

	ghi, gst, err := hs.Get()
	c.Assert(err, IsNil)
	c.Assert(gst, Equals, uniter.StatusStarted)
	c.Assert(ghi, DeepEquals, uniter.HookInfo{
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
