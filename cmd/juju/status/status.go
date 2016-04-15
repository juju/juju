// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package status

import (
	"fmt"
	"os"
	"strconv"

	"github.com/juju/cmd"
	"github.com/juju/errors"
	"github.com/juju/loggo"
	"launchpad.net/gnuflag"

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
}

var usageSummary = `
Displays the current status of Juju, services, and units.`[1:]

var usageDetails = `
By default (without argument), the status of Juju and all services and all
units will be displayed. 
Service or unit names may be used as output filters (the '*' can be used
as a wildcard character).  
In addition to matched services and units, related machines, services, and
units will also be displayed. If a subordinate unit is matched, then its
principal unit will be displayed. If a principal unit is matched, then all
of its subordinates will be displayed. 
Explanation of the different formats:
- {short|line|oneline}: List units and their subordinates. For each
           unit, the IP address and agent status are listed.
- summary: Displays the subnet(s) and port(s) the model utilises.
           Also displays aggregate information about:
           - MACHINES: total #, and # in each state.
           - UNITS: total #, and # in each state.
           - SERVICES: total #, and # exposed of each service.
- tabular (default): Displays information in a tabular format in these sections:
           - Machines: ID, STATE, DNS, INS-ID, SERIES, AZ
           - Services: NAME, EXPOSED, CHARM
           - Units: ID, STATE, VERSION, MACHINE, PORTS, PUBLIC-ADDRESS
             - Also displays subordinate units.
- yaml: Displays information on machines, services, and units in yaml format.
Note: AZ above is the cloud region's availability zone.

Examples:
    juju status
    juju status mysql
    juju status nova-*
`

func (c *statusCommand) Info() *cmd.Info {
	return &cmd.Info{
		Name:    "status",
		Args:    "[filter pattern ...]",
		Purpose: usageSummary,
		Doc:     usageDetails,
		Aliases: []string{"show-status"},
	}
}

func (c *statusCommand) SetFlags(f *gnuflag.FlagSet) {
	f.BoolVar(&c.isoTime, "utc", false, "Display time as UTC in RFC3339 format")

	defaultFormat := "tabular"

	c.out.AddFlags(f, defaultFormat, map[string]cmd.Formatter{
		"yaml":    cmd.FormatYaml,
		"json":    cmd.FormatJson,
		"short":   FormatOneline,
		"oneline": FormatOneline,
		"line":    FormatOneline,
		"tabular": FormatTabular,
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

	formatter := NewStatusFormatter(status, c.isoTime)
	formatted := formatter.format()
	return c.out.Write(ctx, formatted)
}
