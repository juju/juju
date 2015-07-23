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

func (ci cmdInfo) isOptional(arg string) bool {
	for _, optional := range ci.OptionalArgs {
		if arg == optional {
			return true
		}
	}
	return false
}

// TODO(ericsnow) How to convert endpoints (charm.Process.Ports[].Name)
// into actual ports? For now we should error out with such definitions
// (and recommend overriding).

// baseCommand implements the common portions of the workload process
// hook env commands.
type baseCommand struct {
	cmd.CommandBase

	cmdInfo
	handleArgs func(map[string]string) error

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
	c := &baseCommand{
		ctx:     ctx,
		compCtx: compCtx,
	}
	c.handleArgs = c.init
	return c, nil
}

// Info implements cmd.Command.
func (c baseCommand) Info() *cmd.Info {
	var args []string
	for _, name := range append([]string{"name"}, c.cmdInfo.ExtraArgs...) {
		arg := "<" + name + ">"
		if c.cmdInfo.isOptional(name) {
			arg = "[" + arg + "]"
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
	argsMap, err := c.processArgs(args)
	if err != nil {
		return errors.Trace(err)
	}
	return errors.Trace(c.handleArgs(argsMap))
}

func (c *baseCommand) processArgs(args []string) (map[string]string, error) {
	argNames := append([]string{"name"}, c.cmdInfo.ExtraArgs...)
	results := make(map[string]string)
	for i, name := range argNames {
		if len(args) == 0 {
			if !c.cmdInfo.isOptional(name) {
				var missing []string
				for _, name := range argNames[i:] {
					if !c.cmdInfo.isOptional(name) {
						missing = append(missing, name)
					}
				}
				return results, errors.Errorf("missing args %v", missing)
			}
			// Skip the optional arg.
			continue
		}
		results[name], args = args[0], args[1:]
	}
	if err := cmd.CheckEmpty(args); err != nil {
		return results, errors.Trace(err)
	}
	return results, nil
}

func (c *baseCommand) init(args map[string]string) error {
	name := args["name"]
	if name == "" {
		return errors.Errorf("got empty name")
	}
	c.Name = name
	return nil
}

// Run implements cmd.Command.
func (c *baseCommand) Run(ctx *cmd.Context) error {
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

func (c *baseCommand) registeredProcs(ids ...string) (map[string]*process.Info, error) {
	if len(ids) == 0 {
		registered, err := c.compCtx.List()
		if err != nil {
			return nil, errors.Trace(err)
		}
		if len(registered) == 0 {
			return nil, nil
		}
		ids = registered
	}

	procs := make(map[string]*process.Info)
	for _, id := range ids {
		proc, err := c.compCtx.Get(id)
		if errors.IsNotFound(err) {
			proc = nil
		} else if err != nil {
			return nil, errors.Trace(err)
		}
		procs[id] = proc
	}
	return procs, nil
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
	c := &registeringCommand{
		baseCommand: *base,
	}
	c.handleArgs = c.init
	return c, nil
}

// SetFlags implements cmd.Command.
func (c *registeringCommand) SetFlags(f *gnuflag.FlagSet) {
	f.Var(&c.Definition, "definition", "process definition filename (use \"-\" for STDIN)")
	f.Var(cmd.NewAppendStringsValue(&c.Overrides), "override", "override process definition")
	f.Var(cmd.NewAppendStringsValue(&c.Additions), "extend", "extend process definition")
}

// Run implements cmd.Command.
func (c *registeringCommand) Run(ctx *cmd.Context) error {
	if err := c.baseCommand.Run(ctx); err != nil {
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
