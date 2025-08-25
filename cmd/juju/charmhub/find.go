// Copyright 2020 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package charmhub

import (
	"context"
	"fmt"
	"io"

	"github.com/juju/cmd/v3"
	"github.com/juju/errors"
	"github.com/juju/gnuflag"

	"github.com/juju/juju/charmhub"
	"github.com/juju/juju/charmhub/transport"
	jujucmd "github.com/juju/juju/cmd"
)

const (
	findSummary = "Queries the Charmhub store for available charms or bundles."
	findDoc     = `
Queries the Charmhub store for available charms or bundles.
`
	findExamples = `
    juju find wordpress
`
)

// NewFindCommand wraps findCommand with sane model settings.
func NewFindCommand() cmd.Command {
	return &findCommand{
		charmHubCommand: newCharmHubCommand(),
	}
}

// findCommand supplies the "find" CLI command used to display find information.
type findCommand struct {
	*charmHubCommand

	out        cmd.Output
	warningLog Log

	query     string
	category  string
	channel   string
	charmType string
	publisher string

	columns string
}

// Info returns help related info about the command, it implements
// part of the cmd.Command interface.
func (c *findCommand) Info() *cmd.Info {
	info := &cmd.Info{
		Name:     "find",
		Args:     "[options] <query>",
		Purpose:  findSummary,
		Doc:      findDoc,
		Examples: findExamples,
		SeeAlso: []string{
			"info",
			"download",
		},
	}
	return jujucmd.Info(info)
}

// SetFlags defines flags which can be used with the find command.
// It implements part of the cmd.Command interface.
func (c *findCommand) SetFlags(f *gnuflag.FlagSet) {
	c.charmHubCommand.SetFlags(f)

	c.out.AddFlags(f, "tabular", map[string]cmd.Formatter{
		"yaml":    cmd.FormatYaml,
		"json":    cmd.FormatJson,
		"tabular": c.formatter,
	})

	f.StringVar(&c.category, "category", "", `Filter by a category name`)
	f.StringVar(&c.channel, "channel", "", `Filter by channel`)
	f.StringVar(&c.charmType, "type", "", `Search by a given type <charm|bundle>`)
	f.StringVar(&c.publisher, "publisher", "", `Search by a given publisher`)

	f.StringVar(&c.columns, "columns", "nbvps", `Display the columns associated with a find search.

    The following columns are supported:
        `+"`n`"+`: Name;
        `+"`b`"+`: Bundle;
        `+"`v`"+`: Version;
        `+"`p`"+`: Publisher;
        `+"`s`"+`: Summary;
		`+"`a`"+`: Architecture;
		`+"`o`"+`: OS;
        `+"`S`"+`: Supports.
`)
	// TODO (stickupkid): add the following:
	// --narrow
}

// Init initializes the find command, including validating the provided
// flags. It implements part of the cmd.Command interface.
func (c *findCommand) Init(args []string) error {
	if err := c.charmHubCommand.Init(args); err != nil {
		return errors.Trace(err)
	}

	// We allow searching of empty queries, which will return a list of
	// "interesting charms".
	if len(args) > 0 {
		c.query = args[0]
	}

	if c.charmType != "" && (c.charmType != "charm" && c.charmType != "bundle") {
		return errors.Errorf("expected type to be charm or bundle")
	}

	if c.columns == "" {
		return errors.Errorf("expected at least one column")
	}

	columns := DefaultColumns()
	for _, alias := range c.columns {
		if _, ok := columns[alias]; !ok {
			return errors.Errorf("unexpected column type %q", alias)
		}
	}

	return nil
}

// Run is the business logic of the find command.  It implements the meaty
// part of the cmd.Command interface.
func (c *findCommand) Run(cmdContext *cmd.Context) error {
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

	options := populateFindOptions(c)

	results, err := client.Find(ctx, c.query, options...)
	if errors.IsNotFound(err) {
		return errors.Wrap(err, errors.Errorf("Nothing found for query %q.", c.query))
	} else if err != nil {
		return errors.Trace(err)
	}

	// This is a side effect of the formatting code not wanting to error out
	// when we get invalid data from the API.
	// We store it on the command before attempting to output, so we can pick
	// it up later.
	c.warningLog = cmdContext.Warningf

	return c.output(cmdContext, results, c.query == "" && len(options) == 0)
}

func (c *findCommand) output(ctx *cmd.Context, results []transport.FindResponse, emptyQuery bool) error {
	tabular := c.out.Name() == "tabular"
	if tabular {
		// If the results are empty, we should return a helpful message to the
		// operator.
		if len(results) == 0 {
			charmType := "charms or bundles"
			if c.charmType != "" {
				charmType = fmt.Sprintf("%ss", c.charmType)
			}
			fmt.Fprintf(ctx.Stderr, "No matching %s\n", charmType)
			return nil
		}

		// Output some helpful errors messages for operators if the query is empty
		// or not.
		if emptyQuery {
			fmt.Fprintf(ctx.Stdout, "No search term specified. Here are some interesting charms:\n\n")
		}
	}

	view := convertCharmFindResults(results)
	if err := c.out.Write(ctx, view); err != nil {
		return errors.Trace(err)
	}

	if tabular && emptyQuery {
		fmt.Fprintln(ctx.Stdout, "Provide a search term for more specific results.")
	}

	return nil
}

func (c *findCommand) formatter(writer io.Writer, value interface{}) error {
	results, ok := value.([]FindResponse)
	if !ok {
		return errors.Errorf("unexpected results")
	}

	columns, err := MakeColumns(DefaultColumns(), c.columns)
	if err != nil {
		return errors.Trace(err)
	}

	if err := makeFindWriter(writer, c.warningLog, columns, results).Print(); err != nil {
		return errors.Trace(err)
	}

	return nil
}

func populateFindOptions(cmd *findCommand) []charmhub.FindOption {
	var options []charmhub.FindOption

	if cmd.category != "" {
		options = append(options, charmhub.WithFindCategory(cmd.category))
	}
	if cmd.channel != "" {
		options = append(options, charmhub.WithFindChannel(cmd.channel))
	}
	if cmd.charmType != "" {
		options = append(options, charmhub.WithFindType(cmd.charmType))
	}
	if cmd.publisher != "" {
		options = append(options, charmhub.WithFindPublisher(cmd.publisher))
	}

	return options
}
