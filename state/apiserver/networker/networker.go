// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package networker

import (
	"github.com/juju/loggo"
	"github.com/juju/names"

	"github.com/juju/juju/environs/network"
	"github.com/juju/juju/state"
	"github.com/juju/juju/state/api/params"
	"github.com/juju/juju/state/apiserver/common"
)

var logger = loggo.GetLogger("juju.state.apiserver.networker")

// NetworkerAPI provides access to the Networker API facade.
type NetworkerAPI struct {
	st          *state.State
	authorizer  common.Authorizer
	getAuthFunc common.GetAuthFunc
}

// NewNetworkerAPI creates a new client-side Networker API facade.
func NewNetworkerAPI(
	st *state.State,
	_ *common.Resources,
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
				// Only machine tags are allowed.
				return false
			}
			for parentId := state.ParentId(id); parentId != ""; parentId = state.ParentId(parentId) {
				// Until a top-level machine is reached.
				if names.MachineTag(parentId) == authEntityTag {
					// All containers with the authenticated machine as a
					// parent are accessible by it.
					return true
				}
			}
			// Not found authorized machine agent among ancestors of the current one.
			return false
		}, nil
	}

	return &NetworkerAPI{
		st:          st,
		authorizer:  authorizer,
		getAuthFunc: getAuthFunc,
	}, nil
}

func (n *NetworkerAPI) oneMachineInfo(id string) ([]network.Info, error) {
	machine, err := n.st.Machine(id)
	if err != nil {
		return nil, err
	}
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
	id := ""
	for i, entity := range args.Entities {
		if !canAccess(entity.Tag) {
			err = common.ErrPerm
		} else {
			_, id, err = names.ParseTag(entity.Tag, names.MachineTagKind)
			if err == nil {
				result.Results[i].Info, err = n.oneMachineInfo(id)
			}
		}
		result.Results[i].Error = common.ServerError(err)
	}
	return result, nil
}
