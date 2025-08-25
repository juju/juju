// Copyright 2012, 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package application

import (
	"fmt"
	"strings"
	"time"

	"github.com/juju/cmd/v3"
	"github.com/juju/errors"
	"github.com/juju/gnuflag"
	"github.com/juju/names/v5"

	"github.com/juju/juju/api/client/application"
	"github.com/juju/juju/api/client/modelconfig"
	jujucmd "github.com/juju/juju/cmd"
	"github.com/juju/juju/cmd/juju/block"
	"github.com/juju/juju/cmd/modelcmd"
	"github.com/juju/juju/core/model"
	"github.com/juju/juju/rpc/params"
)

// NewRemoveUnitCommand returns a command which removes an application's units.
func NewRemoveUnitCommand() modelcmd.ModelCommand {
	return modelcmd.Wrap(&removeUnitCommand{})
}

// removeUnitCommand is responsible for destroying application units.
type removeUnitCommand struct {
	modelcmd.RemoveConfirmationCommandBase
	modelcmd.ModelCommandBase
	DestroyStorage bool
	NumUnits       int
	EntityNames    []string

	api            RemoveApplicationAPI
	modelConfigApi ModelConfigClient

	unknownModel bool
	Force        bool
	NoWait       bool
	NoPrompt     bool
	DryRun       bool
	fs           *gnuflag.FlagSet
}

const removeUnitDoc = `
Remove application units from the model.

The usage of this command differs depending on whether it is being used on a
Kubernetes or a machine model.

Removing all units of a application is not equivalent to removing the
application itself; for that, the ` + "`juju remove-application`" + ` command
is used.

For Kubernetes models only a single application can be supplied and only the
` + "`--num-units`" + ` argument supported.
Specific units cannot be targeted for removal as that is handled by Kubernetes;
instead the total number of units to be removed is specified.

For cloud models specific units can be targeted for removal.
Units of a application are numbered in sequence upon creation. For example, the
fourth unit of wordpress will be designated ` + "`wordpress/3`" + `. These identifiers
can be supplied in a space delimited list to remove unwanted units from the
model.

Juju will also remove the machine if the removed unit was the only unit left
on that machine (including units in containers).

Sometimes, the removal of the unit may fail as Juju encounters errors
and failures that need to be dealt with before a unit can be removed.
For example, Juju will not remove a unit if there are hook failures.
However, at times, there is a need to remove a unit ignoring
all operational errors. In these rare cases, use --force option but note
that ` + "`--force`" + ` will remove a unit and, potentially, its machine without
given them the opportunity to shutdown cleanly.

Unit removal is a multi-step process. Under normal circumstances, Juju will not
proceed to the next step until the current step has finished.
However, when using ` + "`--force`" + `, users can also specify ` + "`--no-wait`" + `
to progress through steps without delay waiting for each step to complete.
`

const removeUnitExamples = `
    juju remove-unit wordpress/2 wordpress/3 wordpress/4

    juju remove-unit wordpress/2 --destroy-storage

    juju remove-unit wordpress/2 --force

    juju remove-unit wordpress/2 --force --no-wait

	juju remove-unit wordpress --num-units 2
`

var removeUnitMsgNoDryRun = `
This command will remove unit(s) %q
Your controller does not support dry runs`[1:]

var removeUnitMsgPrefix = "This command will perform the following actions:"

func (c *removeUnitCommand) Info() *cmd.Info {
	return jujucmd.Info(&cmd.Info{
		Name:     "remove-unit",
		Args:     "<unit> [...] | <application>",
		Purpose:  "Remove application units from the model.",
		Doc:      removeUnitDoc,
		Examples: removeUnitExamples,
		SeeAlso: []string{
			"remove-application",
			"scale-application",
		},
	})
}

func (c *removeUnitCommand) SetFlags(f *gnuflag.FlagSet) {
	c.ModelCommandBase.SetFlags(f)
	c.RemoveConfirmationCommandBase.SetFlags(f)
	f.BoolVar(&c.DryRun, "dry-run", false, "Print what this command would remove without removing")
	f.IntVar(&c.NumUnits, "num-units", 0, "Number of units to remove (k8s models only)")
	f.BoolVar(&c.DestroyStorage, "destroy-storage", false, "Destroy storage attached to the unit")
	f.BoolVar(&c.Force, "force", false, "Completely remove an unit and all its dependencies")
	f.BoolVar(&c.NoWait, "no-wait", false, "Rush through unit removal without waiting for each individual step to complete")
	c.fs = f
}

func (c *removeUnitCommand) Init(args []string) error {
	c.EntityNames = args
	if err := c.validateArgsByModelType(); err != nil {
		if !errors.IsNotFound(err) {
			return errors.Trace(err)
		}

		c.unknownModel = true
	}
	if !c.Force && c.NoWait {
		return errors.NotValidf("--no-wait without --force")
	}
	return nil
}

func (c *removeUnitCommand) validateArgsByModelType() error {
	modelType, err := c.ModelType()
	if err != nil {
		return err
	}
	if modelType == model.CAAS {
		return c.validateCAASRemoval()
	}

	return c.validateIAASRemoval()
}

func (c *removeUnitCommand) validateCAASRemoval() error {
	if c.DryRun {
		// TODO(caas): enable --dry-run for caas model.
		return errors.New("`--dry-run` is not supported for kubernetes units")
	}
	if c.DestroyStorage {
		// TODO(caas): enable --destroy-storage for caas model.
		return errors.New("k8s models only support --num-units")
	}
	if len(c.EntityNames) == 0 {
		return errors.Errorf("no application specified")
	}
	if len(c.EntityNames) != 1 {
		return errors.Errorf("only single application supported")
	}
	if names.IsValidUnit(c.EntityNames[0]) {
		msg := `
k8s models do not support removing named units.
Instead specify an application with --num-units.
`[1:]
		return errors.New(msg)
	}
	if !names.IsValidApplication(c.EntityNames[0]) {
		return errors.NotValidf("application name %q", c.EntityNames[0])
	}
	if c.NumUnits <= 0 {
		return errors.New("specify the number of units (> 0) to remove using --num-units")
	}
	return nil
}

func (c *removeUnitCommand) validateIAASRemoval() error {
	if c.NumUnits != 0 {
		return errors.NotValidf("--num-units for non k8s models")
	}
	if len(c.EntityNames) == 0 {
		return errors.Errorf("no units specified")
	}
	for _, name := range c.EntityNames {
		if !names.IsValidUnit(name) {
			return errors.Errorf("invalid unit name %q", name)
		}
	}

	return nil
}

func (c *removeUnitCommand) getAPI() (RemoveApplicationAPI, error) {
	if c.api != nil {
		return c.api, nil
	}
	root, err := c.NewAPIRoot()
	if err != nil {
		return nil, errors.Trace(err)
	}
	api := application.NewClient(root)
	return api, nil
}

func (c *removeUnitCommand) getModelConfigAPI() (ModelConfigClient, error) {
	if c.modelConfigApi != nil {
		return c.modelConfigApi, nil
	}
	root, err := c.NewAPIRoot()
	if err != nil {
		return nil, errors.Trace(err)
	}
	c.modelConfigApi = modelconfig.NewClient(root)
	return c.modelConfigApi, nil
}

// Run connects to the environment specified on the command line and destroys
// units therein.
func (c *removeUnitCommand) Run(ctx *cmd.Context) error {
	client, err := c.getAPI()
	if err != nil {
		return err
	}
	defer client.Close()

	if err := c.validateArgsByModelType(); err != nil {
		return errors.Trace(err)
	}

	modelType, err := c.ModelType()
	if err != nil {
		return err
	}
	if modelType == model.CAAS {
		return c.removeCaasUnits(ctx, client)
	}

	return c.removeUnits(ctx, client)
}

func (c *removeUnitCommand) removeUnits(ctx *cmd.Context, client RemoveApplicationAPI) error {
	var maxWait *time.Duration
	if c.NoWait {
		zeroSec := 0 * time.Second
		maxWait = &zeroSec
	}
	if c.DryRun {
		return c.performDryRun(ctx, client)
	}
	modelConfigClient, err := c.getModelConfigAPI()
	if err != nil {
		return err
	}
	defer modelConfigClient.Close()

	needsConfirmation := c.NeedsConfirmation(modelConfigClient)
	if needsConfirmation {
		err := c.performDryRun(ctx, client)
		if err == errDryRunNotSupportedByController {
			ctx.Warningf(removeUnitMsgNoDryRun, strings.Join(c.EntityNames, ", "))
		} else if err != nil {
			return err
		}
		if err := jujucmd.UserConfirmYes(ctx); err != nil {
			return errors.Annotate(err, "unit removal")
		}
	}

	results, err := client.DestroyUnits(application.DestroyUnitsParams{
		Units:          c.EntityNames,
		DestroyStorage: c.DestroyStorage,
		Force:          c.Force,
		MaxWait:        maxWait,
		DryRun:         false,
	})
	if err != nil {
		return block.ProcessBlockedError(err, block.BlockRemove)
	}
	logAll := !needsConfirmation || client.BestAPIVersion() < 16
	if logAll {
		return c.logResults(ctx, results)
	} else {
		return c.logErrors(ctx, results)
	}
}

func (c *removeUnitCommand) performDryRun(ctx *cmd.Context, client RemoveApplicationAPI) error {
	// TODO(jack-w-shaw) Drop this once application 15 support is dropped
	if client.BestAPIVersion() < 16 {
		return errDryRunNotSupportedByController
	}
	results, err := client.DestroyUnits(application.DestroyUnitsParams{
		Units:          c.EntityNames,
		DestroyStorage: c.DestroyStorage,
		DryRun:         true,
	})
	if err != nil {
		return block.ProcessBlockedError(err, block.BlockRemove)
	}
	if err := c.logErrors(ctx, results); err != nil {
		return err
	}
	ctx.Warningf(removeUnitMsgPrefix)
	_ = c.logResults(ctx, results)
	return nil
}

func (c *removeUnitCommand) logErrors(ctx *cmd.Context, results []params.DestroyUnitResult) error {
	return c.log(ctx, results, true)
}

func (c *removeUnitCommand) logResults(ctx *cmd.Context, results []params.DestroyUnitResult) error {
	return c.log(ctx, results, false)
}

func (c *removeUnitCommand) log(
	ctx *cmd.Context,
	results []params.DestroyUnitResult,
	errorOnly bool,
) error {
	anyFailed := false
	for i, name := range c.EntityNames {
		result := results[i]
		if err := c.logResult(ctx, name, result, errorOnly); err != nil {
			anyFailed = true
		}
	}
	if anyFailed {
		return cmd.ErrSilent
	}
	return nil
}

func (c *removeUnitCommand) logResult(
	ctx *cmd.Context,
	name string,
	result params.DestroyUnitResult,
	errorOnly bool,
) error {
	if result.Error != nil {
		err := errors.Annotatef(result.Error, "removing unit %s failed", name)
		cmd.WriteError(ctx.Stderr, err)
		return errors.Trace(err)
	}
	if !errorOnly {
		c.logRemovedUnit(ctx, name, result.Info)
	}
	return nil
}

func (c *removeUnitCommand) logRemovedUnit(ctx *cmd.Context, name string, info *params.DestroyUnitInfo) {
	_, _ = fmt.Fprintf(ctx.Stdout, "will remove unit %s\n", name)
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

func (c *removeUnitCommand) removeCaasUnits(ctx *cmd.Context, client RemoveApplicationAPI) error {
	result, err := client.ScaleApplication(application.ScaleApplicationParams{
		ApplicationName: c.EntityNames[0],
		ScaleChange:     -c.NumUnits,
		Force:           c.Force,
	})
	if params.IsCodeNotSupported(err) {
		return errors.Annotate(err, "can not remove unit")
	}
	if err != nil {
		return block.ProcessBlockedError(err, block.BlockRemove)
	}
	ctx.Infof("scaling down to %d units", result.Info.Scale)
	return nil
}
