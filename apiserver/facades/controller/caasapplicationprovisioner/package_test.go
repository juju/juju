// Copyright 2017 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package caasapplicationprovisioner_test

//go:generate go run go.uber.org/mock/mockgen -typed -package caasapplicationprovisioner_test -destination service_mock_test.go github.com/juju/juju/apiserver/facades/controller/caasapplicationprovisioner ControllerConfigService,ModelConfigService,ModelInfoService,ApplicationService,StatusService,ControllerNodeService,RemovalService
