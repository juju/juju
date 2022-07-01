// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package azure

import (
	legacystorage "github.com/Azure/azure-sdk-for-go/services/storage/mgmt/2017-10-01/storage"

	"github.com/juju/juju/v2/environs"
)

func DisableLegacyStorage(env environs.Environ) {
	azureEnv := env.(*azureEnviron)
	azureEnv.storageAccount = new(*legacystorage.Account)
}

const ComputeAPIVersion = computeAPIVersion
const NetworkAPIVersion = networkAPIVersion
