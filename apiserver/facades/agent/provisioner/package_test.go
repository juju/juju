// Copyright 2024 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package provisioner

//go:generate go run go.uber.org/mock/mockgen -typed -package provisioner -destination service_mock_test.go github.com/juju/juju/apiserver/facades/agent/provisioner AgentProvisionerService,KeyUpdaterService,ApplicationService,ControllerConfigService,MachineService,StatusService,NetworkService,RemovalService
//go:generate go run go.uber.org/mock/mockgen -typed -package provisioner -destination common_mock_test.go github.com/juju/juju/apiserver/common APIAddressAccessor

func ptr[T any](v T) *T {
	return &v
}
