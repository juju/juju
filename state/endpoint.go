// Copyright 2012, 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state

import (
	"github.com/juju/juju/domain/relation"
	"github.com/juju/juju/internal/charm"
)

type epSlice []relation.Endpoint

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
