// Copyright 2022 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package stateconverter

import (
	stdtesting "testing"

	"github.com/juju/names/v6"
	"github.com/juju/tc"

	"github.com/juju/juju/core/logger"
	"github.com/juju/juju/core/watcher"
)

//go:generate go run go.uber.org/mock/mockgen -typed -package mocks -destination mocks/machiner_mock.go github.com/juju/juju/internal/worker/stateconverter Machiner,Machine
//go:generate go run go.uber.org/mock/mockgen -typed -package mocks -destination mocks/dependency_mock.go github.com/juju/worker/v4/dependency Getter
//go:generate go run go.uber.org/mock/mockgen -typed -package mocks -destination mocks/agent_mock.go github.com/juju/juju/agent Agent,Config


func NewConverterForTest(machine Machine, machiner Machiner, logger logger.Logger) watcher.NotifyHandler {
	return &converter{
		machineTag: names.NewMachineTag("3"),
		machiner:   machiner,
		machine:    machine,
		logger:     logger,
	}
}
