// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package service

//go:generate go run go.uber.org/mock/mockgen -typed -package service -destination http_mock_test.go github.com/juju/juju/core/http HTTPClient
//go:generate go run go.uber.org/mock/mockgen -typed -package service -destination objectstore_mock_test.go github.com/juju/juju/core/objectstore ModelObjectStoreGetter,ObjectStore
//go:generate go run go.uber.org/mock/mockgen -typed -package service -destination service_mock_test.go github.com/juju/juju/domain/agentbinary/service AgentBinaryLocalStore,AgentBinaryDiscoverableStore,ModelState,ControllerState
//go:generate go run go.uber.org/mock/mockgen -typed -package service -destination simplestreamstore_mock_test.go github.com/juju/juju/domain/agentbinary/service ProviderForAgentBinaryFinder
//go:generate go run go.uber.org/mock/mockgen -typed -package service -destination store_mock_test.go github.com/juju/juju/domain/agentbinary/service AgentBinaryStoreState
