// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package networker

import (
	"github.com/juju/loggo"

	"github.com/juju/juju/environs/network"
	"github.com/juju/juju/names"
	"github.com/juju/juju/state"
	"github.com/juju/juju/state/api/params"
	"github.com/juju/juju/state/apiserver/common"
)

var logger = loggo.GetLogger("juju.state.apiserver.networker")

// NetworkerAPI provides access to the Networker API facade.
type NetworkerAPI struct {
	st          *state.State
	resources   *common.Resources
	authorizer  common.Authorizer
	getAuthFunc common.GetAuthFunc
}

// NewNetworkerAPI creates a new client-side Networker API facade.
func NewNetworkerAPI(
	st *state.State,
	resources *common.Resources,
	authorizer common.Authorizer,
) (*NetworkerAPI, error) {
	if !authorizer.AuthMachineAgent() {
		return nil, common.ErrPerm
	}
	getAuthFunc := func() (common.AuthFunc, error) {
		authEntityTag := authorizer.GetAuthTag()

		return func(tag string) bool {
			if tag == authEntityTag {
				// A machine agent can always access its own machine.
				return true
			}
			_, id, err := names.ParseTag(tag, names.MachineTagKind)
			if err != nil {
				return false
			}
			parentId := state.ParentId(id)
			if parentId == "" {
				// Top-level machines.
				return false
			}
			// All containers with the authenticated machine as a
			// parent are accessible by it.
			return names.MachineTag(parentId) == authEntityTag
		}, nil
	}

	return &NetworkerAPI{
		st:          st,
		resources:   resources,
		authorizer:  authorizer,
		getAuthFunc: getAuthFunc,
	}, nil
}

func (n *NetworkerAPI) machineNetworkInfo(canAccess common.AuthFunc, tag string) ([]network.Info, error) {
	if !canAccess(tag) {
		return nil, common.ErrPerm
	}
	entity, err := n.st.FindEntity(tag)
	if err != nil {
		return nil, err
	}
	// The authorization function guarantees that the tag represents a machine.
	machine := entity.(*state.Machine)
	ifaces, err := machine.NetworkInterfaces()
	if err != nil {
		return nil, err
	}
	info := make([]network.Info, len(ifaces))
	for i, iface := range ifaces {
		nw, err := n.st.Network(iface.NetworkName())
		if err != nil {
			return nil, err
		}
		info[i] = network.Info{
			MACAddress:    iface.MACAddress(),
			CIDR:          nw.CIDR(),
			NetworkName:   iface.NetworkName(),
			ProviderId:    nw.ProviderId(),
			VLANTag:       nw.VLANTag(),
			InterfaceName: iface.RawInterfaceName(),
			IsVirtual:     iface.IsVirtual(),
		}
	}
	return info, nil
}

// Networks returns the list of networks with related interfaces for a given set of machines.
func (n *NetworkerAPI) MachineNetworkInfo(args params.Entities) (params.MachineNetworkInfoResults, error) {
	result := params.MachineNetworkInfoResults{
		Results: make([]params.MachineNetworkInfoResult, len(args.Entities)),
	}
	canAccess, err := n.getAuthFunc()
	if err != nil {
		return result, err
	}
	for i, entity := range args.Entities {
		result.Results[i].Info, err = n.machineNetworkInfo(canAccess, entity.Tag)
		result.Results[i].Error = common.ServerError(err)
	}
	return result, nil
}
