// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package commands_test

import (
	stdtesting "testing"

	"github.com/juju/juju/component/all"
	"github.com/juju/juju/testing"
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

func init() {
	if err := all.RegisterForClient(); err != nil {
		panic(err)
	}
}

func TestPackage(t *stdtesting.T) {
	testing.MgoTestPackage(t)
}
