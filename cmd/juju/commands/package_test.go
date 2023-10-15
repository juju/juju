// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package commands_test

import (
	"os"
	stdtesting "testing"

	jujutesting "github.com/juju/testing"

	"github.com/juju/juju/testing"
)

//go:generate go run go.uber.org/mock/mockgen -package commands -destination mockenvirons_test.go github.com/juju/juju/environs Environ,PrecheckJujuUpgradeStep
//go:generate go run go.uber.org/mock/mockgen -package commands -destination mockupgradeenvirons_test.go github.com/juju/juju/cmd/juju/commands UpgradePrecheckEnviron
// For ssh_container:
//go:generate go run go.uber.org/mock/mockgen -package mocks -destination mocks/ssh_container_mock.go github.com/juju/juju/cmd/juju/commands CloudCredentialAPI,ApplicationAPI,ModelAPI,CharmsAPI,ModelCommand,SSHControllerAPI
//go:generate go run go.uber.org/mock/mockgen -package mocks -destination mocks/context_mock.go github.com/juju/juju/cmd/juju/commands Context
//go:generate go run go.uber.org/mock/mockgen -package mocks -destination mocks/k8s_exec_mock.go github.com/juju/juju/caas/kubernetes/provider/exec Executor
// For ssh:
//go:generate go run go.uber.org/mock/mockgen -package mocks -destination mocks/statusapi_mock.go github.com/juju/juju/cmd/juju/commands StatusAPI
//go:generate go run go.uber.org/mock/mockgen -package mocks -destination mocks/leaderapi_mock.go github.com/juju/juju/cmd/juju/commands LeaderAPI
// For upgrademodel:
//go:generate go run go.uber.org/mock/mockgen -package mocks -destination mocks/controller_mock.go github.com/juju/juju/cmd/juju/commands ControllerAPI
//go:generate go run go.uber.org/mock/mockgen -package mocks -destination mocks/client_mock.go github.com/juju/juju/cmd/juju/commands ClientAPI
//go:generate go run go.uber.org/mock/mockgen -package mocks -destination mocks/modelmanager_mock.go github.com/juju/juju/cmd/juju/commands ModelManagerAPI
//go:generate go run go.uber.org/mock/mockgen -package mocks -destination mocks/modelupgrader_mock.go github.com/juju/juju/cmd/juju/commands ModelUpgraderAPI
//go:generate go run go.uber.org/mock/mockgen -package mocks -destination mocks/synctool_mock.go github.com/juju/juju/cmd/juju/commands SyncToolAPI
//go:generate go run go.uber.org/mock/mockgen -package mocks -destination mocks/modelconfig_mock.go github.com/juju/juju/cmd/juju/commands ModelConfigAPI
//go:generate go run go.uber.org/mock/mockgen -package mocks -destination mocks/jujuclient_mock.go github.com/juju/juju/jujuclient ClientStore,CookieJar
//go:generate go run go.uber.org/mock/mockgen -package mocks -destination mocks/sshclient_mock.go github.com/juju/juju/cmd/juju/commands SSHClientAPI

func Test(t *stdtesting.T) {
	testing.MgoTestPackage(t)
}

func TestMain(m *stdtesting.M) {
	jujutesting.ExecHelperProcess()
	os.Exit(m.Run())
}
