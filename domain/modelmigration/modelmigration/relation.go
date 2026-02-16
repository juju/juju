// Copyright 2026 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package modelmigration

import (
	"github.com/juju/collections/set"
	"github.com/juju/description/v11"
)

// IsRelationInApplicationsName returns true if any of the relation endpoints
// match any of the given application names.
func IsRelationInApplicationsName(rel description.Relation, applications set.Strings) bool {
	for _, endpoint := range rel.Endpoints() {
		appName := endpoint.ApplicationName()
		if _, ok := applications[appName]; ok {
			return true
		}
	}

	return false
}

// GetUniqueRemoteConsumersNames returns the set of remote consumer applications
// that are involved in the given remote relations.
func GetUniqueRemoteConsumersNames(remoteApps []description.RemoteApplication) set.Strings {
	// If there are no remote applications, then there can't be any remote
	// relations.
	if len(remoteApps) == 0 {
		return set.NewStrings()
	}

	// The only way to really know if a relation is a remote relation is to
	// cross reference the relation endpoints with the remote applications. If
	// any of the relation endpoints belong to a remote application, that is
	// a consumer proxy remote application, then we return true.
	remoteConsumers := set.NewStrings()
	for _, app := range remoteApps {
		if !app.IsConsumerProxy() {
			continue
		}

		remoteConsumers.Add(app.Name())
	}

	return remoteConsumers
}
