// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package application

import (
	"github.com/juju/cmd/v4"

	"github.com/juju/juju/api"
	"github.com/juju/juju/api/base"
	"github.com/juju/juju/cmd/juju/application/deployer"
	"github.com/juju/juju/cmd/juju/application/refresher"
	"github.com/juju/juju/cmd/juju/application/store"
	"github.com/juju/juju/cmd/juju/application/utils"
	"github.com/juju/juju/cmd/modelcmd"
	"github.com/juju/juju/internal/charm"
	"github.com/juju/juju/jujuclient"
)

func NewRefreshCommandForTest(
	store jujuclient.ClientStore,
	apiOpen api.OpenFunc,
	deployResources deployer.DeployResourcesFunc,
	newCharmResolver NewCharmResolverFunc,
	newCharmAdder NewCharmAdderFunc,
	newCharmClient func(base.APICallCloser) utils.CharmClient,
	newCharmRefreshClient func(base.APICallCloser) CharmRefreshClient,
	newResourceLister func(base.APICallCloser) (utils.ResourceLister, error),
	newSpacesClient func(base.APICallCloser) SpacesAPI,
	newModelConfigClient func(base.APICallCloser) ModelConfigClient,
	newCharmHubClient func(string) (store.DownloadBundleClient, error),
) cmd.Command {
	cmd := &refreshCommand{
		DeployResources:       deployResources,
		NewCharmAdder:         newCharmAdder,
		NewCharmClient:        newCharmClient,
		NewCharmRefreshClient: newCharmRefreshClient,
		NewResourceLister:     newResourceLister,
		NewSpacesClient:       newSpacesClient,
		NewCharmResolver:      newCharmResolver,
		NewRefresherFactory:   refresher.NewRefresherFactory,
		ModelConfigClient:     newModelConfigClient,
		NewCharmHubClient:     newCharmHubClient,
	}
	cmd.SetClientStore(store)
	cmd.SetAPIOpen(apiOpen)
	return modelcmd.Wrap(cmd)
}

func NewBindCommandForTest(
	store jujuclient.ClientStore,
	apiOpen api.OpenFunc,
	newApplicationClient func(base.APICallCloser) ApplicationBindClient,
	newSpacesClient func(base.APICallCloser) SpacesAPI,
) cmd.Command {
	cmd := &bindCommand{
		NewApplicationClient: newApplicationClient,
		NewSpacesClient:      newSpacesClient,
	}
	cmd.SetClientStore(store)
	cmd.SetAPIOpen(apiOpen)
	return modelcmd.Wrap(cmd)
}

// NewResolvedCommandForTest returns a ResolvedCommand with the api provided as specified.
func NewResolvedCommandForTest(applicationResolveAPI applicationResolveAPI, store jujuclient.ClientStore) modelcmd.ModelCommand {
	cmd := &resolvedCommand{applicationResolveAPI: applicationResolveAPI}
	cmd.SetClientStore(store)
	return modelcmd.Wrap(cmd)
}

// NewAddUnitCommandForTest returns an AddUnitCommand with the api provided as specified.
func NewAddUnitCommandForTest(api applicationAddUnitAPI, store jujuclient.ClientStore) modelcmd.ModelCommand {
	cmd := &addUnitCommand{api: api}
	cmd.SetClientStore(store)
	return modelcmd.Wrap(cmd)
}

// NewAddUnitCommandForTest returns an AddUnitCommand with the api provided as specified as well as overrides the refresh function.
func NewAddUnitCommandForTestWithRefresh(api applicationAddUnitAPI, store jujuclient.ClientStore, refreshFunc func(jujuclient.ClientStore, string) error) modelcmd.ModelCommand {
	cmd := &addUnitCommand{api: api}
	cmd.SetClientStore(store)
	cmd.SetModelRefresh(refreshFunc)
	return modelcmd.Wrap(cmd)
}

// NewRemoveUnitCommandForTest returns a RemoveUnitCommand with the api provided as specified.
func NewRemoveUnitCommandForTest(api RemoveApplicationAPI, modelConfigApi ModelConfigClient, store jujuclient.ClientStore) modelcmd.ModelCommand {
	cmd := &removeUnitCommand{api: api, modelConfigApi: modelConfigApi}
	cmd.SetClientStore(store)
	return modelcmd.Wrap(cmd)
}

// NewRemoveApplicationCommandForTest returns a RemoveApplicationCommand.
func NewRemoveApplicationCommandForTest(api RemoveApplicationAPI, modelConfigApi ModelConfigClient, store jujuclient.ClientStore) modelcmd.ModelCommand {
	c := &removeApplicationCommand{api: api, modelConfigApi: modelConfigApi}
	c.SetClientStore(store)
	return modelcmd.Wrap(c)
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

// NewSetApplicationBaseCommandForTest returns a SetSeriesCommand with the specified api.
func NewSetApplicationBaseCommandForTest(
	setApplicationBaseAPI setApplicationBaseAPI,
	store jujuclient.ClientStore,
) modelcmd.ModelCommand {
	cmd := &setApplicationBase{
		apiClient: setApplicationBaseAPI,
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

func NewDiffBundleCommandForTest(api base.APICallCloser,
	charmStoreFn func(base.APICallCloser, *charm.URL) (BundleResolver, error),
	modelConsFn func() (ModelConstraintsClient, error),
	store jujuclient.ClientStore,
) modelcmd.ModelCommand {
	cmd := &diffBundleCommand{
		newAPIRootFn: func() (base.APICallCloser, error) {
			return api, nil
		},
		modelConstraintsClientFunc: modelConsFn,
	}
	if charmStoreFn != nil {
		cmd.charmAdaptorFn = charmStoreFn
	} else {
		cmd.charmAdaptorFn = cmd.charmAdaptor
	}
	cmd.SetClientStore(store)
	return modelcmd.Wrap(cmd)
}

func NewShowCommandForTest(api ApplicationsInfoAPI, store jujuclient.ClientStore) cmd.Command {
	cmd := &showApplicationCommand{newAPIFunc: func() (ApplicationsInfoAPI, error) {
		return api, nil
	}}
	cmd.SetClientStore(store)
	return modelcmd.Wrap(cmd)
}

func NewShowUnitCommandForTest(api UnitsInfoAPI, store jujuclient.ClientStore) cmd.Command {
	cmd := &showUnitCommand{newAPIFunc: func() (UnitsInfoAPI, error) {
		return api, nil
	}}
	cmd.SetClientStore(store)
	return modelcmd.Wrap(cmd)
}

// NewConfigCommandForTest returns a SetCommand with the api provided as specified.
func NewConfigCommandForTest(api ApplicationAPI, store jujuclient.ClientStore) modelcmd.ModelCommand {
	c := modelcmd.Wrap(&configCommand{configBase: appConfigBase, api: api})
	c.SetClientStore(store)
	return c
}
