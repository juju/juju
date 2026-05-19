// Copyright 2023 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package deployer

//go:generate go run github.com/canonical/gomock/mockgen -package deployer -destination domain_mock_test.go github.com/juju/juju/apiserver/facades/agent/deployer ControllerConfigGetter,ApplicationService,AgentPasswordService
