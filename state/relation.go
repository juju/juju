package state

import ()

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
