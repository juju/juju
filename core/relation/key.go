// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package relation

import (
	"strings"

	"github.com/juju/names/v6"

	coreerrors "github.com/juju/juju/core/errors"
	"github.com/juju/juju/internal/charm"
	"github.com/juju/juju/internal/errors"
)

// EndpointIdentifier contains the identifying information of a relation
// endpoint.
type EndpointIdentifier struct {
	// ApplicationName is the name of the application the endpoint belongs to.
	ApplicationName string
	// EndpointName is the name of the endpoint.
	EndpointName string
	// Role is the role of the endpoint in the relation.
	Role charm.RelationRole
}

// String returns the string representation of the endpoint.
func (ei *EndpointIdentifier) String() string {
	return ei.ApplicationName + ":" + ei.EndpointName
}

// Key represents the natural key of a relation. It either contains one or two
// endpoint identifiers. If the key represents a peer relation, the key is made
// up of one endpoint identifier with the role Peer. If the key represents a
// regular relation, it is made up of two endpoints, the first having the role
// Provider and the second having the role Requirer.
type Key struct {
	endpointIdentifiers []EndpointIdentifier
}

// NewKey generates a Key representation of relation endpoints.
func NewKey(eids []EndpointIdentifier) (Key, error) {
	switch len(eids) {
	case 1:
		if eids[0].Role != charm.RolePeer {
			return Key{}, errors.Errorf(`one endpoint provided, expected role "peer", got: %q`, eids[0].Role)
		}
		return Key{endpointIdentifiers: eids}, nil
	case 2:
		switch {
		case eids[0].Role == charm.RoleProvider && eids[1].Role == charm.RoleRequirer:
			return Key{
				endpointIdentifiers: []EndpointIdentifier{eids[0], eids[1]},
			}, nil
		case eids[0].Role == charm.RoleRequirer && eids[1].Role == charm.RoleProvider:
			return Key{
				endpointIdentifiers: []EndpointIdentifier{eids[1], eids[0]},
			}, nil
		default:
			return Key{}, errors.Errorf(
				`two endpoints provided, expected roles "provider" and "requirer", got: %q and %q`,
				eids[0].Role, eids[1].Role,
			)
		}
	default:
		return Key{}, errors.Errorf("expected 1 or 2 endpoint identifiers, got %d", len(eids))
	}
}

// NewKeyFromString parses a relation key string and returns a relation Key. It
// expects a string of one of the following forms:
// - "<application-name>:<endpoint-name> <application-name>:<endpoint-name>"
// - "<application-name>:<endpoint-name>"
func NewKeyFromString(relationKey string) (Key, error) {
	endpointStrings := strings.Fields(relationKey)
	var eids []EndpointIdentifier
	switch len(endpointStrings) {
	case 1:
		eid, err := parseEndpointString(endpointStrings[0], charm.RolePeer)
		if err != nil {
			return Key{}, errors.Errorf("parsing key %q: %w", relationKey, err)
		}
		eids = append(eids, eid)
	case 2:
		provider, err := parseEndpointString(endpointStrings[0], charm.RoleProvider)
		if err != nil {
			return Key{}, errors.Errorf("parsing key %q: %w", relationKey, err)
		}
		requirer, err := parseEndpointString(endpointStrings[1], charm.RoleRequirer)
		if err != nil {
			return Key{}, errors.Errorf("parsing key %q: %w", relationKey, err)
		}
		eids = append(eids, provider, requirer)
	default:
		return Key{}, errors.Errorf("expected 1 or 2 endpoints in relation key, got %d: %q", len(endpointStrings), relationKey)
	}

	return Key{endpointIdentifiers: eids}, nil
}

// String returns the string representation of the Key, this can be reformed into a key with ParseKey.
func (k Key) String() string {
	endpoints := make([]string, len(k.endpointIdentifiers))
	for i, ei := range k.endpointIdentifiers {
		endpoints[i] = ei.String()
	}
	return strings.Join(endpoints, " ")
}

// Validate checks that the Key has the required form.
func (k Key) Validate() error {
	switch len(k.endpointIdentifiers) {
	case 1:
		if k.endpointIdentifiers[0].Role != charm.RolePeer {
			return errors.Errorf(
				`one endpoint provided, expected role "peer", got %q: %w`,
				k.endpointIdentifiers[0].Role, coreerrors.NotValid,
			)
		}
	case 2:
		if !(k.endpointIdentifiers[0].Role == charm.RoleProvider && k.endpointIdentifiers[1].Role == charm.RoleRequirer) {
			return errors.Errorf(
				`two endpoints provided, expected roles "provider" and "requirer", got %q and %q: %w`,
				k.endpointIdentifiers[0].Role, k.endpointIdentifiers[1].Role, coreerrors.NotValid,
			)
		}
	default:
		return errors.Errorf("expected 1 or 2 endpoint identifiers, got %d: %w", len(k.endpointIdentifiers), coreerrors.NotValid)
	}
	return nil
}

// EndpointIdentifiers returns the endpoint identifiers for the key.
func (k Key) EndpointIdentifiers() []EndpointIdentifier {
	return k.endpointIdentifiers
}

// ParseKeyFromTagString returns a Key for the given string
// in relation tag format.
func ParseKeyFromTagString(s string) (Key, error) {
	relTag, err := names.ParseRelationTag(s)
	if err != nil {
		return Key{}, err
	}
	return NewKeyFromString(relTag.Id())
}

// parseEndpointString parses a single endpoint string in a relation key string.
func parseEndpointString(endpoint string, role charm.RelationRole) (EndpointIdentifier, error) {
	parts := strings.Split(endpoint, ":")
	if len(parts) != 2 {
		return EndpointIdentifier{}, errors.Errorf("expected endpoints of form <application-name>:<endpoint-name>, got %q", endpoint)
	}

	return EndpointIdentifier{
		ApplicationName: parts[0],
		EndpointName:    parts[1],
		Role:            role,
	}, nil
}
