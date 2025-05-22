// Copyright 2024 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package provisionertask_test

//go:generate go run go.uber.org/mock/mockgen -typed -package provisionertask_test -destination package_mock_test.go github.com/juju/juju/internal/provisionertask ControllerAPI,MachinesAPI
