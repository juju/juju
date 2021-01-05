// Copyright 2020 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package charmhub

import (
	"io"

	"github.com/juju/charm/v9"
	"github.com/juju/cmd"
	"github.com/juju/errors"
	"github.com/juju/gnuflag"

	"github.com/juju/juju/api/base"
	"github.com/juju/juju/api/charmhub"
	"github.com/juju/juju/apiserver/params"
	jujucmd "github.com/juju/juju/cmd"
	"github.com/juju/juju/cmd/modelcmd"
	corecharm "github.com/juju/juju/core/charm"
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
	return modelcmd.Wrap(&infoCommand{
		charmHubCommand: newCharmHubCommand(),
		CharmHubClientFunc: func(api base.APICallCloser) InfoCommandAPI {
			return charmhub.NewClient(api)
		},
	})
}

// infoCommand supplies the "info" CLI command used to display info
// about charm snaps.
type infoCommand struct {
	*charmHubCommand

	out        cmd.Output
	warningLog Log

	CharmHubClientFunc func(base.APICallCloser) InfoCommandAPI

	config        bool
	channel       string
	charmOrBundle string

	unicode string
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
	c.charmHubCommand.SetFlags(f)

	f.StringVar(&c.channel, "channel", "", "specify a channel to use instead of the default release")
	f.BoolVar(&c.config, "config", false, "display config for this charm")
	f.StringVar(&c.unicode, "unicode", "auto", "display output using unicode <auto|never|always>")
	c.out.AddFlags(f, "tabular", map[string]cmd.Formatter{
		"yaml":    cmd.FormatYaml,
		"json":    cmd.FormatJson,
		"tabular": c.formatter,
	})
}

// Init initializes the info command, including validating the provided
// flags. It implements part of the cmd.Command interface.
func (c *infoCommand) Init(args []string) error {
	if err := c.charmHubCommand.Init(args); err != nil {
		return errors.Trace(err)
	}

	if len(args) != 1 {
		return errors.Errorf("expected a charm or bundle name")
	}
	if err := c.validateCharmOrBundle(args[0]); err != nil {
		return errors.Trace(err)
	}
	c.charmOrBundle = args[0]

	switch c.unicode {
	case "auto", "never", "always":
	case "":
		c.unicode = "auto"
	default:
		return errors.Errorf("unexpected unicode flag value %q, expected <auto|never|always>", c.unicode)
	}

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
	if err := c.charmHubCommand.Run(ctx); err != nil {
		return errors.Trace(err)
	}

	apiRoot, err := c.APIRootFunc()
	if err != nil {
		return errors.Trace(err)
	}
	defer func() { _ = apiRoot.Close() }()

	charmHubClient := c.CharmHubClientFunc(apiRoot)

	channel := c.channel
	if channel != "" {
		charmChannel, err := corecharm.ParseChannelNormalize(c.channel)
		if err != nil {
			return errors.Trace(err)
		}
		channel = charmChannel.String()
	}

	info, err := charmHubClient.Info(c.charmOrBundle, channel)
	if params.IsCodeNotFound(err) {
		return errors.Wrap(err, errors.Errorf("No information found for charm or bundle with the name %q", c.charmOrBundle))
	} else if err != nil {
		return errors.Trace(err)
	}

	// This is a side effect of the formatting code not wanting to error out
	// when we get invalid data from the API.
	// We store it on the command before attempting to output, so we can pick
	// it up later.
	c.warningLog = ctx.Warningf

	view, err := convertCharmInfoResult(info, c.arch, c.series)
	if err != nil {
		return errors.Trace(err)
	}
	return c.out.Write(ctx, &view)
}

func (c *infoCommand) formatter(writer io.Writer, value interface{}) error {
	results, ok := value.(*InfoResponse)
	if !ok {
		return errors.Errorf("unexpected results")
	}

	if err := makeInfoWriter(writer, c.warningLog, c.config, c.unicode, results).Print(); err != nil {
		return errors.Trace(err)
	}

	return nil
}
