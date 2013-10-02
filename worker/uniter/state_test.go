// Copyright 2012, 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package uniter_test

import (
	"path/filepath"

	gc "launchpad.net/gocheck"

	"launchpad.net/juju-core/charm"
	"launchpad.net/juju-core/charm/hooks"
	"launchpad.net/juju-core/utils"
	"launchpad.net/juju-core/worker/uniter"
	"launchpad.net/juju-core/worker/uniter/hook"
)

type StateFileSuite struct{}

var _ = gc.Suite(&StateFileSuite{})

var stcurl = charm.MustParseURL("cs:quantal/service-name-123")
var relhook = &hook.Info{
	Kind:       hooks.RelationJoined,
	RemoteUnit: "some-thing/123",
}

var stateTests = []struct {
	st  uniter.State
	err string
}{
	// Invalid op/step.
	{
		st:  uniter.State{Op: uniter.Op("bloviate")},
		err: `unknown operation "bloviate"`,
	}, {
		st: uniter.State{
			Op:     uniter.Continue,
			OpStep: uniter.OpStep("dudelike"),
			Hook:   &hook.Info{Kind: hooks.ConfigChanged},
		},
		err: `unknown operation step "dudelike"`,
	},
	// Install operation.
	{
		st: uniter.State{
			Op:     uniter.Install,
			OpStep: uniter.Pending,
			Hook:   &hook.Info{Kind: hooks.ConfigChanged},
		},
		err: `unexpected hook info`,
	}, {
		st: uniter.State{
			Op:     uniter.Install,
			OpStep: uniter.Pending,
		},
		err: `missing charm URL`,
	}, {
		st: uniter.State{
			Op:       uniter.Install,
			OpStep:   uniter.Pending,
			CharmURL: stcurl,
		},
	},
	// RunHook operation.
	{
		st: uniter.State{
			Op:     uniter.RunHook,
			OpStep: uniter.Pending,
			Hook:   &hook.Info{Kind: hooks.Kind("machine-exploded")},
		},
		err: `unknown hook kind "machine-exploded"`,
	}, {
		st: uniter.State{
			Op:     uniter.RunHook,
			OpStep: uniter.Pending,
			Hook:   &hook.Info{Kind: hooks.RelationJoined},
		},
		err: `"relation-joined" hook requires a remote unit`,
	}, {
		st: uniter.State{
			Op:       uniter.RunHook,
			OpStep:   uniter.Pending,
			Hook:     &hook.Info{Kind: hooks.ConfigChanged},
			CharmURL: stcurl,
		},
		err: `unexpected charm URL`,
	}, {
		st: uniter.State{
			Op:     uniter.RunHook,
			OpStep: uniter.Pending,
			Hook:   &hook.Info{Kind: hooks.ConfigChanged},
		},
	}, {
		st: uniter.State{
			Op:     uniter.RunHook,
			OpStep: uniter.Pending,
			Hook:   relhook,
		},
	},
	// Upgrade operation.
	{
		st: uniter.State{
			Op:     uniter.Upgrade,
			OpStep: uniter.Pending,
		},
		err: `missing charm URL`,
	}, {
		st: uniter.State{
			Op:       uniter.Upgrade,
			OpStep:   uniter.Pending,
			CharmURL: stcurl,
		},
	}, {
		st: uniter.State{
			Op:       uniter.Upgrade,
			OpStep:   uniter.Pending,
			Hook:     relhook,
			CharmURL: stcurl,
		},
	},
	// Continue operation.
	{
		st: uniter.State{
			Op:     uniter.Continue,
			OpStep: uniter.Pending,
		},
		err: `missing hook info`,
	}, {
		st: uniter.State{
			Op:       uniter.Continue,
			OpStep:   uniter.Pending,
			Hook:     relhook,
			CharmURL: stcurl,
		},
		err: `unexpected charm URL`,
	}, {
		st: uniter.State{
			Op:     uniter.Continue,
			OpStep: uniter.Pending,
			Hook:   relhook,
		},
	},
}

func (s *StateFileSuite) TestStates(c *gc.C) {
	for i, t := range stateTests {
		c.Logf("test %d", i)
		path := filepath.Join(c.MkDir(), "uniter")
		file := uniter.NewStateFile(path)
		_, err := file.Read()
		c.Assert(err, gc.Equals, uniter.ErrNoStateFile)
		write := func() {
			err := file.Write(t.st.Started, t.st.Op, t.st.OpStep, t.st.Hook, t.st.CharmURL)
			c.Assert(err, gc.IsNil)
		}
		if t.err != "" {
			c.Assert(write, gc.PanicMatches, "invalid uniter state: "+t.err)
			err := utils.WriteYaml(path, &t.st)
			c.Assert(err, gc.IsNil)
			_, err = file.Read()
			c.Assert(err, gc.ErrorMatches, "cannot read charm state at .*: invalid uniter state: "+t.err)
			continue
		}
		write()
		st, err := file.Read()
		c.Assert(err, gc.IsNil)
		c.Assert(*st, gc.DeepEquals, t.st)
	}
}
