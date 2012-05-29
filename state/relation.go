package state

import (
	"fmt"
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

// CanRelateTo tests whether the "other"`" endpoint can be used in a common 
// relation.
// 
// RelationEndpoints can be related if they share the same interface
// and one is a 'server' while the other is a 'client'; or if both endpoints 
// have a role of 'peers'.
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
		return other.RelationRole == RolePeer
	}
	panic("endpoint role is undefined")
}

// String returns the string representation of the relation endpoint.
func (e *RelationEndpoint) String() string {
	return fmt.Sprintf("%s:%s:%s:%s", e.RelationRole, e.RelationName, e.ServiceName, e.Interface)
}

// Relation represents a connection between one or more services.
type Relation struct {
	st  *State
	key string
}

// ServiceRelation represents a relation between one or more services.
type ServiceRelation struct {
	st         *State
	key        string
	serviceKey string
	scope      RelationScope
	role       RelationRole
}

// Scope returns the scope of a relation.
func (r *ServiceRelation) Scope() RelationScope {
	return r.scope
}

// Role returns the role of a relation.
func (r *ServiceRelation) Role() RelationRole {
	return r.role
}
