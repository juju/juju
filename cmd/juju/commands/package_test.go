// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package commands_test

import (
	"os"
	stdtesting "testing"

	"github.com/juju/tc"

	"github.com/juju/juju/internal/testhelpers"
)

//go:generate go run go.uber.org/mock/mockgen -typed -package commands -destination mockenvirons_test.go github.com/juju/juju/environs Environ,PrecheckJujuUpgradeStep
//go:generate go run go.uber.org/mock/mockgen -typed -package mocks -destination mocks/modelupgrader_mock.go github.com/juju/juju/cmd/juju/commands ModelUpgraderAPI
//go:generate go run go.uber.org/mock/mockgen -typed -package mocks -destination mocks/synctool_mock.go github.com/juju/juju/cmd/juju/commands SyncToolAPI
//go:generate go run go.uber.org/mock/mockgen -typed -package mocks -destination mocks/modelconfig_mock.go github.com/juju/juju/cmd/juju/commands ModelConfigAPI
//go:generate go run go.uber.org/mock/mockgen -typed -package mocks -destination mocks/jujuclient_mock.go github.com/juju/juju/jujuclient ClientStore,CookieJar


func TestMain(m *stdtesting.M) {
	testhelpers.ExecHelperProcess()
	os.Exit(m.Run())
}
