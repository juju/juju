// Copyright 2022 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package stateconverter

//go:generate go run go.uber.org/mock/mockgen -typed -package stateconverter -destination clients_mock_test.go github.com/juju/juju/internal/worker/stateconverter MachineClient,Machine,AgentClient
//go:generate go run go.uber.org/mock/mockgen -typed -package stateconverter -destination dependency_mock_test.go github.com/juju/worker/v4/dependency Getter
//go:generate go run go.uber.org/mock/mockgen -typed -package stateconverter -destination agent_mock_test.go github.com/juju/juju/agent Agent,Config,ConfigSetter
