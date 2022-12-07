// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package application

import (
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/juju/cmd/v3"
	"github.com/juju/collections/set"
	"github.com/juju/errors"
	"github.com/juju/gnuflag"
	"github.com/juju/names/v4"

	"github.com/juju/juju/api/client/application"
	"github.com/juju/juju/api/client/storage"
	jujucmd "github.com/juju/juju/cmd"
	"github.com/juju/juju/cmd/juju/block"
	"github.com/juju/juju/cmd/modelcmd"
	"github.com/juju/juju/juju/osenv"
	"github.com/juju/juju/rpc/params"
)

// NewRemoveApplicationCommand returns a command which removes an application.
func NewRemoveApplicationCommand() cmd.Command {
	c := &removeApplicationCommand{}
	c.newAPIFunc = func() (RemoveApplicationAPI, error) {
		return c.getAPI()
	}
	return modelcmd.Wrap(c)
}

// removeApplicationCommand causes an existing application to be destroyed.
// TODO(jack-w-shaw) This should inherit from ConfirmationCommandBase in
// 3.1, once hebaviours have converged
type removeApplicationCommand struct {
	modelcmd.ModelCommandBase

	newAPIFunc func() (RemoveApplicationAPI, error)

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
all operational errors. In these rare cases, use --force option but note 
that --force will also remove all units of the application, its subordinates
and, potentially, machines without given them the opportunity to shutdown cleanly.

Application removal is a multi-step process. Under normal circumstances, Juju will not
proceed to the next step until the current step has finished. 
However, when using --force, users can also specify --no-wait to progress through steps 
without delay waiting for each step to complete.

Examples:
    juju remove-application hadoop
    juju remove-application --force hadoop
    juju remove-application --force --no-wait hadoop
    juju remove-application -m test-model mariadb`[1:]

var removeApplicationMsgNoDryRun = `
WARNING! This command will remove application(s) %q
Your controller does not support a more in depth dry run
`[1:]

var removeApplicationMsgPrefix = "WARNING! This command:\n"

var errDryRunNotSupported = errors.New("Your controller does not support `--dry-run`")

func (c *removeApplicationCommand) Info() *cmd.Info {
	return jujucmd.Info(&cmd.Info{
		Name:    "remove-application",
		Args:    "<application> [<application>...]",
		Purpose: helpSummaryRmApp,
		Doc:     helpDetailsRmApp,
	})
}

func (c *removeApplicationCommand) SetFlags(f *gnuflag.FlagSet) {
	c.ModelCommandBase.SetFlags(f)
	// This unused var is declared so we can pass a valid ptr into BoolVar
	f.BoolVar(&c.NoPrompt, "no-prompt", false, "Do not prompt for approval")
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

type storageAPI interface {
	Close() error
	ListStorageDetails() ([]params.StorageDetails, error)
}

func (c *removeApplicationCommand) getAPI() (RemoveApplicationAPI, error) {
	root, err := c.NewAPIRoot()
	if err != nil {
		return nil, errors.Trace(err)
	}
	return application.NewClient(root), nil
}

func (c *removeApplicationCommand) getStorageAPI() (storageAPI, error) {
	root, err := c.NewAPIRoot()
	if err != nil {
		return nil, errors.Trace(err)
	}
	return storage.NewClient(root), nil
}

func (c *removeApplicationCommand) applicationsHaveStorage(appNames []string) (bool, error) {
	client, err := c.getStorageAPI()
	if err != nil {
		return false, errors.Trace(err)
	}
	defer client.Close()

	storages, err := client.ListStorageDetails()
	if err != nil {
		return false, errors.Trace(err)
	}
	namesSet := set.NewStrings(appNames...)
	for _, s := range storages {
		if s.OwnerTag == "" {
			continue
		}
		owner, err := names.ParseTag(s.OwnerTag)
		if err != nil {
			return false, errors.Trace(err)
		}
		if owner.Kind() != names.UnitTagKind {
			continue
		}
		appName, err := names.UnitApplication(owner.Id())
		if err != nil {
			return false, errors.Trace(err)
		}
		if namesSet.Contains(appName) {
			return true, nil
		}
	}
	return false, nil
}

func (c *removeApplicationCommand) Run(ctx *cmd.Context) error {

	client, err := c.newAPIFunc()
	if err != nil {
		return err
	}
	defer client.Close()

	return c.removeApplications(ctx, client)
}

func (c *removeApplicationCommand) removeApplications(
	ctx *cmd.Context,
	client RemoveApplicationAPI,
) error {
	var maxWait *time.Duration
	if c.NoWait {
		zeroSec := 0 * time.Second
		maxWait = &zeroSec
	}

	if c.DryRun {
		return c.performDryRun(ctx, client)
	}

	if !c.NoPrompt {
		err := c.performDryRun(ctx, client)
		if err == errDryRunNotSupported {
			fmt.Fprintf(ctx.Stderr, removeApplicationMsgNoDryRun, strings.Join(c.ApplicationNames, ", "))
		} else if err != nil {
			return errors.Trace(err)
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
	logAll := c.NoPrompt || client.BestAPIVersion() < 16
	return c.logResults(ctx, results, !logAll)
}

func (c *removeApplicationCommand) performDryRun(
	ctx *cmd.Context,
	client RemoveApplicationAPI,
) error {
	// TODO(jack-w-shaw) Drop this once application 15 support is dropped
	if client.BestAPIVersion() < 16 {
		return errDryRunNotSupported
	}
	results, err := client.DestroyApplications(application.DestroyApplicationsParams{
		Applications: c.ApplicationNames,
		DryRun:       true,
	})
	if err := block.ProcessBlockedError(err, block.BlockRemove); err != nil {
		return errors.Trace(err)
	}
	fmt.Fprintf(ctx.Stderr, removeApplicationMsgPrefix)
	if err := c.logResults(ctx, results, false); err != nil {
		return errors.Trace(err)
	}
	return nil
}

func (c *removeApplicationCommand) logResults(
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
		fmt.Fprintf(ctx.Stderr, "%s\n", err)
		return errors.Trace(err)
	}
	if !errorsOnly {
		c.logRemovedApplication(ctx, name, result)
	}
	return nil
}

func (c *removeApplicationCommand) logRemovedApplication(
	ctx *cmd.Context,
	name string,
	result params.DestroyApplicationResult,
) {
	fmt.Fprintf(ctx.Stdout, "will remove application %s\n", name)
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
