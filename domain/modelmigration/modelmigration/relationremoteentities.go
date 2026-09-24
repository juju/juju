// Copyright 2026 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package modelmigration

import (
	"sort"
	"strings"

	"github.com/juju/description/v12"

	"github.com/juju/juju/core/relation"
	"github.com/juju/juju/internal/errors"
)

// RelationRemoteEntity is the token a legacy model exported for a relation
// that crosses a model boundary. Legacy models exchange tokens rather than
// UUIDs, and the token of a relation is the relation UUID that the exporting
// model uses for it.
type RelationRemoteEntity struct {
	// RelationKey is the key of the relation the token was recorded for.
	RelationKey relation.Key

	// RelationUUID is the relation token, being the relation UUID of the
	// exporting model.
	RelationUUID string
}

// ExtractRelationUUIDFromRemoteEntities returns the relation tokens recorded
// in the remote entities of the model description.
//
// Both sides of a cross model relation record the relation token, and the
// relation import and the cross model relation import each locate some of the
// relations of the model, so they must agree on this extraction and on the
// relation UUID it yields.
func ExtractRelationUUIDFromRemoteEntities(model description.Model) ([]RelationRemoteEntity, error) {
	var remoteEntities []RelationRemoteEntity
	for _, re := range model.RemoteEntities() {
		// Handle only remote entities that are relation UUIDs.
		remoteEntityID := re.ID()
		if !strings.HasPrefix(remoteEntityID, "relation-") {
			continue
		}

		key, err := relation.ParseKeyFromTagString(relationTagSuffixToKey(remoteEntityID))
		if err != nil {
			return nil, errors.Errorf("parsing relation key from remote entity id %q: %w", remoteEntityID, err)
		}

		remoteEntities = append(remoteEntities, RelationRemoteEntity{
			RelationKey:  key,
			RelationUUID: re.Token(),
		})
	}
	return remoteEntities, nil
}

// FindRelationUUID returns the relation token recorded for the given relation
// key, and whether a token was found. The endpoints of a relation key are not
// guaranteed to be in any particular order, so the lookup is order
// insensitive.
//
// A missing token is not reported as an error here: callers decide whether an
// absent token means an inconsistent description, or whether a new relation
// UUID is to be generated.
func FindRelationUUID(remoteEntities []RelationRemoteEntity, key relation.Key) (string, bool) {
	for _, remoteEntity := range remoteEntities {
		if RelationKeysEqual(remoteEntity.RelationKey, key) {
			return remoteEntity.RelationUUID, true
		}
	}
	return "", false
}

// RelationKeysEqual compares two relation keys for equality, ignoring order.
// Assumes both keys have exactly two endpoints and no scopes.
func RelationKeysEqual(a, b relation.Key) bool {
	if len(a) != len(b) {
		return false
	}

	// Make defensive copies so that sorting does not mutate the caller's
	// slices.
	aCopy := append(relation.Key(nil), a...)
	bCopy := append(relation.Key(nil), b...)

	sort.Slice(aCopy, func(i, j int) bool {
		return aCopy[i].String() < aCopy[j].String()
	})
	sort.Slice(bCopy, func(i, j int) bool {
		return bCopy[i].String() < bCopy[j].String()
	})

	// Note: we ignore scope here, as cross model relations do not have scopes
	// when being imported.
	return relationEndpointEquals(aCopy[0], bCopy[0]) &&
		relationEndpointEquals(aCopy[1], bCopy[1])
}

func relationEndpointEquals(a, b relation.EndpointIdentifier) bool {
	return a.ApplicationName == b.ApplicationName && a.EndpointName == b.EndpointName
}

// relationTagSuffixToKey converts the suffix of a remote entity ID, which is a
// mangled relation key, into the relation key string that the key can be
// parsed from.
func relationTagSuffixToKey(s string) string {
	// Replace both "." with ":" and the "#" with " ".
	s = strings.Replace(s, ".", ":", 2)
	return strings.Replace(s, "#", " ", 1)
}
