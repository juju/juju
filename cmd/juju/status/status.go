// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package status

import (
	"fmt"
	"io"
	"os"
	"strconv"
	"time"

	"github.com/juju/clock"
	"github.com/juju/cmd"
	"github.com/juju/errors"
	"github.com/juju/gnuflag"
	"github.com/juju/loggo"

	"github.com/juju/juju/apiserver/params"
	"github.com/juju/juju/cmd/modelcmd"
	"github.com/juju/juju/juju/osenv"
)

var logger = loggo.GetLogger("juju.cmd.juju.status")

type statusAPI interface {
	Status(patterns []string) (*params.FullStatus, error)
	Close() error
}

// NewStatusCommand returns a new command, which reports on the
// runtime state of various system entities.
func NewStatusCommand() cmd.Command {
	return modelcmd.Wrap(&statusCommand{
		relationsFlagProvidedF: func() bool { return false },
	})
}

// Clock defines the methods needed for the status command.
type Clock interface {
	After(time.Duration) <-chan time.Time
}

type statusCommand struct {
	modelcmd.ModelCommandBase
	out      cmd.Output
	patterns []string
	isoTime  bool
	api      statusAPI
	clock    Clock

	retryCount int
	retryDelay time.Duration

	color bool

	// relations indicates if 'relations' section is displayed
	relations bool

	// relationsFlagProvidedF indicates whether 'relations' option was provided by the user.
	relationsFlagProvidedF func() bool
}

var usageSummary = `
Reports the current status of the model, machines, applications and units.`[1:]

var usageDetails = `
By default (without argument), the status of the model, including all
applications and units will be output.

Application or unit names may be used as output filters (the '*' can be used as
a wildcard character). In addition to matched applications and units, related
machines, applications, and units will also be displayed. If a subordinate unit
is matched, then its principal unit will be displayed. If a principal unit is
matched, then all of its subordinates will be displayed.

Machine numbers may also be used as output filters. This will only display data 
in each section relevant to the specified machines. For example, application 
section will only contain the applications that have units on these machines, etc.

The available output formats are:

- tabular (default): Displays status in a tabular format with a separate table
      for the model, machines, applications, relations (if any) and units.
      Note: in this format, the AZ column refers to the cloud region's
      availability zone.
- {short|line|oneline}: List units and their subordinates. For each unit, the IP
      address and agent status are listed.
- summary: Displays the subnet(s) and port(s) the model utilises. Also displays
      aggregate information about:
      - Machines: total #, and # in each state.
      - Units: total #, and # in each state.
      - Applications: total #, and # exposed of each application.
- yaml: Displays information about the model, machines, applications, and units
      in structured YAML format.
- json: Displays information about the model, machines, applications, and units
      in structured JSON format.
      
In tabular format, 'Relations' section is not displayed by default. 
Use --relations option to see this section. This option is ignored in all other 
formats.

Examples:
    juju show-status
    juju show-status mysql
    juju show-status nova-*
    juju show-status --relations

See also:
    machines
    show-model
    show-status-log
    storage
`

func (c *statusCommand) Info() *cmd.Info {
	return &cmd.Info{
		Name:    "show-status",
		Args:    "[filter pattern ...]",
		Purpose: usageSummary,
		Doc:     usageDetails,
		Aliases: []string{"status"},
	}
}

func (c *statusCommand) SetFlags(f *gnuflag.FlagSet) {
	c.ModelCommandBase.SetFlags(f)
	f.BoolVar(&c.isoTime, "utc", false, "Display time as UTC in RFC3339 format")
	f.BoolVar(&c.color, "color", false, "Force use of ANSI color codes")

	f.BoolVar(&c.relations, "relations", false, "Show 'relations' section")

	f.IntVar(&c.retryCount, "retry-count", 3, "Number of times to retry API failures")
	f.DurationVar(&c.retryDelay, "retry-delay", 100*time.Millisecond, "Time to wait between retry attempts")

	c.relationsFlagProvidedF = func() bool {
		provided := false
		f.Visit(func(flag *gnuflag.Flag) {
			if flag.Name == "relations" {
				provided = true
			}
		})
		return provided
	}

	defaultFormat := "tabular"

	c.out.AddFlags(f, defaultFormat, map[string]cmd.Formatter{
		"yaml":    cmd.FormatYaml,
		"json":    cmd.FormatJson,
		"short":   FormatOneline,
		"oneline": FormatOneline,
		"line":    FormatOneline,
		"tabular": c.FormatTabular,
		"summary": FormatSummary,
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
	return nil
}

var newAPIClientForStatus = func(c *statusCommand) (statusAPI, error) {
	if c.api != nil {
		return c.api, nil
	}
	return c.NewAPIClient()
}

func (c *statusCommand) getStatus() (*params.FullStatus, error) {
	apiclient, err := newAPIClientForStatus(c)
	if err != nil {
		return nil, errors.Trace(err)
	}
	defer apiclient.Close()

	return apiclient.Status(c.patterns)
}

func (c *statusCommand) Run(ctx *cmd.Context) error {
	// Always attempt to get the status at least once, and retry if it fails.
	status, err := c.getStatus()
	if err != nil {
		for i := 0; i < c.retryCount; i++ {
			// Wait for a bit before retries.
			<-c.clock.After(c.retryDelay)
			status, err = c.getStatus()
			if err == nil {
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

	showRelations := true
	if c.out.Name() != "tabular" {
		if c.relationsFlagProvidedF() {
			// For non-tabular formats this is redundant and needs to be mentioned to the user.
			ctx.Infof("provided --relations option is ignored")
		}
	} else {
		showRelations = c.relations
	}
	formatter := newStatusFormatter(status, controllerName, c.isoTime, showRelations)
	formatted, err := formatter.format()
	if err != nil {
		return errors.Trace(err)
	}
	err = c.out.Write(ctx, formatted)
	if err != nil {
		return err
	}

	if !status.IsEmpty() {
		return nil
	}
	if len(c.patterns) == 0 {
		modelName, err := c.ModelName()
		if err != nil {
			return err
		}
		ctx.Infof("Model %q is empty.", modelName)
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

func (c *statusCommand) FormatTabular(writer io.Writer, value interface{}) error {
	return FormatTabular(writer, c.color, value)
}
