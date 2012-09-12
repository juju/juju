package uniter_test

import (
	. "launchpad.net/gocheck"
	"launchpad.net/juju-core/charm"
	"launchpad.net/juju-core/trivial"
	"launchpad.net/juju-core/worker/uniter"
	"launchpad.net/juju-core/worker/uniter/hook"
	"path/filepath"
)

type StateFileSuite struct{}

var _ = Suite(&StateFileSuite{})

var stcurl = charm.MustParseURL("cs:series/service-name-123")
var relhook = &hook.Info{
	Kind:       hook.RelationJoined,
	RemoteUnit: "some-thing/123",
	Members: map[string]map[string]interface{}{
		"blah/0": {"cheese": "gouda"},
	},
}

var stateTests = []struct {
	st  uniter.State
	err string
}{
	// Invalid op/status.
	{
		st:  uniter.State{Op: uniter.Op("bloviate")},
		err: `unknown operation "bloviate"`,
	}, {
		st: uniter.State{
			Op:     uniter.Abide,
			Status: uniter.Status("dudelike"),
			Hook:   &hook.Info{Kind: hook.ConfigChanged},
		},
		err: `unknown operation status "dudelike"`,
	},
	// Install operation.
	{
		st: uniter.State{
			Op:     uniter.Install,
			Status: uniter.Pending,
			Hook:   &hook.Info{Kind: hook.ConfigChanged},
		},
		err: `unexpected hook info`,
	}, {
		st: uniter.State{
			Op:     uniter.Install,
			Status: uniter.Pending,
		},
		err: `missing charm URL`,
	}, {
		st: uniter.State{
			Op:       uniter.Install,
			Status:   uniter.Pending,
			CharmURL: stcurl,
		},
	},
	// RunHook operation.
	{
		st: uniter.State{
			Op:     uniter.RunHook,
			Status: uniter.Pending,
			Hook:   &hook.Info{Kind: hook.Kind("machine-exploded")},
		},
		err: `unknown hook kind "machine-exploded"`,
	}, {
		st: uniter.State{
			Op:     uniter.RunHook,
			Status: uniter.Pending,
			Hook:   &hook.Info{Kind: hook.RelationJoined},
		},
		err: `"relation-joined" hook requires a remote unit`,
	}, {
		st: uniter.State{
			Op:       uniter.RunHook,
			Status:   uniter.Pending,
			Hook:     &hook.Info{Kind: hook.ConfigChanged},
			CharmURL: stcurl,
		},
		err: `unexpected charm URL`,
	}, {
		st: uniter.State{
			Op:     uniter.RunHook,
			Status: uniter.Pending,
			Hook:   &hook.Info{Kind: hook.ConfigChanged},
		},
	}, {
		st: uniter.State{
			Op:     uniter.RunHook,
			Status: uniter.Pending,
			Hook:   relhook,
		},
	},
	// Upgrade operation.
	{
		st: uniter.State{
			Op:     uniter.Upgrade,
			Status: uniter.Pending,
		},
		err: `missing charm URL`,
	}, {
		st: uniter.State{
			Op:       uniter.Upgrade,
			Status:   uniter.Pending,
			CharmURL: stcurl,
		},
	}, {
		st: uniter.State{
			Op:       uniter.Upgrade,
			Status:   uniter.Pending,
			Hook:     relhook,
			CharmURL: stcurl,
		},
	},
	// Abide operation.
	{
		st: uniter.State{
			Op:     uniter.Abide,
			Status: uniter.Pending,
		},
		err: `missing hook info`,
	}, {
		st: uniter.State{
			Op:       uniter.Abide,
			Status:   uniter.Pending,
			Hook:     relhook,
			CharmURL: stcurl,
		},
		err: `unexpected charm URL`,
	}, {
		st: uniter.State{
			Op:     uniter.Abide,
			Status: uniter.Pending,
			Hook:   relhook,
		},
	},
}

func (s *StateFileSuite) TestStates(c *C) {
	for i, t := range stateTests {
		c.Logf("test %d", i)
		path := filepath.Join(c.MkDir(), "uniter")
		file := uniter.NewStateFile(path)
		_, err := file.Read()
		c.Assert(err, Equals, uniter.ErrNoStateFile)
		write := func() {
			err := file.Write(t.st.Op, t.st.Status, t.st.Hook, t.st.CharmURL)
			c.Assert(err, IsNil)
		}
		if t.err != "" {
			c.Assert(write, PanicMatches, t.err)
			err := trivial.WriteYaml(path, &t.st)
			c.Assert(err, IsNil)
			_, err = file.Read()
			c.Assert(err, ErrorMatches, "invalid uniter state at .*: "+t.err)
			continue
		}
		write()
		st, err := file.Read()
		c.Assert(err, IsNil)
		if st.Hook != nil {
			c.Assert(st.Hook.Members, HasLen, 0)
			st.Hook.Members = t.st.Hook.Members
		}
		c.Assert(*st, DeepEquals, t.st)
	}
}
