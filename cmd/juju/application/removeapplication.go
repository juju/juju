// Copyright 2013 Canonical Ltd.
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
	"github.com/juju/juju/rpc/params"
)

// NewRemoveApplicationCommand returns a command which removes an application.
func NewRemoveApplicationCommand() cmd.Command {
	c := &removeApplicationCommand{}
	return modelcmd.Wrap(c)
}

// removeApplicationCommand causes an existing application to be destroyed.
type removeApplicationCommand struct {
	modelcmd.RemoveConfirmationCommandBase
	modelcmd.ModelCommandBase

	api            RemoveApplicationAPI
	modelConfigApi ModelConfigClient

	ApplicationNames []string
	DestroyStorage   bool
	Force            bool
	NoWait           bool
	NoPrompt         bool
	DryRun           bool
	fs               *gnuflag.FlagSet
}

var helpSummaryRmApp = `
Remove applications from the model.`[1:]

var helpDetailsRmApp = `
Removing an application will terminate any relations that application has, remove
all units of the application, and in the case that this leaves machines with
no running applications, Juju will also remove the machine. For this reason,
you should retrieve any logs or data required from applications and units
before removing them. Removing units which are co-located with units of
other charms or a Juju controller will not result in the removal of the
machine.

Sometimes, the removal of the application may fail as Juju encounters errors
and failures that need to be dealt with before an application can be removed.
For example, Juju will not remove an application if there are hook failures.
However, at times, there is a need to remove an application ignoring
all operational errors. In these rare cases, use the ` + "`--force`" + ` option but note
that ` + "`--force`" + ` will also remove all units of the application, its subordinates
and, potentially, machines without given them the opportunity to shutdown cleanly.

Application removal is a multi-step process. Under normal circumstances, Juju will not
proceed to the next step until the current step has finished.
However, when using ` + "`--force`" + `, users can also specify ` + "`--no-wait`" + `
to progress through steps without delay waiting for each step to complete.

`[1:]

const helpExamplesRmApp = `
    juju remove-application hadoop
    juju remove-application --force hadoop
    juju remove-application --force --no-wait hadoop
    juju remove-application -m test-model mariadb
`

var removeApplicationMsgNoDryRun = `
This command will remove application(s) %q
Your controller does not support dry runs`[1:]

var removeApplicationMsgPrefix = "This command will perform the following actions:"

var errDryRunNotSupportedByController = errors.New("Your controller does not support `--dry-run`")

func (c *removeApplicationCommand) Info() *cmd.Info {
	return jujucmd.Info(&cmd.Info{
		Name:     "remove-application",
		Args:     "<application> [<application>...]",
		Purpose:  helpSummaryRmApp,
		Doc:      helpDetailsRmApp,
		Examples: helpExamplesRmApp,
		SeeAlso: []string{
			"scale-application",
			"show-application",
		},
	})
}

func (c *removeApplicationCommand) SetFlags(f *gnuflag.FlagSet) {
	c.ModelCommandBase.SetFlags(f)
	c.RemoveConfirmationCommandBase.SetFlags(f)
	f.BoolVar(&c.DryRun, "dry-run", false, "Print what this command would remove without removing")
	f.BoolVar(&c.DestroyStorage, "destroy-storage", false, "Destroy storage attached to application units")
	f.BoolVar(&c.Force, "force", false, "Completely remove an application and all its dependencies")
	f.BoolVar(&c.NoWait, "no-wait", false, "Rush through application removal without waiting for each individual step to complete")
	c.fs = f
}

func (c *removeApplicationCommand) Init(args []string) error {
	if len(args) == 0 {
		return errors.Errorf("no application specified")
	}
	for _, arg := range args {
		if !names.IsValidApplication(arg) {
			return errors.Errorf("invalid application name %q", arg)
		}
	}
	if !c.Force && c.NoWait {
		return errors.NotValidf("--no-wait without --force")
	}
	c.ApplicationNames = args
	return nil
}

type RemoveApplicationAPI interface {
	Close() error
	ScaleApplication(application.ScaleApplicationParams) (params.ScaleApplicationResult, error)
	DestroyApplications(application.DestroyApplicationsParams) ([]params.DestroyApplicationResult, error)
	DestroyUnits(application.DestroyUnitsParams) ([]params.DestroyUnitResult, error)
	ModelUUID() string
	BestAPIVersion() int
}

func (c *removeApplicationCommand) getAPI() (RemoveApplicationAPI, error) {
	if c.api != nil {
		return c.api, nil
	}
	root, err := c.NewAPIRoot()
	if err != nil {
		return nil, errors.Trace(err)
	}
	return application.NewClient(root), nil
}

func (c *removeApplicationCommand) getModelConfigAPI() (ModelConfigClient, error) {
	if c.modelConfigApi != nil {
		return c.modelConfigApi, nil
	}
	root, err := c.NewAPIRoot()
	if err != nil {
		return nil, errors.Trace(err)
	}
	return modelconfig.NewClient(root), nil
}

func (c *removeApplicationCommand) Run(ctx *cmd.Context) error {
	var maxWait *time.Duration
	if c.NoWait {
		zeroSec := 0 * time.Second
		maxWait = &zeroSec
	}

	client, err := c.getAPI()
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
		if err == errDryRunNotSupportedByController {
			ctx.Warningf(removeApplicationMsgNoDryRun, strings.Join(c.ApplicationNames, ", "))
		} else if err != nil {
			return err
		}
		if err := jujucmd.UserConfirmYes(ctx); err != nil {
			return errors.Annotate(err, "application removal")
		}
	}

	results, err := client.DestroyApplications(application.DestroyApplicationsParams{
		Applications:   c.ApplicationNames,
		DestroyStorage: c.DestroyStorage,
		Force:          c.Force,
		MaxWait:        maxWait,
		DryRun:         false,
	})
	if err := block.ProcessBlockedError(err, block.BlockRemove); err != nil {
		return errors.Trace(err)
	}
	logAll := !needsConfirmation || client.BestAPIVersion() < 16
	if logAll {
		return c.logResults(ctx, results)
	} else {
		return c.logErrors(ctx, results)
	}
}

func (c *removeApplicationCommand) performDryRun(
	ctx *cmd.Context,
	client RemoveApplicationAPI,
) error {
	// TODO(jack-w-shaw) Drop this once application 15 support is dropped
	if client.BestAPIVersion() < 16 {
		return errDryRunNotSupportedByController
	}
	results, err := client.DestroyApplications(application.DestroyApplicationsParams{
		Applications:   c.ApplicationNames,
		DestroyStorage: c.DestroyStorage,
		DryRun:         true,
	})
	if err := block.ProcessBlockedError(err, block.BlockRemove); err != nil {
		return errors.Trace(err)
	}
	if err := c.logErrors(ctx, results); err != nil {
		return err
	}
	ctx.Warningf(removeApplicationMsgPrefix)
	_ = c.logResults(ctx, results)
	return nil
}

func (c *removeApplicationCommand) logErrors(ctx *cmd.Context, results []params.DestroyApplicationResult) error {
	return c.log(ctx, results, true)
}

func (c *removeApplicationCommand) logResults(ctx *cmd.Context, results []params.DestroyApplicationResult) error {
	return c.log(ctx, results, false)
}

func (c *removeApplicationCommand) log(
	ctx *cmd.Context,
	results []params.DestroyApplicationResult,
	errorsOnly bool,
) error {
	anyFailed := false
	for i, name := range c.ApplicationNames {
		result := results[i]
		if err := c.logResult(ctx, name, result, errorsOnly); err != nil {
			anyFailed = true
		}
	}
	if anyFailed {
		return cmd.ErrSilent
	}
	return nil
}

func (c *removeApplicationCommand) logResult(
	ctx *cmd.Context,
	name string,
	result params.DestroyApplicationResult,
	errorsOnly bool,
) error {
	if result.Error != nil {
		var err error = result.Error
		if params.IsCodeNotSupported(result.Error) {
			err = errors.New("another user was updating application; please try again")
		}
		err = errors.Annotatef(err, "removing application %s failed", name)
		cmd.WriteError(ctx.Stderr, err)
		return errors.Trace(err)
	}
	if !errorsOnly {
		c.logRemovedApplication(ctx, name, result.Info)
	}
	return nil
}

func (c *removeApplicationCommand) logRemovedApplication(
	ctx *cmd.Context,
	name string,
	info *params.DestroyApplicationInfo,
) {
	_, _ = fmt.Fprintf(ctx.Stdout, "will remove application %s\n", name)
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
