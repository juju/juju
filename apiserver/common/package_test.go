// Copyright 2021 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package common_test

import (
	stdtesting "testing"

	"github.com/juju/juju/internal/testing"
)

// TODO: Move all generated mocks out of the mocks directory and directly into
// the common or common_test package.

//go:generate go run go.uber.org/mock/mockgen -typed -package mocks -destination mocks/clock_mock.go github.com/juju/clock Clock
//go:generate go run go.uber.org/mock/mockgen -typed -package mocks -destination mocks/authorizer_mock.go github.com/juju/juju/apiserver/common Authorizer
//go:generate go run go.uber.org/mock/mockgen -typed -package mocks -destination mocks/common_mock.go github.com/juju/juju/apiserver/common BlockCommandService,CloudService,ControllerConfigState,ControllerConfigService,ExternalControllerService,ToolsFinder,ToolsFindEntity,ToolsURLGetter,APIHostPortsForAgentsGetter,ToolsStorageGetter,AgentTooler,ModelAgentService,MachineRebootService,EnsureDeadMachineService,WatchableMachineService,UnitStateService,MachineService,StatusService,LeadershipPinningBackend,LeadershipMachine
//go:generate go run go.uber.org/mock/mockgen -typed -package mocks -destination mocks/storage_mock.go github.com/juju/juju/state/binarystorage StorageCloser
//go:generate go run go.uber.org/mock/mockgen -typed -package mocks -destination mocks/state_mocks.go github.com/juju/juju/state EntityFinder,Entity
//go:generate go run go.uber.org/mock/mockgen -typed -package mocks -destination mocks/environs_mock.go github.com/juju/juju/environs BootstrapEnviron
//go:generate go run go.uber.org/mock/mockgen -typed -package mocks -destination mocks/status_mock.go github.com/juju/juju/core/status StatusGetter,StatusSetter
//go:generate go run go.uber.org/mock/mockgen -typed -package mocks -destination mocks/objectstore_mock.go github.com/juju/juju/core/objectstore ObjectStore
//go:generate go run go.uber.org/mock/mockgen -typed -package mocks -destination mocks/facade_mock.go github.com/juju/juju/apiserver/facade WatcherRegistry

func TestAll(t *stdtesting.T) {
	testing.MgoTestPackage(t)
}
