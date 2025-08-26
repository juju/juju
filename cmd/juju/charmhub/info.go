// Copyright 2020 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package charmhub

import (
	"context"
	"fmt"
	"io"

	"github.com/juju/charm/v12"
	"github.com/juju/cmd/v3"
	"github.com/juju/errors"
	"github.com/juju/gnuflag"

	"github.com/juju/juju/charmhub"
	jujucmd "github.com/juju/juju/cmd"
	corebase "github.com/juju/juju/core/base"
)

const (
	infoSummary = "Displays detailed information about CharmHub charms."
	infoDoc     = `
The charm can be specified by name or by path.

Channels displayed are supported by any base.
To see channels supported for only a specific base, use the ` + "`--base`" + ` flag.
` + "`--base`" + ` can be specified using the OS name and the version of the OS,
separated by ` + "`@`" + `.
For example: ` + "`--base ubuntu@22.04`" + `.

Use ` + "`--revision`" + ` to display information about a specific revision of the charm,
which cannot be used together with ` + "`--arch`" + `, ` + "`--base`" + `, ` + "`--channel`" + ` or ` + "`--series`" + `.
For example: ` + "`--revision 42`" + `.

Use ` + "`--track `" + ` to display information about a specific track of the charm,
which cannot be used together with ` + "`--arch`" + `, ` + "`--base`" + `, ` + "`--channel`" + ` or ` + "`--series`" + `.
For example: ` + "`--track 14`" + `.
`
	infoExamples = `
    juju info postgresql
`
)

// NewInfoCommand wraps infoCommand with sane model settings.
func NewInfoCommand() cmd.Command {
	return &infoCommand{
		charmHubCommand: newCharmHubCommand(),
	}
}

// infoCommand supplies the "info" CLI command used to display info
// about charm snaps.
type infoCommand struct {
	*charmHubCommand

	out        cmd.Output
	warningLog Log

	config        bool
	channel       string
	charmOrBundle string
	revision      int
	track         string

	unicode string
}

// Info returns help related info about the command, it implements
// part of the cmd.Command interface.
func (c *infoCommand) Info() *cmd.Info {
	info := &cmd.Info{
		Name:     "info",
		Args:     "[options] <charm>",
		Purpose:  infoSummary,
		Doc:      infoDoc,
		Examples: infoExamples,
		SeeAlso: []string{
			"find",
			"download",
		},
	}
	return jujucmd.Info(info)
}

// SetFlags defines flags which can be used with the info command.
// It implements part of the cmd.Command interface.
func (c *infoCommand) SetFlags(f *gnuflag.FlagSet) {
	c.charmHubCommand.SetFlags(f)

	f.StringVar(&c.arch, "arch", ArchAll, fmt.Sprintf("Specify an arch <%s>", c.archArgumentList()))
	f.StringVar(&c.series, "series", SeriesAll, "Specify a series. DEPRECATED use `--base`")
	f.StringVar(&c.base, "base", "", "Specify a base")
	f.StringVar(&c.channel, "channel", "", "Specify a channel to use instead of the default release")
	f.BoolVar(&c.config, "config", false, "Display config for this charm")
	f.IntVar(&c.revision, "revision", -1, "Specify a revision number")
	f.StringVar(&c.track, "track", "", "Specify a track to use instead of the default track")
	f.StringVar(&c.unicode, "unicode", "auto", "Display output using unicode <auto|never|always>")
	c.out.AddFlags(f, "tabular", map[string]cmd.Formatter{
		"yaml":    cmd.FormatYaml,
		"json":    cmd.FormatJson,
		"tabular": c.formatter,
	})
}

// Init initializes the info command, including validating the provided
// flags. It implements part of the cmd.Command interface.
func (c *infoCommand) Init(args []string) error {
	if c.base != "" && (c.series != "" && c.series != SeriesAll) {
		return errors.New("--series and --base cannot be specified together")
	}

	hasArch := c.arch != ArchAll && c.arch != ""
	hasBase := c.base != ""
	hasChannel := c.channel != ""
	hasSeries := c.series != SeriesAll && c.series != ""
	if c.revision != -1 && (hasArch || hasBase || hasChannel || hasSeries) {
		return errors.New("--revision cannot be specified together with --arch, --base, --channel or --series")
	}

	if c.track != "" && (hasArch || hasBase || hasChannel || hasSeries) {
		return errors.New("--track cannot be specified together with --arch, --base, --channel or --series")
	}

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
		logger.Debugf("%s", err)
		return errors.NotValidf("charm or bundle name, %q, is", charmOrBundle)
	}
	if !charm.CharmHub.Matches(curl.Schema) {
		return errors.Errorf("%q is not a Charm Hub charm", charmOrBundle)
	}
	return nil
}

// Run is the business logic of the info command.  It implements the meaty
// part of the cmd.Command interface.
func (c *infoCommand) Run(cmdContext *cmd.Context) error {
	var (
		base corebase.Base
		err  error
	)
	// Note: we validated that both series and base cannot be specified in
	// Init(), so it's safe to assume that only one of them is set here.
	if c.series == SeriesAll {
		c.series = ""
	} else if c.series != "" {
		cmdContext.Warningf("series flag is deprecated, use --base instead")
		if base, err = corebase.GetBaseFromSeries(c.series); err != nil {
			return errors.Annotatef(err, "attempting to convert %q to a base", c.series)
		}
		c.base = base.String()
		c.series = ""
	}
	if c.base != "" {
		if base, err = corebase.ParseBaseFromString(c.base); err != nil {
			return errors.Trace(err)
		}
	}

	cfg := charmhub.Config{
		URL:    c.charmHubURL,
		Logger: logger,
	}

	client, err := c.CharmHubClientFunc(cfg)
	if err != nil {
		return errors.Trace(err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	var options []charmhub.InfoOption
	var risk charm.Risk
	if c.channel != "" {
		charmChannel, err := charm.ParseChannelNormalize(c.channel)
		if err != nil {
			return errors.Trace(err)
		}
		risk = charmChannel.Risk
		c.track = charmChannel.Track
	}

	info, err := client.Info(ctx, c.charmOrBundle, options...)
	if errors.IsNotFound(err) {
		return errors.Wrap(err, errors.Errorf("No information found for charm or bundle with the name %q", c.charmOrBundle))
	} else if err != nil {
		return errors.Trace(err)
	}

	view, err := convertInfoResponse(info, c.arch, risk, c.revision, c.track, base)
	if err != nil {
		return errors.Trace(err)
	}

	// This is a side effect of the formatting code not wanting to error out
	// when we get invalid data from the API.
	// We store it on the command before attempting to output, so we can pick
	// it up later.
	c.warningLog = cmdContext.Warningf

	return c.out.Write(cmdContext, &view)
}

func (c *infoCommand) formatter(writer io.Writer, value interface{}) error {
	results, ok := value.(*InfoResponse)
	if !ok {
		return errors.Errorf("unexpected results")
	}

	// Default is to include both architecture and bases
	mode := baseModeBoth
	switch {
	case c.arch != ArchAll && c.base != "":
		// If --arch and --base given, don't show arch or bases
		mode = baseModeNone
	case c.arch != ArchAll && c.base == "":
		// If only --arch given, show bases
		mode = baseModeBases
	case c.arch == ArchAll && c.base != "":
		// If only --base given, show arch
		mode = baseModeArches
	}

	if err := makeInfoWriter(writer, c.warningLog, c.config, c.unicode, mode, results, c.revision).Print(); err != nil {
		return errors.Trace(err)
	}

	return nil
}
