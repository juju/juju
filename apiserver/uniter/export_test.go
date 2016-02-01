// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package uniter

import (
	"github.com/juju/juju/apiserver/common"
	"github.com/juju/juju/apiserver/meterstatus"
)

var (
	GetZone = &getZone

	_ meterstatus.MeterStatus = (*UniterAPIV3)(nil)
)

type StorageStateInterface storageStateInterface

func NewStorageAPI(
	st StorageStateInterface,
	resources *common.Resources,
	accessUnit common.GetAuthFunc,
) (*StorageAPI, error) {
	return newStorageAPI(storageStateInterface(st), resources, accessUnit)
}
