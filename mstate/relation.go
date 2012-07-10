package mstate

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

// counterpartRole returns the RelationRole that this RelationRole
// can relate to.
// This should remain an internal method because the relation
// model does not guarantee that for every role there will
// necessarily exist a single counterpart role that is sensible
// for basing algorithms upon.
func (r RelationRole) counterpartRole() RelationRole {
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
	return e.RelationRole.counterpartRole() == other.RelationRole
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

// endpointPair is a type encompassing the one or two endpoints
// of a relation in order to use them as an index in MongoDB.
type endpointPair struct {
	Cardinality int
	P0          RelationEndpoint
	P1          RelationEndpoint `bson:",omitempty"`
}

// newEndpointPair returns a *endpointPair from endpoints.
// Endpoints order does not matter.
func newEndpointPair(endpoints ...RelationEndpoint) *endpointPair {
	if 0 == len(endpoints) || len(endpoints) > 2 {
		panic("must have only one or two endpoints")
	}
	if len(endpoints) == 1 {
		return &endpointPair{P0: endpoints[0], Cardinality: 1}
	}
	if fmt.Sprintf("%v", endpoints[0]) < fmt.Sprintf("%v", endpoints[1]) {
		return &endpointPair{P0: endpoints[0], P1: endpoints[1], Cardinality: 2}
	}
	return &endpointPair{P0: endpoints[1], P1: endpoints[0], Cardinality: 2}
}

func (p endpointPair) RelationEndpointSlice() []RelationEndpoint {
	if 0 == p.Cardinality || p.Cardinality > 2 {
		panic("must have only one or two endpoints")
	}
	if p.Cardinality == 1 {
		return []RelationEndpoint{p.P0}
	}
	return []RelationEndpoint{p.P0, p.P1}
}

// relationDoc is the internal representation of a Relation in MongoDB.
type relationDoc struct {
	Life      Life
	Id        int
	Endpoints endpointPair `bson:"_id"`
}

// Relation represents a relation between one or two service endpoints.
type Relation struct {
	st        *State
	id        int
	endpoints []RelationEndpoint
}

func newRelation(st *State, rdoc *relationDoc) *Relation {
	return &Relation{
		st:        st,
		id:        rdoc.Id,
		endpoints: rdoc.Endpoints.RelationEndpointSlice(),
	}
}

func (r *Relation) String() string {
	return fmt.Sprintf("relation %q", describeEndpoints(r.endpoints))
}

// Id returns the integer internal relation key. This is exposed
// because the unit agent needs to expose a value derived from this
// (as JUJU_RELATION_ID) to allow relation hooks to differentiate
// between relations with different services.
func (r *Relation) Id() int {
	return r.id
}

// Endpoint returns the endpoint of the relation for the named service.
// If the service is not part of the relation, an error will be returned.
func (r *Relation) Endpoint(serviceName string) (RelationEndpoint, error) {
	for _, ep := range r.endpoints {
		if ep.ServiceName == serviceName {
			return ep, nil
		}
	}
	return RelationEndpoint{}, fmt.Errorf("service %q is not a member of %q", serviceName, r)
}

// RelatedEndpoints returns the endpoints of the relation r with which
// units of the named service will establish relations. If the service
// is not part of the relation r, an error will be returned.
func (r *Relation) RelatedEndpoints(serviceName string) ([]RelationEndpoint, error) {
	local, err := r.Endpoint(serviceName)
	if err != nil {
		return nil, err
	}
	role := local.RelationRole.counterpartRole()
	var eps []RelationEndpoint
	for _, ep := range r.endpoints {
		if ep.RelationRole == role {
			eps = append(eps, ep)
		}
	}
	if eps == nil {
		return nil, fmt.Errorf("no endpoints of %q relate to service %q", r, serviceName)
	}
	return eps, nil
}
