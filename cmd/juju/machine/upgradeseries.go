// Copyright 2018 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package machine

import (
	"fmt"
	"sort"
	"strings"

	"github.com/juju/cmd"
	"github.com/juju/errors"
	"github.com/juju/gnuflag"
	"github.com/juju/os/series"
	"github.com/juju/utils/set"
	"gopkg.in/juju/names.v2"
	"gopkg.in/juju/worker.v1/catacomb"

	"github.com/juju/juju/api"
	"github.com/juju/juju/api/facades/client/leadership"
	"github.com/juju/juju/api/machinemanager"
	"github.com/juju/juju/apiserver/params"
	jujucmd "github.com/juju/juju/cmd"
	"github.com/juju/juju/cmd/modelcmd"
	coreleadership "github.com/juju/juju/core/leadership"
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

const UpgradeSeriesPrepareFinishedMessage = `
Juju is now ready for the series to be updated.
Perform any manual steps required along with "do-release-upgrade".
When ready, run the following to complete the upgrade series process:

juju upgrade-series complete %s`

const UpgradeSeriesCompleteFinishedMessage = `
Upgrade series for machine %q has successfully completed`

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
	GetUpgradeSeriesMessages(string, string) ([]string, error)
	Applications(string) ([]string, error)
}

// upgradeSeriesCommand is responsible for updating the series of an application or machine.
type upgradeSeriesCommand struct {
	modelcmd.ModelCommandBase
	modelcmd.IAASOnlyCommand

	upgradeMachineSeriesClient UpgradeMachineSeriesAPI
	leadershipClient           coreleadership.Pinner

	prepCommand   string
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
		Args:    "<command> [args]",
		Purpose: "Upgrade a machine's series.",
		Doc:     upgradeSeriesDoc,
	}
}

func (c *upgradeSeriesCommand) SetFlags(f *gnuflag.FlagSet) {
	c.ModelCommandBase.SetFlags(f)
	f.BoolVar(&c.force, "force", false, "Upgrade even if the series is not supported by the charm and/or related subordinate charms.")
	f.BoolVar(&c.yes, "y", false, "Agree that the operation cannot be reverted or canceled once started without being prompted.")
	f.BoolVar(&c.yes, "yes", false, "")
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
		s, err := checkSeries(series.SupportedSeries(), args[2])
		if err != nil {
			return err
		}
		c.series = s
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
func (c *upgradeSeriesCommand) UpgradeSeriesPrepare(ctx *cmd.Context) (err error) {
	apiRoot, err := c.ensureAPIClients()
	if err != nil {
		return errors.Trace(err)
	}
	if apiRoot != nil {
		defer apiRoot.Close()
	}

	units, err := c.upgradeMachineSeriesClient.UpgradeSeriesValidate(c.machineNumber, c.series)
	if err != nil {
		return errors.Trace(err)
	}
	if err := c.promptConfirmation(ctx, units); err != nil {
		return errors.Trace(err)
	}

	// Any failure during or after pinning leadership, causes applications
	// with units on the machine to be unpinned *except* for when an
	// upgrade-series lock already exists for the machine. This indicates that
	// the prepare command is being run multiple times prior to the completion
	// step, and we don't want to unpin applications for machines still in the
	// upgrade workflow.
	// Note that pinning and unpinning are idempotent.
	defer func() {
		if err != nil && !errors.IsAlreadyExists(err) {
			if unpinErr := c.unpinLeaders(ctx); unpinErr != nil {
				err = errors.Wrap(err, unpinErr)
			}
		}
	}()
	if err = c.pinLeaders(ctx, units); err != nil {
		return errors.Trace(err)
	}

	if err = c.upgradeMachineSeriesClient.UpgradeSeriesPrepare(c.machineNumber, c.series, c.force); err != nil {
		return errors.Trace(err)
	}

	if err = c.handleNotifications(ctx); err != nil {
		return errors.Trace(err)
	}

	m := UpgradeSeriesPrepareFinishedMessage + "\n"
	ctx.Infof(m, c.machineNumber)

	return nil
}

func (c *upgradeSeriesCommand) promptConfirmation(ctx *cmd.Context, affectedUnits []string) error {
	formattedUnitNames := strings.Join(affectedUnits, "\n")
	if c.yes {
		return nil
	}

	fmt.Fprintf(ctx.Stdout, UpgradeSeriesConfirmationMsg, c.machineNumber, c.series, c.machineNumber, formattedUnitNames)
	if err := jujucmd.UserConfirmYes(ctx); err != nil {
		return errors.Annotate(err, "upgrade series")
	}
	return nil
}

// pinLeaders extracts the unique list of applications from the input unit IDs
// and pins the leadership for them.
func (c *upgradeSeriesCommand) pinLeaders(ctx *cmd.Context, units []string) error {
	applications := set.NewStrings()
	for _, unit := range units {
		app, err := names.UnitApplication(unit)
		if err != nil {
			return errors.Annotatef(err, "deriving application for unit %q", unit)
		}
		applications.Add(app)
	}

	for _, app := range applications.SortedValues() {
		if err := c.leadershipClient.PinLeadership(app, names.NewMachineTag(c.machineNumber)); err != nil {
			return errors.Annotatef(err, "freezing leadership for %q", app)
		}
		ctx.Infof("leadership pinned for application %q", app)
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
	ctx.Infof(strings.Join(messages, "\n"))
	return nil
}

// UpgradeSeriesComplete completes a series upgrade for a given machine,
// that is if a machine is marked as upgrading using UpgradeSeriesPrepare,
// then this command will complete that process and the machine will no longer
// be marked as upgrading.
func (c *upgradeSeriesCommand) UpgradeSeriesComplete(ctx *cmd.Context) error {
	apiRoot, err := c.ensureAPIClients()
	if err != nil {
		return errors.Trace(err)
	}
	if apiRoot != nil {
		defer apiRoot.Close()
	}

	if err := c.upgradeMachineSeriesClient.UpgradeSeriesComplete(c.machineNumber); err != nil {
		return errors.Trace(err)
	}

	if err := c.handleNotifications(ctx); err != nil {
		return errors.Trace(err)
	}

	if err := c.unpinLeaders(ctx); err != nil {
		return errors.Trace(err)
	}

	m := UpgradeSeriesCompleteFinishedMessage + "\n"
	ctx.Infof(m, c.machineNumber)

	return nil
}

// ensureAPIClients checks to see if API clients are already instantiated.
// If not, a new api Connection is created and used to instantiate them.
// If they have been set elsewhere (such as by a test) we leave them as is.
func (c *upgradeSeriesCommand) ensureAPIClients() (api.Connection, error) {
	var apiRoot api.Connection
	if c.upgradeMachineSeriesClient == nil || c.leadershipClient == nil {
		var err error
		apiRoot, err = c.NewAPIRoot()
		if err != nil {
			return nil, errors.Trace(err)
		}
	}

	if c.upgradeMachineSeriesClient == nil {
		c.upgradeMachineSeriesClient = machinemanager.NewClient(apiRoot)
	}
	if c.leadershipClient == nil {
		c.leadershipClient = leadership.NewClient(apiRoot)
	}

	return apiRoot, nil
}

// unpinLeaders queries the API for applications running on the machine,
// then unpins the previously pinned leadership for each.
func (c *upgradeSeriesCommand) unpinLeaders(ctx *cmd.Context) error {
	applications, err := c.upgradeMachineSeriesClient.Applications(c.machineNumber)
	if err != nil {
		return errors.Annotate(err, "retrieving machine applications")
	}

	apps := sort.StringSlice(applications)
	apps.Sort()
	for _, app := range apps {
		if err := c.leadershipClient.UnpinLeadership(app, names.NewMachineTag(c.machineNumber)); err != nil {
			return errors.Annotatef(err, "unfreezing leadership for %q", app)
		}
		ctx.Infof("leadership unpinned for application %q", app)
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
