// Copyright 2026 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package modelmigration

import "github.com/juju/description/v11"

// IsRemoteConsumerRelation returns true if the given relation is a remote
// consumer relation.
func IsRemoteConsumerRelation(rel description.Relation, remoteApps []description.RemoteApplication) bool {
	// If there are no remote applications, then there can't be any remote
	// relations.
	if len(remoteApps) == 0 {
		return false
	}

	// The only way to really know if a relation is a remote relation is to
	// cross reference the relation endpoints with the remote applications. If
	// any of the relation endpoints belong to a remote application, that is
	// a consumer proxy remote application, then we return true.
	remoteConsumers := make(map[string]struct{})
	for _, app := range remoteApps {
		if !app.IsConsumerProxy() {
			continue
		}

		remoteConsumers[app.Name()] = struct{}{}
	}

	for _, endpoint := range rel.Endpoints() {
		appName := endpoint.ApplicationName()
		if _, ok := remoteConsumers[appName]; ok {
			return true
		}
	}

	return false
}
