// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package service

//go:generate go run go.uber.org/mock/mockgen -typed -package service -destination controllerstore_mock_test.go github.com/juju/juju/domain/agentbinary/service ControllerStoreState
//go:generate go run go.uber.org/mock/mockgen -typed -package service -destination http_mock_test.go github.com/juju/juju/core/http HTTPClient
//go:generate go run go.uber.org/mock/mockgen -typed -package service -destination modelstore_mock_test.go github.com/juju/juju/domain/agentbinary/service ModelStoreState
//go:generate go run go.uber.org/mock/mockgen -typed -package service -destination objectstore_mock_test.go github.com/juju/juju/core/objectstore ModelObjectStoreGetter,ObjectStore,ModelStoreState
//go:generate go run go.uber.org/mock/mockgen -typed -package service -destination service_mock_test.go github.com/juju/juju/domain/agentbinary/service AgentBinaryModelState,AgentBinaryControllerState,ModelStore,ControllerStore,SimpleStreamStore
//go:generate go run go.uber.org/mock/mockgen -typed -package service -destination simplestreamstore_mock_test.go github.com/juju/juju/domain/agentbinary/service ProviderForAgentBinaryFinder
