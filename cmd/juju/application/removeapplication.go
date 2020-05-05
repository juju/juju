// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package application

import (
	"time"

	"github.com/juju/cmd"
	"github.com/juju/collections/set"
	"github.com/juju/errors"
	"github.com/juju/gnuflag"
	"github.com/juju/names/v4"

	"github.com/juju/juju/api/application"
	"github.com/juju/juju/api/storage"
	"github.com/juju/juju/apiserver/params"
	jujucmd "github.com/juju/juju/cmd"
	"github.com/juju/juju/cmd/juju/block"
	"github.com/juju/juju/cmd/modelcmd"
)

// NewRemoveApplicationCommand returns a command which removes an application.
func NewRemoveApplicationCommand() cmd.Command {
	c := &removeApplicationCommand{}
	c.newAPIFunc = func() (RemoveApplicationAPI, int, error) {
		return c.getAPI()
	}
	return modelcmd.Wrap(c)
}

// removeApplicationCommand causes an existing application to be destroyed.
type removeApplicationCommand struct {
	modelcmd.ModelCommandBase

	newAPIFunc func() (RemoveApplicationAPI, int, error)

	ApplicationNames []string
	DestroyStorage   bool
	Force            bool
	NoWait           bool
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
proceed to a next step until the current step has finished. 
However, when using --force, users can also specify --no-wait to progress through steps 
without delay waiting for each step to complete.

Examples:
    juju remove-application hadoop
    juju remove-application --force hadoop
    juju remove-application --force --no-wait hadoop
    juju remove-application -m test-model mariadb`[1:]

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
	c.ApplicationNames = args
	return nil
}

type RemoveApplicationAPI interface {
	Close() error
	ScaleApplication(application.ScaleApplicationParams) (params.ScaleApplicationResult, error)
	DestroyApplications(application.DestroyApplicationsParams) ([]params.DestroyApplicationResult, error)
	DestroyDeprecated(appName string) error
	DestroyUnits(application.DestroyUnitsParams) ([]params.DestroyUnitResult, error)
	DestroyUnitsDeprecated(unitNames ...string) error
	ModelUUID() string
	BestAPIVersion() int
}

type storageAPI interface {
	Close() error
	ListStorageDetails() ([]params.StorageDetails, error)
}

func (c *removeApplicationCommand) getAPI() (RemoveApplicationAPI, int, error) {
	root, err := c.NewAPIRoot()
	if err != nil {
		return nil, -1, errors.Trace(err)
	}
	version := root.BestFacadeVersion("Application")
	return application.NewClient(root), version, nil
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

	client, apiVersion, err := c.newAPIFunc()
	if err != nil {
		return err
	}
	defer client.Close()

	if apiVersion < 4 {
		return c.removeApplicationsDeprecated(ctx, client)
	}
	if c.DestroyStorage && apiVersion < 5 {
		return errors.New("--destroy-storage is not supported by this controller")
	}
	return c.removeApplications(ctx, client)
}

// TODO(axw) 2017-03-16 #1673323
// Drop this in Juju 3.0.
func (c *removeApplicationCommand) removeApplicationsDeprecated(
	ctx *cmd.Context,
	client RemoveApplicationAPI,
) error {
	for _, name := range c.ApplicationNames {
		err := client.DestroyDeprecated(name)
		if err := block.ProcessBlockedError(err, block.BlockRemove); err != nil {
			return errors.Trace(err)
		}
	}
	return nil
}

func (c *removeApplicationCommand) removeApplications(
	ctx *cmd.Context,
	client RemoveApplicationAPI,
) error {
	var maxWait *time.Duration
	if c.Force {
		if c.NoWait {
			zeroSec := 0 * time.Second
			maxWait = &zeroSec
		}
	}

	results, err := client.DestroyApplications(application.DestroyApplicationsParams{
		Applications:   c.ApplicationNames,
		DestroyStorage: c.DestroyStorage,
		Force:          c.Force,
		MaxWait:        maxWait,
	})
	if err := block.ProcessBlockedError(err, block.BlockRemove); err != nil {
		return errors.Trace(err)
	}
	anyFailed := false
	for i, name := range c.ApplicationNames {
		result := results[i]
		if result.Error != nil {
			anyFailed = true
			err := result.Error.Error()
			if params.IsCodeNotSupported(result.Error) {
				err = errors.New("another user was updating application; please try again").Error()
			}
			ctx.Infof("removing application %s failed: %s", name, err)
			continue
		}
		ctx.Infof("removing application %s", name)
		for _, entity := range result.Info.DestroyedUnits {
			unitTag, err := names.ParseUnitTag(entity.Tag)
			if err != nil {
				logger.Warningf("%s", err)
				continue
			}
			ctx.Verbosef("- will remove %s", names.ReadableString(unitTag))
		}
		for _, entity := range result.Info.DestroyedStorage {
			storageTag, err := names.ParseStorageTag(entity.Tag)
			if err != nil {
				logger.Warningf("%s", err)
				continue
			}
			ctx.Infof("- will remove %s", names.ReadableString(storageTag))
		}
		for _, entity := range result.Info.DetachedStorage {
			storageTag, err := names.ParseStorageTag(entity.Tag)
			if err != nil {
				logger.Warningf("%s", err)
				continue
			}
			ctx.Infof("- will detach %s", names.ReadableString(storageTag))
		}
	}
	if anyFailed {
		return cmd.ErrSilent
	}
	return nil
}
