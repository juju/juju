// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package networkingcommon

import (
	stdtesting "testing"

	"github.com/juju/tc"

	"github.com/juju/juju/apiserver/common"
	"github.com/juju/juju/core/network"
	"github.com/juju/juju/internal/testhelpers"
	"github.com/juju/juju/state"
)

//go:generate go run go.uber.org/mock/mockgen -typed -package mocks -destination mocks/package_mock.go github.com/juju/juju/apiserver/common/networkingcommon LinkLayerDevice,LinkLayerAddress,LinkLayerMachine,LinkLayerState,LinkLayerAndSubnetsState,NetworkService


type BaseSuite struct {
	testhelpers.IsolationSuite
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
