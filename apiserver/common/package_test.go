// Copyright 2021 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package common_test

// TODO: Move all generated mocks out of the mocks directory and directly into
// the common or common_test package.

//go:generate go run go.uber.org/mock/mockgen -typed -package mocks -destination mocks/clock_mock.go github.com/juju/clock Clock
//go:generate go run go.uber.org/mock/mockgen -typed -package mocks -destination mocks/authorizer_mock.go github.com/juju/juju/apiserver/common Authorizer
//go:generate go run go.uber.org/mock/mockgen -typed -package mocks -destination mocks/common_mock.go github.com/juju/juju/apiserver/common BlockCommandService,CloudService,ControllerConfigService,ExternalControllerService,ToolsFinder,ToolsURLGetter,APIHostPortsForAgentsGetter,ModelAgentService,MachineRebootService,WatchableMachineService,UnitStateService,ApplicationService,MachineService,StatusService,AgentPasswordService,AgentBinaryService,ModelService
//go:generate go run go.uber.org/mock/mockgen -typed -package common -destination package_mock.go github.com/juju/juju/apiserver/common APIAddressAccessor
//go:generate go run go.uber.org/mock/mockgen -typed -package mocks -destination mocks/environs_mock.go github.com/juju/juju/environs BootstrapEnviron
//go:generate go run go.uber.org/mock/mockgen -typed -package mocks -destination mocks/status_mock.go github.com/juju/juju/core/status StatusGetter,StatusSetter
//go:generate go run go.uber.org/mock/mockgen -typed -package mocks -destination mocks/objectstore_mock.go github.com/juju/juju/core/objectstore ObjectStore
//go:generate go run go.uber.org/mock/mockgen -typed -package mocks -destination mocks/facade_mock.go github.com/juju/juju/apiserver/facade WatcherRegistry
