// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package firewaller_test

//go:generate go run github.com/canonical/gomock/mockgen -package firewaller_test -destination package_mock_test.go github.com/juju/juju/apiserver/facades/controller/firewaller ControllerConfigAPI
//go:generate go run github.com/canonical/gomock/mockgen -package firewaller_test -destination service_mock_test.go github.com/juju/juju/apiserver/facades/controller/firewaller ControllerConfigService,ModelConfigService,NetworkService,ApplicationService,MachineService,ModelInfoService
