// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package apiserver

import (
	"errors"

	"github.com/juju/utils/set"

	"github.com/juju/juju/rpc"
	"github.com/juju/juju/rpc/rpcreflect"
)

// UpgradingRoot restricts API calls to those supported during an upgrade.
type UpgradingRoot struct {
	rpc.MethodFinder
}

// NewUpgradingRoot returns a new UpgradingRoot.
func newUpgradingRoot(finder rpc.MethodFinder) *UpgradingRoot {
	return &UpgradingRoot{finder}
}

var inUpgradeError = errors.New("upgrade in progress - Juju functionality is limited")

var allowedMethodsDuringUpgrades = set.NewStrings(
	"Client.FullStatus",     // for "juju status"
	"Client.PrivateAddress", // for "juju ssh"
	"Client.PublicAddress",  // for "juju ssh"
	"Client.WatchDebugLog",  // for "juju debug-log"
)

func isMethodAllowedDuringUpgrade(rootName, methodName string) bool {
	fullName := rootName + "." + methodName
	return allowedMethodsDuringUpgrades.Contains(fullName)
}

func (r *UpgradingRoot) FindMethod(rootName string, version int, methodName string) (rpcreflect.MethodCaller, error) {
	if !isMethodAllowedDuringUpgrade(rootName, methodName) {
		return nil, inUpgradeError
	}
	return r.MethodFinder.FindMethod(rootName, version, methodName)
}
