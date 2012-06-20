package state

import (
	"fmt"
	"launchpad.net/gozk/zookeeper"
)

// RelationRole defines the role of a relation endpoint.
type RelationRole string

const (
	RoleProvider RelationRole = "provider"
	RoleRequirer RelationRole = "requirer"
	RolePeer     RelationRole = "peer"
)

// CounterpartRole returns the RelationRole that this RelationRole can
// relate to.
func (r RelationRole) CounterpartRole() RelationRole {
	switch r {
	case RoleProvider:
		return RoleRequirer
	case RoleRequirer:
		return RoleProvider
	case RolePeer:
		return RolePeer
	}
	panic(fmt.Errorf("unknown RelationRole: %q", r))
}

// RelationScope describes the scope of a relation endpoint.
type RelationScope string

const (
	ScopeGlobal    RelationScope = "global"
	ScopeContainer RelationScope = "container"
)

// RelationEndpoint represents one endpoint of a relation.
type RelationEndpoint struct {
	ServiceName   string
	Interface     string
	RelationName  string
	RelationRole  RelationRole
	RelationScope RelationScope
}

// CanRelateTo returns whether a relation may be established between e and other.
func (e *RelationEndpoint) CanRelateTo(other *RelationEndpoint) bool {
	if e.Interface != other.Interface {
		return false
	}
	if e.RelationRole == RolePeer {
		// Peer relations do not currently work with multiple endpoints.
		return false
	}
	return e.RelationRole.CounterpartRole() == other.RelationRole
}

// String returns the unique identifier of the relation endpoint.
func (e RelationEndpoint) String() string {
	return e.ServiceName + ":" + e.RelationName
}

// ServiceRelation represents an established relation from
// the viewpoint of a participant service.
type ServiceRelation struct {
	st            *State
	relationKey   string
	serviceKey    string
	relationScope RelationScope
	relationRole  RelationRole
	relationName  string
}

// RelationScope returns the scope of the relation.
func (r *ServiceRelation) RelationScope() RelationScope {
	return r.relationScope
}

// RelationRole returns the service role within the relation.
func (r *ServiceRelation) RelationRole() RelationRole {
	return r.relationRole
}

// RelationName returns the name this relation has within the service.
func (r *ServiceRelation) RelationName() string {
	return r.relationName
}

// unitScopePath represents a common zookeeper path used by the set of units
// that can (transitively) affect one another within the context of a
// particular relation. For a globally-scoped relation, every unit is part of
// the same scope; but for a container-scoped relation, each unit is is a
// scope shared only with the units that share a container.
// Thus, unitScopePaths will take one of the following forms:
//
//   /relations/<relation-id>
//   /relations/<relation-id>/<container-id>
type unitScopePath string

// settingsPath returns the path to the relation unit settings node for the
// unit identified by unitKey, or to the relation scope settings node if
// unitKey is empty.
func (s unitScopePath) settingsPath(unitKey string) string {
	return s.subpath("settings", unitKey)
}

// presencePath returns the path to the relation unit presence node for a
// unit (identified by unitKey) of a service acting as role; or to the relation
// scope role node if unitKey is empty.
func (s unitScopePath) presencePath(role RelationRole, unitKey string) string {
	return s.subpath(string(role), unitKey)
}

// prepareJoin ensures that ZooKeeper nodes exist such that a unit of a
// service with the supplied role will be able to join the relation.
func (s unitScopePath) prepareJoin(zk *zookeeper.Conn, role RelationRole) error {
	paths := []string{string(s), s.settingsPath(""), s.presencePath(role, "")}
	for _, path := range paths {
		if _, err := zk.Create(path, "", 0, zkPermAll); err != nil {
			if zookeeper.IsError(err, zookeeper.ZNODEEXISTS) {
				continue
			}
			return err
		}
	}
	return nil
}

// subpath returns an absolute ZooKeeper path to the node whose path relative
// to the scope node is composed of parts. Empty parts will be stripped.
func (s unitScopePath) subpath(parts ...string) string {
	path := string(s)
	for _, part := range parts {
		if part != "" {
			path = path + "/" + part
		}
	}
	return path
}
