// Copyright 2018 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package ecs

import (
	"testing"

	gc "gopkg.in/check.v1"

	"github.com/juju/juju/caas"
	"github.com/juju/juju/storage"
)

func TestAll(t *testing.T) {
	gc.TestingT(t)
}

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
