// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package diskmanager

import (
	"github.com/juju/names/v5"

	"github.com/juju/juju/apiserver/common"
	"github.com/juju/juju/apiserver/facade"
)

func NewDiskManagerAPIForTest(auth facade.Authorizer, blockDeviceUpdater blockDeviceUpdater) *DiskManagerAPI {
	return &DiskManagerAPI{
		blockDeviceUpdater: blockDeviceUpdater,
		authorizer:         auth,
		getAuthFunc: func() (common.AuthFunc, error) {
			return func(tag names.Tag) bool {
				return tag == auth.GetAuthTag()
			}, nil
		},
	}
}

var (
	NewDiskManagerAPI = newDiskManagerAPI
)
