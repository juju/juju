// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package context

import (
	"strings"

	"github.com/juju/cmd"
	"github.com/juju/errors"
	"github.com/juju/loggo"
	"gopkg.in/juju/charm.v5"
	"launchpad.net/gnuflag"

	"github.com/juju/juju/process"
)

var logger = loggo.GetLogger("juju.process.persistence")

type cmdInfo struct {
	// Name is the command's name.
	Name string
	// ExtraArgs is the list of arg names that follow "name", if any.
	ExtraArgs []string
	// OptionalArgs is the list of optional args, if any.
	OptionalArgs []string
	// Summary is the one-line description of the command.
	Summary string
	// Doc is the multi-line description of the command.
	Doc string
}

// TODO(ericsnow) How to convert endpoints (charm.Process.Ports[].Name)
// into actual ports? For now we should error out with such definitions
// (and recommend overriding).

// baseCommand implements the common portions of the workload process
// hook env commands.
type baseCommand struct {
	cmd.CommandBase

	cmdInfo

	ctx     HookContext
	compCtx Component

	// Name is the name of the process in charm metadata.
	Name string
	// info is the process info for the named workload process.
	info *process.Info
}

func newCommand(ctx HookContext) (*baseCommand, error) {
	compCtx, err := ContextComponent(ctx)
	if err != nil {
		// The component wasn't registered properly.
		return nil, errors.Trace(err)
	}
	return &baseCommand{
		ctx:     ctx,
		compCtx: compCtx,
	}, nil
}

// Info implements cmd.Command.
func (c baseCommand) Info() *cmd.Info {
	args := []string{"<name>"} // name isn't optional
	for _, name := range c.cmdInfo.ExtraArgs {
		arg := "<" + name + ">"
		for _, optional := range c.cmdInfo.OptionalArgs {
			if name == optional {
				arg = "[" + arg + "]"
				break
			}
		}
		args = append(args, arg)
	}
	return &cmd.Info{
		Name:    c.cmdInfo.Name,
		Args:    strings.Join(args, " "),
		Purpose: c.cmdInfo.Summary,
		Doc:     c.cmdInfo.Doc,
	}
}

// Init implements cmd.Command.
func (c *baseCommand) Init(args []string) error {
	if len(args) == 0 {
		return errors.Errorf("missing process name")
	}
	return errors.Trace(c.init(args[0]))
}

func (c *baseCommand) init(name string) error {
	if name == "" {
		return errors.Errorf("got empty name")
	}
	c.Name = name

	// TODO(ericsnow) Pull the definitions from the metadata here...

	pInfo, err := c.compCtx.Get(c.Name)
	if err != nil && !errors.IsNotFound(err) {
		return errors.Trace(err)
	}
	c.info = pInfo

	return nil
}

func (c *baseCommand) defsFromCharm() (map[string]charm.Process, error) {
	definitions, err := c.compCtx.ListDefinitions()
	if err != nil {
		return nil, errors.Trace(err)
	}
	defMap := make(map[string]charm.Process)
	for _, definition := range definitions {
		// We expect no collisions.
		defMap[definition.Name] = definition
	}
	return defMap, nil
}

// registeringCommand is the base for commands that register a process
// that has been launched.
type registeringCommand struct {
	baseCommand

	// Details is the launch details returned from the process plugin.
	Details process.Details

	// Overrides overwrite the process definition.
	Overrides []string

	// Additions extend the process definition.
	Additions []string

	// UpdatedProcess stores the new process, if there were any overrides OR additions.
	UpdatedProcess *charm.Process

	// Definition is the file definition of the process.
	Definition cmd.FileVar
}

func newRegisteringCommand(ctx HookContext) (*registeringCommand, error) {
	base, err := newCommand(ctx)
	if err != nil {
		return nil, errors.Trace(err)
	}
	return &registeringCommand{
		baseCommand: *base,
	}, nil
}

// SetFlags implements cmd.Command.
func (c *registeringCommand) SetFlags(f *gnuflag.FlagSet) {
	f.Var(&c.Definition, "definition", "process definition filename (use \"-\" for STDIN)")
	f.Var(cmd.NewAppendStringsValue(&c.Overrides), "override", "override process definition")
	f.Var(cmd.NewAppendStringsValue(&c.Additions), "extend", "extend process definition")
}

func (c *registeringCommand) init(name string) error {
	if err := c.baseCommand.init(name); err != nil {
		return errors.Trace(err)
	}

	if c.info != nil {
		return errors.Errorf("process %q already registered", c.Name)
	}

	if err := c.checkSpace(); err != nil {
		return errors.Trace(err)
	}

	// Either the named process must already be defined or the command
	// must have been run with the --definition option.
	if c.Definition.Path != "" {
		if c.info != nil {
			return errors.Errorf("process %q already defined", c.Name)
		}
	}

	return nil
}

// register updates the hook context with the information for the
// registered workload process. An error is returned if the process
// was already registered.
func (c *registeringCommand) register(ctx *cmd.Context) error {
	info, err := c.findValidInfo(ctx)
	if err != nil {
		return errors.Trace(err)
	}

	info.Details = c.Details

	if err := c.compCtx.Set(c.Name, info); err != nil {
		return errors.Trace(err)
	}

	// We flush to state immedeiately so that status reflects the
	// process correctly.
	if err := c.compCtx.Flush(); err != nil {
		return errors.Trace(err)
	}

	return nil
}

func (c *registeringCommand) findValidInfo(ctx *cmd.Context) (*process.Info, error) {
	if c.info == nil {
		info, err := c.findInfo(ctx)
		if err != nil {
			return nil, errors.Trace(err)
		}
		c.info = info
	}
	info := *c.info // copied

	if c.UpdatedProcess == nil {
		logger.Debugf("parsing updates")
		newProcess, err := c.parseUpdates(c.info.Process)
		if err != nil {
			return nil, errors.Trace(err)
		}
		c.UpdatedProcess = newProcess
	}
	info.Process = *c.UpdatedProcess

	// validate
	if err := info.Validate(); err != nil {
		return nil, errors.Trace(err)
	}
	if info.IsRegistered() {
		return nil, errors.Errorf("already registered")
	}
	return &info, nil
}

func (c *registeringCommand) findInfo(ctx *cmd.Context) (*process.Info, error) {
	var definition charm.Process
	if c.Definition.Path == "" {
		defs, err := c.defsFromCharm()
		if err != nil {
			return nil, errors.Trace(err)
		}
		charmDef, ok := defs[c.Name]
		if !ok {
			return nil, errors.NotFoundf(c.Name)
		}
		definition = charmDef
	} else {
		// c.info must be nil at this point.
		data, err := c.Definition.Read(ctx)
		if err != nil {
			return nil, errors.Trace(err)
		}
		cliDef, err := parseDefinition(c.Name, data)
		if err != nil {
			return nil, errors.Trace(err)
		}
		definition = *cliDef
	}
	logger.Debugf("creating new process.Info")
	return &process.Info{Process: definition}, nil
}

// checkSpace ensures that the requested network space is available
// to the hook.
func (c *registeringCommand) checkSpace() error {
	// TODO(wwitzel3) implement this to ensure that the endpoints provided exist in this space
	return nil
}

func (c *registeringCommand) parseUpdates(definition charm.Process) (*charm.Process, error) {
	overrides, err := parseUpdates(c.Overrides)
	if err != nil {
		return nil, errors.Annotate(err, "override")
	}

	additions, err := parseUpdates(c.Additions)
	if err != nil {
		return nil, errors.Annotate(err, "extend")
	}

	newDefinition, err := definition.Apply(overrides, additions)
	if err != nil {
		return nil, errors.Trace(err)
	}

	return newDefinition, nil
}
