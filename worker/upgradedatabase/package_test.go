// Copyright 2019 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package upgradedatabase

import (
	"testing"

	gc "gopkg.in/check.v1"
)

//go:generate go run go.uber.org/mock/mockgen -package mocks -destination mocks/package.go github.com/juju/juju/worker/upgradedatabase Logger,Pool,UpgradeInfo,Clock
//go:generate go run go.uber.org/mock/mockgen -package mocks -destination mocks/lock.go github.com/juju/juju/worker/gate Lock
//go:generate go run go.uber.org/mock/mockgen -package mocks -destination mocks/agent.go github.com/juju/juju/agent Agent,Config,ConfigSetter
//go:generate go run go.uber.org/mock/mockgen -package mocks -destination mocks/watcher.go github.com/juju/juju/state NotifyWatcher

func Test(t *testing.T) {
	gc.TestingT(t)
}
