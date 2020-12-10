// Copyright 2020 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package ecs

import (
	"github.com/juju/juju/caas"
	"github.com/juju/juju/storage"
)

type (
	ECSEnviron = environ
)

var (
	CloudSpecToAWSConfig    = cloudSpecToAWSConfig
	NewEnviron              = newEnviron
	ValidateCloudCredential = validateCloudCredential
	NewNotifyWatcher        = newNotifyWatcher
)

func NewProvider() caas.ContainerEnvironProvider {
	return environProvider{}
}

func StorageProvider(e *environ) storage.Provider {
	return &storageProvider{e}
}
