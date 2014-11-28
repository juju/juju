// Copyright 2012, 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package operation_test

import (
	"path/filepath"

	jc "github.com/juju/testing/checkers"
	"github.com/juju/utils"
	gc "gopkg.in/check.v1"
	"gopkg.in/juju/charm.v4"
	"gopkg.in/juju/charm.v4/hooks"

	"github.com/juju/juju/worker/uniter/hook"
	"github.com/juju/juju/worker/uniter/operation"
)

type StateFileSuite struct{}

var _ = gc.Suite(&StateFileSuite{})

var stcurl = charm.MustParseURL("cs:quantal/service-name-123")
var relhook = &hook.Info{
	Kind:       hooks.RelationJoined,
	RemoteUnit: "some-thing/123",
}

var stateTests = []struct {
	st  operation.State
	err string
}{
	// Invalid op/step.
	{
		st:  operation.State{Kind: operation.Kind("bloviate")},
		err: `unknown operation "bloviate"`,
	}, {
		st: operation.State{
			Kind: operation.Continue,
			Step: operation.Step("dudelike"),
			Hook: &hook.Info{Kind: hooks.ConfigChanged},
		},
		err: `unknown operation step "dudelike"`,
	},
	// Install operation.
	{
		st: operation.State{
			Kind: operation.Install,
			Step: operation.Pending,
			Hook: &hook.Info{Kind: hooks.ConfigChanged},
		},
		err: `unexpected hook info`,
	}, {
		st: operation.State{
			Kind: operation.Install,
			Step: operation.Pending,
		},
		err: `missing charm URL`,
	}, {
		st: operation.State{
			Kind:     operation.Install,
			Step:     operation.Pending,
			CharmURL: stcurl,
			ActionId: &someActionId,
		},
		err: `unexpected action id`,
	}, {
		st: operation.State{
			Kind:     operation.Install,
			Step:     operation.Pending,
			CharmURL: stcurl,
		},
	},
	// RunAction operation.
	{
		st: operation.State{
			Kind: operation.RunAction,
			Step: operation.Pending,
			Hook: &hook.Info{Kind: hooks.Install},
		},
		err: `missing action id`,
	}, {
		st: operation.State{
			Kind:     operation.RunAction,
			Step:     operation.Pending,
			Hook:     &hook.Info{Kind: hooks.Install},
			CharmURL: stcurl,
		},
		err: `unexpected charm URL`,
	}, {
		st: operation.State{
			Kind:     operation.RunAction,
			Step:     operation.Pending,
			Hook:     &hook.Info{Kind: hooks.Install},
			ActionId: &someActionId,
		},
	},
	// RunHook operation.
	{
		st: operation.State{
			Kind: operation.RunHook,
			Step: operation.Pending,
			Hook: &hook.Info{Kind: hooks.Kind("machine-exploded")},
		},
		err: `unknown hook kind "machine-exploded"`,
	}, {
		st: operation.State{
			Kind: operation.RunHook,
			Step: operation.Pending,
			Hook: &hook.Info{Kind: hooks.RelationJoined},
		},
		err: `"relation-joined" hook requires a remote unit`,
	}, {
		st: operation.State{
			Kind:     operation.RunHook,
			Step:     operation.Pending,
			Hook:     &hook.Info{Kind: hooks.ConfigChanged},
			ActionId: &someActionId,
		},
		err: `unexpected action id`,
	}, {
		st: operation.State{
			Kind:     operation.RunHook,
			Step:     operation.Pending,
			Hook:     &hook.Info{Kind: hooks.ConfigChanged},
			CharmURL: stcurl,
		},
		err: `unexpected charm URL`,
	}, {
		st: operation.State{
			Kind: operation.RunHook,
			Step: operation.Pending,
			Hook: &hook.Info{Kind: hooks.ConfigChanged},
		},
	}, {
		st: operation.State{
			Kind: operation.RunHook,
			Step: operation.Pending,
			Hook: relhook,
		},
	}, {
		st: operation.State{
			Kind: operation.RunHook,
			Step: operation.Pending,
			Hook: &hook.Info{
				Kind:     hooks.Action,
				ActionId: "feedface-dead-4567-beef-123456789012",
			},
		},
	}, {
		st: operation.State{
			Kind: operation.RunHook,
			Step: operation.Pending,
			Hook: &hook.Info{
				Kind:     hooks.Action,
				ActionId: "foo",
			},
		},
		err: `action id "foo" cannot be parsed as an action tag`,
	},
	// Upgrade operation.
	{
		st: operation.State{
			Kind: operation.Upgrade,
			Step: operation.Pending,
		},
		err: `missing charm URL`,
	}, {
		st: operation.State{
			Kind:     operation.Upgrade,
			Step:     operation.Pending,
			CharmURL: stcurl,
			ActionId: &someActionId,
		},
		err: `unexpected action id`,
	}, {
		st: operation.State{
			Kind:     operation.Upgrade,
			Step:     operation.Pending,
			CharmURL: stcurl,
		},
	}, {
		st: operation.State{
			Kind:     operation.Upgrade,
			Step:     operation.Pending,
			Hook:     relhook,
			CharmURL: stcurl,
		},
	},
	// Continue operation.
	{
		st: operation.State{
			Kind: operation.Continue,
			Step: operation.Pending,
		},
		err: `missing hook info`,
	}, {
		st: operation.State{
			Kind:     operation.Continue,
			Step:     operation.Pending,
			Hook:     relhook,
			CharmURL: stcurl,
		},
		err: `unexpected charm URL`,
	}, {
		st: operation.State{
			Kind:     operation.Continue,
			Step:     operation.Pending,
			Hook:     relhook,
			ActionId: &someActionId,
		},
		err: `unexpected action id`,
	}, {
		st: operation.State{
			Kind:               operation.Continue,
			Step:               operation.Pending,
			Hook:               relhook,
			CollectMetricsTime: 98765432,
		},
	},
}

func (s *StateFileSuite) TestStates(c *gc.C) {
	for i, t := range stateTests {
		c.Logf("test %d", i)
		path := filepath.Join(c.MkDir(), "uniter")
		file := operation.NewStateFile(path)
		_, err := file.Read()
		c.Assert(err, gc.Equals, operation.ErrNoStateFile)
		write := func() {
			err := file.Write(&t.st)
			c.Assert(err, jc.ErrorIsNil)
		}
		if t.err != "" {
			c.Assert(write, gc.PanicMatches, "invalid operation state: "+t.err)
			err := utils.WriteYaml(path, &t.st)
			c.Assert(err, jc.ErrorIsNil)
			_, err = file.Read()
			c.Assert(err, gc.ErrorMatches, `cannot read ".*": invalid operation state: `+t.err)
			continue
		}
		write()
		st, err := file.Read()
		c.Assert(err, jc.ErrorIsNil)
		c.Assert(*st, gc.DeepEquals, t.st)
	}
}
