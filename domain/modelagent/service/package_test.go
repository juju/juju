// Copyright 2024 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package service_test

//go:generate go run go.uber.org/mock/mockgen -typed -package service -destination service_mock_test.go github.com/juju/juju/domain/modelagent/service AgentBinaryFinder,State
