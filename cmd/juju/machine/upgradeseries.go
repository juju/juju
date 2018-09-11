// Copyright 2018 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package machine

import (
	"fmt"
	"strings"

	"github.com/juju/cmd"
	"github.com/juju/errors"
	"github.com/juju/gnuflag"
	"github.com/juju/os/series"
	"gopkg.in/juju/names.v2"
	"gopkg.in/juju/worker.v1/catacomb"

	"github.com/juju/juju/api"
	"github.com/juju/juju/api/machinemanager"
	jujucmd "github.com/juju/juju/cmd"
	"github.com/juju/juju/cmd/modelcmd"
	"github.com/juju/juju/core/watcher"
)

// Actions
const (
	PrepareCommand  = "prepare"
	CompleteCommand = "complete"
)

var UpgradeSeriesConfirmationMsg = `
WARNING This command will mark machine %q as being upgraded to series %q
This operation cannot be reverted or canceled once started. The units
of machine %q will also be upgraded. These units include:

%s

Continue [y/N]?`[1:]

// NewUpgradeSeriesCommand returns a command which upgrades the series of
// an application or machine.
func NewUpgradeSeriesCommand() cmd.Command {
	return modelcmd.Wrap(&upgradeSeriesCommand{})
}

//go:generate mockgen -package mocks -destination mocks/upgradeMachineSeriesAPI_mock.go github.com/juju/juju/cmd/juju/machine UpgradeMachineSeriesAPI
type UpgradeMachineSeriesAPI interface {
	BestAPIVersion() int
	Close() error
	UpgradeSeriesValidate(string, string) ([]string, error)
	UpgradeSeriesPrepare(string, string, bool) error
	UpgradeSeriesComplete(string) error
	WatchUpgradeSeriesNotifications(string) (watcher.NotifyWatcher, string, error)
	GetUpgradeSeriesNotification(string, string) ([]string, error)
}

// upgradeSeriesCommand is responsible for updating the series of an application or machine.
type upgradeSeriesCommand struct {
	modelcmd.ModelCommandBase
	modelcmd.IAASOnlyCommand

	upgradeMachineSeriesClient UpgradeMachineSeriesAPI

	prepCommand   string
	force         bool
	machineNumber string
	series        string
	agree         bool

	catacomb catacomb.Catacomb
}

var upgradeSeriesDoc = `
Upgrade a machine's operating system series.

upgrade-series allows users to perform a managed upgrade of the operating system
series of a machine. This command is performed in two steps; prepare and complete.

The "prepare" step notifies Juju that a series upgrade is taking place for a given
machine and as such Juju guards that machine against operations that would
interfere with the upgrade process.

The "complete" step notifies juju that the managed upgrade has been successfully completed.

It should be noted that once the prepare command is issued there is no way to
cancel or abort the process. Once you commit to prepare you must complete the
process or you will end up with an unusable machine!

The requested series must be explicitly supported by all charms deployed to
the specified machine. To override this constraint the --force option may be used.

The --force option should be used with caution since using a charm on a machine
running an unsupported series may cause unexpected behavior. Alternately, if the
requested series is supported in later revisions of the charm, upgrade-charm can
run beforehand.

Examples:

Prepare <machine> for upgrade to series <series>:

	juju upgrade-series prepare <machine> <series>

Prepare <machine> for upgrade to series <series> even if there are applications
running units that do not support the target series:

	juju upgrade-series prepare <machine> <series> --force

Complete upgrade of <machine> to <series> indicating that all automatic and any
necessary manual upgrade steps have completed successfully:

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
	// TODO (hml) 2018-06-28
	// agree should be hidden, or available only during initial testing?
	f.BoolVar(&c.agree, "agree", false, "Agree this operation cannot be reverted or canceled once started.")
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
		return errors.Errorf("%q is an invalid machine name", args[1])
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
	if c.prepCommand == PrepareCommand {
		err := c.UpgradeSeriesPrepare(ctx)
		if err != nil {
			return errors.Trace(err)
		}
	}
	if c.prepCommand == CompleteCommand {
		err := c.UpgradeSeriesComplete(ctx)
		if err != nil {
			return errors.Trace(err)
		}
	}
	return nil
}

// UpgradeSeriesPrepare is the interface to the API server endpoint of the same
// name. Since this function's interface will be mocked as an external test
// dependency this function should contain minimal logic other than gathering an
// API handle and making the API call.
func (c *upgradeSeriesCommand) UpgradeSeriesPrepare(ctx *cmd.Context) error {
	var apiRoot api.Connection

	// If the upgradeMachineSeries is nil then we collect a handle to the
	// API. If it is not nil it is likely the client has been set elsewhere
	// (i.e. a test mock) so we don't reset it.
	if c.upgradeMachineSeriesClient == nil {
		var err error
		apiRoot, err = c.NewAPIRoot()
		if err != nil {
			return errors.Trace(err)
		}
		defer apiRoot.Close()
		c.upgradeMachineSeriesClient = machinemanager.NewClient(apiRoot)
	}

	err := c.promptConfirmation(ctx)
	if err != nil {
		return err
	}

	err = c.upgradeMachineSeriesClient.UpgradeSeriesPrepare(c.machineNumber, c.series, c.force)
	if err != nil {
		return errors.Trace(err)
	}

	err = catacomb.Invoke(catacomb.Plan{
		Site: &c.catacomb,
		Work: c.displayNotifications(ctx),
	})
	if err != nil {
		return errors.Trace(err)
	}

	// Here we wait for the loop to finish by waiting for the catacomb's
	// worker to finish.
	err = c.catacomb.Wait()
	fmt.Fprintf(ctx.Stdout, "\n\n")
	if err != nil {
		return errors.Trace(err)
	}

	return nil
}

// displayNotifications handles the writing of upgrade series notifications to
// standard out.
func (c *upgradeSeriesCommand) displayNotifications(ctx *cmd.Context) func() error {
	// We return and anonymous function here to satisfy the catacomb plan's
	// need for a work function and to close over the commands context.
	return func() error {
		uw, wid, err := c.upgradeMachineSeriesClient.WatchUpgradeSeriesNotifications(c.machineNumber)
		if err != nil {
			return errors.Trace(err)
		}
		err = c.catacomb.Add(uw)
		if err != nil {
			return errors.Trace(err)
		}
		for {
			select {
			case <-c.catacomb.Dying():
				return c.catacomb.ErrDying()
			case <-uw.Changes():
				err = c.handleUpgradeSeriesChange(ctx, wid)
				if err != nil {
					return errors.Trace(err)
				}
			}
		}
	}
}

func (c *upgradeSeriesCommand) handleUpgradeSeriesChange(ctx *cmd.Context, wid string) error {
	notifications, err := c.upgradeMachineSeriesClient.GetUpgradeSeriesNotification(c.machineNumber, wid)
	if err != nil {
		return errors.Trace(err)
	}
	if len(notifications) > 0 {
		fmt.Fprint(ctx.Stdout, "\n")
	}
	_, err = fmt.Fprint(ctx.Stdout, strings.Join(notifications, "\n"))
	if err != nil {
		errors.Trace(err)
	}
	return nil
}

// UpgradeSeriesComplete completes a series for a given machine, that is
// if a machine is marked as upgrading using UpgradeSeriesPrepare, then this
// command will complete that process and the machine will no longer be marked
// as upgrading.
func (c *upgradeSeriesCommand) UpgradeSeriesComplete(ctx *cmd.Context) error {
	var apiRoot api.Connection
	var err error

	if c.upgradeMachineSeriesClient == nil {
		apiRoot, err = c.NewAPIRoot()
		if err != nil {
			return errors.Trace(err)
		}
		defer apiRoot.Close()
		c.upgradeMachineSeriesClient = machinemanager.NewClient(apiRoot)
	}

	err = c.upgradeMachineSeriesClient.UpgradeSeriesComplete(c.machineNumber)
	if err != nil {
		return errors.Trace(err)
	}
	err = catacomb.Invoke(catacomb.Plan{
		Site: &c.catacomb,
		Work: c.displayNotifications(ctx),
	})
	if err != nil {
		return errors.Trace(err)
	}

	// Here we wait for the loop to finish by waiting for the catacomb's
	// worker to finish.
	err = c.catacomb.Wait()
	fmt.Fprintf(ctx.Stdout, "\n\n")
	if err != nil {
		return errors.Trace(err)
	}

	return nil
}

func (c *upgradeSeriesCommand) promptConfirmation(ctx *cmd.Context) error {
	if c.agree {
		return nil
	}

	affectedUnits, err := c.upgradeMachineSeriesClient.UpgradeSeriesValidate(c.machineNumber, c.series)
	if err != nil {
		return errors.Trace(err)
	}

	formattedUnitNames := strings.Join(affectedUnits, "\n")
	fmt.Fprintf(ctx.Stdout, UpgradeSeriesConfirmationMsg, c.machineNumber, c.series, c.machineNumber, formattedUnitNames)

	if err := jujucmd.UserConfirmYes(ctx); err != nil {
		return errors.Annotate(err, "upgrade series")
	}
	return nil
}

func checkPrepCommands(prepCommands []string, argCommand string) (string, error) {
	for _, prepCommand := range prepCommands {
		if prepCommand == argCommand {
			return prepCommand, nil
		}
	}

	return "", errors.Errorf("%q is an invalid upgrade-series command; valid commands are: %s.",
		argCommand, strings.Join(prepCommands, ", "))
}

func checkSeries(supportedSeries []string, seriesArgument string) (string, error) {
	for _, s := range supportedSeries {
		if s == strings.ToLower(seriesArgument) {
			return s, nil
		}
	}

	return "", errors.Errorf("%q is an unsupported series", seriesArgument)
}
