// Copyright 2020 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package charmhub

import (
	"fmt"
	"io"

	"github.com/juju/cmd"
	"github.com/juju/errors"
	"github.com/juju/gnuflag"

	"github.com/juju/juju/api/base"
	"github.com/juju/juju/api/charmhub"
	"github.com/juju/juju/apiserver/params"
	jujucmd "github.com/juju/juju/cmd"
	"github.com/juju/juju/cmd/modelcmd"
)

const (
	findSummary = "Queries the CharmHub store for available charms or bundles."
	findDoc     = `
The find command queries the CharmHub store for available charms or bundles.

Examples:
    juju find wordpress

See also:
    info
    download
`
)

// NewFindCommand wraps findCommand with sane model settings.
func NewFindCommand() cmd.Command {
	cmd := &findCommand{
		CharmHubClientFunc: func(api base.APICallCloser) FindCommandAPI {
			return charmhub.NewClient(api)
		},
	}
	cmd.APIRootFunc = func() (base.APICallCloser, error) {
		return cmd.NewAPIRoot()
	}
	return modelcmd.Wrap(cmd)
}

// findCommand supplies the "find" CLI command used to display find information.
type findCommand struct {
	modelcmd.ModelCommandBase

	APIRootFunc        func() (base.APICallCloser, error)
	CharmHubClientFunc func(base.APICallCloser) FindCommandAPI

	out        cmd.Output
	warningLog Log

	query     string
	category  string
	channel   string
	charmType string
	publisher string

	columns string
}

// Find returns help related info about the command, it implements
// part of the cmd.Command interface.
func (c *findCommand) Info() *cmd.Info {
	info := &cmd.Info{
		Name:    "find",
		Args:    "[options] <query>",
		Purpose: findSummary,
		Doc:     findDoc,
	}
	return jujucmd.Info(info)
}

// SetFlags defines flags which can be used with the find command.
// It implements part of the cmd.Command interface.
func (c *findCommand) SetFlags(f *gnuflag.FlagSet) {
	c.ModelCommandBase.SetFlags(f)

	c.out.AddFlags(f, "tabular", map[string]cmd.Formatter{
		"yaml":    cmd.FormatYaml,
		"json":    cmd.FormatJson,
		"tabular": c.formatter,
	})

	f.StringVar(&c.category, "category", "", `filter by a category name`)
	f.StringVar(&c.channel, "channel", "", `filter by channel. "latest" can be omitted, so "stable" also matches "latest/stable"`)
	f.StringVar(&c.charmType, "type", "", `search by a given type <charm|bundle>`)
	f.StringVar(&c.publisher, "publisher", "", `search by a given publisher`)

	f.StringVar(&c.columns, "columns", "nbvps", `display the columns associated with a find search.

    The following columns are supported:
        - n: Name
        - b: Bundle
        - v: Version
        - p: Publisher
        - s: Summary
		- a: Architecture
		- o: OS
        - S: Supports
`)
	// TODO (stickupkid): add the following:
	// --narrow
}

// Init initializes the find command, including validating the provided
// flags. It implements part of the cmd.Command interface.
func (c *findCommand) Init(args []string) error {
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
func (c *findCommand) Run(ctx *cmd.Context) error {
	apiRoot, err := c.APIRootFunc()
	if err != nil {
		return errors.Trace(err)
	}
	defer func() { _ = apiRoot.Close() }()
	if apiRoot.BestFacadeVersion("CharmHub") < 1 {
		ctx.Warningf("juju find not supported with controllers < 2.9")
		return nil
	}

	charmHubClient := c.CharmHubClientFunc(apiRoot)

	results, err := charmHubClient.Find(c.query, populateFindOptions(c)...)
	if params.IsCodeNotFound(err) {
		return errors.Wrap(err, errors.Errorf("Nothing found for query %q.", c.query))
	} else if err != nil {
		return errors.Trace(err)
	}

	// This is a side effect of the formatting code not wanting to error out
	// when we get invalid data from the API.
	// We store it on the command before attempting to output, so we can pick
	// it up later.
	c.warningLog = ctx.Warningf

	return c.output(ctx, results)
}

func (c *findCommand) output(ctx *cmd.Context, results []charmhub.FindResponse) error {
	tabular := c.out.Name() == "tabular"
	if tabular {
		// If the results are empty, we should return a helpful message to the
		// operator.
		if len(results) == 0 {
			fmt.Fprintf(ctx.Stderr, "No matching charms for %q\n", c.query)
			return nil
		}

		// Output some helpful errors messages for operators if the query is empty
		// or not.
		if c.query == "" {
			fmt.Fprintf(ctx.Stdout, "No search term specified. Here are some interesting charms:\n\n")
		}
	}

	view := convertCharmFindResults(results)
	if err := c.out.Write(ctx, view); err != nil {
		return errors.Trace(err)
	}

	if tabular && c.query == "" {
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
