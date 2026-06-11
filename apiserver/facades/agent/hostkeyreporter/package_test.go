// Copyright 2022 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package hostkeyreporter

//go:generate go run github.com/canonical/gomock/mockgen -package hostkeyreporter -destination domain_mock_test.go github.com/juju/juju/apiserver/facades/agent/hostkeyreporter MachineService
