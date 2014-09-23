// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package apiserver

import (
	"github.com/juju/errors"
	"github.com/juju/utils/set"

	"github.com/juju/juju/rpc/rpcreflect"
	"github.com/juju/juju/state"
)

var inUpgradeError = errors.New("upgrade in progress - Juju functionality is limited")

type upgradingRoot struct {
	srvRoot
}

var _ apiRoot = (*upgradingRoot)(nil)

// newUpgradingRoot creates a root where all but a few "safe" API
// calls fail with inUpgradeError.
func newUpgradingRoot(root *initialRoot, entity state.Entity) *upgradingRoot {
	return &upgradingRoot{
		srvRoot: *newSrvRoot(root, entity),
	}
}

// FindMethod extended srvRoot.FindMethod. It returns inUpgradeError
// for most API calls except those that are deemed safe or important
// for use while Juju is upgrading.
func (r *upgradingRoot) FindMethod(rootName string, version int, methodName string) (rpcreflect.MethodCaller, error) {
	if _, _, err := r.lookupMethod(rootName, version, methodName); err != nil {
		return nil, err
	}
	if !IsMethodAllowedDuringUpgrade(rootName, methodName) {
		return nil, inUpgradeError
	}
	return r.srvRoot.FindMethod(rootName, version, methodName)
}

var allowedMethodsDuringUpgrades = set.NewStrings(
	"FullStatus",     // for "juju status"
	"EnvironmentGet", // for "juju ssh"
	"PrivateAddress", // for "juju ssh"
	"PublicAddress",  // for "juju ssh"
	"WatchDebugLog",  // for "juju debug-log"
)

func IsMethodAllowedDuringUpgrade(rootName, methodName string) bool {
	if rootName != "Client" {
		return false
	}
	return allowedMethodsDuringUpgrades.Contains(methodName)
}
