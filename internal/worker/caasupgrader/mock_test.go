// Copyright 2019 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package caasupgrader_test

import (
	"context"

	"github.com/juju/testing"

	"github.com/juju/juju/core/watcher"
	"github.com/juju/juju/internal/version"
)

type mockUpgraderClient struct {
	testing.Stub

	desired version.Number
	actual  version.Binary
	watcher watcher.NotifyWatcher
}

func (m *mockUpgraderClient) DesiredVersion(_ context.Context, tag string) (version.Number, error) {
	m.Stub.AddCall("DesiredVersion", tag)
	return m.desired, nil
}

func (m *mockUpgraderClient) SetVersion(_ context.Context, tag string, v version.Binary) error {
	m.Stub.AddCall("SetVersion", tag, v)
	m.actual = v
	return nil
}

func (m *mockUpgraderClient) WatchAPIVersion(_ context.Context, agentTag string) (watcher.NotifyWatcher, error) {
	return m.watcher, nil
}

type mockOperatorUpgrader struct {
	testing.Stub
}

func (m *mockOperatorUpgrader) Upgrade(_ context.Context, appName string, vers version.Number) error {
	m.AddCall("Upgrade", appName, vers)
	return nil
}
