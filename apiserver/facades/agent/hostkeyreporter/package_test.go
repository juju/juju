// Copyright 2022 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package hostkeyreporter

//go:generate go run go.uber.org/mock/mockgen -typed -package hostkeyreporter -destination domain_mock_test.go github.com/juju/juju/apiserver/facades/agent/hostkeyreporter MachineService
