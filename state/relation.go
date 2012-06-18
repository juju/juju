package state

import (
	"fmt"
	"strings"
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

// describeEndpoints returns a string describing the relation defined by
// endpoints, for use in various contexts (including error messages).
func describeEndpoints(endpoints []RelationEndpoint) string {
	names := []string{}
	for _, ep := range endpoints {
		names = append(names, ep.String())
	}
	return strings.Join(names, " ")
}

// Relation represents a relation between one or two service endpoints.
type Relation struct {
	st        *State
	key       string
	endpoints []RelationEndpoint
}

func (r *Relation) String() string {
	return fmt.Sprintf("relation %q", describeEndpoints(r.endpoints))
}

// Endpoint returns the endpoint of the relation attached to the named service.
// If the service is not part of the relation, an error will be returned.
func (r *Relation) Endpoint(serviceName string) (RelationEndpoint, error) {
	for _, ep := range r.endpoints {
		if ep.ServiceName == serviceName {
			return ep, nil
		}
	}
	return RelationEndpoint{}, fmt.Errorf("service %q is not a member of %q", serviceName, r)
}

// RelatedEndpoint returns the endpoint of the relation to which units of the
// named service relate. In the case of a peer relation, this will have the same
// result as calling Endpoint with the same serviceName. If the service is not
// part of the relation, an error will be returned.
func (r *Relation) RelatedEndpoint(serviceName string) (RelationEndpoint, error) {
	valid := false
	index := 0
	for i, poss := range r.endpoints {
		if poss.ServiceName == serviceName {
			valid = true
		} else {
			index = i
		}
	}
	if !valid {
		return RelationEndpoint{}, fmt.Errorf("service %q is not a member of %q", serviceName, r)
	}
	return r.endpoints[index], nil
}
