package state

import (
	"launchpad.net/gozk/zookeeper"
)

// RelationRole defines the role of a relation endpoint.
type RelationRole string

const (
	RoleProvider RelationRole = "provider"
	RoleRequirer RelationRole = "requirer"
	RolePeer     RelationRole = "peer"
)

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
	switch e.RelationRole {
	case RoleProvider:
		return other.RelationRole == RoleRequirer
	case RoleRequirer:
		return other.RelationRole == RoleProvider
	case RolePeer:
		return false
	}
	panic("endpoint role is undefined")
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

// unitScope represents a set of units that can (transitively) affect one
// another within the context of a particular relation. For a globally-scoped
// relation, the unitScope holds every unit of every service in the relation;
// for a container-scoped relation, the unitScope holds every unit of the
// relation that is located within a particular container.
type unitScope struct {
	zk   *zookeeper.Conn
	path string
}

// SettingsPath returns the path to the relation unit settings node for the
// unit identified by key, or to the relation group settings node if key is
// empty.
func (s *unitScope) SettingsPath(key string) string {
	return s.subpath("settings", key)
}

// PresencePath returns the path to the relation unit presence node for a
// unit (identified by key) of a service acting as role; or to the relation
// group role node if key is empty.
func (s *unitScope) PresencePath(role RelationRole, key string) string {
	return s.subpath(string(role), key)
}

// PrepareJoin ensures that ZooKeeper nodes exist such that a unit of a
// service with the supplied role will be able to join the relation.
func (s *unitScope) PrepareJoin(role RelationRole) error {
	paths := []string{
		s.path,
		s.SettingsPath(""),
		s.PresencePath(role, ""),
	}
	for _, path := range paths {
		if _, err := s.zk.Create(path, "", 0, zkPermAll); err != nil {
			if zookeeper.IsError(err, zookeeper.ZNODEEXISTS) {
				continue
			}
			return err
		}
	}
	return nil
}

// subpath returns an absolute ZooKeeper path to the node whose path relative
// to the group node is composed of parts. Empty parts will be stripped.
func (s *unitScope) subpath(parts ...string) string {
	path := s.path
	for _, part := range parts {
		if part != "" {
			path = path + "/" + part
		}
	}
	return path
}
