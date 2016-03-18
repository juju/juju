// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package status

import (
	"fmt"
	"os"
	"strconv"

	"launchpad.net/gnuflag"

	"github.com/juju/cmd"
	"github.com/juju/errors"
	"github.com/juju/juju/apiserver/params"
	"github.com/juju/juju/cmd/modelcmd"
	"github.com/juju/juju/juju/osenv"
)

type statusHealthCommand struct {
	modelcmd.ModelCommandBase
	out     cmd.Output
	isoTime bool
}

var statusHealthDoc = `
This command will report the health of the Juju environment
The command outputs a single line summary with any unhealthy units 1 per line on following lines.
The return code of this is 0 for Okay, 1 for warning, 2 for critical, 3 for unknown
`

// NewHealthCommand returns a command that reports the health of the Juju deployment
func NewHealthCommand() cmd.Command {
	return modelcmd.Wrap(&statusHealthCommand{})
}

func (c *statusHealthCommand) Info() *cmd.Info {
	return &cmd.Info{
		Name:    "status-health",
		Purpose: "output the health of the Juju environment",
		Doc:     statusHealthDoc,
		Aliases: []string{"health"},
	}
}

func (c *statusHealthCommand) SetFlags(f *gnuflag.FlagSet) {
	f.BoolVar(&c.isoTime, "utc", false, "display time as UTC in RFC3339 format")
}

func (c *statusHealthCommand) Init(args []string) error {
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

// statusHealthCommand.Run output and return code should conform to this commonly used standard
// https://assets.nagios.com/downloads/nagioscore/docs/nagioscore/3/en/pluginapi.html
func (c *statusHealthCommand) Run(ctx *cmd.Context) error {
	var exitCode int
	var notOkay []string

	// First determine the status
	apiclient, err := c.NewAPIClient()
	if err != nil {
		return fmt.Errorf(connectionError, c.ConnectionName(), err)
	}
	defer apiclient.Close()

	status, err := apiclient.Status([]string{})
	if err != nil {
		// problems getting status report Unknown
		exitCode = 3
		notOkay = append(notOkay, err.Error())
	}

	//Parse status setting exitCode and summary message to the most severe status
	if status != nil {
		sf := NewStatusFormatter(status, c.isoTime)
		formatted := sf.format()
		code, issues := formatted.Health()
		if code > exitCode {
			exitCode = code
		}
		notOkay = append(notOkay, issues...)
	}

	//set summary line based on most sever exitcode
	summary := "Juju Health "
	switch exitCode {
	case 0:
		summary += "Okay"
	case 1:
		summary += "Warning"
	case 2:
		summary += "Critical"
	default:
		summary += "Unknown"
	}

	//Output the statuses which aren't okay and exit
	fmt.Println(summary)
	for _, line := range notOkay {
		fmt.Println(line)
	}
	return cmd.NewRcPassthroughError(exitCode)
}

func (m *machineStatus) Health() (int, []string) {
	var exitCode int
	var notOkay []string
	if m.Err != nil {
		if 2 > exitCode {
			exitCode = 2
		}
		notOkay = append(notOkay, m.Err.Error())
	} else if m.AgentState != params.StatusStarted {
		if 1 > exitCode {
			exitCode = 1
		}
		notOkay = append(notOkay, fmt.Sprintf("Status: %s", m.AgentState))
	}

	for cname, cstatus := range m.Containers {
		ccode, cissues := cstatus.Health()

		if ccode > exitCode {
			exitCode = ccode
		}
		for _, issue := range cissues {
			notOkay = append(notOkay, fmt.Sprintf("Container: %s\t %s", cname, issue))
		}
	}
	return exitCode, notOkay
}

func (s *serviceStatus) Health() (int, []string) {
	var exitCode int
	var notOkay []string

	if s.Err != nil {
		if 2 > exitCode {
			exitCode = 2
		}
		notOkay = append(notOkay, s.Err.Error())
	} else if s.StatusInfo.Current != params.StatusStarted && s.StatusInfo.Current != params.StatusIdle {
		if 1 > exitCode {
			exitCode = 1
		}
		notOkay = append(notOkay, fmt.Sprintf("Status: %s, Error:%s", s.StatusInfo.Current, s.StatusInfo.Err))
	}

	//parse all units
	for uname, ustatus := range s.Units {
		ucode, uissues := ustatus.Health()

		if ucode > exitCode {
			exitCode = ucode
		}
		for _, issue := range uissues {
			notOkay = append(notOkay, fmt.Sprintf("Unit: %s\t %s", uname, issue))
		}
	}

	return exitCode, notOkay
}

func (u *unitStatus) Health() (int, []string) {
	var exitCode int
	var notOkay []string

	if u.AgentStatusInfo.Err != nil {
		if 2 > exitCode {
			exitCode = 2
		}
		notOkay = append(notOkay, u.AgentStatusInfo.Err.Error())
	} else if u.AgentStatusInfo.Current != params.StatusActive {
		if 1 > exitCode {
			exitCode = 1
		}
		notOkay = append(notOkay, fmt.Sprintf("AgentStatus: %s", u.AgentStatusInfo.Current))
		// AgentStatus Info is reliably implemented, WorkloadStatusInfo isn't so skip err and allow unknown
	} else if u.WorkloadStatusInfo.Current != params.StatusActive && u.WorkloadStatusInfo.Current != params.StatusUnknown {
		if 1 > exitCode {
			exitCode = 1
		}
		notOkay = append(notOkay, fmt.Sprintf("WorkloadStatus: %s", u.WorkloadStatusInfo.Current))
	}

	//parse subordinates
	for sname, sstatus := range u.Subordinates {
		scode, sissues := sstatus.Health()

		if scode > exitCode {
			exitCode = scode
		}
		for _, issue := range sissues {
			notOkay = append(notOkay, fmt.Sprintf("Subordinate: %s\t %s", sname, issue))
		}
	}
	return exitCode, notOkay
}

// Health step through the formatted status returning the highest health return code and slice of issues
func (f *formattedStatus) Health() (int, []string) {
	var exitCode int
	var notOkay []string

	//parse machines
	for mname, mstatus := range f.Machines {
		mcode, missues := mstatus.Health()

		if mcode > exitCode {
			exitCode = mcode
		}
		for _, issue := range missues {
			notOkay = append(notOkay, fmt.Sprintf("Machine: %s\t %s", mname, issue))
		}
	}

	//parse services
	for sname, sstatus := range f.Services {
		scode, sissues := sstatus.Health()

		if scode > exitCode {
			exitCode = scode
		}
		for _, issue := range sissues {
			notOkay = append(notOkay, fmt.Sprintf("Service: %s\t %s", sname, issue))
		}
	}

	//parse networks
	for nname, nstatus := range f.Networks {
		if nstatus.Err != nil {
			if 2 > exitCode {
				exitCode = 2
			}
			notOkay = append(notOkay, fmt.Sprintf("Network: %s\tError: %s", nname, nstatus.Err))
		}
	}

	return exitCode, notOkay
}
