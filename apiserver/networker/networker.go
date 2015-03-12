// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package networker

import (
	"github.com/juju/loggo"
	"github.com/juju/names"

	"github.com/juju/juju/apiserver/common"
	"github.com/juju/juju/apiserver/params"
	"github.com/juju/juju/state"
	"github.com/juju/juju/state/watcher"
)

func init() {
	// TODO: When the client can handle new versions, this should really be
	// registered as version 1, since it was not present in the API in Juju
	// 1.18
	common.RegisterStandardFacade("Networker", 0, NewNetworkerAPI)
}

var logger = loggo.GetLogger("juju.apiserver.networker")

// NetworkerAPI provides access to the Networker API facade.
type NetworkerAPI struct {
	st          *state.State
	resources   *common.Resources
	authorizer  common.Authorizer
	getAuthFunc common.GetAuthFunc
}

// NewNetworkerAPI creates a new server-side Networker API facade.
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

		return func(tag names.Tag) bool {
			if tag == authEntityTag {
				// A machine agent can always access its own machine.
				return true
			}
			if _, ok := tag.(names.MachineTag); !ok {
				// Only machine tags are allowed.
				return false
			}
			id := tag.Id()
			for parentId := state.ParentId(id); parentId != ""; parentId = state.ParentId(parentId) {
				// Until a top-level machine is reached.

				// TODO (thumper): remove the names.Tag conversion when gccgo
				// implements concrete-type-to-interface comparison correctly.
				if names.Tag(names.NewMachineTag(parentId)) == authEntityTag {
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

func (n *NetworkerAPI) oneMachineConfig(id string) ([]params.NetworkConfig, error) {
	machine, err := n.st.Machine(id)
	if err != nil {
		return nil, err
	}
	ifaces, err := machine.NetworkInterfaces()
	if err != nil {
		return nil, err
	}
	configs := make([]params.NetworkConfig, len(ifaces))
	for i, iface := range ifaces {
		nw, err := n.st.Network(iface.NetworkName())
		if err != nil {
			return nil, err
		}
		configs[i] = params.NetworkConfig{
			MACAddress:    iface.MACAddress(),
			CIDR:          nw.CIDR(),
			NetworkName:   iface.NetworkName(),
			ProviderId:    string(nw.ProviderId()),
			VLANTag:       nw.VLANTag(),
			InterfaceName: iface.RawInterfaceName(),
			Disabled:      iface.IsDisabled(),
			// TODO(dimitern) Add the rest of the fields, once we
			// store them in state.
		}
	}
	return configs, nil
}

// MachineNetworkInfo returns the list of networks with related interfaces for a
// given set of machines.
// DEPRECATED: Use MachineNetworkConfig() instead.
func (n *NetworkerAPI) MachineNetworkInfo(args params.Entities) (params.MachineNetworkConfigResults, error) {
	return n.MachineNetworkConfig(args)
}

// MachineNetworkConfig returns the list of networks with related interfaces
// for a given set of machines.
func (n *NetworkerAPI) MachineNetworkConfig(args params.Entities) (params.MachineNetworkConfigResults, error) {
	result := params.MachineNetworkConfigResults{
		Results: make([]params.MachineNetworkConfigResult, len(args.Entities)),
	}
	canAccess, err := n.getAuthFunc()
	if err != nil {
		return result, err
	}
	for i, entity := range args.Entities {
		tag, err := names.ParseTag(entity.Tag)
		if err != nil {
			result.Results[i].Error = common.ServerError(common.ErrPerm)
			continue
		}

		if !canAccess(tag) {
			err = common.ErrPerm
		} else {
			tag, ok := tag.(names.MachineTag)
			if ok {
				id := tag.Id()
				result.Results[i].Config, err = n.oneMachineConfig(id)
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
	return "", watcher.EnsureErr(watch)
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
	for i, entity := range args.Entities {
		tag, err := names.ParseTag(entity.Tag)
		if err != nil {
			result.Results[i].Error = common.ServerError(common.ErrPerm)
			continue
		}
		if !canAccess(tag) {
			err = common.ErrPerm
		} else {
			tag, ok := tag.(names.MachineTag)
			if ok {
				id := tag.Id()
				result.Results[i].NotifyWatcherId, err = n.watchOneMachineInterfaces(id)
			}
		}
		result.Results[i].Error = common.ServerError(err)
	}
	return result, nil
}
