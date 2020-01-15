// Copyright 2012, 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package jujuc

import (
	"bufio"
	"io"
	"strings"

	"github.com/juju/cmd"
	"github.com/juju/errors"
	"github.com/juju/gnuflag"
	"github.com/juju/utils/keyvalues"

	jujucmd "github.com/juju/juju/cmd"
)

// StateValueSetCommand implements the state-value-set command.
type StateValueSetCommand struct {
	cmd.CommandBase
	ctx          Context
	StateValues  map[string]string
	keyValueFile cmd.FileVar
	out          cmd.Output
}

func NewStateValueSetCommand(ctx Context) (cmd.Command, error) {
	return &StateValueSetCommand{ctx: ctx}, nil
}

func (c *StateValueSetCommand) Info() *cmd.Info {
	return jujucmd.Info(&cmd.Info{
		Name:    "state-value-set",
		Args:    "key=value [key=value ...]",
		Purpose: "set server-side-state values",
	})
}

func (c *StateValueSetCommand) SetFlags(f *gnuflag.FlagSet) {
	c.keyValueFile.SetStdin()
	f.Var(&c.keyValueFile, "file", "file containing value")
}

func (c *StateValueSetCommand) Init(args []string) error {
	if args == nil {
		return nil
	}

	// The overrides will be applied during Run when c.keyValueFile is handled.
	overrides, err := keyvalues.Parse(args, true)
	if err != nil {
		return errors.Trace(err)
	}
	c.StateValues = overrides
	return nil
}

func (c *StateValueSetCommand) Run(ctx *cmd.Context) error {
	if err := c.handleKeyValueFile(ctx); err != nil {
		return errors.Trace(err)
	}

	for k, v := range c.StateValues {
		if v != "" {
			if err := c.ctx.SetStateValue(k, v); err != nil {
				return err
			}
			continue
		}

		if err := c.ctx.DeleteStateValue(k); err != nil {
			return err
		}
	}
	return nil
}

func (c *StateValueSetCommand) handleKeyValueFile(ctx *cmd.Context) error {
	if c.keyValueFile.Path == "" {
		return nil
	}

	file, err := c.keyValueFile.Open(ctx)
	if err != nil {
		return errors.Trace(err)
	}
	defer file.Close()

	kvs, err := c.readKeyValuePairs(file)
	if err != nil {
		return errors.Trace(err)
	}

	overrides := c.StateValues
	for k, v := range overrides {
		kvs[k] = v
	}
	c.StateValues = kvs
	return nil
}

func (c *StateValueSetCommand) readKeyValuePairs(in io.Reader) (map[string]string, error) {
	kvs := make(map[string]string)
	scanner := bufio.NewScanner(in)
	for line := 1; scanner.Scan(); line++ {
		tokens := strings.SplitN(scanner.Text(), "=", 2)
		if len(tokens) != 2 {
			return nil, errors.NotValidf("key/value pair on line %d", line)
		}
		kvs[tokens[0]] = tokens[1]
	}
	if err := scanner.Err(); err != nil {
		return nil, err
	}
	return kvs, nil
}
