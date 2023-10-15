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

//go:generate go run go.uber.org/mock/mockgen -package mocks -destination mocks/package_mock.go github.com/juju/juju/apiserver/common/networkingcommon BackingSpace,BackingSubnet,LinkLayerDevice,LinkLayerAddress,LinkLayerMachine,LinkLayerState,AddSubnetsState,LinkLayerAndSubnetsState

func Test(t *testing.T) {
	gc.TestingT(t)
}

type BaseSuite struct {
	jujutesting.IsolationSuite
}

func (s *BaseSuite) NewUpdateMachineLinkLayerOp(
	machine LinkLayerMachine, incoming network.InterfaceInfos, discoverSubnets bool, st AddSubnetsState,
) *updateMachineLinkLayerOp {
	return newUpdateMachineLinkLayerOp(machine, incoming, discoverSubnets, st)
}

func (s *BaseSuite) NewNetworkConfigAPI(
	st LinkLayerAndSubnetsState,
	getModelOp func(machine LinkLayerMachine, incoming network.InterfaceInfos) state.ModelOperation,
) *NetworkConfigAPI {
	return &NetworkConfigAPI{
		st:           st,
		getCanModify: common.AuthAlways(),
		getModelOp:   getModelOp,
	}
}
