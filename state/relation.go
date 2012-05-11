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
