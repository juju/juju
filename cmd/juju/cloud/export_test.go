// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package cloud

import (
	"github.com/juju/juju/jujuclient"
)

func NewListCredentialsCommandForTest(testStore jujuclient.CredentialGetter) *listCredentialsCommand {
	return &listCredentialsCommand{
		store: testStore,
	}
}

func NewDetectCredentialsCommandForTest(testStore jujuclient.CredentialStore) *detectCredentialsCommand {
	return &detectCredentialsCommand{
		store: testStore,
	}
}

func NewsetDefaultRegionCommandForTest(testStore jujuclient.CredentialStore) *setDefaultRegionCommand {
	return &setDefaultRegionCommand{
		store: testStore,
	}
}

func NewSetDefaultCredentialCommandForTest(testStore jujuclient.CredentialStore) *setDefaultCredentialCommand {
	return &setDefaultCredentialCommand{
		store: testStore,
	}
}
