// Copyright 2021 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package ssh_test

import (
	stdtesting "testing"

	"github.com/juju/juju/testing"
)

//go:generate go run github.com/golang/mock/mockgen -package mocks -destination mocks/ssh_container_mock.go github.com/juju/juju/cmd/juju/ssh ApplicationAPI,CharmsAPI
//go:generate go run github.com/golang/mock/mockgen -package mocks -destination mocks/context_mock.go github.com/juju/juju/cmd/juju/ssh Context
//go:generate go run github.com/golang/mock/mockgen -package mocks -destination mocks/leaderapi_mock.go github.com/juju/juju/cmd/juju/ssh LeaderAPI
//go:generate go run github.com/golang/mock/mockgen -package mocks -destination mocks/sshclient_mock.go github.com/juju/juju/cmd/juju/ssh SSHClientAPI
//go:generate go run github.com/golang/mock/mockgen -package mocks -destination mocks/k8s_exec_mock.go github.com/juju/juju/caas/kubernetes/provider/exec Executor

func TestPackage(t *stdtesting.T) {
	testing.MgoTestPackage(t)
}
