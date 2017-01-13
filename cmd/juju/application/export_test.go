// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package application

import (
	"github.com/juju/cmd"
	"gopkg.in/juju/charmrepo.v2-unstable/csclient"
	"gopkg.in/macaroon-bakery.v1/httpbakery"

	"github.com/juju/juju/api"
	"github.com/juju/juju/cmd/modelcmd"
	"github.com/juju/juju/jujuclient"
	"github.com/juju/juju/resource/resourceadapters"
)

func NewUpgradeCharmCommandForTest(
	store jujuclient.ClientStore,
	apiOpener modelcmd.APIOpener,
	deployResources resourceadapters.DeployResourcesFunc,
	resolveCharm ResolveCharmFunc,
	newCharmAdder NewCharmAdderFunc,
	newCharmClient func(api.Connection) CharmClient,
	newCharmUpgradeClient func(api.Connection) CharmUpgradeClient,
	newModelConfigGetter func(api.Connection) ModelConfigGetter,
	newResourceLister func(api.Connection) (ResourceLister, error),
) cmd.Command {
	cmd := &upgradeCharmCommand{
		DeployResources:       deployResources,
		ResolveCharm:          resolveCharm,
		NewCharmAdder:         newCharmAdder,
		NewCharmClient:        newCharmClient,
		NewCharmUpgradeClient: newCharmUpgradeClient,
		NewModelConfigGetter:  newModelConfigGetter,
		NewResourceLister:     newResourceLister,
	}
	cmd.SetClientStore(store)
	cmd.SetAPIOpener(apiOpener)
	return modelcmd.Wrap(cmd)
}

// NewConfigCommandForTest returns a SetCommand with the api provided as specified.
func NewConfigCommandForTest(api configCommandAPI) cmd.Command {
	return modelcmd.Wrap(&configCommand{
		api: api,
	})
}

// NewAddUnitCommandForTest returns an AddUnitCommand with the api provided as specified.
func NewAddUnitCommandForTest(api serviceAddUnitAPI) cmd.Command {
	return modelcmd.Wrap(&addUnitCommand{
		api: api,
	})
}

// NewAddRelationCommandForTest returns an AddRelationCommand with the api provided as specified.
func NewAddRelationCommandForTest(api ApplicationAddRelationAPI) cmd.Command {
	cmd := &addRelationCommand{newAPIFunc: func() (ApplicationAddRelationAPI, error) {
		return api, nil
	}}
	return modelcmd.Wrap(cmd)
}

// NewRemoveRelationCommandForTest returns an RemoveRelationCommand with the api provided as specified.
func NewRemoveRelationCommandForTest(api ApplicationDestroyRelationAPI) cmd.Command {
	cmd := &removeRelationCommand{newAPIFunc: func() (ApplicationDestroyRelationAPI, error) {
		return api, nil
	}}
	return modelcmd.Wrap(cmd)
}

// NewConsumeCommandForTest returns a ConsumeCommand with the specified api.
func NewConsumeCommandForTest(api applicationConsumeAPI) cmd.Command {
	return modelcmd.Wrap(&consumeCommand{api: api})
}

type Patcher interface {
	PatchValue(dest, value interface{})
}

func PatchNewCharmStoreClient(s Patcher, url string) {
	s.PatchValue(&newCharmStoreClient, func(bakeryClient *httpbakery.Client) *csclient.Client {
		return csclient.New(csclient.Params{
			URL:          url,
			BakeryClient: bakeryClient,
		})
	})
}
