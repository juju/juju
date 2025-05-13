// Copyright 2019 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package caasupgrader_test

import (
	"context"

	"github.com/juju/juju/core/semversion"
	"github.com/juju/juju/core/watcher"
	"github.com/juju/juju/internal/testhelpers"
)

type mockUpgraderClient struct {
	testhelpers.Stub

	desired semversion.Number
	actual  semversion.Binary
	watcher watcher.NotifyWatcher
}

func (m *mockUpgraderClient) DesiredVersion(_ context.Context, tag string) (semversion.Number, error) {
	m.Stub.AddCall("DesiredVersion", tag)
	return m.desired, nil
}

func (m *mockUpgraderClient) SetVersion(_ context.Context, tag string, v semversion.Binary) error {
	m.Stub.AddCall("SetVersion", tag, v)
	m.actual = v
	return nil
}

func (m *mockUpgraderClient) WatchAPIVersion(_ context.Context, agentTag string) (watcher.NotifyWatcher, error) {
	return m.watcher, nil
}

type mockOperatorUpgrader struct {
	testhelpers.Stub
}

func (m *mockOperatorUpgrader) Upgrade(_ context.Context, appName string, vers semversion.Number) error {
	m.AddCall("Upgrade", appName, vers)
	return nil
}
