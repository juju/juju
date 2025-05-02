// Copyright 2018 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package firewall

import (
	"context"

	"github.com/juju/errors"
	"github.com/juju/names/v6"

	apiservererrors "github.com/juju/juju/apiserver/errors"
	"github.com/juju/juju/apiserver/facade"
	"github.com/juju/juju/internal/charm"
	internallogger "github.com/juju/juju/internal/logger"
	"github.com/juju/juju/rpc/params"
	"github.com/juju/juju/state/watcher"
)

var logger = internallogger.GetLogger("juju.apiserver.crossmodelrelations")

// WatchEgressAddressesForRelations creates a watcher that notifies when addresses, from which
// connections will originate for the relation, change.
// Each event contains the entire set of addresses which are required for ingress for the relation.
func WatchEgressAddressesForRelations(ctx context.Context, resources facade.Resources, st State, modelConfigService ModelConfigService, relations params.Entities) (params.StringsWatchResults, error) {
	results := params.StringsWatchResults{
		Results: make([]params.StringsWatchResult, len(relations.Entities)),
	}

	one := func(ctx context.Context, tag string) (id string, changes []string, _ error) {
		logger.Debugf(ctx, "Watching egress addresses for %+v", tag)

		relationTag, err := names.ParseRelationTag(tag)
		if err != nil {
			return "", nil, errors.Trace(err)
		}

		// Load the relation details for the current token.
		localEndpoint, err := localApplication(st, relationTag)
		if err != nil {
			return "", nil, errors.Trace(err)
		}

		w, err := NewEgressAddressWatcher(st, modelConfigService, localEndpoint.relation, localEndpoint.application)
		if err != nil {
			return "", nil, errors.Trace(err)
		}

		// TODO(wallyworld) - we will need to watch subnets too, but only
		// when we support using cloud local addresses
		//filter := func(id interface{}) bool {
		//	include, err := includeAsIngressSubnet(id.(string))
		//	if err != nil {
		//		logger.Warningf(ctx, "invalid CIDR %q", id)
		//	}
		//	return include
		//}
		//w := api.st.WatchSubnets(filter)

		changes, ok := <-w.Changes()
		if !ok {
			return "", nil, apiservererrors.ServerError(watcher.EnsureErr(w))
		}
		return resources.Register(w), changes, nil
	}

	for i, e := range relations.Entities {
		watcherId, changes, err := one(ctx, e.Tag)
		if err != nil {
			results.Results[i].Error = apiservererrors.ServerError(err)
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

func localApplication(st State, relationTag names.RelationTag) (*localEndpointInfo, error) {
	rel, err := st.KeyRelation(relationTag.Id())
	if err != nil {
		return nil, errors.Trace(err)
	}

	// Gather info about the local (this model) application of the relation.
	// We'll use the info to figure out what addresses/subnets to include.
	localEndpoint := localEndpointInfo{relation: rel}
	for _, ep := range rel.Endpoints() {
		// Try looking up the info for the local application.
		_, err = st.Application(ep.ApplicationName)
		if err != nil && !errors.Is(err, errors.NotFound) {
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
			"egress network for application %v without requires endpoint", localEndpoint.application)
	}
	return &localEndpoint, nil
}

// TODO(wallyworld) - this is unused until we query subnets again
/*
func includeAsEgressSubnet(cidr string) (bool, error) {
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
*/
