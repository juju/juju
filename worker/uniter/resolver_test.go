// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package uniter_test

import (
	"github.com/juju/errors"
	"github.com/juju/names"
	jc "github.com/juju/testing/checkers"
	"github.com/juju/utils/exec"
	gc "gopkg.in/check.v1"
	"gopkg.in/juju/charm.v6-unstable"

	"github.com/juju/juju/worker/uniter"
	uniteractions "github.com/juju/juju/worker/uniter/actions"
	"github.com/juju/juju/worker/uniter/hook"
	"github.com/juju/juju/worker/uniter/leadership"
	"github.com/juju/juju/worker/uniter/operation"
	"github.com/juju/juju/worker/uniter/relation"
	"github.com/juju/juju/worker/uniter/remotestate"
	"github.com/juju/juju/worker/uniter/resolver"
	"github.com/juju/juju/worker/uniter/runner"
	runnercontext "github.com/juju/juju/worker/uniter/runner/context"
	"github.com/juju/juju/worker/uniter/storage"
)

type resolverSuite struct {
	charmURL         *charm.URL
	remoteState      remotestate.Snapshot
	dummyRunner      dummyRunner
	opFactory        operation.Factory
	resolver         resolver.Resolver
	commands         uniter.Commands
	runCommands      func(string) (*exec.ExecResponse, error)
	commandCompleted func(string)
}

var _ = gc.Suite(&resolverSuite{})

func (s *resolverSuite) SetUpTest(c *gc.C) {
	s.charmURL = charm.MustParseURL("cs:precise/mysql-2")
	s.remoteState = remotestate.Snapshot{
		CharmURL: s.charmURL,
	}
	s.dummyRunner = dummyRunner{runCommands: func(commands string) (*exec.ExecResponse, error) {
		return s.runCommands(commands)
	}}
	s.opFactory = operation.NewFactory(operation.FactoryParams{
		Callbacks: &dummyCallbacks{},
		RunnerFactory: &dummyRunnerFactory{
			newCommandRunner: func(info runnercontext.CommandInfo) (runner.Runner, error) {
				return &s.dummyRunner, nil
			},
		},
	})

	attachments, err := storage.NewAttachments(&dummyStorageAccessor{}, names.NewUnitTag("u/0"), c.MkDir(), nil)
	c.Assert(err, jc.ErrorIsNil)

	s.commands = uniter.NewCommands()
	s.commandCompleted = nil
	s.resolver = uniter.NewUniterResolver(
		func() error { return errors.New("unexpected resolved") },
		func(_ hook.Info) error { return errors.New("unexpected report hook error") },
		func() error { return nil },
		uniteractions.NewResolver(),
		leadership.NewResolver(),
		relation.NewRelationsResolver(&dummyRelations{}),
		storage.NewResolver(attachments),
		uniter.NewCommandsResolver(s.commands, func(id string) {
			if s.commandCompleted != nil {
				s.commandCompleted(id)
			}
		}),
	)
}

// TestStartedNotInstalled tests whether the Started flag overrides the
// Installed flag being unset, in the event of an unexpected inconsistency in
// local state.
func (s *resolverSuite) TestStartedNotInstalled(c *gc.C) {
	localState := resolver.LocalState{
		CharmURL: s.charmURL,
		State: operation.State{
			Kind:      operation.Continue,
			Installed: false,
			Started:   true,
		},
	}
	_, err := s.resolver.NextOp(localState, s.remoteState, s.opFactory)
	c.Assert(err, gc.Equals, resolver.ErrNoOperation)
}

// TestNotStartedNotInstalled tests whether the next operation for an
// uninstalled local state is an install hook operation.
func (s *resolverSuite) TestNotStartedNotInstalled(c *gc.C) {
	localState := resolver.LocalState{
		CharmURL: s.charmURL,
		State: operation.State{
			Kind:      operation.Continue,
			Installed: false,
			Started:   false,
		},
	}
	op, err := s.resolver.NextOp(localState, s.remoteState, s.opFactory)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(op.String(), gc.Equals, "run install hook")
}

func (s *resolverSuite) TestRunCommands(c *gc.C) {
	localState := resolver.LocalState{
		CharmURL: s.charmURL,
		State: operation.State{
			Kind:      operation.Continue,
			Installed: false,
			Started:   false,
		},
	}
	id := s.commands.AddCommand(operation.CommandArgs{
		Commands: "echo foxtrot",
	}, func(*exec.ExecResponse, error) {})
	s.remoteState.Commands = []string{id}
	op, err := s.resolver.NextOp(localState, s.remoteState, s.opFactory)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(op.String(), gc.Equals, "run commands (0)")
}

func (s *resolverSuite) TestRunCommandsCallbacks(c *gc.C) {
	var completed []string
	s.commandCompleted = func(id string) {
		completed = append(completed, id)
	}

	var run []string
	s.runCommands = func(commands string) (*exec.ExecResponse, error) {
		run = append(run, commands)
		return &exec.ExecResponse{}, nil
	}
	localState := resolver.LocalState{
		CharmURL: s.charmURL,
		State: operation.State{
			Kind:      operation.Continue,
			Installed: false,
			Started:   false,
		},
	}

	id := s.commands.AddCommand(operation.CommandArgs{
		Commands: "echo foxtrot",
	}, func(*exec.ExecResponse, error) {})
	s.remoteState.Commands = []string{id}

	op, err := s.resolver.NextOp(localState, s.remoteState, s.opFactory)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(op.String(), gc.Equals, "run commands (0)")

	_, err = op.Prepare(operation.State{})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(run, gc.HasLen, 0)
	c.Assert(completed, gc.HasLen, 0)

	_, err = op.Execute(operation.State{})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(run, jc.DeepEquals, []string{"echo foxtrot"})
	c.Assert(completed, gc.HasLen, 0)

	_, err = op.Commit(operation.State{})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(completed, jc.DeepEquals, []string{id})
}
