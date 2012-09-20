package state

import (
	"fmt"
	"launchpad.net/juju-core/charm"
	"launchpad.net/juju-core/trivial"
	"sort"
	"strconv"
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
	Key       string `bson:"_id"`
	Id        int
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
	err := r.st.relations.FindId(r.doc.Key).One(&doc)
	if err != nil {
		return fmt.Errorf("cannot refresh relation %v: %v", r, err)
	}
	r.doc = doc
	return nil
}

func (r *Relation) Life() Life {
	return r.doc.Life
}

// Kill sets the relation lifecycle to Dying if it is Alive.
// It does nothing otherwise.
func (r *Relation) Kill() error {
	err := ensureLife(r.st, r.st.relations, r.doc.Key, Dying, "relation")
	if err != nil {
		return err
	}
	r.doc.Life = Dying
	return nil
}

// Die sets the relation lifecycle to Dead if it is Alive or Dying.
// It does nothing otherwise.
func (r *Relation) Die() error {
	err := ensureLife(r.st, r.st.relations, r.doc.Key, Dead, "relation")
	if err != nil {
		return err
	}
	r.doc.Life = Dead
	return nil
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

// Unit returns a RelationUnit for the supplied unit.
func (r *Relation) Unit(u *Unit) (*RelationUnit, error) {
	ep, err := r.Endpoint(u.doc.Service)
	if err != nil {
		return nil, err
	}
	scope := []string{"r", strconv.Itoa(r.doc.Id)}
	if ep.RelationScope == charm.ScopeContainer {
		container := u.doc.Principal
		if container == "" {
			container = u.doc.Name
		}
		scope = append(scope, container)
	}
	return &RelationUnit{
		st:       r.st,
		relation: r,
		unit:     u,
		endpoint: ep,
		scope:    strings.Join(scope, "#"),
	}, nil
}

// RelationUnit holds information about a single unit in a relation, and
// allows clients to conveniently access unit-specific functionality.
type RelationUnit struct {
	st       *State
	relation *Relation
	unit     *Unit
	endpoint RelationEndpoint
	scope    string
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

// Settings returns a ConfigNode which allows access to the unit's settings
// within the relation.
func (ru *RelationUnit) Settings() (*ConfigNode, error) {
	key, err := ru.key(ru.unit.Name())
	if err != nil {
		return nil, err
	}
	return readConfigNode(ru.st, key)
}

// ReadSettings returns a map holding the settings of the unit with the
// supplied name within this relation. An error will be returned if the
// relation no longer exists, or if the unit's service is not part of the
// relation, or the settings are invalid; but mere non-existence of the
// unit is not grounds for an error, because the unit settings are
// guaranteed to persist for the lifetime of the relation, regardless
// of the lifetime of the unit.
func (ru *RelationUnit) ReadSettings(uname string) (m map[string]interface{}, err error) {
	defer trivial.ErrorContextf(&err, "cannot read settings for unit %q in relation %q", uname, ru.relation)
	if !ValidUnit.MatchString(uname) {
		return nil, fmt.Errorf("%q is not a valid unit name", uname)
	}
	key, err := ru.key(uname)
	if err != nil {
		return nil, err
	}
	// TODO drop Count once readConfigNode refuses to read
	// non-existent settings (which it should).
	if n, err := ru.st.settings.FindId(key).Count(); err != nil {
		return nil, err
	} else if n == 0 {
		return nil, fmt.Errorf("not found")
	}
	node, err := readConfigNode(ru.st, key)
	if err != nil {
		return nil, err
	}
	return node.Map(), nil
}

// key returns a string, based on the relation and the supplied unit name,
// which is used as a key for that unit within this relation in the settings,
// presence, and relationRefs collections.
func (ru *RelationUnit) key(uname string) (string, error) {
	uparts := strings.Split(uname, "/")
	sname := uparts[0]
	ep, err := ru.relation.Endpoint(sname)
	if err != nil {
		return "", err
	}
	parts := []string{ru.scope, string(ep.RelationRole), uname}
	return strings.Join(parts, "#"), nil
}
