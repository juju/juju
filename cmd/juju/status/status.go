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
	"github.com/juju/juju/cmd/envcmd"
	"github.com/juju/juju/juju/osenv"
)

var logger = loggo.GetLogger("juju.cmd.juju.status")

type StatusCommand struct {
	envcmd.EnvCommandBase
	out      cmd.Output
	patterns []string
	isoTime  bool
}

var statusDoc = `
This command will report on the runtime state of various system entities.

There are a number of ways to format the status output:

- {short|line|oneline}: List units and their subordinates. For each
           unit, the IP address and agent status are listed.
- summary: Displays the subnet(s) and port(s) the environment utilises.
           Also displays aggregate information about:
           - MACHINES: total #, and # in each state.
           - UNITS: total #, and # in each state.
           - SERVICES: total #, and # exposed of each service.
- tabular: Displays information in a tabular format in these sections:
           - Machines: ID, STATE, VERSION, DNS, INS-ID, SERIES, HARDWARE
           - Services: NAME, EXPOSED, CHARM
           - Units: ID, STATE, VERSION, MACHINE, PORTS, PUBLIC-ADDRESS
             - Also displays subordinate units.
- yaml (DEFAULT): Displays information on machines, services, and units
                  in the yaml format.

Service or unit names may be specified to filter the status to only those
services and units that match, along with the related machines, services
and units. If a subordinate unit is matched, then its principal unit will
be displayed. If a principal unit is matched, then all of its subordinates
will be displayed.

Wildcards ('*') may be specified in service/unit names to match any sequence
of characters. For example, 'nova-*' will match any service whose name begins
with 'nova-': 'nova-compute', 'nova-volume', etc.
`

func (c *StatusCommand) Info() *cmd.Info {
	return &cmd.Info{
		Name:    "status",
		Args:    "[pattern ...]",
		Purpose: "output status information about an environment",
		Doc:     statusDoc,
		Aliases: []string{"stat"},
	}
}

func (c *StatusCommand) SetFlags(f *gnuflag.FlagSet) {
	f.BoolVar(&c.isoTime, "utc", false, "display time as UTC in RFC3339 format")

	oneLineFormatter := FormatOneline
	defaultFormat := "yaml"
	if c.CompatVersion() > 1 {
		defaultFormat = "tabular"
		oneLineFormatter = FormatOnelineV2
	}

	c.out.AddFlags(f, defaultFormat, map[string]cmd.Formatter{
		"yaml":    cmd.FormatYaml,
		"json":    cmd.FormatJson,
		"short":   oneLineFormatter,
		"oneline": oneLineFormatter,
		"line":    oneLineFormatter,
		"tabular": FormatTabular,
		"summary": FormatSummary,
	})
}

func (c *StatusCommand) Init(args []string) error {
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

var connectionError = `Unable to connect to environment %q.
Please check your credentials or use 'juju bootstrap' to create a new environment.

Error details:
%v
`

type statusAPI interface {
	Status(patterns []string) (*params.FullStatus, error)
	Close() error
}

var newApiClientForStatus = func(c *StatusCommand) (statusAPI, error) {
	return c.NewAPIClient()
}

func (c *StatusCommand) Run(ctx *cmd.Context) error {

	apiclient, err := newApiClientForStatus(c)
	if err != nil {
		return errors.Errorf(connectionError, c.ConnectionName(), err)
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

	formatter := newStatusFormatter(status, c.CompatVersion(), c.isoTime)
	formatted := formatter.format()
	return c.out.Write(ctx, formatted)
}
