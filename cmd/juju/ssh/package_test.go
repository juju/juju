// Copyright 2021 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package ssh_test

import (
	stdtesting "testing"

	gc "gopkg.in/check.v1"
)

//go:generate go run github.com/golang/mock/mockgen -package mocks -destination mocks/package_mock.go github.com/juju/juju/cmd/juju/ssh Context,LeaderAPI,SSHClientAPI,SSHControllerAPI,StatusClientAPI,CloudCredentialAPI,ApplicationAPI,CharmAPI,ModelCommand
//go:generate go run github.com/golang/mock/mockgen -package mocks -destination mocks/k8s_exec_mock.go github.com/juju/juju/caas/kubernetes/provider/exec Executor

func TestPackage(t *stdtesting.T) {
	gc.TestingT(t)
}
