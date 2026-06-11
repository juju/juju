// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package agentbinaryfetcher

//go:generate go run github.com/canonical/gomock/mockgen -package agentbinaryfetcher -destination service_mock_test.go github.com/juju/juju/internal/worker/agentbinaryfetcher ModelAgentService,AgentBinaryService
