// Copyright 2012, 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state

import (
	"fmt"

	"launchpad.net/juju-core/charm"
)

// counterpartRole returns the RelationRole that this RelationRole
// can relate to.
// This should remain an internal method because the relation
// model does not guarantee that for every role there will
// necessarily exist a single counterpart role that is sensible
// for basing algorithms upon.
func counterpartRole(r charm.RelationRole) charm.RelationRole {
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
	ServiceName string
	charm.Relation
}

// String returns the unique identifier of the relation endpoint.
func (ep Endpoint) String() string {
	return ep.ServiceName + ":" + ep.Name
}

// CanRelateTo returns whether a relation may be established between e and other.
func (ep Endpoint) CanRelateTo(other Endpoint) bool {
	return ep.ServiceName != other.ServiceName &&
		ep.Interface == other.Interface &&
		ep.Role != charm.RolePeer &&
		counterpartRole(ep.Role) == other.Role
}

type epSlice []Endpoint

var roleOrder = map[charm.RelationRole]int{
	charm.RoleRequirer: 0,
	charm.RoleProvider: 1,
	charm.RolePeer:     2,
}

func (eps epSlice) Len() int      { return len(eps) }
func (eps epSlice) Swap(i, j int) { eps[i], eps[j] = eps[j], eps[i] }
func (eps epSlice) Less(i, j int) bool {
	ep1 := eps[i]
	ep2 := eps[j]
	if ep1.Role != ep2.Role {
		return roleOrder[ep1.Role] < roleOrder[ep2.Role]
	}
	return ep1.String() < ep2.String()
}
