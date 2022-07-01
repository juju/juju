// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package commands_test

import (
	stdtesting "testing"

	"github.com/juju/juju/v2/testing"
)

//go:generate go run github.com/golang/mock/mockgen -package commands -destination mockenvirons_test.go github.com/juju/juju/environs Environ,PrecheckJujuUpgradeStep
//go:generate go run github.com/golang/mock/mockgen -package commands -destination mockupgradeenvirons_test.go github.com/juju/juju/cmd/juju/commands UpgradePrecheckEnviron
// For ssh_container:
//go:generate go run github.com/golang/mock/mockgen -package mocks -destination mocks/ssh_container_mock.go github.com/juju/juju/cmd/juju/commands CloudCredentialAPI,ApplicationAPI,ModelAPI,CharmsAPI
//go:generate go run github.com/golang/mock/mockgen -package mocks -destination mocks/context_mock.go github.com/juju/juju/cmd/juju/commands Context
//go:generate go run github.com/golang/mock/mockgen -package mocks -destination mocks/k8s_exec_mock.go github.com/juju/juju/caas/kubernetes/provider/exec Executor
// For ssh:
//go:generate go run github.com/golang/mock/mockgen -package mocks -destination mocks/statusapi_mock.go github.com/juju/juju/cmd/juju/commands StatusAPI
//go:generate go run github.com/golang/mock/mockgen -package mocks -destination mocks/leaderapi_mock.go github.com/juju/juju/cmd/juju/commands LeaderAPI
// For upgrademodel:
//go:generate go run github.com/golang/mock/mockgen -package mocks -destination mocks/controller_mock.go github.com/juju/juju/cmd/juju/commands ControllerAPI
//go:generate go run github.com/golang/mock/mockgen -package mocks -destination mocks/client_mock.go github.com/juju/juju/cmd/juju/commands ClientAPI
//go:generate go run github.com/golang/mock/mockgen -package mocks -destination mocks/modelmanager_mock.go github.com/juju/juju/cmd/juju/commands ModelManagerAPI
//go:generate go run github.com/golang/mock/mockgen -package mocks -destination mocks/modelupgrader_mock.go github.com/juju/juju/cmd/juju/commands ModelUpgraderAPI

func TestPackage(t *stdtesting.T) {
	testing.MgoTestPackage(t)
}
