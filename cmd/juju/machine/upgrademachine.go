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
	"github.com/juju/names/v5"
	"github.com/juju/worker/v3/catacomb"

	"github.com/juju/juju/api"
	"github.com/juju/juju/api/client/client"
	"github.com/juju/juju/api/client/machinemanager"
	jujucmd "github.com/juju/juju/cmd"
	"github.com/juju/juju/cmd/modelcmd"
	"github.com/juju/juju/core/base"
	corebase "github.com/juju/juju/core/base"
	"github.com/juju/juju/core/model"
	"github.com/juju/juju/core/watcher"
	"github.com/juju/juju/rpc/params"
)

// Actions
const (
	PrepareCommand  = "prepare"
	CompleteCommand = "complete"
)

// For testing.
var SupportedJujuBases = corebase.WorkloadBases

var upgradeMachineConfirmationMsg = `
WARNING: This command will mark machine %q as being upgraded to %q.
This operation cannot be reverted or canceled once started.
%s`[1:]

var upgradeMachineAffectedMsg = `
Units running on the machine will also be upgraded. These units include:
  %s

Leadership for the following applications will be pinned and not
subject to change until the "complete" command is run:
  %s
`[1:]

const UpgradeMachinePrepareFinishedMessage = `
Juju is now ready for the machine base to be updated.
Perform any manual steps required along with "do-release-upgrade".
When ready, run the following to complete the upgrade base process:

juju upgrade-machine %s complete`

const UpgradeMachineCompleteFinishedMessage = `
Upgrade machine base %q has successfully completed`

const UpgradeMachinePrepareOngoingMessage = `
Upgrade machine base is currently being prepared for machine %q.
`

const UpgradeMachinePrepareCompletedMessage = `
Upgrade machine base is already prepared for machine %[1]q and the
current state is %q.

Juju is now ready for the machine base to be updated.
Perform any manual steps required along with "do-release-upgrade".
When ready, run the following to complete the upgrade process:

juju upgrade-machine %[1]s complete`

const UpgradeMachineCompleteOngoingMessage = `
Upgrade base is currently completing for machine %q.
`

// NewUpgradeMachineCommand returns a command which upgrades the base of
// an application or machine.
func NewUpgradeMachineCommand() cmd.Command {
	return modelcmd.Wrap(&upgradeMachineCommand{})
}

type UpgradeMachineAPI interface {
	Close() error
	UpgradeSeriesPrepare(string, string, bool) error
	UpgradeSeriesComplete(string) error
	WatchUpgradeSeriesNotifications(string) (watcher.NotifyWatcher, string, error)
	GetUpgradeSeriesMessages(string, string) ([]string, error)
}

type StatusAPI interface {
	Status(*client.StatusArgs) (*params.FullStatus, error)
}

// upgradeMachineCommand is responsible for updating the base of an application
// or machine.
type upgradeMachineCommand struct {
	modelcmd.ModelCommandBase
	modelcmd.IAASOnlyCommand

	upgradeMachineClient UpgradeMachineAPI
	statusClient         StatusAPI

	subCommand    string
	force         bool
	machineNumber string
	releaseArg    string
	yes           bool

	catacomb catacomb.Catacomb
	plan     catacomb.Plan
}

var upgradeMachineDoc = `
Upgrade a machine's operating system release.

` + "`upgrade-machine `" + `allows users to perform a managed upgrade of the operating system
release of a machine using a base. This command is performed in two steps;
` + "`prepare`" + ` and ` + "`complete`" + `.

The ` + "`prepare`" + ` step notifies Juju that a base upgrade is taking place for a given
machine and as such Juju guards that machine against operations that would
interfere with the upgrade process. A base can be specified using the OS name
and the version of the OS, separated by ` + "`@`" + `.

The ` + "`complete`" + ` step notifies juju that the managed upgrade has been successfully
completed.

It should be noted that once the prepare command is issued there is no way to
cancel or abort the process. Once you commit to prepare you must complete the
process or you will end up with an unusable machine!

The requested base must be explicitly supported by all charms deployed to
the specified machine. To override this constraint the ` + "`--force`" + ` option may be used.

The ` + "`--force`" + ` option should be used with caution since using a charm on a machine
running an unsupported base may cause unexpected behavior. Alternately, if the
requested base is supported in later revisions of the charm, ` + "`upgrade-charm`" + ` can
run beforehand.

`

const upgradeMachineExamples = `
Prepare machine ` + "`3`" + ` for upgrade to base ` + "`ubuntu@18.04`" + `:

	juju upgrade-machine 3 prepare ubuntu@18.04

Prepare machine ` + "`4`" + ` for upgrade to base ` + "`ubuntu@20.04`" + ` even if there are
applications running units that do not support the target base:

	juju upgrade-machine 4 prepare ubuntu@20.04 --force

Complete upgrade of machine ` + "`5`" + `, indicating that all automatic and any
necessary manual upgrade steps have completed successfully:

	juju upgrade-machine 5 complete
`

func (c *upgradeMachineCommand) Info() *cmd.Info {
	return jujucmd.Info(&cmd.Info{
		Name:     "upgrade-machine",
		Args:     "<machine> <command> [args]",
		Purpose:  "Upgrade the Ubuntu base of a machine.",
		Doc:      upgradeMachineDoc,
		Examples: upgradeMachineExamples,
		SeeAlso: []string{
			"machines",
			"status",
			"refresh",
			"set-application-base",
		},
	})
}

func (c *upgradeMachineCommand) SetFlags(f *gnuflag.FlagSet) {
	c.ModelCommandBase.SetFlags(f)
	f.BoolVar(&c.force, "force", false,
		"Upgrade even if the base is not supported by the charm and/or related subordinate charms.")
	f.BoolVar(&c.yes, "y", false,
		"Agree that the operation cannot be reverted or canceled once started without being prompted.")
	f.BoolVar(&c.yes, "yes", false, "")
}

// Init implements cmd.Command.
func (c *upgradeMachineCommand) Init(args []string) error {
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
		c.releaseArg = args[2]
	}

	return nil
}

// Run implements cmd.Run.
func (c *upgradeMachineCommand) Run(ctx *cmd.Context) error {
	if c.subCommand == PrepareCommand {
		err := c.UpgradePrepare(ctx)
		if err != nil {
			return errors.Trace(err)
		}
	}
	if c.subCommand == CompleteCommand {
		err := c.UpgradeComplete(ctx)
		if err != nil {
			return errors.Trace(err)
		}
	}
	return nil
}

func (c *upgradeMachineCommand) parseBase(ctx *cmd.Context, arg string) (corebase.Base, error) {
	// If this doesn't contain an @ then it's a series and not a base.
	var (
		base corebase.Base
		err  error
	)
	if strings.Contains(arg, "@") {
		base, err = corebase.ParseBaseFromString(arg)
	} else {
		ctx.Warningf("series argument is deprecated, use base instead")
		base, err = corebase.GetBaseFromSeries(strings.ToLower(arg))
	}
	if err != nil {
		return corebase.Base{}, errors.Trace(err)
	}

	workloadBases, err := SupportedJujuBases(time.Now(), base, "")
	if err != nil {
		return corebase.Base{}, errors.Trace(err)
	}
	err = checkBase(workloadBases, base)
	if err != nil {
		return corebase.Base{}, err
	}
	return base, nil
}

func (c *upgradeMachineCommand) trapInterrupt(ctx *cmd.Context) func() {
	// Handle Ctrl-C during upgrade machine.
	interrupted := make(chan os.Signal, 1)
	ctx.InterruptNotify(interrupted)

	cancelCtx, cancel := context.WithCancel(context.TODO())
	go func() {
		for range interrupted {
			select {
			case <-cancelCtx.Done():
				// Ctrl-C already pressed
				_, _ = fmt.Fprintln(ctx.Stdout, "\nCtrl-C pressed, cancelling an upgrade-machine.")
				os.Exit(1)
				return
			default:
				// Newline prefix is intentional, so output appears as
				// "^C\nCtrl-C pressed" instead of "^CCtrl-C pressed".
				_, _ = fmt.Fprintln(ctx.Stdout, "\nCtrl-C pressed, cancelling an upgrade-machine is not recommended. Ctrl-C to proceed anyway.")
				cancel()
			}
		}
	}()

	return func() {
		close(interrupted)
		ctx.StopInterruptNotify(interrupted)
	}
}

// UpgradePrepare is the interface to the API server endpoint of the same
// name. Since this function's interface will be mocked as an external test
// dependency this function should contain minimal logic other than gathering an
// API handle and making the API call.
func (c *upgradeMachineCommand) UpgradePrepare(ctx *cmd.Context) (err error) {
	base, err := c.parseBase(ctx, c.releaseArg)
	if err != nil {
		return errors.Trace(err)
	}

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

	if err := c.promptConfirmation(ctx, base, units); err != nil {
		return errors.Trace(err)
	}

	close := c.trapInterrupt(ctx)
	defer close()

	if err = c.upgradeMachineClient.UpgradeSeriesPrepare(c.machineNumber, base.Channel.String(), c.force); err != nil {
		return c.displayProgressFromError(ctx, err)
	}

	if err = c.handleNotifications(ctx); err != nil {
		return errors.Trace(err)
	}

	m := UpgradeMachinePrepareFinishedMessage + "\n"
	ctx.Infof(m, c.machineNumber)

	return nil
}

func (c *upgradeMachineCommand) retrieveUnits() ([]string, error) {
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
func (c *upgradeMachineCommand) displayProgressFromError(ctx *cmd.Context, err error) error {
	errResp, ok := errors.Cause(err).(*params.Error)
	if !ok {
		return errors.Trace(err)
	}
	var info params.UpgradeSeriesValidationErrorInfo
	if unmarshalErr := errResp.UnmarshalInfo(&info); unmarshalErr == nil && info.Status != "" {
		// Lift the raw status into a upgrade machine status type. Then perform
		// a switch on the value.
		// If the status is in a terminal state (not started, completed, error),
		// then we shouldn't show a helpful message. Instead we should the
		// underlying error.
		switch model.UpgradeSeriesStatus(info.Status) {
		case model.UpgradeSeriesPrepareStarted,
			model.UpgradeSeriesPrepareRunning:
			return errors.Errorf(UpgradeMachinePrepareOngoingMessage[1:]+"\n", c.machineNumber)

		case model.UpgradeSeriesPrepareCompleted:
			return errors.Errorf(UpgradeMachinePrepareCompletedMessage[1:]+"\n", c.machineNumber, info.Status)

		case model.UpgradeSeriesCompleteStarted,
			model.UpgradeSeriesCompleteRunning:
			return errors.Errorf(UpgradeMachineCompleteOngoingMessage[1:]+"\n", c.machineNumber)
		}
	}
	return errors.Trace(err)
}

func (c *upgradeMachineCommand) promptConfirmation(ctx *cmd.Context, base corebase.Base, affectedUnits []string) error {
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
			upgradeMachineAffectedMsg, strings.Join(units.SortedValues(), "\n  "), strings.Join(apps.SortedValues(), "\n  "))
	}

	fmt.Fprintf(ctx.Stdout, upgradeMachineConfirmationMsg, c.machineNumber, base.DisplayString(), affectedMsg)
	if err := jujucmd.UserConfirmYes(ctx); err != nil {
		return errors.Annotate(err, "upgrade machine")
	}
	return nil
}

func (c *upgradeMachineCommand) handleNotifications(ctx *cmd.Context) error {
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
			logger.Debugf("the upgrade machine watcher has been stopped")
		} else {
			return errors.Trace(err)
		}
	}
	return nil
}

// displayNotifications handles the writing of upgrade machine notifications to
// standard out.
func (c *upgradeMachineCommand) displayNotifications(ctx *cmd.Context) func() error {
	// We return and anonymous function here to satisfy the catacomb plan's
	// need for a work function and to close over the commands context.
	return func() error {
		uw, wid, err := c.upgradeMachineClient.WatchUpgradeSeriesNotifications(c.machineNumber)
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
				err = c.handleUpgradeChange(ctx, wid)
				if err != nil {
					return errors.Trace(err)
				}
			}
		}
	}
}

func (c *upgradeMachineCommand) handleUpgradeChange(ctx *cmd.Context, wid string) error {
	messages, err := c.upgradeMachineClient.GetUpgradeSeriesMessages(c.machineNumber, wid)
	if err != nil {
		return errors.Trace(err)
	}
	if len(messages) == 0 {
		return nil
	}
	ctx.Infof("%s", strings.Join(messages, "\n"))
	return nil
}

// UpgradeComplete completes a base upgrade for a given machine,
// that is if a machine is marked as upgrading using UpgradePrepare,
// then this command will complete that process and the machine will no longer
// be marked as upgrading.
func (c *upgradeMachineCommand) UpgradeComplete(ctx *cmd.Context) error {
	apiRoot, err := c.ensureAPIClient()
	if err != nil {
		return errors.Trace(err)
	}
	if apiRoot != nil {
		defer apiRoot.Close()
	}

	close := c.trapInterrupt(ctx)
	defer close()

	if err := c.upgradeMachineClient.UpgradeSeriesComplete(c.machineNumber); err != nil {
		return errors.Trace(err)
	}

	if err := c.handleNotifications(ctx); err != nil {
		return errors.Trace(err)
	}

	m := UpgradeMachineCompleteFinishedMessage + "\n"
	ctx.Infof(m, c.machineNumber)

	return nil
}

// ensureAPIClient checks to see if the API client is already instantiated.
// If not, a new api Connection is created and used to instantiate it.
// If it has been set elsewhere (such as by a test) we leave it as is.
func (c *upgradeMachineCommand) ensureAPIClient() (api.Connection, error) {
	var apiRoot api.Connection
	if c.upgradeMachineClient == nil {
		var err error
		apiRoot, err = c.NewAPIRoot()
		if err != nil {
			return nil, errors.Trace(err)
		}
		c.upgradeMachineClient = machinemanager.NewClient(apiRoot)
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

	return "", errors.Errorf("%q is an invalid upgrade-machine command; valid commands are: %s.",
		argCommand, strings.Join(validCommands, ", "))
}

func checkBase(supportedBases []corebase.Base, baseArgument base.Base) error {
	for _, b := range supportedBases {
		if baseArgument.IsCompatible(b) {
			return nil
		}
	}

	return errors.Errorf("%q is an unsupported base", baseArgument)
}
