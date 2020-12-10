// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package mongo

import (
	"github.com/golang/mock/gomock"
)

var (
	SharedSecretPath = sharedSecretPath
	SSLKeyPath       = sslKeyPath

	InstallMongo = &installMongo
	SupportsIPv6 = &supportsIPv6

	HostWordSize     = &hostWordSize
	RuntimeGOOS      = &runtimeGOOS
	AvailSpace       = &availSpace
	SmallOplogSizeMB = &smallOplogSizeMB

	DefaultOplogSize = defaultOplogSize
	FsAvailSpace     = fsAvailSpace
)

func SysctlEditableEnsureServer(args EnsureServerParams, sysctlFiles map[string]string) error {
	return ensureServer(args, sysctlFiles)
}

func NewMongodFinderWithMockSearch(ctrl *gomock.Controller) (*MongodFinder, *MockSearchTools) {
	tools := NewMockSearchTools(ctrl)
	return &MongodFinder{
		search: tools,
	}, tools
}
