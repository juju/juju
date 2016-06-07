// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package apiserver

import (
	"github.com/juju/errors"
	"github.com/juju/utils/set"

	"github.com/juju/juju/apiserver/params"
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

// allowedMethodsDuringUpgrades stores api calls
// that are not blocked during the upgrade process
// as well as  their respective facade names.
// When needed, at some future point, this solution
// will need to be adjusted to cater for different
// facade versions as well.
var allowedMethodsDuringUpgrades = map[string]set.Strings{
	"Client": set.NewStrings(
		"FullStatus",          // for "juju status"
		"FindTools",           // for "juju upgrade-juju", before we can reset upgrade to re-run
		"AbortCurrentUpgrade", // for "juju upgrade-juju", so that we can reset upgrade to re-run

	),
	"SSHClient": set.NewStrings( // allow all SSH client related calls
		"PublicAddress",
		"PrivateAddress",
		"PublicKeys",
		"Proxy",
	),
	"Pinger": set.NewStrings(
		"Ping",
	),
	"Backups": set.NewStrings(
		"FinishRestore",
	),
}

func IsMethodAllowedDuringUpgrade(rootName, methodName string) bool {
	methods, ok := allowedMethodsDuringUpgrades[rootName]
	if !ok {
		return false
	}
	return methods.Contains(methodName)
}

// FindMethod returns UpgradeInProgressError for most API calls except those that are
// deemed safe or important for use while Juju is upgrading.
func (r *upgradingRoot) FindMethod(rootName string, version int, methodName string) (rpcreflect.MethodCaller, error) {
	caller, err := r.MethodFinder.FindMethod(rootName, version, methodName)
	if err != nil {
		return nil, errors.Trace(err)
	}
	if !IsMethodAllowedDuringUpgrade(rootName, methodName) {
		logger.Debugf("Facade (%v) method (%v) was called during the upgrade but it was blocked.", rootName, methodName)
		return nil, params.UpgradeInProgressError
	}
	return caller, nil
}
