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

// Id returns the unique identifier of the relation endpoint.
func (e *RelationEndpoint) Id() string {
	return e.ServiceName + ":" + e.RelationName
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
	}
	panic("endpoint role is undefined")
}
