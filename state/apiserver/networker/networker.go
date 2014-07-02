// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package networker

import (
	"github.com/juju/loggo"
	"github.com/juju/names"

	"github.com/juju/juju/network"
	"github.com/juju/juju/state"
	"github.com/juju/juju/state/api/params"
	"github.com/juju/juju/state/apiserver/common"
	"github.com/juju/juju/state/watcher"
)

func init() {
	// TODO: When the client can handle new versions, this should really be
	// registered as version 1, since it was not present in the API in Juju
	// 1.18
	common.RegisterStandardFacade("Networker", 0, NewNetworkerAPI)
}

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
		authEntityTag := authorizer.GetAuthTag().String()

		return func(tag string) bool {
			if tag == authEntityTag {
				// A machine agent can always access its own machine.
				return true
			}
			t, err := names.ParseMachineTag(tag)
			if err != nil {
				// Only machine tags are allowed.
				return false
			}
			id := t.Id()
			for parentId := state.ParentId(id); parentId != ""; parentId = state.ParentId(parentId) {
				// Until a top-level machine is reached.
				// TODO(dfc) comparing the two interfaces caused a compiler crash with
				// gcc version 4.9.0 (Ubuntu 4.9.0-7ubuntu1). Work around the issue
				// by comparing by string value.
				if names.NewMachineTag(parentId).String() == authEntityTag {
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
		resources:   resources,
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
	var tag names.Tag
	for i, entity := range args.Entities {
		if !canAccess(entity.Tag) {
			err = common.ErrPerm
		} else {
			tag, err = names.ParseMachineTag(entity.Tag)
			if err == nil {
				id := tag.Id()
				result.Results[i].Info, err = n.oneMachineInfo(id)
			}
		}
		result.Results[i].Error = common.ServerError(err)
	}
	return result, nil
}

func (n *NetworkerAPI) watchOneMachineInterfaces(id string) (string, error) {
	machine, err := n.st.Machine(id)
	if err != nil {
		return "", err
	}
	watch := machine.WatchInterfaces()
	// Consume the initial event.
	if _, ok := <-watch.Changes(); ok {
		return n.resources.Register(watch), nil
	}
	return "", watcher.MustErr(watch)
}

// WatchInterfaces returns a NotifyWatcher for observing changes
// to each unit's service configuration settings.
func (n *NetworkerAPI) WatchInterfaces(args params.Entities) (params.NotifyWatchResults, error) {
	result := params.NotifyWatchResults{
		Results: make([]params.NotifyWatchResult, len(args.Entities)),
	}
	canAccess, err := n.getAuthFunc()
	if err != nil {
		return result, err
	}
	var tag names.Tag
	for i, entity := range args.Entities {
		if !canAccess(entity.Tag) {
			err = common.ErrPerm
		} else {
			tag, err = names.ParseMachineTag(entity.Tag)
			if err == nil {
				id := tag.Id()
				result.Results[i].NotifyWatcherId, err = n.watchOneMachineInterfaces(id)
			}
		}
		result.Results[i].Error = common.ServerError(err)
	}
	return result, nil
}
