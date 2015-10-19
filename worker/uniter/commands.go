// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package uniter

import (
	"fmt"
	"sync"

	"github.com/juju/juju/worker/uniter/operation"
	"github.com/juju/juju/worker/uniter/remotestate"
	"github.com/juju/juju/worker/uniter/resolver"
)

// Commands is an interface providing a means of storing and retrieving
// arguments for running commands.
type Commands interface {
	// AddCommand adds the given command arguments and response function
	// and returns a unique identifier.
	AddCommand(operation.CommandArgs, operation.CommandResponseFunc) string

	// GetCommand returns the command arguments and response function
	// with the specified ID, as registered in AddCommand.
	GetCommand(id string) (operation.CommandArgs, operation.CommandResponseFunc)

	// RemoveCommand removes the command arguments and response function
	// associated with the specified ID.
	RemoveCommand(id string)
}

type commands struct {
	mu      sync.Mutex
	nextId  int
	pending map[string]command
}

type command struct {
	args     operation.CommandArgs
	response operation.CommandResponseFunc
}

func newCommands() *commands {
	return &commands{
		nextId:  0,
		pending: make(map[string]command),
	}
}

func (c *commands) AddCommand(args operation.CommandArgs, response operation.CommandResponseFunc) string {
	c.mu.Lock()
	defer c.mu.Unlock()
	id := fmt.Sprint(c.nextId)
	c.nextId++
	c.pending[id] = command{args, response}
	return id
}

func (c *commands) RemoveCommand(id string) {
	c.mu.Lock()
	delete(c.pending, id)
	c.mu.Unlock()
}

func (c *commands) GetCommand(id string) (operation.CommandArgs, operation.CommandResponseFunc) {
	c.mu.Lock()
	defer c.mu.Unlock()
	command := c.pending[id]
	return command.args, command.response
}

// commandsResolver is a Resolver that returns operations to run pending
// commands. When a command is completed, the "commandCompleted" callback
// is invoked to remove the pending command from the remote state.
type commandsResolver struct {
	commands         Commands
	commandCompleted func(id string)
}

func newCommandsResolver(commands Commands, commandCompleted func(string)) *commandsResolver {
	return &commandsResolver{commands, commandCompleted}
}

func (s *commandsResolver) NextOp(
	localState resolver.LocalState,
	remoteState remotestate.Snapshot,
	opFactory operation.Factory,
) (operation.Operation, error) {
	if len(remoteState.Commands) == 0 {
		return nil, resolver.ErrNoOperation
	}
	id := remoteState.Commands[0]
	op, err := opFactory.NewCommands(s.commands.GetCommand(id))
	if err != nil {
		return nil, err
	}
	commandCompleted := func() {
		s.commands.RemoveCommand(id)
		s.commandCompleted(id)
	}
	return &commandCompleter{op, commandCompleted}, nil
}

type commandCompleter struct {
	operation.Operation
	commandCompleted func()
}

func (c *commandCompleter) Commit(st operation.State) (*operation.State, error) {
	result, err := c.Operation.Commit(st)
	if err == nil {
		c.commandCompleted()
	}
	return result, err
}
