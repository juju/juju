// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package mongo

import (
	"github.com/golang/mock/gomock"

	"github.com/juju/juju/service/common"
	svctesting "github.com/juju/juju/service/common/testing"
)

var (
	MakeJournalDirs = makeJournalDirs
	MongoConfigPath = &mongoConfigPath

	SharedSecretPath = sharedSecretPath
	SSLKeyPath       = sslKeyPath

	NewConf      = newConf
	GenerateConf = generateConfig

	HostWordSize     = &hostWordSize
	RuntimeGOOS      = &runtimeGOOS
	AvailSpace       = &availSpace
	SmallOplogSizeMB = &smallOplogSizeMB
	PreallocFile     = &preallocFile

	DefaultOplogSize  = defaultOplogSize
	FsAvailSpace      = fsAvailSpace
	PreallocFileSizes = preallocFileSizes
	PreallocFiles     = preallocFiles
)

func PatchService(patchValue func(interface{}, interface{}), data *svctesting.FakeServiceData) {
	patchValue(&discoverService, func(name string) (mongoService, error) {
		svc := svctesting.NewFakeService(name, common.Conf{})
		svc.FakeServiceData = data
		return svc, nil
	})
	patchValue(&newService, func(name string, _ bool, conf common.Conf) (mongoService, error) {
		svc := svctesting.NewFakeService(name, conf)
		svc.FakeServiceData = data
		return svc, nil
	})
}

func SysctlEditableEnsureServer(args EnsureServerParams, sysctlFiles map[string]string) (Version, error) {
	return ensureServer(args, sysctlFiles)
}

func NewMongodFinderWithMockSearch(ctrl *gomock.Controller) (*MongodFinder, *MockSearchTools) {
	tools := NewMockSearchTools(ctrl)
	return &MongodFinder{
		search: tools,
	}, tools
}
