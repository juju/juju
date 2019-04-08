// Copyright 2019 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package caascontrollerupgrader_test

import (
	"github.com/juju/testing"
	"github.com/juju/version"

	"github.com/juju/juju/caas"
	"github.com/juju/juju/core/watcher"
)

type mockUpgrader struct {
	testing.Stub

	desired version.Number
	actual  version.Binary
	watcher watcher.NotifyWatcher
}

func (m *mockUpgrader) DesiredVersion(tag string) (version.Number, error) {
	m.Stub.AddCall("DesiredVersion", tag)
	return m.desired, nil
}

func (m *mockUpgrader) SetVersion(tag string, v version.Binary) error {
	m.Stub.AddCall("SetVersion", tag, v)
	m.actual = v
	return nil
}

func (m *mockUpgrader) WatchAPIVersion(agentTag string) (watcher.NotifyWatcher, error) {
	return m.watcher, nil
}

type mockBroker struct {
	testing.Stub
	caas.Broker
}

func (m *mockBroker) Upgrade(appName string, vers version.Number) error {
	m.AddCall("Upgrade", appName, vers)
	return nil
}
