// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package relation

import (
	"fmt"
	"slices"
	"strings"

	corerelation "github.com/juju/juju/core/relation"
	"github.com/juju/juju/internal/charm"
)

// roleOrder maps RelationRole values to integers to define their order
// of precedence in relation endpoints. This is used to compute the relation's
// natural key.
var roleOrder = map[charm.RelationRole]int{
	charm.RoleRequirer: 0,
	charm.RoleProvider: 1,
	charm.RolePeer:     2,
}

// CounterpartRole returns the RelationRole that this RelationRole
// can relate to.
// This should remain an internal method because the relation
// model does not guarantee that for every role there will
// necessarily exist a single counterpart role that is sensible
// for basing algorithms upon.
func CounterpartRole(r charm.RelationRole) charm.RelationRole {
	switch r {
	case charm.RoleProvider:
		return charm.RoleRequirer
	case charm.RoleRequirer:
		return charm.RoleProvider
	case charm.RolePeer:
		return charm.RolePeer
	}
	panic(fmt.Errorf("unknown relation role %q", r))
}

// Endpoint represents one endpoint of a relation.
type Endpoint struct {
	ApplicationName string
	charm.Relation
}

// String returns the unique identifier of the relation endpoint.
func (ep Endpoint) String() string {
	return ep.ApplicationName + ":" + ep.Name
}

// CanRelateTo returns whether a relation may be established between e and other.
func (ep Endpoint) CanRelateTo(other Endpoint) bool {
	return ep.ApplicationName != other.ApplicationName &&
		ep.Interface == other.Interface &&
		ep.Role != charm.RolePeer &&
		CounterpartRole(ep.Role) == other.Role
}

// NaturalKey generates a unique sorted string representation of relation
// endpoints based on their roles and identifiers. It can be used as a natural key
// for relations.
func NaturalKey(endpoints []Endpoint) corerelation.Key {
	eps := slices.SortedFunc(slices.Values(endpoints), func(ep1 Endpoint, ep2 Endpoint) int {
		if ep1.Role != ep2.Role {
			return roleOrder[ep1.Role] - roleOrder[ep2.Role]
		}
		return strings.Compare(ep1.String(), ep2.String())
	})
	endpointNames := make([]string, 0, len(eps))
	for _, ep := range eps {
		endpointNames = append(endpointNames, ep.String())
	}
	return corerelation.Key(strings.Join(endpointNames, " "))
}
