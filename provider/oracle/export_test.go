// Copyright 2017 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package oracle

import "github.com/juju/juju/storage"

var (
	DefaultTypes               = []storage.ProviderType{oracleStorageProvideType}
	DefaultProvider            = &environProvider{}
	NewOracleEnviron           = newOracleEnviron
	DefaultStorageProviderType = oracleStorageProvideType
	OracleVolumeType           = oracleVolumeType
	OracleLatencyPool          = latencyPool
	OracleCloudSchema          = cloudSchema
	OracleCredentials          = credentials
)
