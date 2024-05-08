// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package networkingcommon

import (
	"testing"

	jujutesting "github.com/juju/testing"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/apiserver/common"
	"github.com/juju/juju/core/network"
	"github.com/juju/juju/state"
)

//go:generate go run go.uber.org/mock/mockgen -typed -package mocks -destination mocks/package_mock.go github.com/juju/juju/apiserver/common/networkingcommon LinkLayerDevice,LinkLayerAddress,LinkLayerMachine,LinkLayerState,LinkLayerAndSubnetsState,NetworkService

func TestPackage(t *testing.T) {
	gc.TestingT(t)
}

type BaseSuite struct {
	jujutesting.IsolationSuite
}

func (s *BaseSuite) NewUpdateMachineLinkLayerOp(
	machine LinkLayerMachine, networkService NetworkService, incoming network.InterfaceInfos, discoverSubnets bool,
) *updateMachineLinkLayerOp {
	return newUpdateMachineLinkLayerOp(machine, networkService, incoming, discoverSubnets)
}

func (s *BaseSuite) NewNetworkConfigAPI(
	st LinkLayerAndSubnetsState,
	networkService NetworkService,
	getModelOp func(machine LinkLayerMachine, incoming network.InterfaceInfos) state.ModelOperation,
) *NetworkConfigAPI {
	return &NetworkConfigAPI{
		st:             st,
		networkService: networkService,
		getCanModify:   common.AuthAlways(),
		getModelOp:     getModelOp,
	}
}
