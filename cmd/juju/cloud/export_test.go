// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package cloud

import (
	"github.com/juju/cmd"

	jujucloud "github.com/juju/juju/cloud"
	"github.com/juju/juju/cmd/modelcmd"
	"github.com/juju/juju/environs"
	"github.com/juju/juju/environs/context"
	sstesting "github.com/juju/juju/environs/simplestreams/testing"
	"github.com/juju/juju/jujuclient"
)

var (
	ShouldFinalizeCredential = shouldFinalizeCredential
)

type (
	UpdateCloudCommand   = updateCloudCommand
	AddCredentialCommand = addCredentialCommand
	UpdateCloudAPI       = updateCloudAPI
	ShowCloudAPI         = showCloudAPI
	RemoveCloudAPI       = removeCloudAPI
)

var (
	CredentialsFromLocalCache = credentialsFromLocalCache
	CredentialsFromFile       = credentialsFromFile
)

func NewAddCloudCommandForTest(
	cloudMetadataStore CloudMetadataStore,
	store jujuclient.ClientStore,
	cloudAPI func() (AddCloudAPI, error),
) *AddCloudCommand {
	cloudCallCtx := context.NewCloudCallContext()
	return &AddCloudCommand{
		OptionalControllerCommand: modelcmd.OptionalControllerCommand{Store: store},
		cloudMetadataStore:        cloudMetadataStore,
		CloudCallCtx:              cloudCallCtx,
		Ping: func(p environs.EnvironProvider, endpoint string) error {
			return nil
		},
		store:           store,
		addCloudAPIFunc: cloudAPI,
	}
}

func NewListCloudCommandForTest(store jujuclient.ClientStore, cloudAPI func(string) (ListCloudsAPI, error)) *listCloudsCommand {
	return &listCloudsCommand{
		OptionalControllerCommand: modelcmd.OptionalControllerCommand{Store: store},
		listCloudsAPIFunc:         cloudAPI,
	}
}

func NewShowCloudCommandForTest(store jujuclient.ClientStore, cloudAPI func(string) (showCloudAPI, error)) *showCloudCommand {
	return &showCloudCommand{
		OptionalControllerCommand: modelcmd.OptionalControllerCommand{Store: store},
		store:                     store,
		showCloudAPIFunc:          cloudAPI,
	}
}

func NewRemoveCloudCommandForTest(store jujuclient.ClientStore, cloudAPI func(string) (removeCloudAPI, error)) *removeCloudCommand {
	return &removeCloudCommand{
		OptionalControllerCommand: modelcmd.OptionalControllerCommand{Store: store},
		store:                     store,
		removeCloudAPIFunc:        cloudAPI,
	}
}

func NewUpdatePublicCloudsCommandForTest(publicCloudURL string) *updatePublicCloudsCommand {
	return &updatePublicCloudsCommand{
		// TODO(wallyworld) - move testing key elsewhere
		publicSigningKey: sstesting.SignedMetadataPublicKey,
		publicCloudURL:   publicCloudURL,
	}
}

func NewUpdateCloudCommandForTest(
	cloudMetadataStore CloudMetadataStore,
	store jujuclient.ClientStore,
	cloudAPI func(string) (UpdateCloudAPI, error),
) *updateCloudCommand {
	return &updateCloudCommand{
		OptionalControllerCommand: modelcmd.OptionalControllerCommand{Store: store},
		cloudMetadataStore:        cloudMetadataStore,
		updateCloudAPIFunc:        cloudAPI,
		store:                     store,
	}
}

func NewListCredentialsCommandForTest(
	testStore jujuclient.ClientStore,
	personalCloudsFunc func() (map[string]jujucloud.Cloud, error),
	cloudByNameFunc func(string) (*jujucloud.Cloud, error),
	apiF func(controllerName string) (ListCredentialsAPI, error),
) *listCredentialsCommand {
	return &listCredentialsCommand{
		OptionalControllerCommand: modelcmd.OptionalControllerCommand{
			Store: testStore,
		},
		personalCloudsFunc:     personalCloudsFunc,
		cloudByNameFunc:        cloudByNameFunc,
		listCredentialsAPIFunc: apiF,
	}
}

func NewDetectCredentialsCommandForTest(
	testStore jujuclient.CredentialStore,
	registeredProvidersFunc func() []string,
	allCloudsFunc func() (map[string]jujucloud.Cloud, error),
	cloudsByNameFunc func(string) (*jujucloud.Cloud, error),
	cloudType string,
) *detectCredentialsCommand {
	return &detectCredentialsCommand{
		store:                   testStore,
		registeredProvidersFunc: registeredProvidersFunc,
		allCloudsFunc:           allCloudsFunc,
		cloudByNameFunc:         cloudsByNameFunc,
		cloudType:               cloudType,
	}
}

func NewAddCredentialCommandForTest(
	testStore jujuclient.CredentialStore,
	cloudByNameFunc func(string) (*jujucloud.Cloud, error),
) *AddCredentialCommand {
	return &addCredentialCommand{
		store:           testStore,
		cloudByNameFunc: cloudByNameFunc,
	}
}

func NewRemoveCredentialCommandForTest(testStore jujuclient.CredentialStore) *removeCredentialCommand {
	return &removeCredentialCommand{
		store: testStore,
	}
}

func NewSetDefaultCredentialCommandForTest(testStore jujuclient.CredentialStore) *setDefaultCredentialCommand {
	return &setDefaultCredentialCommand{
		store: testStore,
	}
}

func NewSetDefaultRegionCommandForTest(testStore jujuclient.CredentialStore) *setDefaultRegionCommand {
	return &setDefaultRegionCommand{
		store: testStore,
	}
}

func NewUpdateCredentialCommandForTest(testStore jujuclient.ClientStore, api CredentialAPI) cmd.Command {
	c := &updateCredentialCommand{
		api: api,
	}
	c.SetClientStore(testStore)
	return modelcmd.WrapController(c)
}

func NewShowCredentialCommandForTest(testStore jujuclient.ClientStore, api CredentialContentAPI) cmd.Command {
	command := &showCredentialCommand{
		store: testStore,
		newAPIFunc: func() (CredentialContentAPI, error) {
			return api, nil
		},
	}
	return modelcmd.WrapBase(command)
}
