// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package machine

import (
	"fmt"
	"strings"
	"time"

	"github.com/juju/cmd/v3"
	"github.com/juju/errors"
	"github.com/juju/gnuflag"
	"github.com/juju/names/v5"

	"github.com/juju/juju/api"
	"github.com/juju/juju/api/client/machinemanager"
	"github.com/juju/juju/api/client/modelconfig"
	jujucmd "github.com/juju/juju/cmd"
	"github.com/juju/juju/cmd/juju/block"
	"github.com/juju/juju/cmd/modelcmd"
	"github.com/juju/juju/rpc/params"
)

// NewRemoveCommand returns a command used to remove a specified machine.
func NewRemoveCommand() cmd.Command {
	return modelcmd.Wrap(&removeCommand{})
}

// removeCommand causes an existing machine to be destroyed.
type removeCommand struct {
	modelcmd.RemoveConfirmationCommandBase
	baseMachinesCommand
	apiRoot api.Connection

	machineAPI     RemoveMachineAPI
	modelConfigApi ModelConfigAPI

	MachineIds   []string
	Force        bool
	KeepInstance bool
	NoWait       bool
	DryRun       bool
	fs           *gnuflag.FlagSet
}

const destroyMachineDoc = `
Machines are specified by their numbers, which may be retrieved from the
output of ` + "`juju status`." + `

It is possible to remove a machine from Juju model without affecting
the corresponding cloud instance by using the ` + "`--keep-instance`" + ` option.

Machines responsible for the model cannot be removed.

Machines running units or containers can be removed using the ` + "`--force`" + `
option; this will also remove those units and containers without giving
them an opportunity to shut down cleanly.

Machine removal is a multi-step process. Under normal circumstances, Juju will not
proceed to the next step until the current step has finished.
However, when using ` + "`--force`" + `, users can also specify ` + "`--no-wait`" + `
to progress through steps without delay waiting for each step to complete.
`

const destroyMachineExamples = `
    juju remove-machine 5
    juju remove-machine 6 --force
    juju remove-machine 6 --force --no-wait
    juju remove-machine 7 --keep-instance
`

var removeMachineMsgNoDryRun = `
This command will remove machine(s) %q
Your controller does not support dry runs`[1:]

var removeMachineMsgPrefix = "This command will perform the following actions:"

var errDryRunNotSupported = errors.New("Your controller does not support `--dry-run`")

// Info implements Command.Info.
func (c *removeCommand) Info() *cmd.Info {
	return jujucmd.Info(&cmd.Info{
		Name:     "remove-machine",
		Args:     "<machine number> ...",
		Purpose:  "Removes one or more machines from a model.",
		Doc:      destroyMachineDoc,
		Examples: destroyMachineExamples,
		SeeAlso: []string{
			"add-machine",
		},
	})
}

// SetFlags implements Command.SetFlags.
func (c *removeCommand) SetFlags(f *gnuflag.FlagSet) {
	c.ModelCommandBase.SetFlags(f)
	c.RemoveConfirmationCommandBase.SetFlags(f)
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
	if !c.Force && c.NoWait {
		return errors.NotValidf("--no-wait without --force")
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

func (c *removeCommand) getModelConfigAPI() (ModelConfigAPI, error) {
	if c.modelConfigApi != nil {
		return c.modelConfigApi, nil
	}
	root, err := c.NewAPIRoot()
	if err != nil {
		return nil, errors.Trace(err)
	}
	return modelconfig.NewClient(root), nil
}

// Run implements Command.Run.
func (c *removeCommand) Run(ctx *cmd.Context) error {
	var maxWait *time.Duration
	if c.NoWait {
		zeroSec := 0 * time.Second
		maxWait = &zeroSec
	}

	client, err := c.getRemoveMachineAPI()
	if err != nil {
		return err
	}
	defer client.Close()

	modelConfigClient, err := c.getModelConfigAPI()
	if err != nil {
		return err
	}
	defer modelConfigClient.Close()

	if c.DryRun {
		return c.performDryRun(ctx, client)
	}

	needsConfirmation := c.NeedsConfirmation(modelConfigClient)
	if needsConfirmation {
		err := c.performDryRun(ctx, client)
		if err == errDryRunNotSupported {
			ctx.Warningf(removeMachineMsgNoDryRun, strings.Join(c.MachineIds, ", "))
		} else if err != nil {
			return err
		}
		if err := jujucmd.UserConfirmYes(ctx); err != nil {
			return errors.Annotate(err, "machine removal")
		}
	}

	results, err := client.DestroyMachinesWithParams(c.Force, c.KeepInstance, false, maxWait, c.MachineIds...)
	if err := block.ProcessBlockedError(err, block.BlockRemove); err != nil {
		return errors.Trace(err)
	}

	logAll := !needsConfirmation || client.BestAPIVersion() < 10
	if logAll {
		return c.logResults(ctx, results)
	} else {
		return c.logErrors(ctx, results)
	}
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
	if err := c.logErrors(ctx, results); err != nil {
		return err
	}
	ctx.Warningf(removeMachineMsgPrefix)
	_ = c.logResults(ctx, results)
	if c.runNeedsForce(results) && !c.Force {
		ctx.Infof("\nThis will require `--force`")
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

func (c *removeCommand) logErrors(ctx *cmd.Context, results []params.DestroyMachineResult) error {
	return c.log(ctx, results, true)
}

func (c *removeCommand) logResults(ctx *cmd.Context, results []params.DestroyMachineResult) error {
	return c.log(ctx, results, false)
}

func (c *removeCommand) log(ctx *cmd.Context, results []params.DestroyMachineResult, errorOnly bool) error {
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
		cmd.WriteError(ctx.Stderr, err)
		return errors.Trace(err)
	}
	if !errorOnly {
		c.logRemovedMachine(ctx, result.Info)
	}
	return c.log(ctx, result.Info.DestroyedContainers, errorOnly)
}

func (c *removeCommand) logRemovedMachine(ctx *cmd.Context, info *params.DestroyMachineInfo) {
	id := info.MachineId
	if c.KeepInstance {
		_, _ = fmt.Fprintf(ctx.Stdout, "will remove machine %s (but retaining cloud instance)\n", id)
	} else {
		_, _ = fmt.Fprintf(ctx.Stdout, "will remove machine %s\n", id)
	}
	for _, entity := range info.DestroyedUnits {
		unitTag, err := names.ParseUnitTag(entity.Tag)
		if err != nil {
			ctx.Warningf("%s", err)
			continue
		}
		_, _ = fmt.Fprintf(ctx.Stdout, "- will remove %s\n", names.ReadableString(unitTag))
	}
	for _, entity := range info.DestroyedStorage {
		storageTag, err := names.ParseStorageTag(entity.Tag)
		if err != nil {
			ctx.Warningf("%s", err)
			continue
		}
		_, _ = fmt.Fprintf(ctx.Stdout, "- will remove %s\n", names.ReadableString(storageTag))
	}
	for _, entity := range info.DetachedStorage {
		storageTag, err := names.ParseStorageTag(entity.Tag)
		if err != nil {
			ctx.Warningf("%s", err)
			continue
		}
		_, _ = fmt.Fprintf(ctx.Stdout, "- will detach %s\n", names.ReadableString(storageTag))
	}
}
