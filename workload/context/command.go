// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package context

import (
	"strings"

	"github.com/juju/cmd"
	"github.com/juju/errors"
	"gopkg.in/juju/charm.v5"
	"launchpad.net/gnuflag"

	"github.com/juju/juju/workload"
)

const idArg = "name-or-id"

type cmdInfo struct {
	// Name is the command's name.
	Name string
	// ExtraArgs is the list of arg names that follow "name-or-id", if any.
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

// TODO(ericsnow) How to convert endpoints (charm.Workload.Ports[].Name)
// into actual ports? For now we should error out with such definitions
// (and recommend overriding).

// baseCommand implements the common portions of the workload
// hook env commands.
type baseCommand struct {
	cmd.CommandBase

	cmdInfo
	handleArgs func(map[string]string) error

	ctx     HookContext
	compCtx Component

	// Name is the name of the workload in charm metadata.
	Name string
	// ID is the full ID of the tracked workload.
	ID string
	// info is the workload info for the named workload.
	info *workload.Info
}

func newCommand(ctx HookContext) (*baseCommand, error) {
	compCtx, err := ContextComponent(ctx)
	if err != nil {
		// The component wasn't tracked properly.
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
	for _, name := range append([]string{idArg}, c.cmdInfo.ExtraArgs...) {
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
	argsMap, err := c.workloadArgs(args)
	if err != nil {
		return errors.Trace(err)
	}
	return errors.Trace(c.handleArgs(argsMap))
}

func (c *baseCommand) workloadArgs(args []string) (map[string]string, error) {
	argNames := append([]string{idArg}, c.cmdInfo.ExtraArgs...)
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
	id := args[idArg]
	if id == "" {
		return errors.Errorf("got empty " + idArg)
	}
	name, _ := workload.ParseID(id)
	c.Name = name
	c.ID = id
	return nil
}

// Run implements cmd.Command.
func (c *baseCommand) Run(ctx *cmd.Context) error {
	id, err := c.findID()
	if err != nil {
		return errors.Trace(err)
	}

	pInfo, err := c.compCtx.Get(id)
	if err != nil && !errors.IsNotFound(err) {
		return errors.Trace(err)
	}
	c.info = pInfo
	return nil
}

func (c *baseCommand) defsFromCharm() (map[string]charm.Workload, error) {
	definitions, err := c.compCtx.Definitions()
	if err != nil {
		return nil, errors.Trace(err)
	}
	defMap := make(map[string]charm.Workload)
	for _, definition := range definitions {
		// We expect no collisions.
		defMap[definition.Name] = definition
	}
	return defMap, nil
}

func (c *baseCommand) trackedWorkloads(ids ...string) (map[string]*workload.Info, error) {
	if len(ids) == 0 {
		tracked, err := c.compCtx.List()
		if err != nil {
			return nil, errors.Trace(err)
		}
		if len(tracked) == 0 {
			return nil, nil
		}
		ids = tracked
	}

	workloads := make(map[string]*workload.Info)
	for _, id := range ids {
		wl, err := c.compCtx.Get(id)
		if errors.IsNotFound(err) {
			wl = nil
		} else if err != nil {
			return nil, errors.Trace(err)
		}
		workloads[id] = wl
	}
	return workloads, nil
}

func (c *baseCommand) findID() (string, error) {
	if c.ID != c.Name {
		return c.ID, nil
	}
	id, err := findID(c.compCtx, c.Name)
	if err == nil {
		c.ID = id
	}
	return id, err
}

// TODO(natefinch): move to findID API server side.

func findID(compCtx Component, name string) (string, error) {
	ids, err := idsForName(compCtx, name)
	if err != nil {
		return "", errors.Trace(err)
	}
	if len(ids) == 0 {
		return "", errors.NotFoundf("ID for %q", name)
	}
	// For now we only support a single workload for a given name.
	if len(ids) > 1 {
		return "", errors.Errorf("found more than one tracked workload for %q", name)
	}

	return ids[0], nil
}

func idsForName(compCtx Component, name string) ([]string, error) {
	tracked, err := compCtx.List()
	if err != nil {
		return nil, errors.Trace(err)
	}
	var ids []string
	for _, id := range tracked {
		trackedName, _ := workload.ParseID(id)
		// For now we only support a single workload for a given name.
		if name == trackedName {
			ids = append(ids, id)
		}
	}
	return ids, nil
}

// trackingCommand is the base for commands that track workloads
// that has been launched.
type trackingCommand struct {
	baseCommand

	// Status is the juju-level status to set for the workload.
	Status workload.Status

	// Details is the launch details returned from the workload plugin.
	Details workload.Details

	// Overrides overwrite the workload definition.
	Overrides []string

	// Additions extend the workload definition.
	Additions []string

	// UpdatedWorkload stores the new workload, if there were any overrides OR additions.
	UpdatedWorkload *charm.Workload

	// Definition is the file definition of the workload.
	Definition cmd.FileVar
}

func newTrackingCommand(ctx HookContext) (*trackingCommand, error) {
	base, err := newCommand(ctx)
	if err != nil {
		return nil, errors.Trace(err)
	}
	c := &trackingCommand{
		baseCommand: *base,
	}
	c.handleArgs = c.init
	return c, nil
}

// SetFlags implements cmd.Command.
func (c *trackingCommand) SetFlags(f *gnuflag.FlagSet) {
	f.Var(&c.Definition, "definition", "workload definition filename (use \"-\" for STDIN)")
	f.Var(cmd.NewAppendStringsValue(&c.Overrides), "override", "override workload definition")
	f.Var(cmd.NewAppendStringsValue(&c.Additions), "extend", "extend workload definition")
}

// Run implements cmd.Command.
func (c *trackingCommand) Run(ctx *cmd.Context) error {
	// We do not call baseCommand.Run here since we do not yet know the ID.

	// TODO(ericsnow) Ensure that c.ID == c.Name?

	ids, err := idsForName(c.compCtx, c.Name)
	if err != nil {
		return errors.Trace(err)
	}
	if len(ids) > 0 {
		// For now we only support a single workload for a given name.
		return errors.Errorf("workload %q already tracked", c.Name)
	}

	if err := c.checkSpace(); err != nil {
		return errors.Trace(err)
	}

	c.Status = workload.Status{
		State: workload.StateRunning,
		// TODO(ericsnow) Set a default Message?
	}

	return nil
}

// track updates the hook context with the information for the
// tracked workload. An error is returned if the workload
// is already being tracked.
func (c *trackingCommand) track(ctx *cmd.Context) error {
	info, err := c.findValidInfo(ctx)
	if err != nil {
		return errors.Trace(err)
	}

	info.Status = c.Status
	info.Details = c.Details

	logger.Tracef("tracking %#v", info)

	if err := c.compCtx.Track(*info); err != nil {
		return errors.Trace(err)
	}

	// We flush to state immedeiately so that status reflects the
	// workload correctly.
	if err := c.compCtx.Flush(); err != nil {
		return errors.Trace(err)
	}

	return nil
}

func (c *trackingCommand) findValidInfo(ctx *cmd.Context) (*workload.Info, error) {
	if c.info == nil {
		info, err := c.findInfo(ctx)
		if err != nil {
			return nil, errors.Trace(err)
		}
		c.info = info
	}
	info := *c.info // copied

	if c.UpdatedWorkload == nil {
		logger.Tracef("parsing updates")
		newWorkload, err := c.parseUpdates(c.info.Workload)
		if err != nil {
			return nil, errors.Trace(err)
		}
		c.UpdatedWorkload = newWorkload
	}
	info.Workload = *c.UpdatedWorkload

	// validate
	if err := info.Workload.Validate(); err != nil {
		return nil, errors.Trace(err)
	}
	if info.IsTracked() {
		return nil, errors.Errorf("already tracked")
	}
	return &info, nil
}

func (c *trackingCommand) findInfo(ctx *cmd.Context) (*workload.Info, error) {
	var definition charm.Workload
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
	logger.Tracef("creating new workload.Info")
	return &workload.Info{Workload: definition}, nil
}

// checkSpace ensures that the requested network space is available
// to the hook.
func (c *trackingCommand) checkSpace() error {
	// TODO(wwitzel3) implement this to ensure that the endpoints provided exist in this space
	return nil
}

func (c *trackingCommand) parseUpdates(definition charm.Workload) (*charm.Workload, error) {
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
