// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package cloud

import (
	jujucloud "github.com/juju/juju/cloud"
	"github.com/juju/juju/jujuclient"
)

func NewListCredentialsCommandForTest(testStore jujuclient.CredentialGetter) *listCredentialsCommand {
	return &listCredentialsCommand{
		store: testStore,
	}
}

func NewDetectCredentialsCommandForTest(
	testStore jujuclient.CredentialStore,
	registeredProvidersFunc func() []string,
	allCloudsFunc func() (map[string]jujucloud.Cloud, error),
) *detectCredentialsCommand {
	return &detectCredentialsCommand{
		store: testStore,
		registeredProvdersFunc: registeredProvidersFunc,
		allCloudsFunc:          allCloudsFunc,
	}
}
