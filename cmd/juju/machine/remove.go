// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package machine

import (
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/juju/cmd/v3"
	"github.com/juju/errors"
	"github.com/juju/gnuflag"
	"github.com/juju/names/v4"

	"github.com/juju/juju/api"
	"github.com/juju/juju/api/client/machinemanager"
	jujucmd "github.com/juju/juju/cmd"
	"github.com/juju/juju/cmd/juju/block"
	"github.com/juju/juju/cmd/modelcmd"
	"github.com/juju/juju/juju/osenv"
	"github.com/juju/juju/rpc/params"
)

// NewRemoveCommand returns a command used to remove a specified machine.
func NewRemoveCommand() cmd.Command {
	return modelcmd.Wrap(&removeCommand{})
}

// removeCommand causes an existing machine to be destroyed.
// TODO(jack-w-shaw) This should inherit from ConfirmationCommandBase
// in 3.1, once behaviours have converged
type removeCommand struct {
	baseMachinesCommand
	apiRoot      api.Connection
	machineAPI   RemoveMachineAPI
	MachineIds   []string
	Force        bool
	KeepInstance bool
	NoWait       bool
	NoPrompt     bool
	DryRun       bool
	fs           *gnuflag.FlagSet
}

const destroyMachineDoc = `
Machines are specified by their numbers, which may be retrieved from the
output of ` + "`juju status`." + `

It is possible to remove machine from Juju model without affecting
the corresponding cloud instance by using --keep-instance option.

Machines responsible for the model cannot be removed.

Machines running units or containers can be removed using the '--force'
option; this will also remove those units and containers without giving
them an opportunity to shut down cleanly.

Machine removal is a multi-step process. Under normal circumstances, Juju will not
proceed to the next step until the current step has finished. 
However, when using --force, users can also specify --no-wait to progress through steps 
without delay waiting for each step to complete.

Examples:

    juju remove-machine 5
    juju remove-machine 6 --force
    juju remove-machine 6 --force --no-wait
    juju remove-machine 7 --keep-instance

See also:
    add-machine
`

var removeMachineMsgNoDryRun = `
WARNING! This command will remove machine(s) %q
Your controller does not support a more in depth dry run
`[1:]

var removeMachineMsgPrefix = "WARNING! This command:\n"

var errDryRunNotSupported = errors.New("Your controller does not support `--dry-run`")

// Info implements Command.Info.
func (c *removeCommand) Info() *cmd.Info {
	return jujucmd.Info(&cmd.Info{
		Name:    "remove-machine",
		Args:    "<machine number> ...",
		Purpose: "Removes one or more machines from a model.",
		Doc:     destroyMachineDoc,
	})
}

// SetFlags implements Command.SetFlags.
func (c *removeCommand) SetFlags(f *gnuflag.FlagSet) {
	c.ModelCommandBase.SetFlags(f)
	f.BoolVar(&c.NoPrompt, "no-prompt", false, "Do not prompt for approval")
	f.BoolVar(&c.DryRun, "dry-run", false, "Print what this command would be removed without removing")
	f.BoolVar(&c.Force, "force", false, "Completely remove a machine and all its dependencies")
	f.BoolVar(&c.KeepInstance, "keep-instance", false, "Do not stop the running cloud instance")
	f.BoolVar(&c.NoWait, "no-wait", false, "Rush through machine removal without waiting for each individual step to complete")
	c.fs = f
}

func (c *removeCommand) Init(args []string) error {
	if len(args) == 0 {
		return errors.Errorf("no machines specified")
	}
	for _, id := range args {
		if !names.IsValidMachine(id) {
			return errors.Errorf("invalid machine id %q", id)
		}
	}

	// To maintain compatibility, in 3.0 NoPrompt should default to true.
	// However, we still need to take into account the env var and the
	// flag. So default initially to false, but if the env var and flag
	// are not present, set to true.
	// TODO(jack-w-shaw) use CheckSkipConfirmEnvVar in 3.1
	if !c.NoPrompt {
		if skipConf, ok := os.LookupEnv(osenv.JujuSkipConfirmationEnvKey); ok {
			skipConfBool, err := strconv.ParseBool(skipConf)
			if err != nil {
				return errors.Annotatef(err, "value %q of env var %q is not a valid bool", skipConf, osenv.JujuSkipConfirmationEnvKey)
			}
			c.NoPrompt = skipConfBool
		} else {
			c.NoPrompt = true
		}
	}

	c.MachineIds = args
	return nil
}

type RemoveMachineAPI interface {
	DestroyMachinesWithParams(force, keep, dryRun bool, maxWait *time.Duration, machines ...string) ([]params.DestroyMachineResult, error)
	BestAPIVersion() int
	Close() error
}

func (c *removeCommand) getAPIRoot() (api.Connection, error) {
	if c.apiRoot != nil {
		return c.apiRoot, nil
	}
	return c.NewAPIRoot()
}

func (c *removeCommand) getRemoveMachineAPI() (RemoveMachineAPI, error) {
	if c.machineAPI != nil {
		return c.machineAPI, nil
	}
	root, err := c.getAPIRoot()
	if err != nil {
		return nil, err
	}
	return machinemanager.NewClient(root), nil
}

// Run implements Command.Run.
func (c *removeCommand) Run(ctx *cmd.Context) error {
	noWaitSet := false
	forceSet := false
	c.fs.Visit(func(flag *gnuflag.Flag) {
		if flag.Name == "no-wait" {
			noWaitSet = true
		} else if flag.Name == "force" {
			forceSet = true
		}
	})
	if !forceSet && noWaitSet {
		return errors.NotValidf("--no-wait without --force")
	}
	var maxWait *time.Duration
	if c.Force {
		if c.NoWait {
			zeroSec := 0 * time.Second
			maxWait = &zeroSec
		}
	}

	client, err := c.getRemoveMachineAPI()
	if err != nil {
		return err
	}
	defer client.Close()

	if c.DryRun {
		return c.performDryRun(ctx, client)
	}

	if !c.NoPrompt {
		err := c.performDryRun(ctx, client)
		if err == errDryRunNotSupported {
			fmt.Fprintf(ctx.Stderr, removeMachineMsgNoDryRun, strings.Join(c.MachineIds, ", "))
		} else if err != nil {
			return errors.Trace(err)
		}
		if err := jujucmd.UserConfirmYes(ctx); err != nil {
			return errors.Annotate(err, "machine removal")
		}
	}

	results, err := client.DestroyMachinesWithParams(c.Force, c.KeepInstance, false, maxWait, c.MachineIds...)
	if err := block.ProcessBlockedError(err, block.BlockRemove); err != nil {
		return errors.Trace(err)
	}

	logAll := c.NoPrompt || client.BestAPIVersion() < 10
	return c.logResults(ctx, results, !logAll)
}

func (c *removeCommand) performDryRun(ctx *cmd.Context, client RemoveMachineAPI) error {
	// TODO(jack-w-shaw) Drop this once machinemanager 9 support is dropped
	if client.BestAPIVersion() < 10 {
		return errDryRunNotSupported
	}
	results, err := client.DestroyMachinesWithParams(c.Force, c.KeepInstance, true, nil, c.MachineIds...)
	if err := block.ProcessBlockedError(err, block.BlockRemove); err != nil {
		return errors.Trace(err)
	}
	fmt.Fprint(ctx.Stderr, removeMachineMsgPrefix)
	if err := c.logResults(ctx, results, false); err != nil {
		return errors.Trace(err)
	}
	if c.runNeedsForce(results) {
		fmt.Fprint(ctx.Stdout, "\nThis will require `--force`\n")
	}
	return nil
}

func (c *removeCommand) runNeedsForce(results []params.DestroyMachineResult) bool {
	for _, result := range results {
		if result.Error != nil {
			continue
		}
		if len(result.Info.DestroyedContainers) > 0 || len(result.Info.DestroyedUnits) > 0 {
			return true
		}
	}
	return false
}

func (c *removeCommand) logResults(ctx *cmd.Context, results []params.DestroyMachineResult, errorOnly bool) error {
	anyFailed := false
	for _, result := range results {
		if err := c.logResult(ctx, result, errorOnly); err != nil {
			anyFailed = true
		}
	}
	if anyFailed {
		return cmd.ErrSilent
	}
	return nil
}

func (c *removeCommand) logResult(ctx *cmd.Context, result params.DestroyMachineResult, errorOnly bool) error {
	if result.Error != nil {
		err := errors.Annotate(result.Error, "removing machine failed")
		fmt.Fprintf(ctx.Stderr, "%s\n", err)
		return errors.Trace(err)
	}
	if !errorOnly {
		c.logRemovedMachine(ctx, result)
	}
	return c.logResults(ctx, result.Info.DestroyedContainers, errorOnly)
}

func (c *removeCommand) logRemovedMachine(ctx *cmd.Context, result params.DestroyMachineResult) {
	id := result.Info.MachineId
	if c.KeepInstance {
		fmt.Fprintf(ctx.Stdout, "will remove machine %s (but retaining cloud instance)\n", id)
	} else {
		fmt.Fprintf(ctx.Stdout, "will remove machine %s\n", id)
	}
	for _, entity := range result.Info.DestroyedUnits {
		unitTag, err := names.ParseUnitTag(entity.Tag)
		if err != nil {
			logger.Warningf("%s", err)
			continue
		}
		fmt.Fprintf(ctx.Stdout, "- will remove %s\n", names.ReadableString(unitTag))
	}
	for _, entity := range result.Info.DestroyedStorage {
		storageTag, err := names.ParseStorageTag(entity.Tag)
		if err != nil {
			logger.Warningf("%s", err)
			continue
		}
		fmt.Fprintf(ctx.Stdout, "- will remove %s\n", names.ReadableString(storageTag))
	}
	for _, entity := range result.Info.DetachedStorage {
		storageTag, err := names.ParseStorageTag(entity.Tag)
		if err != nil {
			logger.Warningf("%s", err)
			continue
		}
		fmt.Fprintf(ctx.Stdout, "- will detach %s\n", names.ReadableString(storageTag))
	}
}
