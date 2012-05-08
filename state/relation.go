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
	ServiceName   string        `yaml:"service_name"`
	RelationType  string        `yaml:"relation_type"`
	RelationName  string        `yaml:"relation_name"`
	RelationRole  RelationRole  `yaml:"relation_role"`
	RelationScope RelationScope `yaml:"relation_scope"`
}

// MayRelateTo tests whether the "other"`" endpoint may be used in a common 
// relation.
// 
// RelationEndpoints may be related if they share the same RelationType
// (which is called an "interface" in charms) and one is a 'provides'
// and the other is a 'requires'; or if both endpoints have a
// RelationRole of 'peers'.
func (e *RelationEndpoint) MayRelateTo(other *RelationEndpoint) bool {
	return (e.RelationType == other.RelationType &&
		((e.RelationRole == RoleServer && other.RelationRole == RoleClient) ||
			(e.RelationRole == RoleClient && other.RelationRole == RoleServer) ||
			(e.RelationRole == RolePeer && other.RelationRole == RolePeer)))
}
