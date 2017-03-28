// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package application

import (
	"github.com/juju/cmd"
	"github.com/juju/errors"
	"github.com/juju/romulus/api/budget"
	wireformat "github.com/juju/romulus/wireformat/budget"
	"gopkg.in/juju/charm.v6-unstable"
	"gopkg.in/juju/names.v2"
	"gopkg.in/macaroon-bakery.v1/httpbakery"

	"github.com/juju/juju/api/application"
	"github.com/juju/juju/api/charms"
	"github.com/juju/juju/apiserver/params"
	"github.com/juju/juju/cmd/juju/block"
	"github.com/juju/juju/cmd/modelcmd"
)

// NewRemoveApplicationCommand returns a command which removes an application.
func NewRemoveApplicationCommand() cmd.Command {
	return modelcmd.Wrap(&removeApplicationCommand{})
}

// removeServiceCommand causes an existing application to be destroyed.
type removeApplicationCommand struct {
	modelcmd.ModelCommandBase
	ApplicationNames []string
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

Examples:
    juju remove-application hadoop
    juju remove-application -m test-model mariadb`[1:]

func (c *removeApplicationCommand) Info() *cmd.Info {
	return &cmd.Info{
		Name:    "remove-application",
		Args:    "<application> [<application>...]",
		Purpose: helpSummaryRmApp,
		Doc:     helpDetailsRmApp,
	}
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

type removeApplicationAPI interface {
	Close() error
	DestroyApplications(appName ...string) ([]params.DestroyApplicationResult, error)
	DestroyDeprecated(appName string) error
	DestroyUnits(unitNames ...string) ([]params.DestroyUnitResult, error)
	DestroyUnitsDeprecated(unitNames ...string) error
	GetCharmURL(appName string) (*charm.URL, error)
	ModelUUID() string
}

func (c *removeApplicationCommand) getAPI() (removeApplicationAPI, int, error) {
	root, err := c.NewAPIRoot()
	if err != nil {
		return nil, -1, errors.Trace(err)
	}
	version := root.BestFacadeVersion("Application")
	return application.NewClient(root), version, nil
}

func (c *removeApplicationCommand) Run(ctx *cmd.Context) error {
	client, apiVersion, err := c.getAPI()
	if err != nil {
		return err
	}
	defer client.Close()

	if apiVersion < 4 {
		return c.removeApplicationsDeprecated(ctx, client)
	}
	return c.removeApplications(ctx, client)
}

// TODO(axw) 2017-03-16 #1673323
// Drop this in Juju 3.0.
func (c *removeApplicationCommand) removeApplicationsDeprecated(
	ctx *cmd.Context,
	client removeApplicationAPI,
) error {
	for _, name := range c.ApplicationNames {
		err := client.DestroyDeprecated(name)
		if err := block.ProcessBlockedError(err, block.BlockRemove); err != nil {
			return errors.Trace(err)
		}
		if err := c.removeAllocation(ctx, client, name); err != nil {
			return errors.Trace(err)
		}
	}
	return nil
}

func (c *removeApplicationCommand) removeApplications(
	ctx *cmd.Context,
	client removeApplicationAPI,
) error {
	results, err := client.DestroyApplications(c.ApplicationNames...)
	if err := block.ProcessBlockedError(err, block.BlockRemove); err != nil {
		return errors.Trace(err)
	}
	anyFailed := false
	for i, name := range c.ApplicationNames {
		result := results[i]
		if result.Error != nil {
			ctx.Infof("removing application %s failed: %s", name, result.Error)
			anyFailed = true
			continue
		}
		if err := c.removeAllocation(ctx, client, name); err != nil {
			ctx.Infof("removing allocation for application %s failed: %s", name, result.Error)
			anyFailed = true
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

func (c *removeApplicationCommand) removeAllocation(
	ctx *cmd.Context,
	client removeApplicationAPI,
	applicationName string,
) error {
	charmURL, err := client.GetCharmURL(applicationName)
	// Not all apps have charms, eg remote applications.
	if params.ErrCode(err) == params.CodeNotFound {
		return nil
	}
	if err != nil {
		return errors.Trace(err)
	}
	if charmURL.Schema == "local" {
		return nil
	}

	root, err := c.NewAPIRoot()
	if err != nil {
		return errors.Trace(err)
	}
	charmsClient := charms.NewClient(root)
	metered, err := charmsClient.IsMetered(charmURL.String())
	if err != nil {
		return errors.Trace(err)
	}
	if !metered {
		return nil
	}

	modelUUID := client.ModelUUID()
	bakeryClient, err := c.BakeryClient()
	if err != nil {
		return errors.Trace(err)
	}
	budgetClient := getBudgetAPIClient(bakeryClient)

	resp, err := budgetClient.DeleteAllocation(modelUUID, applicationName)
	if wireformat.IsNotAvail(err) {
		logger.Warningf("allocation not removed: %v", err)
	} else if err != nil {
		return err
	}
	if resp != "" {
		logger.Infof(resp)
	}
	return nil
}

var getBudgetAPIClient = getBudgetAPIClientImpl

func getBudgetAPIClientImpl(bakeryClient *httpbakery.Client) budgetAPIClient {
	return budget.NewClient(bakeryClient)
}

type budgetAPIClient interface {
	DeleteAllocation(string, string) (string, error)
}
