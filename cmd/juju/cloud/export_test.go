// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package cloud

import (
	jujucloud "github.com/juju/juju/cloud"
	"github.com/juju/juju/jujuclient"
)

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
