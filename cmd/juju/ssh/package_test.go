// Copyright 2021 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package ssh_test

//go:generate go run go.uber.org/mock/mockgen -typed -package mocks -destination mocks/package_mock.go github.com/juju/juju/cmd/juju/ssh Context,LeaderAPI,SSHClientAPI,SSHControllerAPI,StatusClientAPI,CloudCredentialAPI,ApplicationAPI,CharmAPI,ModelCommand
//go:generate go run go.uber.org/mock/mockgen -typed -package mocks -destination mocks/k8s_exec_mock.go github.com/juju/juju/internal/provider/kubernetes/exec Executor
