// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package mongo

import (
	"github.com/juju/juju/service/common"
)

var (
	MakeJournalDirs = makeJournalDirs
	MongoConfigPath = &mongoConfigPath
	NoauthCommand   = noauthCommand
	ProcessSignal   = &processSignal

	SharedSecretPath = sharedSecretPath
	SSLKeyPath       = sslKeyPath

	NewConf = newConf

	HostWordSize   = &hostWordSize
	RuntimeGOOS    = &runtimeGOOS
	AvailSpace     = &availSpace
	MinOplogSizeMB = &minOplogSizeMB
	MaxOplogSizeMB = &maxOplogSizeMB
	PreallocFile   = &preallocFile

	DefaultOplogSize  = defaultOplogSize
	FsAvailSpace      = fsAvailSpace
	PreallocFileSizes = preallocFileSizes
	PreallocFiles     = preallocFiles
)

func PatchService(patchValue func(interface{}, interface{}), data *ServiceData) {
	patchValue(&discoverService, func(name string) (mongoService, error) {
		svc := &stubService{
			ServiceData: data,
			name:        name,
		}
		return svc, nil
	})
	patchValue(&newService, func(name string, conf common.Conf) (mongoService, error) {
		svc := &stubService{
			ServiceData: data,
			name:        name,
			conf:        conf,
		}
		return svc, nil
	})
}
