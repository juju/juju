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

// String returns the unique identifier of the relation endpoint.
func (e RelationEndpoint) String() string {
	return e.ServiceName + ":" + e.RelationName
}

// Relation represents a link between services, or within a
// service (in the case of peer relations).
type Relation struct {
	st  *State
	key string
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
