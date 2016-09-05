// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package status

import (
	"fmt"
	"io"
	"os"
	"strconv"

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
	return modelcmd.Wrap(&statusCommand{})
}

type statusCommand struct {
	modelcmd.ModelCommandBase
	out      cmd.Output
	patterns []string
	isoTime  bool
	api      statusAPI

	color bool
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

The available output formats are:

- tabular (default): Displays status in a tabular format with a separate table
      for the model, machines, applications, relations (if any) and units.
      Note: in this format, the AZ column refers to the cloud region's
      availability zone.
- {short|line|oneline}: List units and their subordinates. For each unit, the IP
      address and agent status are listed.
- summary: Displays the subnet(s) and port(s) the model utilises. Also displays
      aggregate information about:
      - MACHINES: total #, and # in each state.
      - UNITS: total #, and # in each state.
      - APPLICATIONS: total #, and # exposed of each application.
- yaml: Displays information about the model, machines, applications, and units
      in structured YAML format.
- json: Displays information about the model, machines, applications, and units
      in structured JSON format.

Examples:
    juju show-status
    juju show-status mysql
    juju show-status nova-*

See Also:
    juju show-model
    juju show-status-log
    juju machines
    juju storage
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
	return nil
}

var newApiClientForStatus = func(c *statusCommand) (statusAPI, error) {
	return c.NewAPIClient()
}

func (c *statusCommand) Run(ctx *cmd.Context) error {
	apiclient, err := newApiClientForStatus(c)
	if err != nil {
		return errors.Trace(err)
	}
	defer apiclient.Close()

	status, err := apiclient.Status(c.patterns)
	if err != nil {
		if status == nil {
			// Status call completely failed, there is nothing to report
			return err
		}
		// Display any error, but continue to print status if some was returned
		fmt.Fprintf(ctx.Stderr, "%v\n", err)
	} else if status == nil {
		return errors.Errorf("unable to obtain the current status")
	}

	formatter := newStatusFormatter(status, c.ControllerName(), c.isoTime)
	formatted := formatter.format()
	return c.out.Write(ctx, formatted)
}

func (c *statusCommand) FormatTabular(writer io.Writer, value interface{}) error {
	return FormatTabular(writer, c.color, value)
}
