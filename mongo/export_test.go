// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package mongo

import (
	"github.com/juju/juju/service"
	"github.com/juju/juju/service/common"
)

var (
	MakeJournalDirs = makeJournalDirs
	MongoConfigPath = &mongoConfigPath
	NoauthCommand   = noauthCommand
	ProcessSignal   = &processSignal

	SharedSecretPath = sharedSecretPath
	SSLKeyPath       = sslKeyPath

	NewServices    = &newServices
	NewService     = &newService
	InstallService = &installService
	MongodPath     = &mongodPath

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

func NewServicesClosure(s services) func(string) (services, error) {
	return func(string) (services, error) {
		return s, nil
	}
}

func NewTestService(name string, conf common.Conf, raw service.Service) Service {
	return Service{name, conf, raw}
}
