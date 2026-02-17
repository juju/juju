// Copyright 2026 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package modelmigration

import (
	"fmt"
	"sort"
	"strings"

	"github.com/juju/collections/set"
	"github.com/juju/description/v11"

	"github.com/juju/juju/internal/errors"
)

// ContainsRelationEndpointApplicationName returns true if any of the relation
// endpoints match any of the given application names.
func ContainsRelationEndpointApplicationName(rel description.Relation, applications set.Strings) bool {
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

// UniqueRemoteOfferApplications de-duplicates remote applications based on
// offer UUID and endpoints, and verifies that there are no conflicting remote
// applications with the same offer UUID.
func UniqueRemoteOfferApplications(remoteApps []description.RemoteApplication, relations []description.Relation) (map[string]description.RemoteApplication, error) {
	// In 3.x it's possible to have multiple remote applications with the same
	// offer UUID and endpoints, but have different names. In this case, we need
	// to merge and de-duplicate these remote applications when importing them.
	// 4.x doesn't allow to have multiple remote applications with the same
	// offer UUID.
	unique := map[string]description.RemoteApplication{}
	for _, remoteApp := range remoteApps {
		// We don't care about remove application consumers, so duplications
		// here don't matter.
		if remoteApp.IsConsumerProxy() {
			continue
		}

		// If the remote application is not used by a relation, then we can
		// ignore it as well.
		if !remoteAppUsedByRelation(remoteApp, relations) {
			continue
		}

		// Verify that if there are multiple remote application offerers with
		// the same offer UUID, they also have the same source model UUID and
		// endpoints, otherwise the import would be ambiguous.
		offerUUID := remoteApp.OfferUUID()
		if existing, ok := unique[offerUUID]; ok {
			if existing.SourceModelUUID() != remoteApp.SourceModelUUID() {
				return nil, errors.Errorf("multiple remote application offerers with the same offer UUID %q, but different source model UUIDs: %q and %q",
					offerUUID, existing.SourceModelUUID(), remoteApp.SourceModelUUID())
			}
			if !remoteEndpointsEqual(existing.Endpoints(), remoteApp.Endpoints()) {
				return nil, errors.Errorf("multiple remote application offerers with the same offer UUID %q, but different endpoints: %v and %v",
					offerUUID, remoteEndpointString(existing.Endpoints()), remoteEndpointString(remoteApp.Endpoints()))
			}
		}

		unique[offerUUID] = remoteApp
	}

	return unique, nil
}

func remoteAppUsedByRelation(remoteApp description.RemoteApplication, relations []description.Relation) bool {
	for _, rel := range relations {
		for _, ep := range rel.Endpoints() {
			if ep.ApplicationName() == remoteApp.Name() {
				return true
			}
		}
	}

	return false
}

func remoteEndpointsEqual(a, b []description.RemoteEndpoint) bool {
	if len(a) != len(b) {
		return false
	}

	aCopy := append([]description.RemoteEndpoint(nil), a...)
	bCopy := append([]description.RemoteEndpoint(nil), b...)

	sort.Slice(aCopy, func(i, j int) bool {
		return aCopy[i].Name() < aCopy[j].Name()
	})
	sort.Slice(bCopy, func(i, j int) bool {
		return bCopy[i].Name() < bCopy[j].Name()
	})

	for i := range aCopy {
		if aCopy[i].Name() != bCopy[i].Name() ||
			aCopy[i].Role() != bCopy[i].Role() ||
			aCopy[i].Interface() != bCopy[i].Interface() {
			return false
		}
	}

	return true
}

func remoteEndpointString(eps []description.RemoteEndpoint) string {
	var parts []string
	for _, ep := range eps {
		parts = append(parts, fmt.Sprintf("%s:%s", ep.Interface(), ep.Name()))
	}
	return strings.Join(parts, " ")
}
