// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package cloud

import (
	"github.com/juju/juju/jujuclient"
)

func NewListCredentialsCommandForTest(testStore jujuclient.CredentialsGetter) *listCredentialsCommand {
	return &listCredentialsCommand{
		store: testStore,
	}
}

func NewDetectCredentialsCommandForTest(testStore jujuclient.CredentialsStore) *detectCredentialsCommand {
	return &detectCredentialsCommand{
		store: testStore,
	}
}
