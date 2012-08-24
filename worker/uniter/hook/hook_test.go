package hook_test

import (
	"io/ioutil"
	. "launchpad.net/gocheck"
	"launchpad.net/juju-core/worker/uniter/hook"
	"path/filepath"
	"testing"
)

func Test(t *testing.T) {
	TestingT(t)
}

type StateFileSuite struct{}

var _ = Suite(&StateFileSuite{})

func (s *StateFileSuite) TestStateFile(c *C) {
	path := filepath.Join(c.MkDir(), "hook")
	f := hook.NewStateFile(path)
	_, err := f.Read()
	c.Assert(err, Equals, hook.ErrNoStateFile)

	err = ioutil.WriteFile(path, []byte("roflcopter"), 0644)
	c.Assert(err, IsNil)
	_, err = f.Read()
	c.Assert(err, ErrorMatches, "invalid hook state at "+path)

	bad := func() { f.Write(hook.Info{}, hook.Status("nonsense")) }
	c.Assert(bad, PanicMatches, `unknown hook status "nonsense"`)

	bad = func() { f.Write(hook.Info{Kind: hook.Kind("incoherent")}, hook.Committing) }
	c.Assert(bad, PanicMatches, `unknown hook kind "incoherent"`)

	hi := hook.Info{
		Kind:          hook.RelationChanged,
		RelationId:    123,
		RemoteUnit:    "abc/999",
		ChangeVersion: 321,
		Members: map[string]map[string]interface{}{
			"abc/999":  {"foo": 1, "bar": 2},
			"abc/1000": {"baz": 3, "qux": 4},
		},
	}
	err = f.Write(hi, hook.Pending)
	c.Assert(err, IsNil)

	st, err := f.Read()
	c.Assert(err, IsNil)
	c.Assert(st.Status, Equals, hook.Pending)
	c.Assert(st.Info, DeepEquals, hook.Info{
		Kind:          hook.RelationChanged,
		RelationId:    123,
		RemoteUnit:    "abc/999",
		ChangeVersion: 321,
		Members: map[string]map[string]interface{}{
			"abc/999":  nil,
			"abc/1000": nil,
		},
	})
}
