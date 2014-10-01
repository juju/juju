// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package apiserver

import (
	"errors"

	"github.com/juju/utils/set"

	"github.com/juju/juju/rpc"
	"github.com/juju/juju/rpc/rpcreflect"
)

// upgradingRoot restricts API calls to those supported during an upgrade.
type upgradingRoot struct {
	rpc.MethodFinder
}

// newUpgradingRoot returns a new upgradingRoot.
func newUpgradingRoot(finder rpc.MethodFinder) *upgradingRoot {
	return &upgradingRoot{finder}
}

var inUpgradeError = errors.New("upgrade in progress - Juju functionality is limited")

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

// FindMethod returns inUpgradeError for most API calls except those that are
// deemed safe or important for use while Juju is upgrading.
func (r *upgradingRoot) FindMethod(rootName string, version int, methodName string) (rpcreflect.MethodCaller, error) {
	caller, err := r.MethodFinder.FindMethod(rootName, version, methodName)
	if err != nil {
		return nil, err
	}
	if !IsMethodAllowedDuringUpgrade(rootName, methodName) {
		return nil, inUpgradeError
	}
	return caller, nil
}
