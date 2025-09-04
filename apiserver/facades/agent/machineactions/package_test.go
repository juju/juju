// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package machineactions

//go:generate go run go.uber.org/mock/mockgen -typed -package machineactions -destination package_mock_test.go github.com/juju/juju/apiserver/facades/agent/machineactions OperationService
