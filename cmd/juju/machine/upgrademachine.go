// Copyright 2018 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package machine

import (
	"context"
	"fmt"
	"os"
	"sort"
	"strings"
	"time"

	"github.com/juju/cmd/v3"
	"github.com/juju/collections/set"
	"github.com/juju/errors"
	"github.com/juju/gnuflag"
	"github.com/juju/names/v4"
	"github.com/juju/worker/v3/catacomb"

	"github.com/juju/juju/api"
	"github.com/juju/juju/api/client/machinemanager"
	jujucmd "github.com/juju/juju/cmd"
	"github.com/juju/juju/cmd/modelcmd"
	"github.com/juju/juju/core/model"
	"github.com/juju/juju/core/series"
	"github.com/juju/juju/core/watcher"
	"github.com/juju/juju/rpc/params"
)

// Actions
const (
	PrepareCommand  = "prepare"
	CompleteCommand = "complete"
)

var upgradeSeriesConfirmationMsg = `
WARNING: This command will mark machine %q as being upgraded to series %q.
This operation cannot be reverted or canceled once started.
%s
Continue [y/N]?`[1:]

var upgradeSeriesAffectedMsg = `
Units running on the machine will also be upgraded. These units include:
  %s

Leadership for the following applications will be pinned and not
subject to change until the "complete" command is run:
  %s
`[1:]

const UpgradeSeriesPrepareFinishedMessage = `
Juju is now ready for the series to be updated.
Perform any manual steps required along with "do-release-upgrade".
When ready, run the following to complete the upgrade series process:

juju upgrade-series %s complete`

const UpgradeSeriesCompleteFinishedMessage = `
Upgrade series for machine %q has successfully completed`

const UpgradeSeriesPrepareOngoingMessage = `
Upgrade series is currently being prepared for machine %q.
`

const UpgradeSeriesPrepareCompletedMessage = `
Upgrade series is already prepared for machine %[1]q and the current
state is %q.

Juju is now ready for the series to be updated.
Perform any manual steps required along with "do-release-upgrade".
When ready, run the following to complete the upgrade series process:

juju upgrade-series %[1]s complete`

const UpgradeSeriesCompleteOngoingMessage = `
Upgrade series is currently completing for machine %q.
`

// NewUpgradeSeriesCommand returns a command which upgrades the series of
// an application or machine.
func NewUpgradeSeriesCommand() cmd.Command {
	return modelcmd.Wrap(&upgradeSeriesCommand{})
}

type UpgradeMachineSeriesAPI interface {
	BestAPIVersion() int
	Close() error
	UpgradeSeriesPrepare(string, string, bool) error
	UpgradeSeriesComplete(string) error
	WatchUpgradeSeriesNotifications(string) (watcher.NotifyWatcher, string, error)
	GetUpgradeSeriesMessages(string, string) ([]string, error)
}

type StatusAPI interface {
	Status(pattern []string) (*params.FullStatus, error)
}

// upgradeSeriesCommand is responsible for updating the series of an application or machine.
type upgradeSeriesCommand struct {
	modelcmd.ModelCommandBase
	modelcmd.IAASOnlyCommand

	upgradeMachineSeriesClient UpgradeMachineSeriesAPI
	statusClient               StatusAPI

	subCommand    string
	force         bool
	machineNumber string
	series        string
	yes           bool

	catacomb catacomb.Catacomb
	plan     catacomb.Plan
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

Prepare machine 3 for upgrade to series "bionic"":

	juju upgrade-series 3 prepare bionic

Prepare machine 4 for upgrade to series "focal" even if there are applications
running units that do not support the target series:

	juju upgrade-series 4 prepare focal --force

Complete upgrade of machine 5, indicating that all automatic and any
necessary manual upgrade steps have completed successfully:

	juju upgrade-series 5 complete

See also:
    machines
    status
    upgrade-charm
    set-series
`

func (c *upgradeSeriesCommand) Info() *cmd.Info {
	return jujucmd.Info(&cmd.Info{
		Name:    "upgrade-series",
		Aliases: []string{"upgrade-machine"},
		Args:    "<machine> <command> [args]",
		Purpose: "Upgrade the Ubuntu series of a machine.",
		Doc:     upgradeSeriesDoc,
	})
}

func (c *upgradeSeriesCommand) SetFlags(f *gnuflag.FlagSet) {
	c.ModelCommandBase.SetFlags(f)
	f.BoolVar(&c.force, "force", false,
		"Upgrade even if the series is not supported by the charm and/or related subordinate charms.")
	f.BoolVar(&c.yes, "y", false,
		"Agree that the operation cannot be reverted or canceled once started without being prompted.")
	f.BoolVar(&c.yes, "yes", false, "")
}

// Init implements cmd.Command.
func (c *upgradeSeriesCommand) Init(args []string) error {
	if len(args) < 2 {
		return errors.Errorf("wrong number of arguments")
	}

	subCommand, err := checkSubCommands([]string{PrepareCommand, CompleteCommand}, args[1])
	if err != nil {
		return errors.Annotate(err, "invalid argument")
	}
	c.subCommand = subCommand

	numArguments := 3
	if c.subCommand == CompleteCommand {
		numArguments = 2
	}
	if len(args) != numArguments {
		return errors.Errorf("wrong number of arguments")
	}

	if names.IsValidMachine(args[0]) {
		c.machineNumber = args[0]
	} else {
		return errors.Errorf("%q is an invalid machine name", args[0])
	}

	if c.subCommand == PrepareCommand {
		seriesArg := args[2]
		workloadSeries, err := series.WorkloadSeries(time.Now(), seriesArg, "")
		if err != nil {
			return errors.Trace(err)
		}
		s, err := checkSeries(workloadSeries.Values(), seriesArg)
		if err != nil {
			return err
		}
		c.series = s
	}

	return nil
}

// Run implements cmd.Run.
func (c *upgradeSeriesCommand) Run(ctx *cmd.Context) error {
	if c.subCommand == PrepareCommand {
		err := c.UpgradeSeriesPrepare(ctx)
		if err != nil {
			return errors.Trace(err)
		}
	}
	if c.subCommand == CompleteCommand {
		err := c.UpgradeSeriesComplete(ctx)
		if err != nil {
			return errors.Trace(err)
		}
	}
	return nil
}

func (c *upgradeSeriesCommand) trapInterrupt(ctx *cmd.Context) func() {
	// Handle Ctrl-C during upgrade series.
	interrupted := make(chan os.Signal, 1)
	ctx.InterruptNotify(interrupted)

	cancelCtx, cancel := context.WithCancel(context.TODO())
	go func() {
		for range interrupted {
			select {
			case <-cancelCtx.Done():
				// Ctrl-C already pressed
				_, _ = fmt.Fprintln(ctx.Stdout, "\nCtrl-C pressed, cancelling an upgrade-series.")
				os.Exit(1)
				return
			default:
				// Newline prefix is intentional, so output appears as
				// "^C\nCtrl-C pressed" instead of "^CCtrl-C pressed".
				_, _ = fmt.Fprintln(ctx.Stdout, "\nCtrl-C pressed, cancelling an upgrade-series is not recommended. Ctrl-C to proceed anyway.")
				cancel()
			}
		}
	}()

	return func() {
		close(interrupted)
		ctx.StopInterruptNotify(interrupted)
	}
}

// UpgradeSeriesPrepare is the interface to the API server endpoint of the same
// name. Since this function's interface will be mocked as an external test
// dependency this function should contain minimal logic other than gathering an
// API handle and making the API call.
func (c *upgradeSeriesCommand) UpgradeSeriesPrepare(ctx *cmd.Context) (err error) {
	apiRoot, err := c.ensureAPIClient()
	if err != nil {
		return errors.Trace(err)
	}
	if apiRoot != nil {
		defer apiRoot.Close()
	}

	units, err := c.retrieveUnits()
	if err != nil {
		return errors.Trace(err)
	} else if len(units) == 0 {
		return errors.NotFoundf("units for machine %q", c.machineNumber)
	}

	if err := c.promptConfirmation(ctx, units); err != nil {
		return errors.Trace(err)
	}

	close := c.trapInterrupt(ctx)
	defer close()

	if err = c.upgradeMachineSeriesClient.UpgradeSeriesPrepare(c.machineNumber, c.series, c.force); err != nil {
		return c.displayProgressFromError(ctx, err)
	}

	if err = c.handleNotifications(ctx); err != nil {
		return errors.Trace(err)
	}

	m := UpgradeSeriesPrepareFinishedMessage + "\n"
	ctx.Infof(m, c.machineNumber)

	return nil
}

func (c *upgradeSeriesCommand) retrieveUnits() ([]string, error) {
	// get the units for a given machine.
	fullStatus, err := c.statusClient.Status(nil)
	if err != nil {
		return nil, errors.Trace(err)
	}

	machineID, err := getMachineID(fullStatus, c.machineNumber)
	if err != nil {
		return nil, errors.Annotatef(err, "unable to locate instance")
	}
	var units []string
	for _, application := range fullStatus.Applications {
		for name, unit := range application.Units {
			if unit.Machine != machineID {
				continue
			}
			units = append(units, name)
			for subName, subordinate := range unit.Subordinates {
				if subordinate.Machine != "" && subordinate.Machine != machineID {
					return nil, errors.Errorf("subordinate %q machine has unexpected machine id %s", subName, machineID)
				}
				units = append(units, subName)
			}
		}
	}

	sort.Strings(units)

	return units, nil
}

func getMachineID(fullStatus *params.FullStatus, id string) (string, error) {
	if machine, ok := fullStatus.Machines[id]; ok {
		return machine.Id, nil
	}
	for _, machine := range fullStatus.Machines {
		if container, ok := machine.Containers[id]; ok {
			return container.Id, nil
		}
	}
	return "", errors.NotFoundf("instance %q", id)
}

// Display any progress information from the error. If there isn't any info
// or the info was malformed, we will fall back to the underlying error
// and not provide any hints.
func (c *upgradeSeriesCommand) displayProgressFromError(ctx *cmd.Context, err error) error {
	if errors.IsNotSupported(err) {
		return errors.Wrap(err, errors.Errorf(`upgrade-series is not supported.
Please upgrade your controller to perform the operation.`))
	}

	errResp, ok := errors.Cause(err).(*params.Error)
	if !ok {
		return errors.Trace(err)
	}
	var info params.UpgradeSeriesValidationErrorInfo
	if unmarshalErr := errResp.UnmarshalInfo(&info); unmarshalErr == nil && info.Status != "" {
		// Lift the raw status into a upgrade series status type. Then perform
		// a switch on the value.
		// If the status is in a terminal state (not started, completed, error),
		// then we shouldn't show a helpful message. Instead we should the
		// underlying error.
		switch model.UpgradeSeriesStatus(info.Status) {
		case model.UpgradeSeriesPrepareStarted,
			model.UpgradeSeriesPrepareRunning:
			return errors.Errorf(UpgradeSeriesPrepareOngoingMessage[1:]+"\n", c.machineNumber)

		case model.UpgradeSeriesPrepareCompleted:
			return errors.Errorf(UpgradeSeriesPrepareCompletedMessage[1:]+"\n", c.machineNumber, info.Status)

		case model.UpgradeSeriesCompleteStarted,
			model.UpgradeSeriesCompleteRunning:
			return errors.Errorf(UpgradeSeriesCompleteOngoingMessage[1:]+"\n", c.machineNumber)
		}
	}
	return errors.Trace(err)
}

func (c *upgradeSeriesCommand) promptConfirmation(ctx *cmd.Context, affectedUnits []string) error {
	if c.yes {
		return nil
	}

	affectedMsg := ""
	if len(affectedUnits) > 0 {
		apps := set.NewStrings()
		units := set.NewStrings()
		for _, unit := range affectedUnits {
			app, err := names.UnitApplication(unit)
			if err != nil {
				return errors.Annotatef(err, "deriving application for unit %q", unit)
			}
			apps.Add(fmt.Sprintf("- %s", app))
			units.Add(fmt.Sprintf("- %s", unit))
		}

		affectedMsg = fmt.Sprintf(
			upgradeSeriesAffectedMsg, strings.Join(units.SortedValues(), "\n  "), strings.Join(apps.SortedValues(), "\n  "))
	}

	fmt.Fprintf(ctx.Stdout, upgradeSeriesConfirmationMsg, c.machineNumber, c.series, affectedMsg)
	if err := jujucmd.UserConfirmYes(ctx); err != nil {
		return errors.Annotate(err, "upgrade series")
	}
	return nil
}

func (c *upgradeSeriesCommand) handleNotifications(ctx *cmd.Context) error {
	if c.plan.Work == nil {
		c.plan = catacomb.Plan{
			Site: &c.catacomb,
			Work: c.displayNotifications(ctx),
		}
	}
	err := catacomb.Invoke(c.plan)
	if err != nil {
		return errors.Trace(err)
	}
	err = c.catacomb.Wait()
	if err != nil {
		if params.IsCodeStopped(err) {
			logger.Debugf("the upgrade series watcher has been stopped")
		} else {
			return errors.Trace(err)
		}
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
	messages, err := c.upgradeMachineSeriesClient.GetUpgradeSeriesMessages(c.machineNumber, wid)
	if err != nil {
		return errors.Trace(err)
	}
	if len(messages) == 0 {
		return nil
	}
	ctx.Infof("%s", strings.Join(messages, "\n"))
	return nil
}

// UpgradeSeriesComplete completes a series upgrade for a given machine,
// that is if a machine is marked as upgrading using UpgradeSeriesPrepare,
// then this command will complete that process and the machine will no longer
// be marked as upgrading.
func (c *upgradeSeriesCommand) UpgradeSeriesComplete(ctx *cmd.Context) error {
	apiRoot, err := c.ensureAPIClient()
	if err != nil {
		return errors.Trace(err)
	}
	if apiRoot != nil {
		defer apiRoot.Close()
	}

	close := c.trapInterrupt(ctx)
	defer close()

	if err := c.upgradeMachineSeriesClient.UpgradeSeriesComplete(c.machineNumber); err != nil {
		return errors.Trace(err)
	}

	if err := c.handleNotifications(ctx); err != nil {
		return errors.Trace(err)
	}

	m := UpgradeSeriesCompleteFinishedMessage + "\n"
	ctx.Infof(m, c.machineNumber)

	return nil
}

// ensureAPIClient checks to see if the API client is already instantiated.
// If not, a new api Connection is created and used to instantiate it.
// If it has been set elsewhere (such as by a test) we leave it as is.
func (c *upgradeSeriesCommand) ensureAPIClient() (api.Connection, error) {
	var apiRoot api.Connection
	if c.upgradeMachineSeriesClient == nil {
		var err error
		apiRoot, err = c.NewAPIRoot()
		if err != nil {
			return nil, errors.Trace(err)
		}
		c.upgradeMachineSeriesClient = machinemanager.NewClient(apiRoot)
	}
	if c.statusClient == nil {
		var err error
		c.statusClient, err = c.NewAPIClient()
		if err != nil {
			return nil, errors.Trace(err)
		}
	}
	return apiRoot, nil
}

func checkSubCommands(validCommands []string, argCommand string) (string, error) {
	for _, subCommand := range validCommands {
		if subCommand == argCommand {
			return subCommand, nil
		}
	}

	return "", errors.Errorf("%q is an invalid upgrade-series command; valid commands are: %s.",
		argCommand, strings.Join(validCommands, ", "))
}

func checkSeries(supportedSeries []string, seriesArgument string) (string, error) {
	for _, s := range supportedSeries {
		if s == strings.ToLower(seriesArgument) {
			return s, nil
		}
	}

	return "", errors.Errorf("%q is an unsupported series", seriesArgument)
}
