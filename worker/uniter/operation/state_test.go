// Copyright 2012, 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package operation_test

import (
	"path/filepath"

	jc "github.com/juju/testing/checkers"
	"github.com/juju/utils"
	gc "gopkg.in/check.v1"
	"gopkg.in/juju/charm.v6"
	"gopkg.in/juju/charm.v6/hooks"

	"github.com/juju/juju/worker/uniter/hook"
	"github.com/juju/juju/worker/uniter/operation"
)

type StateFileSuite struct{}

var _ = gc.Suite(&StateFileSuite{})

var stcurl = charm.MustParseURL("cs:quantal/application-name-123")
var relhook = &hook.Info{
	Kind:              hooks.RelationJoined,
	RemoteUnit:        "some-thing/123",
	RemoteApplication: "some-thing",
}

var stateTests = []struct {
	description string
	st          operation.State
	err         string
}{
	// Invalid op/step.
	{
		description: "unknown operation",
		st:          operation.State{Kind: operation.Kind("bloviate")},
		err:         `unknown operation "bloviate"`,
	}, {
		st: operation.State{
			Kind: operation.Continue,
			Step: operation.Step("dudelike"),
		},
		err: `unknown operation step "dudelike"`,
	},
	// Install operation.
	{
		description: "mismatched operation and hook",
		st: operation.State{
			Kind:      operation.Install,
			Installed: true,
			Step:      operation.Pending,
			CharmURL:  stcurl,
			Hook:      &hook.Info{Kind: hooks.ConfigChanged},
		},
		err: `unexpected hook info with Kind Install`,
	}, {
		description: "missing charm URL",
		st: operation.State{
			Kind: operation.Install,
			Step: operation.Pending,
		},
		err: `missing charm URL`,
	}, {
		description: "install with action-id",
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
		description: "run action without action id",
		st: operation.State{
			Kind: operation.RunAction,
			Step: operation.Pending,
		},
		err: `missing action id`,
	}, {
		description: "run action without action id",
		st: operation.State{
			Kind:     operation.RunAction,
			Step:     operation.Pending,
			ActionId: &someActionId,
			CharmURL: stcurl,
		},
		err: `unexpected charm URL`,
	}, {
		description: "run action with proper action id",
		st: operation.State{
			Kind:     operation.RunAction,
			Step:     operation.Pending,
			ActionId: &someActionId,
		},
	},
	// RunHook operation.
	{
		description: "run-hook with unknown hook",
		st: operation.State{
			Kind: operation.RunHook,
			Step: operation.Pending,
			Hook: &hook.Info{Kind: hooks.Kind("machine-exploded")},
		},
		err: `unknown hook kind "machine-exploded"`,
	}, {
		description: "run-hook without remote unit",
		st: operation.State{
			Kind: operation.RunHook,
			Step: operation.Pending,
			Hook: &hook.Info{Kind: hooks.RelationJoined},
		},
		err: `"relation-joined" hook requires a remote unit`,
	}, {
		description: "run-hook relation-joined without remote application",
		st: operation.State{
			Kind: operation.RunHook,
			Step: operation.Pending,
			Hook: &hook.Info{
				Kind:       hooks.RelationJoined,
				RemoteUnit: "some-thing/0",
			},
		},
		err: `"relation-joined" hook has a remote unit but no application`,
	}, {
		description: "run-hook relation-changed without remote application",
		st: operation.State{
			Kind: operation.RunHook,
			Step: operation.Pending,
			Hook: &hook.Info{
				Kind:       hooks.RelationChanged,
				RemoteUnit: "some-thing/0",
			},
		},
		err: `"relation-changed" hook has a remote unit but no application`,
	}, {
		description: "run-hook with actionId",
		st: operation.State{
			Kind:     operation.RunHook,
			Step:     operation.Pending,
			Hook:     &hook.Info{Kind: hooks.ConfigChanged},
			ActionId: &someActionId,
		},
		err: `unexpected action id`,
	}, {
		description: "run-hook with charm URL",
		st: operation.State{
			Kind:     operation.RunHook,
			Step:     operation.Pending,
			Hook:     &hook.Info{Kind: hooks.ConfigChanged},
			CharmURL: stcurl,
		},
		err: `unexpected charm URL`,
	}, {
		description: "run-hook config-changed",
		st: operation.State{
			Kind: operation.RunHook,
			Step: operation.Pending,
			Hook: &hook.Info{Kind: hooks.ConfigChanged},
		},
	}, {
		description: "run-hook relation-joined",
		st: operation.State{
			Kind: operation.RunHook,
			Step: operation.Pending,
			Hook: relhook,
		},
	},
	// Upgrade operation.
	{
		description: "upgrade without charmURL",
		st: operation.State{
			Kind: operation.Upgrade,
			Step: operation.Pending,
		},
		err: `missing charm URL`,
	}, {
		description: "upgrade with actionID",
		st: operation.State{
			Kind:     operation.Upgrade,
			Step:     operation.Pending,
			CharmURL: stcurl,
			ActionId: &someActionId,
		},
		err: `unexpected action id`,
	}, {
		description: "upgrade operation",
		st: operation.State{
			Kind:     operation.Upgrade,
			Step:     operation.Pending,
			CharmURL: stcurl,
		},
	}, {
		description: "upgrade operation with a relation hook (?)",
		st: operation.State{
			Kind:     operation.Upgrade,
			Step:     operation.Pending,
			Hook:     relhook,
			CharmURL: stcurl,
		},
	},
	// Continue operation.
	{
		description: "continue operation with charmURL",
		st: operation.State{
			Kind:     operation.Continue,
			Step:     operation.Pending,
			CharmURL: stcurl,
		},
		err: `unexpected charm URL`,
	}, {
		description: "continue operation with actionID",
		st: operation.State{
			Kind:     operation.Continue,
			Step:     operation.Pending,
			ActionId: &someActionId,
		},
		err: `unexpected action id`,
	}, {
		description: "continue operation",
		st: operation.State{
			Kind:   operation.Continue,
			Step:   operation.Pending,
			Leader: true,
		},
	},
}

func (s *StateFileSuite) TestStates(c *gc.C) {
	for i, t := range stateTests {
		c.Logf("test %d: %s", i, t.description)
		path := filepath.Join(c.MkDir(), "uniter")
		file := operation.NewStateFile(path)
		_, err := file.Read()
		c.Assert(err, gc.Equals, operation.ErrNoStateFile)

		err = file.Write(&t.st)
		if t.err == "" {
			c.Assert(err, jc.ErrorIsNil)
		} else {
			c.Assert(err, gc.ErrorMatches, "invalid operation state: "+t.err)
			err := utils.WriteYaml(path, &t.st)
			c.Assert(err, jc.ErrorIsNil)
			_, err = file.Read()
			c.Assert(err, gc.ErrorMatches, `cannot read ".*": invalid operation state: `+t.err)
			continue
		}
		st, err := file.Read()
		c.Assert(err, jc.ErrorIsNil)
		c.Assert(st, jc.DeepEquals, &t.st)
	}
}
