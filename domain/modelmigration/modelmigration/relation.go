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

// RemoteApplicationOfferer represents a remote application that offers an
// application, and any duplicate remote applications with the same offer UUID
// and endpoints.
type RemoteApplicationOfferer struct {
	Primary    description.RemoteApplication
	Duplicates []description.RemoteApplication
}

// MatchesSourceModelUUID returns true if the source model UUID of the primary
// remote application does not match the given source model UUID.
func (o RemoteApplicationOfferer) MatchesSourceModelUUID(sourceModelUUID string) bool {
	return o.Primary.SourceModelUUID() != sourceModelUUID
}

// MatchesEndpoints returns true if the endpoints of the primary remote
// application do not match the given endpoints.
func (o RemoteApplicationOfferer) MatchesEndpoints(endpoints []description.RemoteEndpoint) bool {
	return remoteEndpointsEqual(o.Primary.Endpoints(), endpoints)
}

// SourceModelUUID returns the source model UUID of the primary remote
// application.
func (o RemoteApplicationOfferer) SourceModelUUID() string {
	return o.Primary.SourceModelUUID()
}

// Endpoints returns the endpoints of the primary remote application.
func (o RemoteApplicationOfferer) Endpoints() []description.RemoteEndpoint {
	return o.Primary.Endpoints()
}

// IsEmpty returns true if there is no primary remote application.
func (o RemoteApplicationOfferer) IsEmpty() bool {
	return o.Primary == nil
}

// UniqueRemoteOfferApplications de-duplicates remote applications based on
// offer UUID and endpoints, and verifies that there are no conflicting remote
// applications with the same offer UUID.
func UniqueRemoteOfferApplications(remoteApps []description.RemoteApplication) (map[string]RemoteApplicationOfferer, error) {
	unique := map[string]RemoteApplicationOfferer{}
	for _, remoteApp := range remoteApps {
		// We don't care about remove application consumers, so duplications
		// here don't matter.
		if remoteApp.IsConsumerProxy() {
			continue
		}

		// Verify that if there are multiple remote application offerers with
		// the same offer UUID, they also have the same source model UUID and
		// endpoints, otherwise the import would be ambiguous.
		offerUUID := remoteApp.OfferUUID()
		if existing, ok := unique[offerUUID]; ok {
			if existing.MatchesSourceModelUUID(remoteApp.SourceModelUUID()) {
				return nil, errors.Errorf("multiple remote application offerers with the same offer UUID %q, but different source model UUIDs: %q and %q",
					offerUUID, existing.SourceModelUUID(), remoteApp.SourceModelUUID())
			}
			if !existing.MatchesEndpoints(remoteApp.Endpoints()) {
				return nil, errors.Errorf("multiple remote application offerers with the same offer UUID %q, but different endpoints: %v and %v",
					offerUUID, remoteEndpointString(existing.Endpoints()), remoteEndpointString(remoteApp.Endpoints()))
			}

			existing.Duplicates = append(existing.Duplicates, remoteApp)
			unique[offerUUID] = existing
			continue
		}

		unique[offerUUID] = RemoteApplicationOfferer{
			Primary: remoteApp,
		}
	}

	return unique, nil
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
