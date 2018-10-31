// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package application

import (
	"github.com/juju/cmd"

	"github.com/juju/juju/api"
	"github.com/juju/juju/api/base"
	"github.com/juju/juju/cmd/modelcmd"
	"github.com/juju/juju/jujuclient"
	"github.com/juju/juju/resource/resourceadapters"
)

func NewUpgradeCharmCommandForTest(
	store jujuclient.ClientStore,
	apiOpen api.OpenFunc,
	deployResources resourceadapters.DeployResourcesFunc,
	resolveCharm ResolveCharmFunc,
	newCharmAdder NewCharmAdderFunc,
	newCharmClient func(api.Connection) CharmClient,
	newCharmUpgradeClient func(api.Connection) CharmUpgradeClient,
	newModelConfigGetter func(api.Connection) ModelConfigGetter,
	newResourceLister func(api.Connection) (ResourceLister, error),
	charmStoreURLGetter func(api.Connection) (string, error),
	newLXDProfileUpgradeClient func(api.Connection) LXDProfileUpgradeAPI,
) cmd.Command {
	cmd := &upgradeCharmCommand{
		DeployResources:            deployResources,
		ResolveCharm:               resolveCharm,
		NewCharmAdder:              newCharmAdder,
		NewCharmClient:             newCharmClient,
		NewCharmUpgradeClient:      newCharmUpgradeClient,
		NewModelConfigGetter:       newModelConfigGetter,
		NewResourceLister:          newResourceLister,
		CharmStoreURLGetter:        charmStoreURLGetter,
		NewLXDProfileUpgradeClient: newLXDProfileUpgradeClient,
	}
	cmd.SetClientStore(store)
	cmd.SetAPIOpen(apiOpen)
	return modelcmd.Wrap(cmd)
}

// NewResolvedCommandForTest returns a ResolvedCommand with the api provided as specified.
func NewResolvedCommandForTest(applicationResolveAPI applicationResolveAPI, clientAPI clientAPI, store jujuclient.ClientStore) modelcmd.ModelCommand {
	cmd := &resolvedCommand{applicationResolveAPI: applicationResolveAPI, clientAPI: clientAPI}
	cmd.SetClientStore(store)
	return modelcmd.Wrap(cmd)
}

// NewAddUnitCommandForTest returns an AddUnitCommand with the api provided as specified.
func NewAddUnitCommandForTest(api applicationAddUnitAPI, store jujuclient.ClientStore) modelcmd.ModelCommand {
	cmd := &addUnitCommand{api: api}
	cmd.SetClientStore(store)
	return modelcmd.Wrap(cmd)
}

// NewAddRelationCommandForTest returns an AddRelationCommand with the api provided as specified.
func NewAddRelationCommandForTest(addAPI applicationAddRelationAPI, consumeAPI applicationConsumeDetailsAPI) modelcmd.ModelCommand {
	cmd := &addRelationCommand{addRelationAPI: addAPI, consumeDetailsAPI: consumeAPI}
	return modelcmd.Wrap(cmd)
}

// NewRemoveRelationCommandForTest returns an RemoveRelationCommand with the api provided as specified.
func NewRemoveRelationCommandForTest(api ApplicationDestroyRelationAPI, store jujuclient.ClientStore) modelcmd.ModelCommand {
	cmd := &removeRelationCommand{newAPIFunc: func() (ApplicationDestroyRelationAPI, error) {
		return api, nil
	}}
	cmd.SetClientStore(store)
	return modelcmd.Wrap(cmd)
}

// NewConsumeCommandForTest returns a ConsumeCommand with the specified api.
func NewConsumeCommandForTest(
	store jujuclient.ClientStore,
	sourceAPI applicationConsumeDetailsAPI,
	targetAPI applicationConsumeAPI,
) cmd.Command {
	c := &consumeCommand{sourceAPI: sourceAPI, targetAPI: targetAPI}
	c.SetClientStore(store)
	return modelcmd.Wrap(c)
}

func NewUpdateSeriesCommandForTest(
	appAPI updateApplicationSeriesAPI,
	machAPI updateMachineSeriesAPI,
	store jujuclient.ClientStore,
) modelcmd.ModelCommand {
	cmd := &updateSeriesCommand{
		updateApplicationSeriesClient: appAPI,
		updateMachineSeriesClient:     machAPI,
	}
	cmd.SetClientStore(store)
	return modelcmd.Wrap(cmd)
}

// NewSetSeriesCommandForTest returns a SetSeriesCommand with the specified api.
func NewSetSeriesCommandForTest(
	seriesAPI setSeriesAPI,
	store jujuclient.ClientStore,
) modelcmd.ModelCommand {
	cmd := &setSeriesCommand{
		setSeriesClient: seriesAPI,
	}
	cmd.SetClientStore(store)
	return modelcmd.Wrap(cmd)
}

// NewSuspendRelationCommandForTest returns a SuspendRelationCommand with the api provided as specified.
func NewSuspendRelationCommandForTest(api SetRelationSuspendedAPI, store jujuclient.ClientStore) modelcmd.ModelCommand {
	cmd := &suspendRelationCommand{newAPIFunc: func() (SetRelationSuspendedAPI, error) {
		return api, nil
	}}
	cmd.SetClientStore(store)
	return modelcmd.Wrap(cmd)
}

// NewResumeRelationCommandForTest returns a ResumeRelationCommand with the api provided as specified.
func NewResumeRelationCommandForTest(api SetRelationSuspendedAPI, store jujuclient.ClientStore) modelcmd.ModelCommand {
	cmd := &resumeRelationCommand{newAPIFunc: func() (SetRelationSuspendedAPI, error) {
		return api, nil
	}}
	cmd.SetClientStore(store)
	return modelcmd.Wrap(cmd)
}

// NewRemoveSaasCommandForTest returns a RemoveSaasCommand with the api provided as specified.
func NewRemoveSaasCommandForTest(api RemoveSaasAPI, store jujuclient.ClientStore) modelcmd.ModelCommand {
	cmd := &removeSaasCommand{newAPIFunc: func() (RemoveSaasAPI, error) {
		return api, nil
	}}
	cmd.SetClientStore(store)
	return modelcmd.Wrap(cmd)
}

// NewScaleCommandForTest returns a ScaleCommand with the api provided as specified.
func NewScaleCommandForTest(api scaleApplicationAPI, store jujuclient.ClientStore) modelcmd.ModelCommand {
	cmd := &scaleApplicationCommand{newAPIFunc: func() (scaleApplicationAPI, error) {
		return api, nil
	}}
	cmd.SetClientStore(store)
	return modelcmd.Wrap(cmd)
}

func NewBundleDiffCommandForTest(api base.APICallCloser, charmStore BundleResolver, store jujuclient.ClientStore) modelcmd.ModelCommand {
	cmd := &bundleDiffCommand{
		_apiRoot:    api,
		_charmStore: charmStore,
	}
	cmd.SetClientStore(store)
	return modelcmd.Wrap(cmd)
}
