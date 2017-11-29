// Copyright 2017 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package hooks

import (
	"github.com/juju/cmd"
	"github.com/juju/errors"
)

// CmdSuffix is the filename suffix to use for executables.
const CmdSuffix = cmdSuffix

// NewCommandFunc defines a function that returns a
// hook command from a context.
type NewCommandFunc func(Context) (cmd.Command, error)

// EnabledCommandsFunc defines a function that returns the
// enabed hook commands, keyed on name.
type EnabledCommandsFunc func() map[string]NewCommandFunc

// NewCommand returns an instance of the named Command, initialized to execute
// against the supplied Context.
func NewCommand(ctx Context, name string, enabledCommands EnabledCommandsFunc) (cmd.Command, error) {
	f := enabledCommands()[name]
	if f == nil {
		return nil, errors.Errorf("unknown command: %s", name)
	}
	command, err := f(ctx)
	if err != nil {
		return nil, errors.Trace(err)
	}
	return command, nil
}
