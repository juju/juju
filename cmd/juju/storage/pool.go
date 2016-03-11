// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package storage

import (
	"github.com/juju/juju/apiserver/params"
)

// PoolCommandBase is a helper base structure for pool commands.
type PoolCommandBase struct {
	StorageCommandBase
}

// PoolInfo defines the serialization behaviour of the storage pool information.
type PoolInfo struct {
	Provider string                 `yaml:"provider" json:"provider"`
	Attrs    map[string]interface{} `yaml:"attrs,omitempty" json:"attrs,omitempty"`
}

func formatPoolInfo(all []params.StoragePool) map[string]PoolInfo {
	output := make(map[string]PoolInfo)
	for _, one := range all {
		output[one.Name] = PoolInfo{
			Provider: one.Provider,
			Attrs:    one.Attrs,
		}
	}
	return output
}
