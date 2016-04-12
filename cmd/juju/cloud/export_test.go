// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package cloud

import (
	jujucloud "github.com/juju/juju/cloud"
	sstesting "github.com/juju/juju/environs/simplestreams/testing"
	"github.com/juju/juju/jujuclient"
)

var (
	AdjustPlurality = adjustPlurality
	FormatSlice     = formatSlice
	DiffClouds      = diffClouds
)

func NewUpdateCloudsCommandForTest(publicCloudURL string) *updateCloudsCommand {
	return &updateCloudsCommand{
		// TODO(wallyworld) - move testing key elsewhere
		publicSigningKey: sstesting.SignedMetadataPublicKey,
		publicCloudURL:   publicCloudURL,
	}
}

func NewListCredentialsCommandForTest(
	testStore jujuclient.CredentialGetter,
	personalCloudsFunc func() (map[string]jujucloud.Cloud, error),
	cloudByNameFunc func(string) (*jujucloud.Cloud, error),
) *listCredentialsCommand {
	return &listCredentialsCommand{
		store:              testStore,
		personalCloudsFunc: personalCloudsFunc,
		cloudByNameFunc:    cloudByNameFunc,
	}
}

func NewDetectCredentialsCommandForTest(
	testStore jujuclient.CredentialStore,
	registeredProvidersFunc func() []string,
	allCloudsFunc func() (map[string]jujucloud.Cloud, error),
	cloudsByNameFunc func(string) (*jujucloud.Cloud, error),
) *detectCredentialsCommand {
	return &detectCredentialsCommand{
		store: testStore,
		registeredProvidersFunc: registeredProvidersFunc,
		allCloudsFunc:           allCloudsFunc,
		cloudByNameFunc:         cloudsByNameFunc,
	}
}

func NewAddCredentialCommandForTest(
	testStore jujuclient.CredentialStore,
	cloudByNameFunc func(string) (*jujucloud.Cloud, error),
) *addCredentialCommand {
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
