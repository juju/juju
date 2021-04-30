// Copyright 2018 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package provisioner

import "github.com/juju/juju/apiserver/params"

// TODO (manadart 2019-09-26): This file should be deleted via these steps:
// - Rename provisioner_test.go to provisioner_integration_test.go
// - Relocate the provisionerMockSuite tests to provisioner_test *inside*
//   the provisioner package.
// - Instantiate these contexts directly instead of requiring these methods.

func NewPrepareOrGetContext(result params.MachineNetworkConfigResults) *prepareOrGetContext {
	return &prepareOrGetContext{result: result}
}

func NewContainerProfileContext(result params.ContainerProfileResults, modelName string) *containerProfileContext {
	return &containerProfileContext{result: result, modelName: modelName}
}
