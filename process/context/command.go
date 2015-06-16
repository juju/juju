// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package context

import (
	"strings"

	"github.com/juju/cmd"
	"github.com/juju/errors"
	"gopkg.in/juju/charm.v5"
	"gopkg.in/yaml.v1"
	"launchpad.net/gnuflag"

	"github.com/juju/juju/process"
	"github.com/juju/juju/worker/uniter/runner/jujuc"
)

// baseCommand implements the common portions of the workload process
// hook env commands.
type baseCommand struct {
	cmd.CommandBase

	ctx     jujuc.Context
	compCtx jujuc.ContextComponent

	// Name is the name of the process in charm metadata.
	Name string
	// info is the process info for the named workload process.
	info *process.Info
	// notFounderr is the cached error in the case that the named
	// process was not found.
	notFoundErr error
}

func newCommand(ctx jujuc.Context) (*baseCommand, error) {
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
func (c *baseCommand) Info() *cmd.Info {
	panic("not implemented")
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

	pInfo, maybeErr, err := c.getInfo()
	if err != nil {
		return errors.Trace(err)
	}
	c.notFoundErr = maybeErr
	c.info = pInfo
	return nil
}

func (c baseCommand) getInfo() (*process.Info, error, error) {
	var pInfo process.Info
	err := c.compCtx.Get(c.Name, &pInfo)
	if errors.IsNotFound(err) {
		return nil, err, nil
	}
	if err != nil {
		return nil, nil, errors.Trace(err)
	}
	if pInfo.Status != process.StatusPending {
		return nil, nil, errors.Errorf("process %q already registered", c.Name)
	}
	return &pInfo, nil, nil
}

// Run implements cmd.Command.
func (c *baseCommand) Run(ctx *cmd.Context) error {
	panic("not implemented")
}

// registeringCommand is the base for commands that register a process
// that has been launched.
type registeringCommand struct {
	baseCommand

	// Id is the unique ID for the launched process.
	Id string
	// Details is the launch details returned from the process plugin.
	Details process.LaunchDetails

	// Overrides overwrite the process definition.
	Overrides []string

	// Additions extend the process definition.
	Additions []string

	// UpdatedProcess stores the new process, if there were any overrides OR additions.
	UpdatedProcess *charm.Process

	// Definition is the file definition of the process.
	Definition cmd.FileVar
}

func newRegisteringCommand(ctx jujuc.Context) (*registeringCommand, error) {
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
	if err := c.checkSpace(); err != nil {
		return errors.Trace(err)
	}

	// Either the named process must already be defined or the command
	// must have been run with the --definition option.
	if c.Definition.Path != "" {
		if c.info != nil {
			return errors.Errorf("process %q already defined", c.Name)
		}
	} else if c.info == nil {
		// c.info is nil only when the named process was not found. In
		// that case we can return the orignal error from when we looked
		// up the process.
		return errors.Trace(c.notFoundErr)
	}

	return nil
}

// checkSpace ensures that the requested network space is available
// to the hook.
func (c *registeringCommand) checkSpace() error {
	// TODO(wwitzel3) implement this to ensure that the endpoints provided exist in this space
	return nil
}

func (c *registeringCommand) parseUpdates(info *process.Info) error {
	overrides, err := parseUpdates(c.Overrides)
	if err != nil {
		return errors.Annotate(err, "override")
	}

	additions, err := parseUpdates(c.Additions)
	if err != nil {
		return errors.Annotate(err, "extend")
	}

	newProcess, err := info.Process.Apply(overrides, additions)
	if err != nil {
		return errors.Trace(err)
	}

	c.UpdatedProcess = newProcess
	info.Process = *newProcess
	return nil
}

// parseUpdate builds a charm.ProcessFieldValue from an update string.
func parseUpdate(update string) (charm.ProcessFieldValue, error) {
	var pfv charm.ProcessFieldValue

	parts := strings.SplitN(update, ":", 2)
	if len(parts) == 1 {
		return pfv, errors.Errorf("missing value")
	}
	pfv.Field, pfv.Value = parts[0], parts[1]

	if pfv.Field == "" {
		return pfv, errors.Errorf("missing field")
	}
	if pfv.Value == "" {
		return pfv, errors.Errorf("missing value")
	}

	fieldParts := strings.SplitN(pfv.Field, "/", 2)
	if len(fieldParts) == 2 {
		pfv.Field = fieldParts[0]
		pfv.Subfield = fieldParts[1]
	}

	return pfv, nil
}

// parseUpdates parses the updates list in to a charm.ProcessFieldValue list.
func parseUpdates(updates []string) ([]charm.ProcessFieldValue, error) {
	var results []charm.ProcessFieldValue
	for _, update := range updates {
		pfv, err := parseUpdate(update)
		if err != nil {
			return nil, errors.Trace(err)
		}
		results = append(results, pfv)
	}
	return results, nil
}

func (c *registeringCommand) parseDefinition(data []byte) (*process.Info, error) {
	raw := make(map[interface{}]interface{})
	if err := yaml.Unmarshal(data, raw); err != nil {
		return nil, errors.Trace(err)
	}
	definition, err := charm.ParseProcess(c.Name, raw)
	if err != nil {
		return nil, errors.Trace(err)
	}
	if definition.Name == "" {
		definition.Name = c.Name
	} else if definition.Name != c.Name {
		return nil, errors.Errorf("process name mismatch; %q != %q", definition.Name, c.Name)
	}
	info := &process.Info{
		Process: *definition,
	}
	return info, nil
}

// register updates the hook context with the information for the
// registered workload process. An error is returned if the process
// was already registered.
func (c *registeringCommand) register(ctx *cmd.Context, status process.Status) error {
	if c.info != nil && c.info.IsRegistered() {
		return errors.Errorf("already registered")
	}

	info := c.info
	if c.Definition.Path != "" {
		// c.info must be nil at this point.
		data, err := c.Definition.Read(ctx)
		if err != nil {
			return errors.Trace(err)
		}
		info, err = c.parseDefinition(data)
		if err != nil {
			return errors.Trace(err)
		}
	}
	if err := c.parseUpdates(info); err != nil {
		return errors.Trace(err)
	}
	info.Details = c.Details
	info.Status = status

	if err := c.compCtx.Set(c.Name, info); err != nil {
		return errors.Trace(err)
	}
	// TODO(ericsnow) flush here?
	return nil
}
