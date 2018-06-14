// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package uniter

import (
	"github.com/juju/juju/apiserver/common"
	"github.com/juju/juju/apiserver/facade"
	"github.com/juju/juju/apiserver/facades/agent/meterstatus"
)

var (
	GetZone                = &getZone
	WatchStorageAttachment = watchStorageAttachment

	_ meterstatus.MeterStatus = (*UniterAPI)(nil)
)

type (
	Backend                    backend
	StorageStateInterface      storageInterface
	StorageVolumeInterface     = storageVolumeInterface
	StorageFilesystemInterface = storageFilesystemInterface
)

func NewStorageAPI(
	backend backend,
	storage storageAccess,
	resources facade.Resources,
	accessUnit common.GetAuthFunc,
) (*StorageAPI, error) {
	return newStorageAPI(backend, storage, resources, accessUnit)
}
