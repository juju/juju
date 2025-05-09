// Copyright 2021 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package ssh_test

import (
	stdtesting "testing"

	"github.com/juju/juju/testing"
)

//go:generate go run go.uber.org/mock/mockgen -typed -package mocks -destination mocks/package_mock.go github.com/juju/juju/cmd/juju/ssh Context,LeaderAPI,SSHClientAPI,SSHAPIJump,SSHControllerAPI,CloudCredentialAPI,ApplicationAPI,CharmsAPI,ModelCommand
//go:generate go run go.uber.org/mock/mockgen -typed -package mocks -destination mocks/k8s_exec_mock.go github.com/juju/juju/caas/kubernetes/provider/exec Executor

func TestPackage(t *stdtesting.T) {
	testing.MgoTestPackage(t)
}
