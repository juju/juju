// Copyright 2021 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package common

import (
	stdtesting "testing"

	"github.com/juju/juju/testing"
)

//go:generate go run go.uber.org/mock/mockgen -package mocks -destination mocks/authorizer_mock.go github.com/juju/juju/apiserver/common Authorizer
//go:generate go run go.uber.org/mock/mockgen -package mocks -destination mocks/domain_mock.go github.com/juju/juju/apiserver/common ControllerConfigGetter
//go:generate go run go.uber.org/mock/mockgen -package mocks -destination mocks/credential_mock.go github.com/juju/juju/apiserver/common CredentialService
//go:generate go run go.uber.org/mock/mockgen -package mocks -destination mocks/controllerconfig_mock.go github.com/juju/juju/apiserver/common ControllerConfigState,ExternalControllerService
//go:generate go run go.uber.org/mock/mockgen -package mocks -destination mocks/tools_mock.go github.com/juju/juju/apiserver/common ToolsFinder,ToolsFindEntity,ToolsURLGetter,APIHostPortsForAgentsGetter,ToolsStorageGetter,AgentTooler
//go:generate go run go.uber.org/mock/mockgen -package mocks -destination mocks/storage.go github.com/juju/juju/state/binarystorage StorageCloser
//go:generate go run go.uber.org/mock/mockgen -package mocks -destination mocks/environs.go github.com/juju/juju/environs EnvironConfigGetter,BootstrapEnviron

func TestAll(t *stdtesting.T) {
	testing.MgoTestPackage(t)
}
