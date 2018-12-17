// Copyright 2017 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package oracle

import (
	"github.com/juju/juju/storage"
)

var (
	DefaultTypes               = []storage.ProviderType{DefaultStorageProviderType}
	DefaultStorageProviderType = oracleStorageProviderType
	OracleVolumeType           = oracleVolumeType
	OracleLatencyPool          = latencyPool
	OracleCredentials          = credentials
	NewOracleVolumeSource      = newOracleVolumeSource
	NewOracleInstance          = newInstance
	GetImageName               = getImageName
	InstanceTypes              = instanceTypes
	FindInstanceSpec           = findInstanceSpec
	ParseImageName             = parseImageName
	CheckImageList             = checkImageList
)

func SetEnvironAPI(o *OracleEnviron, client EnvironAPI) {
	if o == nil {
		return
	}
	o.mutex.Lock()
	defer o.mutex.Unlock()
	o.client = client
}

func CreateHostname(o *OracleEnviron, id string) (string, error) {
	return o.namespace.Hostname(id)
}
