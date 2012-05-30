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

// Relation represents a connection between one or more services.
type Relation interface {
	relationKey() string
}

// relation is the implementation of the relation interface and
// represents the entire connection.
type relation struct {
	st  *State
	key string
}

// relationKey returns the key of the relation.
func (r *relation) relationKey() string {
	return r.key
}

// ServiceRelation represents a relation between one or more services.
type ServiceRelation struct {
	st         *State
	key        string
	serviceKey string
	scope      RelationScope
	role       RelationRole
	name       string
}

// Scope returns the scope of a relation.
func (r *ServiceRelation) Scope() RelationScope {
	return r.scope
}

// Role returns the role of a relation.
func (r *ServiceRelation) Role() RelationRole {
	return r.role
}

// Name returns the name of a relation.
func (r *ServiceRelation) Name() string {
	return r.name
}

// relationKey returns the key of the relation.
func (r *ServiceRelation) relationKey() string {
	return r.key
}
