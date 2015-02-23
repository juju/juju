// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package mongo

import (
	"github.com/juju/juju/service"
	"github.com/juju/juju/service/common"
)

var (
	MakeJournalDirs = makeJournalDirs
	MongoConfigPath = &configPath
	NoauthCommand   = noauthCommand
	ProcessSignal   = &processSignal

	SharedSecretPath = sharedSecretPath
	SSLKeyPath       = sslKeyPath

	NewAdminService = &newAdminService
	NewService      = &newService
	InstallService  = &installService
	MongodPath      = &mongodPath

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

func NewServiceClosure(s adminService) func(string, string) (adminService, error) {
	return func(string, string) (adminService, error) {
		return s, nil
	}
}

func NewTestService(name string, conf common.Conf, raw service.Service) Service {
	return Service{name, conf, raw}
}
