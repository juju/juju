// Copyright 2017 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package remotefirewaller

import (
	"net"

	"github.com/juju/errors"
	"github.com/juju/loggo"
	"gopkg.in/juju/charm.v6-unstable"
	"gopkg.in/juju/names.v2"

	"github.com/juju/juju/apiserver/common"
	"github.com/juju/juju/apiserver/facade"
	"github.com/juju/juju/apiserver/params"
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

// WatchIngressAddressesForRelation creates a watcher that notifies when address, from which
// connections will originate for the relation, change.
// Each event contains the entire set of addresses which are required for ingress for the relation.
func (api *FirewallerAPI) WatchIngressAddressesForRelation(remoteEntities params.RemoteEntities) (params.StringsWatchResults, error) {
	results := params.StringsWatchResults{
		make([]params.StringsWatchResult, len(remoteEntities.Entities)),
	}

	one := func(remoteRelationId params.RemoteEntityId) (id string, changes []string, _ error) {
		logger.Debugf("Watching ingress addresses for %+v from model %v", remoteRelationId, api.st.ModelUUID())

		// Load the relation details for the current token.
		localEndpoint, err := api.localApplication(remoteRelationId)
		if err != nil {
			return "", nil, errors.Trace(err)
		}

		w, err := NewIngressAddressWatcher(api.st, localEndpoint.relation, localEndpoint.application)
		if err != nil {
			return "", nil, errors.Trace(err)
		}

		// TODO(wallyworld) - we will need to watch subnets too, but only
		// when we support using cloud local addresses
		//filter := func(id interface{}) bool {
		//	include, err := includeAsIngressSubnet(id.(string))
		//	if err != nil {
		//		logger.Warningf("invalid CIDR %q", id)
		//	}
		//	return include
		//}
		//w := api.st.WatchSubnets(filter)

		changes, ok := <-w.Changes()
		if !ok {
			return "", nil, common.ServerError(watcher.EnsureErr(w))
		}
		return api.resources.Register(w), changes, nil
	}

	for i, remoteRelationId := range remoteEntities.Entities {
		watcherId, changes, err := one(remoteRelationId)
		if err != nil {
			results.Results[i].Error = common.ServerError(err)
			continue
		}
		results.Results[i].StringsWatcherId = watcherId
		results.Results[i].Changes = changes
	}
	return results, nil
}

type localEndpointInfo struct {
	relation    Relation
	application string
	name        string
	role        charm.RelationRole
}

func (api *FirewallerAPI) localApplication(remoteRelationId params.RemoteEntityId) (*localEndpointInfo, error) {
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
	// We'll use the info to figure out what addresses/subnets to include.
	localEndpoint := localEndpointInfo{relation: rel}
	for _, ep := range rel.Endpoints() {
		// Try looking up the info for the local application.
		_, err = api.st.Application(ep.ApplicationName)
		if err != nil && !errors.IsNotFound(err) {
			return nil, errors.Trace(err)
		} else if err == nil {
			localEndpoint.application = ep.ApplicationName
			localEndpoint.name = ep.Name
			localEndpoint.role = ep.Role
			break
		}
	}
	// Until networking support becomes more sophisticated, for now
	// we only care about opening access to applications with an endpoint
	// having the "provider" role. The assumption is that such endpoints listen
	// to incoming connections and thus require ingress. An exception to this
	// would be applications which accept connections onto an endpoint which
	// has a "requirer" role.
	// We are operating in the model hosting the "consuming" application, so check
	// that its endpoint has the "requirer" role, meaning that we need to notify
	// the offering model of subnets from this model required for ingress.
	if localEndpoint.role != charm.RoleRequirer {
		return nil, errors.NotSupportedf(
			"ingress network for application %v without requires endpoint", localEndpoint.application)
	}
	return &localEndpoint, nil
}

// TODO(wallyworld) - this is unused until we query subnets again
func includeAsIngressSubnet(cidr string) (bool, error) {
	ip, _, err := net.ParseCIDR(cidr)
	if err != nil {
		return false, errors.Trace(err)
	}
	if ip.IsLoopback() || ip.IsMulticast() {
		return false, nil
	}
	// TODO(wallyworld) - We only support IPv4 addresses as not all providers support IPv6.
	if ip.To4() == nil {
		return false, nil
	}
	return true, nil
}
