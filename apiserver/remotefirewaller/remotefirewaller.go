// Copyright 2017 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package remotefirewaller

import (
	"net"

	"github.com/juju/errors"
	"github.com/juju/loggo"
	"github.com/juju/utils/set"
	"gopkg.in/juju/charm.v6-unstable"
	"gopkg.in/juju/names.v2"

	"github.com/juju/juju/apiserver/common"
	"github.com/juju/juju/apiserver/facade"
	"github.com/juju/juju/apiserver/params"
	"github.com/juju/juju/network"
	"github.com/juju/juju/state"
	"github.com/juju/juju/state/watcher"
)

var logger = loggo.GetLogger("juju.apiserver.remotefirewaller")

// FirewallerAPI provides access to the Remote Firewaller API facade.
type FirewallerAPI struct {
	st         State
	resources  facade.Resources
	authorizer facade.Authorizer
}

// NewStateRemoteFirewallerAPI creates a new server-side RemoteFirewallerAPI facade.
func NewStateRemoteFirewallerAPI(ctx facade.Context) (*FirewallerAPI, error) {
	return NewRemoteFirewallerAPI(stateShim{ctx.State()}, ctx.Resources(), ctx.Auth())
}

// NewRemoteFirewallerAPI creates a new server-side FirewallerAPI facade.
func NewRemoteFirewallerAPI(
	st State,
	resources facade.Resources,
	authorizer facade.Authorizer,
) (*FirewallerAPI, error) {
	if !authorizer.AuthController() {
		return nil, common.ErrPerm
	}
	return &FirewallerAPI{
		st:         st,
		resources:  resources,
		authorizer: authorizer,
	}, nil
}

// stringsWatcherWrapper wraps a StringsWatcher and turns it
// into a notify watcher.
// TODO(wallwyworld) - this is only needed until the proper
// backend logic is available for WatchIngressAddressesForRelation
type stringsWatcherWrapper struct {
	state.StringsWatcher
	changes chan struct{}
}

func (w *stringsWatcherWrapper) Changes() <-chan struct{} {
	return w.changes
}

func newWatcherWrapper(sw state.StringsWatcher) state.NotifyWatcher {
	w := &stringsWatcherWrapper{
		StringsWatcher: sw,
		changes:        make(chan struct{}),
	}
	go func() {
		for {
			_, ok := <-w.StringsWatcher.Changes()
			if !ok {
				close(w.changes)
				return
			}
			w.changes <- struct{}{}
		}
	}()
	return w
}

// WatchIngressAddressesForRelation creates a watcher that notifies when address from which
// connections will originate for the relation change.
func (api *FirewallerAPI) WatchIngressAddressesForRelation(remoteEntities params.RemoteEntities) (params.NotifyWatchResult, error) {
	var result params.NotifyWatchResult

	// TODO(wallyworld) - instead of just watching subnets, we need to watch unit addresses
	// It will depend on whether the relation can use cloud local addresses or not.
	watch := newWatcherWrapper(api.st.WatchSubnets())
	// Consume the initial event.
	_, ok := <-watch.Changes()
	if !ok {
		return params.NotifyWatchResult{}, watcher.EnsureErr(watch)
	}

	result.NotifyWatcherId = api.resources.Register(watch)
	return result, nil
}

// IngressSubnetsForRelations returns any CIDRs for which ingress is required to allow
// the specified relations to properly function.
func (api *FirewallerAPI) IngressSubnetsForRelations(remoteEntities params.RemoteEntities) (params.IngressSubnetResults, error) {
	results := params.IngressSubnetResults{
		Results: make([]params.IngressSubnetResult, len(remoteEntities.Entities)),
	}
	one := func(remoteRelationId params.RemoteEntityId) (*params.IngressSubnetInfo, error) {
		logger.Debugf("Getting ingress subnets for %+v from model %v", remoteRelationId, api.st.ModelUUID())

		// Load the relation details for the current token.
		relTag, err := api.st.GetRemoteEntity(names.NewModelTag(remoteRelationId.ModelUUID), remoteRelationId.Token)
		if err != nil {
			return nil, errors.Trace(err)
		}
		rel, err := api.st.KeyRelation(relTag.Id())
		if err != nil {
			return nil, errors.Trace(err)
		}

		// Gather info about the local (this model) application of the relation.
		// We'll use the info to figure out what addresses to include.
		var localApplication Application
		var endpoint state.Endpoint
		for _, ep := range rel.Endpoints() {
			// Try looking up the info for the local application.
			app, err := api.st.Application(ep.ApplicationName)
			if err != nil && !errors.IsNotFound(err) {
				return nil, errors.Trace(err)
			} else if err == nil {
				localApplication = app
				endpoint = ep
				break
			}
		}

		var result params.IngressSubnetInfo
		// Until networking support becomes more sophisticated, for now
		// we only care about opening access to applications with an endpoint
		// having the "provider" role. The assumption is that such endpoints listen
		// to incoming connections and thus require ingress. An exception to this
		// would be applications which accept connections onto an endpoint which
		// has a "requirer" role.
		// We are operating in the model hosting the "consuming" application, so check
		// that its endpoint has the "requirer" role, meaning that we need to notify
		// the offering model of subnets from this model required for ingress.
		if endpoint.Role != charm.RoleRequirer {
			return &result, errors.NotSupportedf(
				"ingress network for application %v without requires endpoint %v", localApplication.Name(), endpoint.Name)
		}

		cidrs := set.NewStrings()
		if err != nil {
			return nil, errors.Trace(err)
		}

		units, err := localApplication.AllUnits()
		if err != nil {
			return nil, errors.Trace(err)
		}
		for _, unit := range units {
			address, err := unit.PublicAddress()
			if err != nil {
				return nil, errors.Annotatef(err, "getting public address for %q", unit.Name())
			}
			// TODO(wallyworld) - We only support IPv4 addresses as not all providers support IPv6.
			if address.Type != network.IPv4Address {
				continue
			}
			ip := net.ParseIP(address.Value)
			if ip.IsLoopback() || ip.IsMulticast() {
				continue
			}
			cidrs.Add(formatAsCIDR(address))
		}
		result.CIDRs = cidrs.SortedValues()
		logger.Debugf("Ingress CIDRS for remote relation %v from model %v: %v", relTag, api.st.ModelUUID(), result.CIDRs)
		return &result, nil
	}

	for i, remoteRelationId := range remoteEntities.Entities {
		networks, err := one(remoteRelationId)
		if err != nil {
			results.Results[i].Error = common.ServerError(err)
			continue
		}
		results.Results[i].Result = networks
	}
	return results, nil
}

func formatAsCIDR(address network.Address) string {
	return address.Value + "/32"
}
