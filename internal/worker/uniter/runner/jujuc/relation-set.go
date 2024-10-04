// Copyright 2012, 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package jujuc

import (
	"fmt"
	"io"

	"github.com/juju/cmd/v4"
	"github.com/juju/errors"
	"github.com/juju/gnuflag"
	"github.com/juju/utils/v4/keyvalues"
	goyaml "gopkg.in/yaml.v2"

	jujucmd "github.com/juju/juju/cmd"
)

const relationSetDoc = `
"relation-set" writes the local unit's settings for some relation.
If no relation is specified then the current relation is used. The
setting values are not inspected and are stored as strings. Setting
an empty string causes the setting to be removed. Duplicate settings
are not allowed.

If the unit is the leader, it can set the application settings using
"--app". These are visible to related applications via 'relation-get --app'
or by supplying the application name to 'relation-get' in place of
a unit name.

The --file option should be used when one or more key-value pairs are
too long to fit within the command length limit of the shell or
operating system. The file will contain a YAML map containing the
settings.  Settings in the file will be overridden by any duplicate
key-value arguments. A value of "-" for the filename means <stdin>.

Further details:
relation-set writes the local unit’s settings for some relation. If it’s not running in a
relation hook, -r needs to be specified. The value part of an argument is not inspected,
and is stored directly as a string. Setting an empty string causes the setting to be removed.

relation-set is the tool for communicating information between units of related applications.
By convention the charm that provides an interface is likely to set values, and a charm that
requires that interface will read values; but there is nothing enforcing this. Whatever
information you need to propagate for the remote charm to work must be propagated via relation-set,
with the single exception of the private-address key, which is always set before the unit joins.

For some charms you may wish to overwrite the private-address setting, for example if you’re
writing a charm that serves as a proxy for some external application. It is rarely a good idea
to remove that key though, as most charms expect that value to exist unconditionally and may
fail if it is not present.

All values are set in a transaction at the point when the hook terminates successfully
(i.e. the hook exit code is 0). At that point all changed values will be communicated to
the rest of the system, causing -changed hooks to run in all related units.

There is no way to write settings for any unit other than the local unit. However, any hook
on the local unit can write settings for any relation which the local unit is participating in.
`

const relationSetExamples = `
    relation-set port=80 tuning=default

    relation-set -r server:3 username=jim password=12345
`

// RelationSetCommand implements the relation-set command.
type RelationSetCommand struct {
	cmd.CommandBase
	ctx             Context
	RelationId      int
	relationIdProxy gnuflag.Value
	Settings        map[string]string
	settingsFile    cmd.FileVar
	formatFlag      string // deprecated
	Application     bool
}

func NewRelationSetCommand(ctx Context) (cmd.Command, error) {
	c := &RelationSetCommand{ctx: ctx}

	rV, err := NewRelationIdValue(ctx, &c.RelationId)
	if err != nil {
		return nil, errors.Trace(err)
	}
	c.relationIdProxy = rV
	c.Application = false

	return c, nil
}

func (c *RelationSetCommand) Info() *cmd.Info {
	return jujucmd.Info(&cmd.Info{
		Name:     "relation-set",
		Args:     "key=value [key=value ...]",
		Purpose:  "Set relation settings.",
		Doc:      relationSetDoc,
		Examples: relationSetExamples,
	})
}

func (c *RelationSetCommand) SetFlags(f *gnuflag.FlagSet) {
	f.Var(c.relationIdProxy, "r", "specify a relation by id")
	f.Var(c.relationIdProxy, "relation", "")

	c.settingsFile.SetStdin()
	f.Var(&c.settingsFile, "file", "file containing key-value pairs")

	f.BoolVar(&c.Application, "app", false, `pick whether you are setting "application" settings or "unit" settings`)

	f.StringVar(&c.formatFlag, "format", "", "deprecated format flag")
}

func (c *RelationSetCommand) Init(args []string) error {
	if c.RelationId == -1 {
		return errors.Errorf("no relation id specified")
	}

	// The overrides will be applied during Run when c.settingsFile is handled.
	overrides, err := keyvalues.Parse(args, true)
	if err != nil {
		return errors.Trace(err)
	}
	c.Settings = overrides
	return nil
}

func readSettings(in io.Reader) (map[string]string, error) {
	data, err := io.ReadAll(in)
	if err != nil {
		return nil, errors.Trace(err)
	}

	kvs := make(map[string]string)
	if err := goyaml.Unmarshal(data, kvs); err != nil {
		return nil, errors.Trace(err)
	}

	return kvs, nil
}

func (c *RelationSetCommand) handleSettingsFile(ctx *cmd.Context) error {
	if c.settingsFile.Path == "" {
		return nil
	}

	file, err := c.settingsFile.Open(ctx)
	if err != nil {
		return errors.Trace(err)
	}
	defer file.Close()

	settings, err := readSettings(file)
	if err != nil {
		return errors.Trace(err)
	}

	overrides := c.Settings
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
	if err := c.handleSettingsFile(ctx); err != nil {
		return errors.Trace(err)
	}

	r, err := c.ctx.Relation(c.RelationId)
	if err != nil {
		return errors.Trace(err)
	}
	var settings Settings
	if c.Application {
		isLeader, lErr := c.ctx.IsLeader()
		if lErr != nil {
			return errors.Annotate(lErr, "cannot determine leadership status")
		} else if isLeader == false {
			return errors.Errorf("cannot write relation settings")
		}
		settings, err = r.ApplicationSettings(ctx)
		if err != nil {
			return errors.Annotate(err, "cannot read relation application settings")
		}
	} else {
		settings, err = r.Settings(ctx)
		if err != nil {
			return errors.Annotate(err, "cannot read relation settings")
		}
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
