// Copyright 2012, 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package jujuc

import (
	"fmt"
	"io"
	"io/ioutil"
	"os"
	"strings"

	"github.com/juju/cmd"
	"github.com/juju/errors"
	"github.com/juju/utils/keyvalues"
	"launchpad.net/gnuflag"
)

const relationSetDoc = `
"relation-set" writes the local unit's settings for some relation.
If no relation is specified then the current relation is used. The
setting values are not inspected and are stored as strings. Setting
an empty string causes the setting to be removed. Duplicate settings
are not allowed.

The --file option should be used when one or more key-value pairs are
too long to fit within the command length limit of the shell or
operating system. The file will contain one key-value pair per line
in the same format as on the commandline. Blank lines and lines
starting with # are ignored. Settings in the file will be overridden
by any duplicate key-value arguments. A value of "-" for the filename
means "read from stdin".
`

// RelationSetCommand implements the relation-set command.
type RelationSetCommand struct {
	cmd.CommandBase
	ctx          Context
	RelationId   int
	Settings     map[string]string
	settingsFile string
	formatFlag   string // deprecated
}

func NewRelationSetCommand(ctx Context) cmd.Command {
	return &RelationSetCommand{ctx: ctx}
}

func (c *RelationSetCommand) Info() *cmd.Info {
	return &cmd.Info{
		Name:    "relation-set",
		Args:    "key=value [key=value ...]",
		Purpose: "set relation settings",
		Doc:     relationSetDoc,
	}
}

func (c *RelationSetCommand) SetFlags(f *gnuflag.FlagSet) {
	rV := newRelationIdValue(c.ctx, &c.RelationId)

	f.Var(rV, "r", "specify a relation by id")
	f.Var(rV, "relation", "")
	f.StringVar(&c.settingsFile, "file", "", "file containing key-value pairs")

	f.StringVar(&c.formatFlag, "format", "", "deprecated format flag")
}

func (c *RelationSetCommand) Init(args []string) error {
	if c.RelationId == -1 {
		return errors.Errorf("no relation id specified")
	}

	if err := c.handleSettings(args); err != nil {
		return errors.Trace(err)
	}
	return nil
}

func (c *RelationSetCommand) readSettings(in io.Reader) (map[string]string, error) {
	data, err := ioutil.ReadAll(in)
	if err != nil {
		return nil, errors.Trace(err)
	}

	var kvs []string
	for _, line := range strings.Split(string(data), "\n") {
		line = strings.TrimSpace(line)
		if line == "" || line[0] == '#' {
			continue
		}
		kvs = append(kvs, line) // We lose trailing whitespace...
	}

	settings, err := keyvalues.Parse(kvs, true)
	if err != nil {
		return nil, errors.Trace(err)
	}
	return settings, nil
}

func (c *RelationSetCommand) handleSettings(args []string) error {
	overrides, err := keyvalues.Parse(args, true)
	if err != nil {
		return errors.Trace(err)
	}
	c.Settings = overrides

	if c.settingsFile == "" {
		return nil
	}
	if c.settingsFile == "-" {
		// We handle stdin in Run.
		return nil
	}

	// Read the settings from the file.
	file, err := os.Open(c.settingsFile)
	if err != nil {
		return errors.Trace(err)
	}
	defer file.Close()
	settings, err := c.readSettings(file)
	if err != nil {
		return errors.Trace(err)
	}
	for k, v := range overrides {
		settings[k] = v
	}
	c.Settings = settings
	return nil
}

func (c *RelationSetCommand) Run(ctx *cmd.Context) (err error) {
	if c.formatFlag != "" {
		fmt.Fprintf(ctx.Stderr, "--format flag deprecated for command %q", c.Info().Name)
	}

	if c.settingsFile == "-" {
		settings, err := c.readSettings(ctx.Stdin)
		if err != nil {
			return errors.Trace(err)
		}
		for k, v := range c.Settings {
			settings[k] = v
		}
		c.Settings = settings
	}

	r, found := c.ctx.Relation(c.RelationId)
	if !found {
		return fmt.Errorf("unknown relation id")
	}
	settings, err := r.Settings()
	if err != nil {
		return errors.Annotate(err, "cannot read relation settings")
	}
	for k, v := range c.Settings {
		if v != "" {
			settings.Set(k, v)
		} else {
			settings.Delete(k)
		}
	}
	return nil
}
