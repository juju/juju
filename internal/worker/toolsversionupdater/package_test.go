// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package toolsversionupdater

//go:generate go run go.uber.org/mock/mockgen -typed -package toolsversionupdater -destination service_mock_test.go github.com/juju/juju/internal/worker/toolsversionupdater ModelAgentService,AgentBinaryService
