// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package status

import (
	"fmt"
	"io"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/juju/clock"
	"github.com/juju/cmd/v3"
	"github.com/juju/collections/set"
	"github.com/juju/errors"
	"github.com/juju/gnuflag"
	"github.com/juju/loggo"
	"github.com/juju/viddy"

	"github.com/juju/juju/api/client/client"
	jujucmd "github.com/juju/juju/cmd"
	"github.com/juju/juju/cmd/juju/storage"
	"github.com/juju/juju/cmd/modelcmd"
	"github.com/juju/juju/cmd/output"
	"github.com/juju/juju/juju/osenv"
	"github.com/juju/juju/rpc/params"
)

var logger = loggo.GetLogger("juju.cmd.juju.status")

type statusAPI interface {
	Status(*client.StatusArgs) (*params.FullStatus, error)
	Close() error
}

// NewStatusCommand returns a new command, which reports on the
// runtime state of various system entities.
func NewStatusCommand() cmd.Command {
	return modelcmd.Wrap(&statusCommand{
		checkProvidedIgnoredFlagF: func() set.Strings { return set.NewStrings() },
	})
}

// Clock defines the methods needed for the status command.
type Clock interface {
	After(time.Duration) <-chan time.Time
}

type statusCommand struct {
	modelcmd.ModelCommandBase
	out       cmd.Output
	patterns  []string
	isoTime   bool
	statusAPI statusAPI
	clock     Clock

	retryCount int
	retryDelay time.Duration

	color   bool
	noColor bool

	// integrations indicates if the integrations/relations section is displayed
	integrations bool
	relations    bool

	// checkProvidedIgnoredFlagF indicates whether ignored options were provided by the user
	checkProvidedIgnoredFlagF func() set.Strings

	// storage indicates if 'storage' section is displayed
	storage bool

	// watch indicates the time to wait between consecutive status queries
	watch time.Duration
}

var usageSummary = `
Report the status of the model, its machines, applications and units.`[1:]

var usageDetails = `
Report the model's status, optionally filtered by names of applications or
units. When selectors are present, filter the report to exclude entities that
do not match.

    juju status [<selector> [...]]

` + "`<selector>`" + ` selects machines, units or applications from the model to display.
Wildcard characters (` + "`*`" + `) enable multiple entities to be matched at the same
time.

    (<machine>|<unit>|<application>)[*]

When an entity that matches <selector> is integrated with other applications, the
status of those applications will also be presented. By default (without a
` + "`<selector>`" + `) the status of all applications and their units will be displayed.


### Altering the output format

The ` + "`--format`" + ` option allows you to specify how the status report is formatted.

- ` + "`--format=tabular`" + ` (default):
Displays information about all aspects of the model in a human-centric manner.
Omits some information by default.
Use the ` + "`--relations`" + ` and ` + "`--storage`" + ` options to include all available information.
- ` + "`--format=line`" + `, ` + "`--format=short`" + `, ` + "`--format=oneline `" + `:
Reports information from units. Includes their IP address, open ports and the status of the workload and agent.
- ` + "`--format=summary`" + `:
Reports aggregated information about the model. Includes a description of subnets and ports that are in use,
the counts of applications, units, and machines by status code.
- ` + "`--format=json`" + `, ` + "`--format=yaml`" + `:
Provides information in a ` + "`JSON`" + ` or ` + "`YAML`" + ` format for programmatic use.

`

const usageExamples = `
Report the status of units hosted on machine ` + "`0`" + `:

    juju status 0

Report the status of the ` + "`mysql`" + ` application:

    juju status mysql

Report the status for applications that start with ` + "`nova-`" + `:

    juju status nova-*

Include information about storage and relations in output:

    juju status --storage --relations

Provide output as valid ` + "`JSON`" + `:

    juju status --format=json

Watch the status every five seconds:

    juju status --watch 5s

Show only applications/units in active status:

    juju status active

Show only applications/units in error status:

    juju status error
`

func (c *statusCommand) Info() *cmd.Info {
	return jujucmd.Info(&cmd.Info{
		Name:     "status",
		Args:     "[<selector> [...]]",
		Purpose:  usageSummary,
		Doc:      usageDetails,
		Examples: usageExamples,
		SeeAlso: []string{
			"machines",
			"show-model",
			"show-status-log",
			"storage",
		},
	})
}

func (c *statusCommand) SetFlags(f *gnuflag.FlagSet) {
	c.ModelCommandBase.SetFlags(f)
	f.BoolVar(&c.isoTime, "utc", false, "Display timestamps in the UTC timezone")

	f.BoolVar(&c.color, "color", false, "Use ANSI color codes in tabular output")
	f.BoolVar(&c.noColor, "no-color", false, "Disable ANSI color codes in tabular output")
	f.BoolVar(&c.integrations, "integrations", false, "Same as `--relations`")
	f.BoolVar(&c.relations, "relations", false, "Show relations section in tabular output")
	f.BoolVar(&c.storage, "storage", false, "Show storage section in tabular output")

	f.IntVar(&c.retryCount, "retry-count", 3, "Number of times to retry API failures")
	f.DurationVar(&c.retryDelay, "retry-delay", 100*time.Millisecond, "Time to wait between retry attempts")

	f.DurationVar(&c.watch, "watch", 0, "Watch the status every period of time")

	c.checkProvidedIgnoredFlagF = func() set.Strings {
		ignoredFlagForNonTabularFormat := set.NewStrings(
			"integrations",
			"relations",
			"storage",
		)
		provided := set.NewStrings()
		f.Visit(func(flag *gnuflag.Flag) {
			if ignoredFlagForNonTabularFormat.Contains(flag.Name) {
				provided.Add(flag.Name)
			}

		})
		return provided
	}

	defaultFormat := "tabular"

	c.out.AddFlags(f, defaultFormat, map[string]cmd.Formatter{
		"yaml":    c.formatYaml,
		"json":    c.formatJson,
		"short":   c.formatOneline,
		"oneline": c.formatOneline,
		"line":    c.formatOneline,
		"tabular": c.FormatTabular,
		"summary": c.formatSummary,
	})
}

func (c *statusCommand) Init(args []string) error {
	c.patterns = args
	// If use of ISO time not specified on command line,
	// check env var.
	if !c.isoTime {
		var err error
		envVarValue := os.Getenv(osenv.JujuStatusIsoTimeEnvKey)
		if envVarValue != "" {
			if c.isoTime, err = strconv.ParseBool(envVarValue); err != nil {
				return errors.Annotatef(err, "invalid %s env var, expected true|false", osenv.JujuStatusIsoTimeEnvKey)
			}
		}
	}
	if c.clock == nil {
		c.clock = clock.WallClock
	}

	if c.color && c.noColor {
		return errors.Errorf("cannot mix --no-color and --color")
	}

	return nil
}

var newAPIClientForStatus = func(c *statusCommand) (statusAPI, error) {
	if c.statusAPI == nil {
		api, err := c.NewAPIClient()
		if err != nil {
			return nil, errors.Trace(err)
		}
		c.statusAPI = api
	}
	return c.statusAPI, nil
}

func (c *statusCommand) close() {
	// We really don't care what the errors are if there are some.
	// The user can't do anything about it.  Just try.
	if c.statusAPI != nil {
		c.statusAPI.Close()
	}
}

func (c *statusCommand) getStatus(includeStorage bool) (*params.FullStatus, error) {
	apiclient, err := newAPIClientForStatus(c)
	if err != nil {
		return nil, errors.Trace(err)
	}
	return apiclient.Status(&client.StatusArgs{
		Patterns:       c.patterns,
		IncludeStorage: includeStorage,
	})
}

func (c *statusCommand) runStatus(ctx *cmd.Context) error {
	showIntegrations := c.integrations || c.relations
	showStorage := c.storage
	if c.out.Name() != "tabular" {
		showIntegrations = true
		showStorage = true
		providedIgnoredFlags := c.checkProvidedIgnoredFlagF()
		if !providedIgnoredFlags.IsEmpty() {
			// For non-tabular formats this is redundant and needs to be mentioned to the user.
			joinedMsg := strings.Join(providedIgnoredFlags.SortedValues(), ", ")
			if providedIgnoredFlags.Size() > 1 {
				joinedMsg += " options are"
			} else {
				joinedMsg += " option is"
			}
			ctx.Infof("provided %s always enabled in non tabular formats", joinedMsg)
		}
	}

	// Always attempt to get the status at least once, and retry if it fails.
	status, err := c.getStatus(showStorage)
	if err != nil && !modelcmd.IsModelMigratedError(err) {
		for i := 0; i < c.retryCount; i++ {
			// fun bit - make sure a new api connection is used for each new call
			c.SetModelAPI(nil)
			// Wait for a bit before retries.
			<-c.clock.After(c.retryDelay)
			status, err = c.getStatus(showStorage)
			if err == nil || modelcmd.IsModelMigratedError(err) {
				break
			}
		}
	}

	if err != nil {
		if status == nil {
			// Status call completely failed, there is nothing to report
			return errors.Trace(err)
		}
		// Display any error, but continue to print status if some was returned
		fmt.Fprintf(ctx.Stderr, "%v\n", err)
	} else if status == nil {
		return errors.Errorf("unable to obtain the current status")
	}

	controllerName, err := c.ControllerName()
	if err != nil {
		return errors.Trace(err)
	}
	activeBranch, err := c.ActiveBranch()
	if err != nil {
		return errors.Trace(err)
	}

	formatterParams := NewStatusFormatterParams{
		Status:         status,
		ControllerName: controllerName,
		OutputName:     c.out.Name(),
		ISOTime:        c.isoTime,
		ShowRelations:  showIntegrations,
		ActiveBranch:   activeBranch,
	}
	if showStorage {
		// TODO: move this into StatusFormatter
		storageInfo, err := storage.CombinedStorageFromParams(status.Storage, status.Filesystems, status.Volumes)
		if err != nil {
			return errors.Trace(err)
		}
		formatterParams.Storage = storageInfo
		if storageInfo == nil || storageInfo.Empty() {
			if c.out.Name() == "tabular" {
				// hide storage section for tabular view if nothing to show.
				formatterParams.Storage = nil
			}
		}
	}

	formatted, err := NewStatusFormatter(formatterParams).Format()
	if err != nil {
		return errors.Trace(err)
	}

	if err = c.out.Write(ctx, formatted); err != nil {
		return err
	}

	if !status.IsEmpty() {
		return nil
	}
	if len(c.patterns) == 0 {
		modelName, err := c.ModelIdentifier()
		if err != nil {
			return err
		}
		// A change was made in cmd/v3.0.2 output.go that broke the consistency in output for the
		// default formatter by removing the newline delimiter. Hence we prefix '\n' in the text below.
		// https://github.com/juju/cmd/commit/be22fa661a798055c801f1511aee226db249ef95
		ctx.Infof("\nModel %q is empty.", modelName)
	} else {
		plural := func() string {
			if len(c.patterns) == 1 {
				return ""
			}
			return "s"
		}
		ctx.Infof("Nothing matched specified filter%v.", plural())
	}

	return nil
}

// statusCommandForViddy returns the full juju command including all args
// except the '--watch' flag.
func (c *statusCommand) statusCommandForViddy(args []string) []string {
	var jujuStatusArgsWithoutWatchFlag []string

	for i := range args {
		// In order to support gnu flags, we must first check if the
		// watch flag is using gnu style. In that case, we must remove
		// the entire arg, since it's one entire string (e.g.
		// --watch=1s).
		if strings.HasPrefix(args[i], "--watch=") {
			jujuStatusArgsWithoutWatchFlag = append(args[:i], args[i+1:]...)
			break
		}
		// If the flag is not using gnu style, we must remove both the
		// flag and the argument (e.g --watch 1s)
		if args[i] == "--watch" {
			jujuStatusArgsWithoutWatchFlag = append(args[:i], args[i+2:]...)
			break
		}
	}

	if !c.noColor {
		jujuStatusArgsWithoutWatchFlag = append(jujuStatusArgsWithoutWatchFlag, "--color")
	}
	return jujuStatusArgsWithoutWatchFlag
}

func (c *statusCommand) Run(ctx *cmd.Context) error {
	defer c.close()

	if c.watch != 0 {
		jujuStatusArgs := c.statusCommandForViddy(os.Args)

		viddyArgs := append([]string{"--no-title", "--interval", c.watch.String()}, jujuStatusArgs...)

		// Define tview styles and launch preconfiged Viddy watcher
		app := viddy.NewPreconfigedViddy(viddyArgs)
		if err := app.Run(); err != nil {
			return errors.Annotate(err, "unable to run Viddy (watcher for status command)")
		}
	} else {
		err := c.runStatus(ctx)
		if err != nil {
			return err
		}
	}

	return nil
}

func (c *statusCommand) formatYaml(writer io.Writer, value interface{}) error {
	var noColor bool

	if _, ok := os.LookupEnv("NO_COLOR"); (ok || os.Getenv("TERM") == "dumb") && !c.color || c.noColor {
		return cmd.FormatYaml(writer, value)
	}

	if noColor && c.color {
		return output.FormatYamlWithColor(writer, value)
	}

	if isTerminal(writer) && !noColor {
		return output.FormatYamlWithColor(writer, value)
	}

	if !isTerminal(writer) && c.color {
		return output.FormatYamlWithColor(writer, value)
	}

	return cmd.FormatYaml(writer, value)
}

func (c *statusCommand) formatOneline(writer io.Writer, value interface{}) error {
	if _, ok := os.LookupEnv("NO_COLOR"); (ok || os.Getenv("TERM") == "dumb") && !c.color || c.noColor {
		return FormatOneline(writer, false, value)
	}

	if c.color {
		return FormatOneline(writer, c.color, value)
	}

	if isTerminal(writer) && !c.noColor {
		return FormatOneline(writer, true, value)
	}

	if !isTerminal(writer) && c.color {
		return FormatOneline(writer, true, value)
	}

	return FormatOneline(writer, false, value)
}

func (c *statusCommand) formatJson(writer io.Writer, value interface{}) error {
	if _, ok := os.LookupEnv("NO_COLOR"); (ok || os.Getenv("TERM") == "dumb") && !c.color || c.noColor {
		return cmd.FormatJson(writer, value)
	}
	// NO_COLOR="" and --color=true
	if c.color {
		return output.FormatJsonWithColor(writer, value)
	}

	if isTerminal(writer) && !c.noColor {
		return output.FormatJsonWithColor(writer, value)
	}

	if !isTerminal(writer) && c.color {
		return output.FormatJsonWithColor(writer, value)
	}

	return cmd.FormatJson(writer, value)
}

func (c *statusCommand) FormatTabular(writer io.Writer, value interface{}) error {
	if c.noColor {
		if _, ok := os.LookupEnv("NO_COLOR"); !ok {
			defer os.Unsetenv("NO_COLOR")
			os.Setenv("NO_COLOR", "")
		}
	}

	return FormatTabular(writer, c.color, value)
}
