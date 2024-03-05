// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package cloud

import (
	"github.com/juju/cmd/v4"

	jujucloud "github.com/juju/juju/cloud"
	"github.com/juju/juju/cmd/modelcmd"
	"github.com/juju/juju/environs"
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
	return &AddCloudCommand{
		OptionalControllerCommand: modelcmd.OptionalControllerCommand{Store: store},
		cloudMetadataStore:        cloudMetadataStore,
		Ping: func(p environs.EnvironProvider, endpoint string) error {
			return nil
		},
		addCloudAPIFunc: cloudAPI,
	}
}

func NewListCloudCommandForTest(store jujuclient.ClientStore, cloudAPI func() (ListCloudsAPI, error)) *listCloudsCommand {
	return &listCloudsCommand{
		OptionalControllerCommand: modelcmd.OptionalControllerCommand{Store: store, ReadOnly: true},
		listCloudsAPIFunc:         cloudAPI,
	}
}

func NewShowCloudCommandForTest(store jujuclient.ClientStore, cloudAPI func() (showCloudAPI, error)) *showCloudCommand {
	return &showCloudCommand{
		OptionalControllerCommand: modelcmd.OptionalControllerCommand{Store: store, ReadOnly: true},
		showCloudAPIFunc:          cloudAPI,
	}
}

func NewRemoveCloudCommandForTest(store jujuclient.ClientStore, cloudAPI func() (RemoveCloudAPI, error)) *removeCloudCommand {
	return &removeCloudCommand{
		OptionalControllerCommand: modelcmd.OptionalControllerCommand{Store: store},
		removeCloudAPIFunc:        cloudAPI,
	}
}

func NewUpdatePublicCloudsCommandForTest(store jujuclient.ClientStore, api updatePublicCloudAPI, publicCloudURL string) *updatePublicCloudsCommand {
	return &updatePublicCloudsCommand{
		OptionalControllerCommand: modelcmd.OptionalControllerCommand{Store: store},
		addCloudAPIFunc:           func() (updatePublicCloudAPI, error) { return api, nil },
		// TODO(wallyworld) - move testing key elsewhere
		publicSigningKey: sstesting.SignedMetadataPublicKey,
		publicCloudURL:   publicCloudURL,
	}
}

func NewUpdateCloudCommandForTest(
	cloudMetadataStore CloudMetadataStore,
	store jujuclient.ClientStore,
	cloudAPI func() (UpdateCloudAPI, error),
) *updateCloudCommand {
	return &updateCloudCommand{
		OptionalControllerCommand: modelcmd.OptionalControllerCommand{Store: store},
		cloudMetadataStore:        cloudMetadataStore,
		updateCloudAPIFunc:        cloudAPI,
	}
}

func NewListCredentialsCommandForTest(
	testStore jujuclient.ClientStore,
	personalCloudsFunc func() (map[string]jujucloud.Cloud, error),
	cloudByNameFunc func(string) (*jujucloud.Cloud, error),
	apiF func() (ListCredentialsAPI, error),
) *listCredentialsCommand {
	return &listCredentialsCommand{
		OptionalControllerCommand: modelcmd.OptionalControllerCommand{
			Store:    testStore,
			ReadOnly: true,
		},
		personalCloudsFunc:     personalCloudsFunc,
		cloudByNameFunc:        cloudByNameFunc,
		listCredentialsAPIFunc: apiF,
	}
}

func NewDetectCredentialsCommandForTest(
	testStore jujuclient.ClientStore,
	registeredProvidersFunc func() []string,
	allCloudsFunc func(*cmd.Context) (map[string]jujucloud.Cloud, error),
	cloudsByNameFunc func(string) (*jujucloud.Cloud, error),
	f func() (CredentialAPI, error),
) *detectCredentialsCommand {
	command := &detectCredentialsCommand{
		OptionalControllerCommand: modelcmd.OptionalControllerCommand{Store: testStore},
		registeredProvidersFunc:   registeredProvidersFunc,
		cloudByNameFunc:           jujucloud.CloudByName,
		credentialAPIFunc:         f,
	}
	if allCloudsFunc != nil {
		command.allCloudsFunc = allCloudsFunc
	} else {
		command.allCloudsFunc = command.allClouds
	}
	if cloudsByNameFunc != nil {
		command.cloudByNameFunc = cloudsByNameFunc
	}
	return command
}

func NewAddCredentialCommandForTest(
	testStore jujuclient.ClientStore,
	cloudByNameFunc func(string) (*jujucloud.Cloud, error),
	f func() (CredentialAPI, error),
) *AddCredentialCommand {
	return &addCredentialCommand{
		OptionalControllerCommand: modelcmd.OptionalControllerCommand{Store: testStore},
		cloudByNameFunc:           cloudByNameFunc,
		credentialAPIFunc:         f,
	}
}

func NewRemoveCredentialCommandForTest(testStore jujuclient.ClientStore,
	cloudByNameFunc func(string) (*jujucloud.Cloud, error),
	f func() (RemoveCredentialAPI, error),
) *removeCredentialCommand {
	return &removeCredentialCommand{
		OptionalControllerCommand: modelcmd.OptionalControllerCommand{Store: testStore},
		cloudByNameFunc:           cloudByNameFunc,
		credentialAPIFunc:         f,
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
	command := &updateCredentialCommand{
		OptionalControllerCommand: modelcmd.OptionalControllerCommand{Store: testStore},
		updateCredentialAPIFunc: func() (CredentialAPI, error) {
			return api, nil
		},
	}
	return modelcmd.WrapBase(command)
}

func NewShowCredentialCommandForTest(testStore jujuclient.ClientStore, api CredentialContentAPI) cmd.Command {
	command := &showCredentialCommand{
		OptionalControllerCommand: modelcmd.OptionalControllerCommand{Store: testStore, ReadOnly: true},
		newAPIFunc: func() (CredentialContentAPI, error) {
			return api, nil
		},
	}
	return modelcmd.WrapBase(command)
}

func AddLoadedCredentialForTest(
	all map[string]map[string]map[string]jujucloud.Credential,
	cloudName, regionName, credentialName string,
	credential jujucloud.Credential,
) {

	discovered := discoveredCredential{
		region:         regionName,
		credential:     credential,
		credentialName: credentialName,
	}
	addLoadedCredential(all, cloudName, discovered)
}

func NewListRegionsCommandForTest(store jujuclient.ClientStore, cloudAPI func() (CloudRegionsAPI, error)) *listRegionsCommand {
	return &listRegionsCommand{
		OptionalControllerCommand: modelcmd.OptionalControllerCommand{Store: store, ReadOnly: true},
		cloudAPIFunc:              cloudAPI,
	}
}
