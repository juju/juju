package mstate

import (
	"fmt"
	"labix.org/v2/mgo"
	"labix.org/v2/mgo/bson"
	"launchpad.net/juju-core/charm"
	"sort"
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

// RelationEndpoint represents one endpoint of a relation.
type RelationEndpoint struct {
	ServiceName   string
	Interface     string
	RelationName  string
	RelationRole  RelationRole
	RelationScope charm.RelationScope
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

// relationKey returns a string describing the relation defined by
// endpoints, for use in various contexts (including error messages).
func relationKey(endpoints []RelationEndpoint) string {
	names := []string{}
	for _, ep := range endpoints {
		names = append(names, ep.String())
	}
	sort.Strings(names)
	return strings.Join(names, " ")
}

// relationDoc is the internal representation of a Relation in MongoDB.
type relationDoc struct {
	Id        int `bson:"_id"`
	Key       string
	Endpoints []RelationEndpoint
	Life      Life
}

// Relation represents a relation between one or two service endpoints.
type Relation struct {
	st  *State
	doc relationDoc
}

func newRelation(st *State, doc *relationDoc) *Relation {
	return &Relation{
		st:  st,
		doc: *doc,
	}
}

func (r *Relation) String() string {
	return r.doc.Key
}

func (r *Relation) Refresh() error {
	doc := relationDoc{}
	err := r.st.relations.FindId(r.doc.Id).One(&doc)
	if err != nil {
		return fmt.Errorf("cannot refresh relation %v: %v", r, err)
	}
	r.doc = doc
	return nil
}

func (r *Relation) Life() Life {
	return r.doc.Life
}

// ensureLife changes the lifecycle state of the relation.
// See the Life type for more details.
func (r *Relation) ensureLife(life Life) error {
	if life == Alive {
		panic("cannot set life to alive")
	}
	sel := bson.D{
		{"_id", r.doc.Id},
		// $lte is used so that we don't overwrite a previous
		// change we don't know about. 
		{"life", bson.D{{"$lte", life}}},
	}
	change := bson.D{{"$set", bson.D{{"life", life}}}}
	err := r.st.relations.Update(sel, change)
	if err == mgo.ErrNotFound {
		return nil
	}
	if err != nil {
		return fmt.Errorf("cannot set life to %v for relation %v: %v", life, r, err)
	}
	r.doc.Life = life
	return nil
}

// Kill sets the relation lifecycle to Dying if it is Alive.
// It does nothing otherwise.
func (r *Relation) Kill() error {
	return r.ensureLife(Dying)
}

// Die sets the relation lifecycle to Dead if it is Alive or Dying.
// It does nothing otherwise.
func (r *Relation) Die() error {
	return r.ensureLife(Dead)
}

// Id returns the integer internal relation key. This is exposed
// because the unit agent needs to expose a value derived from this
// (as JUJU_RELATION_ID) to allow relation hooks to differentiate
// between relations with different services.
func (r *Relation) Id() int {
	return r.doc.Id
}

// Endpoint returns the endpoint of the relation for the named service.
// If the service is not part of the relation, an error will be returned.
func (r *Relation) Endpoint(serviceName string) (RelationEndpoint, error) {
	for _, ep := range r.doc.Endpoints {
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
	for _, ep := range r.doc.Endpoints {
		if ep.RelationRole == role {
			eps = append(eps, ep)
		}
	}
	if eps == nil {
		return nil, fmt.Errorf("no endpoints of %q relate to service %q", r, serviceName)
	}
	return eps, nil
}

// RelationUnit holds information about a single unit in a relation, and
// allows clients to conveniently access unit-specific functionality.
type RelationUnit struct {
	st       *State
	relation *Relation
	unit     *Unit
	endpoint RelationEndpoint
}

// Relation returns the relation associated with the unit.
func (ru *RelationUnit) Relation() *Relation {
	return ru.relation
}

// Endpoint returns the relation endpoint that defines the unit's
// participation in the relation.
func (ru *RelationUnit) Endpoint() RelationEndpoint {
	return ru.endpoint
}

// Unit returns a RelationUnit for the supplied unit.
func (r *Relation) Unit(u *Unit) (*RelationUnit, error) {
	ep, err := r.Endpoint(u.doc.Service)
	if err != nil {
		return nil, err
	}
	return &RelationUnit{
		st:       r.st,
		relation: r,
		unit:     u,
		endpoint: ep,
	}, nil
}
