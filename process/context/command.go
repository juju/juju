// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package context

import (
	"os"
	"path/filepath"
	"strings"

	"github.com/juju/cmd"
	"github.com/juju/errors"
	"gopkg.in/juju/charm.v5"
	"gopkg.in/yaml.v1"
	"launchpad.net/gnuflag"

	"github.com/juju/juju/process"
)

// TODO(ericsnow) How to convert endpoints (charm.Process.Ports[].Name)
// into actual ports? For now we should error out with such definitions
// (and recommend overriding).

// baseCommand implements the common portions of the workload process
// hook env commands.
type baseCommand struct {
	cmd.CommandBase

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

	pInfo, err := c.compCtx.Get(c.Name)
	if err != nil && !errors.IsNotFound(err) {
		return errors.Trace(err)
	}
	c.info = pInfo

	return nil
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

	// ReadMetadata extracts charm metadata from the given file.
	ReadMetadata func(filename string) (*charm.Meta, error)
}

func newRegisteringCommand(ctx HookContext) (*registeringCommand, error) {
	base, err := newCommand(ctx)
	if err != nil {
		return nil, errors.Trace(err)
	}
	return &registeringCommand{
		baseCommand:  *base,
		ReadMetadata: readMetadata,
	}, nil
}

func readMetadata(filename string) (*charm.Meta, error) {
	file, err := os.Open(filename)
	if err != nil {
		return nil, errors.Trace(err)
	}
	defer file.Close()

	meta, err := charm.ReadMeta(file)
	if err != nil {
		return nil, errors.Trace(err)
	}

	return meta, nil
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

	newProcess, err := c.parseUpdates(info.Process)
	if err != nil {
		return errors.Trace(err)
	}
	c.UpdatedProcess = newProcess
	info.Process = *newProcess

	info.Details = c.Details

	if err := c.compCtx.Set(c.Name, info); err != nil {
		return errors.Trace(err)
	}
	// TODO(ericsnow) flush here?
	return nil
}

func (c *registeringCommand) findValidInfo(ctx *cmd.Context) (*process.Info, error) {
	if c.info != nil {
		copied := *c.info
		return &copied, nil
	}

	var definition charm.Process
	if c.Definition.Path == "" {
		filename := filepath.Join(ctx.Dir, "metadata.yaml")
		charmDef, err := c.defFromMetadata(c.Name, filename)
		if err != nil {
			return nil, errors.Trace(err)
		}
		definition = *charmDef
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
	info := &process.Info{Process: definition}
	c.info = info

	// validate
	if err := info.Validate(); err != nil {
		return nil, errors.Trace(err)
	}
	if info.IsRegistered() {
		return nil, errors.Errorf("already registered")
	}
	return info, nil
}

func (c *registeringCommand) defFromMetadata(name, filename string) (*charm.Process, error) {
	meta, err := c.ReadMetadata(filename)
	if err != nil {
		return nil, errors.Trace(err)
	}
	for _, definition := range meta.Processes {
		if name == definition.Name {
			return &definition, nil
		}
	}
	return nil, errors.NotFoundf(name)
}

func parseDefinition(name string, data []byte) (*charm.Process, error) {
	raw := make(map[interface{}]interface{})
	if err := yaml.Unmarshal(data, raw); err != nil {
		return nil, errors.Trace(err)
	}
	definition, err := charm.ParseProcess(name, raw)
	if err != nil {
		return nil, errors.Trace(err)
	}
	if definition.Name == "" {
		definition.Name = name
	} else if definition.Name != name {
		return nil, errors.Errorf("process name mismatch; %q != %q", definition.Name, name)
	}
	return definition, nil
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
