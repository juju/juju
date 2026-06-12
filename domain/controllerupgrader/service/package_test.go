// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package service

//go:generate go run github.com/canonical/gomock/mockgen -package service -destination service_mock_test.go github.com/juju/juju/domain/controllerupgrader/service AgentBinaryFinder,ControllerState,ControllerModelState,SimpleStreamsAgentFinder,AgentFinderControllerState,AgentFinderControllerModelState
//go:generate go run github.com/canonical/gomock/mockgen -package service -destination environ_mock_test.go github.com/juju/juju/environs BootstrapEnviron
