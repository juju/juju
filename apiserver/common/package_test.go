// Copyright 2021 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package common

import (
	stdtesting "testing"

	"github.com/juju/juju/testing"
)

//go:generate go run go.uber.org/mock/mockgen -typed -package mocks -destination mocks/authorizer_mock.go github.com/juju/juju/apiserver/common Authorizer
//go:generate go run go.uber.org/mock/mockgen -typed -package mocks -destination mocks/credential_mock.go github.com/juju/juju/apiserver/common CredentialService,CloudService
//go:generate go run go.uber.org/mock/mockgen -typed -package mocks -destination mocks/controllerconfig_mock.go github.com/juju/juju/apiserver/common ControllerConfigState,ControllerConfigService,ExternalControllerService
//go:generate go run go.uber.org/mock/mockgen -typed -package mocks -destination mocks/modelconfig_mock.go github.com/juju/juju/apiserver/common ModelConfigService
//go:generate go run go.uber.org/mock/mockgen -typed -package mocks -destination mocks/tools_mock.go github.com/juju/juju/apiserver/common ToolsFinder,ToolsFindEntity,ToolsURLGetter,APIHostPortsForAgentsGetter,ToolsStorageGetter,AgentTooler
//go:generate go run go.uber.org/mock/mockgen -typed -package mocks -destination mocks/storage.go github.com/juju/juju/state/binarystorage StorageCloser
//go:generate go run go.uber.org/mock/mockgen -typed -package mocks -destination mocks/environs.go github.com/juju/juju/environs EnvironConfigGetter,BootstrapEnviron
//go:generate go run go.uber.org/mock/mockgen -typed -package mocks -destination mocks/objectstore.go github.com/juju/juju/core/objectstore ObjectStore

func TestAll(t *stdtesting.T) {
	testing.MgoTestPackage(t)
}
