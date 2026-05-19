// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package upgrader_test

//go:generate go run github.com/canonical/gomock/mockgen -package upgrader_test -destination domain_mock_test.go github.com/juju/juju/apiserver/facades/agent/upgrader ControllerConfigGetter,ModelAgentService,ControllerNodeService,MachineService
//go:generate go run github.com/canonical/gomock/mockgen -package upgrader -destination watch_mocks_test.go github.com/juju/juju/apiserver/facades/agent/upgrader ModelAgentService
