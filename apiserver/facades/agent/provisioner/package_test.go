// Copyright 2024 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package provisioner_test

import (
	"testing"

	gc "gopkg.in/check.v1"
)

//go:generate go run go.uber.org/mock/mockgen -typed -package provisioner -destination service_mock_test.go github.com/juju/juju/apiserver/facades/agent/provisioner AgentProvisionerService,KeyUpdaterService,ApplicationService
//go:generate go run go.uber.org/mock/mockgen -typed -package provisioner -destination interface_mock_test.go github.com/juju/juju/apiserver/facades/agent/provisioner Machine,BridgePolicy,Unit,Application
//go:generate go run go.uber.org/mock/mockgen -typed -package provisioner -destination containerizer_mock_test.go github.com/juju/juju/internal/network/containerizer LinkLayerDevice

func TestPackage(t *testing.T) {
	gc.TestingT(t)
}
