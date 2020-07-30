// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package networkingcommon

import (
	"testing"

	"github.com/juju/juju/state"

	jujutesting "github.com/juju/testing"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/apiserver/common"
	"github.com/juju/juju/core/network"
)

//go:generate go run github.com/golang/mock/mockgen -package mocks -destination mocks/package_mock.go github.com/juju/juju/apiserver/common/networkingcommon BackingSpace,BackingSubnet,LinkLayerDevice,LinkLayerAddress,LinkLayerMachine,LinkLayerState

func TestPackage(t *testing.T) {
	gc.TestingT(t)
}

type BaseSuite struct {
	jujutesting.IsolationSuite
}

func (s *BaseSuite) NewUpdateMachineLinkLayerOp(
	machine LinkLayerMachine, incoming network.InterfaceInfos,
) *updateMachineLinkLayerOp {
	return newUpdateMachineLinkLayerOp(machine, incoming)
}

func (s *BaseSuite) NewNetworkConfigAPI(
	st LinkLayerState,
	getModelOp func(machine LinkLayerMachine, incoming network.InterfaceInfos) state.ModelOperation,
) *NetworkConfigAPI {
	return &NetworkConfigAPI{
		st:           st,
		getCanModify: common.AuthAlways(),
		getModelOp:   getModelOp,
	}
}
