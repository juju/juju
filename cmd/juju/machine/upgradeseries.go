// Copyright 2018 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package machine

import (
	"strings"

	"github.com/juju/cmd"
	"github.com/juju/errors"
	"github.com/juju/gnuflag"
	"github.com/juju/os/series"
	"gopkg.in/juju/names.v2"

	"github.com/juju/juju/cmd/modelcmd"
)

// Actions
const (
	PrepareCommand  = "prepare"
	CompleteCommand = "complete"
)

// NewUpgradeSeriesCommand returns a command which upgrades the series of
// an application or machine.
func NewUpgradeSeriesCommand() cmd.Command {
	return modelcmd.Wrap(&upgradeSeriesCommand{})
}

type upgradeMachineSeriesAPI interface {
}

// upgradeSeriesCommand is responsible for updating the series of an application or machine.
type upgradeSeriesCommand struct {
	modelcmd.ModelCommandBase
	// modelcmd.IAASOnlyCommand

	// upgradeMachineSeriesClient upgradeMachineSeriesAPI

	prepCommand   string
	force         bool
	machineNumber string
	series        string
}

var upgradeSeriesDoc = `
Upgrade a machine's operating system series.

When a machine's series, the --force option should be used
with caution since using a charm on a machine running an unsupported series may
cause unexpected behavior. Alternately, if the requested series supported in
later revisions of the charm, upgrade-charm can run beforehand.

Examples:
	juju upgrade-series prepare <machine> <series>
	juju upgrade-series complete <machine>

See also:
    machines
    status
    upgrade-charm
    set-series
`

func (c *upgradeSeriesCommand) Info() *cmd.Info {
	return &cmd.Info{
		Name:    "upgrade-series",
		Args:    "<action> [args]",
		Purpose: "Upgrade a machine's series.",
		Doc:     upgradeSeriesDoc,
	}
}

func (c *upgradeSeriesCommand) SetFlags(f *gnuflag.FlagSet) {
	c.ModelCommandBase.SetFlags(f)
	f.BoolVar(&c.force, "force", false, "Upgrade even if the series is not supported by the charm and/or related subordinate charms.")
	f.BoolVar(&c.force, "agree", false, "Agree to upgrade the series. Otherwise, a confirmation prompt will be displayed after the command is run.")
}

// Init implements cmd.Command.
func (c *upgradeSeriesCommand) Init(args []string) error {
	numArguments := 3

	if len(args) < 1 {
		return errors.Errorf("wrong number of arguments")
	}

	prepCommandStrings := []string{PrepareCommand, CompleteCommand}
	prepCommand, err := checkPrepCommands(prepCommandStrings, args[0])
	if err != nil {
		return errors.Annotate(err, "invalid argument")
	}
	c.prepCommand = prepCommand

	if c.prepCommand == CompleteCommand {
		numArguments = 2
	}

	if len(args) != numArguments {
		return errors.Errorf("wrong number of arguments")
	}

	if names.IsValidMachine(args[1]) {
		c.machineNumber = args[1]
	} else {
		return errors.Errorf("invalid machine name %q", args[1])
	}

	if c.prepCommand == PrepareCommand {
		series, err := checkSeries(series.SupportedSeries(), args[2])
		if err != nil {
			return err
		}
		c.series = series
	}

	return nil
}

// Run implements cmd.Run.
func (c *upgradeSeriesCommand) Run(ctx *cmd.Context) error {
	return nil
}

func checkPrepCommands(prepCommands []string, argCommand string) (string, error) {
	for _, prepCommand := range prepCommands {
		if prepCommand == argCommand {
			return prepCommand, nil
		}
	}

	return "", errors.New("%q is an invalid upgrade-series command")
}

func checkSeries(supportedSeries []string, seriesArgument string) (string, error) {
	for _, series := range supportedSeries {
		if series == strings.ToLower(seriesArgument) {
			return series, nil
		}
	}

	return "", errors.Errorf("%q is an unsupported series", seriesArgument)
}
