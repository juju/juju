// Copyright 2020 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package charmhub

import (
	"io"

	"github.com/juju/charm/v8"
	"github.com/juju/cmd"
	"github.com/juju/errors"
	"github.com/juju/gnuflag"

	"github.com/juju/juju/api/charmhub"
	"github.com/juju/juju/api/modelconfig"
	jujucmd "github.com/juju/juju/cmd"
	"github.com/juju/juju/cmd/modelcmd"
	"github.com/juju/juju/environs/config"
)

const (
	infoSummary = "Displays detailed information about CharmHub charms."
	infoDoc     = `
The charm can be specified by name or by path. Names are looked for both in the
store and in the deployed charms.

Channels displayed are supported by the default series for this model.  To see
channels supported with other series, use the --series flag.

Examples:
    juju info postgresql

See also:
    find
    download
`
)

// NewInfoCommand wraps infoCommand with sane model settings.
func NewInfoCommand() cmd.Command {
	return modelcmd.Wrap(&infoCommand{})
}

// infoCommand supplies the "info" CLI command used to display info
// about charm snaps.
type infoCommand struct {
	modelcmd.ModelCommandBase
	out        cmd.Output
	warningLog Log

	infoCommandAPI InfoCommandAPI
	modelConfigAPI ModelConfigGetter

	config        bool
	charmOrBundle string
	series        string
}

// Info returns help related info about the command, it implements
// part of the cmd.Command interface.
func (c *infoCommand) Info() *cmd.Info {
	info := &cmd.Info{
		Name:    "info",
		Args:    "[options] <charm>",
		Purpose: infoSummary,
		Doc:     infoDoc,
	}
	return jujucmd.Info(info)
}

// SetFlags defines flags which can be used with the info command.
// It implements part of the cmd.Command interface.
func (c *infoCommand) SetFlags(f *gnuflag.FlagSet) {
	c.ModelCommandBase.SetFlags(f)
	f.BoolVar(&c.config, "config", false, "display config for this charm")
	c.out.AddFlags(f, "tabular", map[string]cmd.Formatter{
		"yaml":    cmd.FormatYaml,
		"json":    cmd.FormatJson,
		"tabular": c.formatter,
	})
	f.StringVar(&c.series, "series", "", "display channels supported by provided series")
}

// Init initializes the info command, including validating the provided
// flags. It implements part of the cmd.Command interface.
func (c *infoCommand) Init(args []string) error {
	if len(args) != 1 {
		return errors.Errorf("expected a charm or bundle name")
	}
	if err := c.validateCharmOrBundle(args[0]); err != nil {
		return errors.Trace(err)
	}
	c.charmOrBundle = args[0]
	return nil
}

func (c *infoCommand) validateCharmOrBundle(charmOrBundle string) error {
	curl, err := charm.ParseURL(charmOrBundle)
	if err != nil {
		return errors.Trace(err)
	}
	if !charm.CharmHub.Matches(curl.Schema) {
		return errors.Errorf("%q is not a Charm Hub charm", charmOrBundle)
	}
	return nil
}

// Run is the business logic of the info command.  It implements the meaty
// part of the cmd.Command interface.
func (c *infoCommand) Run(ctx *cmd.Context) error {
	charmHubClient, modelConfigClient, err := c.getAPI()
	if err != nil {
		return errors.Trace(err)
	}
	defer func() { _ = charmHubClient.Close() }()

	if err := c.verifySeries(modelConfigClient); err != nil {
		return errors.Trace(err)
	}
	info, err := charmHubClient.Info(c.charmOrBundle)
	if err != nil {
		return errors.Trace(err)
	}

	// This is a side effect of the formatting code not wanting to error out
	// when we get invalid data from the API.
	// We store it on the command before attempting to output, so we can pick
	// it up later.
	c.warningLog = ctx.Warningf

	view, err := convertCharmInfoResult(info, c.series)
	if err != nil {
		return errors.Trace(err)
	}
	return c.out.Write(ctx, &view)
}

func (c *infoCommand) verifySeries(modelConfigClient ModelConfigGetter) error {
	if c.series != "" {
		return nil
	}
	attrs, err := modelConfigClient.ModelGet()
	if err != nil {
		return errors.Trace(err)
	}
	cfg, err := config.New(config.NoDefaults, attrs)
	if err != nil {
		return errors.Trace(err)
	}
	if defaultSeries, explicit := cfg.DefaultSeries(); explicit {
		c.series = defaultSeries
	}
	return nil
}

// getAPI returns the API that supplies methods
// required to execute this command.
func (c *infoCommand) getAPI() (InfoCommandAPI, ModelConfigGetter, error) {
	if c.infoCommandAPI != nil && c.modelConfigAPI != nil {
		// This is for testing purposes, for testing, both values
		// should be set.
		return c.infoCommandAPI, c.modelConfigAPI, nil
	}
	api, err := c.NewAPIRoot()
	if err != nil {
		return nil, nil, errors.Annotate(err, "opening API connection")
	}
	return charmhub.NewClient(api), modelconfig.NewClient(api), nil
}

func (c *infoCommand) formatter(writer io.Writer, value interface{}) error {
	results, ok := value.(*InfoResponse)
	if !ok {
		return errors.Errorf("unexpected results")
	}

	if err := makeInfoWriter(writer, c.warningLog, c.config, results).Print(); err != nil {
		return errors.Trace(err)
	}

	return nil
}
