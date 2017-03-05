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

// WatchSubnets creates a strings watcher that notifies of the addition,
// removal, and lifecycle changes of subnets in the model.
func (f *FirewallerAPI) WatchSubnets() (params.StringsWatchResult, error) {
	var result params.StringsWatchResult

	watch := f.st.WatchSubnets()
	// Consume the initial event and forward it to the result.
	initial, ok := <-watch.Changes()
	if !ok {
		return params.StringsWatchResult{}, watcher.EnsureErr(watch)
	}
	result.StringsWatcherId = f.resources.Register(watch)
	result.Changes = initial
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
		// We'll use the info to figure out what subnets to include.
		var localAppName, endpointName string
		var endpointRole charm.RelationRole
		for _, ep := range rel.Endpoints() {
			// Try looking up the info for the local application.
			_, err = api.st.Application(ep.ApplicationName)
			if err != nil && !errors.IsNotFound(err) {
				return nil, errors.Trace(err)
			} else if err == nil {
				localAppName = ep.ApplicationName
				endpointName = ep.Name
				endpointRole = ep.Role
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
		if endpointRole != charm.RoleRequirer {
			return &result, errors.NotSupportedf(
				"ingress network for application %v without requires endpoint %v", localAppName, endpointName)
		}

		// TODO(wallyworld) - as a first implementation, just get all CIDRs from the model
		all, err := api.st.AllSubnets()
		cidrs := set.NewStrings()
		if err != nil {
			return nil, errors.Trace(err)
		}

		for _, subnet := range all {
			ip, _, err := net.ParseCIDR(subnet.CIDR())
			if err != nil {
				return nil, errors.Trace(err)
			}
			if ip.IsLoopback() || ip.IsMulticast() {
				continue
			}
			// TODO(wallyworld) - We only support IPv4 addresses as not all providers support IPv6.
			if ip.To4() == nil {
				continue
			}
			cidrs.Add(subnet.CIDR())
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
