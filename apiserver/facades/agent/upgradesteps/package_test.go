// Copyright 2019 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package upgradesteps_test

import (
	"testing"

	gc "gopkg.in/check.v1"
)

//go:generate go run go.uber.org/mock/mockgen -package upgradesteps_test -destination upgradesteps_mock_test.go github.com/juju/juju/apiserver/facades/agent/upgradesteps ControllerConfigGetter,UpgradeStepsState,Machine,Unit
//go:generate go run go.uber.org/mock/mockgen -package upgradesteps_test -destination state_mock_test.go github.com/juju/juju/state EntityFinder,Entity
//go:generate go run go.uber.org/mock/mockgen -package upgradesteps_test -destination facade_mock_test.go github.com/juju/juju/apiserver/facade Resources,Authorizer,WatcherRegistry

func TestPackage(t *testing.T) {
	gc.TestingT(t)
}
