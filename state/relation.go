package state

import ()

// RelationRole defines the role of a relation endpoint.
type RelationRole string

const (
	RoleNone   RelationRole = ""
	RoleServer RelationRole = "server"
	RoleClient RelationRole = "client"
	RolePeer   RelationRole = "peer"
)

// RelationScope describes the scope of a relation endpoint.
type RelationScope string

const (
	ScopeNone      RelationScope = ""
	ScopeGlobal    RelationScope = "global"
	ScopeContainer RelationScope = "container"
)

// RelationEndpoint represents one endpoint of a relation.
type RelationEndpoint struct {
	ServiceName   string `yaml:"service-name"`
	Interface     string
	RelationRole  RelationRole  `yaml:"relation-role"`
	RelationScope RelationScope `yaml:"relation-scope"`
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
	case RoleServer:
		return other.RelationRole == RoleClient
	case RoleClient:
		return other.RelationRole == RoleServer
	case RolePeer:
		return other.RelationRole == RolePeer
	}
	panic("invalid endpoint role")
}

// Relation represents a connection between one or more services.
type Relation struct {
	st  *State
	key string
}

// Key returns the internal key of a relation.
func (r *Relation) Key() string {
	return r.key
}

// ServiceRelation represents a relation between one or more services.
type ServiceRelation struct {
	st         *State
	key        string
	serviceKey string
	scope      RelationScope
	role       RelationRole
}

// Key returns the internal key of a relation.
func (r *ServiceRelation) Key() string {
	return r.key
}

// ServiceKey returns the service key of a relation.
func (r *ServiceRelation) ServiceKey() string {
	return r.serviceKey
}

// Scope returns the scope of a relation.
func (r *ServiceRelation) Scope() RelationScope {
	return r.scope
}

// Role returns the role of a relation.
func (r *ServiceRelation) Role() RelationRole {
	return r.role
}
