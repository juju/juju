package state

import (
	"fmt"
	"launchpad.net/gozk/zookeeper"
	"launchpad.net/juju-core/charm"
	"launchpad.net/juju-core/state/presence"
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
	return describeEndpoints(r.endpoints)
}

// Id returns the integer part of the internal relation key. This is
// exposed because the unit agent needs to expose a value derived from
// this (as JUJU_RELATION_ID) to allow relation hooks to differentiate
// between relations with different services.
func (r *Relation) Id() int {
	keyParts := strings.Split(r.key, "-")
	id, err := strconv.Atoi(keyParts[1])
	if err != nil {
		panic(fmt.Errorf("relation key %q in unknown format", r.key))
	}
	return id
}

// Endpoint returns the endpoint of the relation for the named service.
// If the service is not part of the relation, an error will be returned.
func (r *Relation) Endpoint(serviceName string) (RelationEndpoint, error) {
	for _, ep := range r.endpoints {
		if ep.ServiceName == serviceName {
			return ep, nil
		}
	}
	return RelationEndpoint{}, fmt.Errorf("service %q is not a member of relation %q", serviceName, r)
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
		return nil, fmt.Errorf("no endpoints of relation %q relate to service %q", r, serviceName)
	}
	return eps, nil
}

// Unit returns a RelationUnit for the supplied unit.
func (r *Relation) Unit(u *Unit) (*RelationUnit, error) {
	ep, err := r.Endpoint(u.serviceName)
	if err != nil {
		return nil, err
	}
	path := []string{"/relations", r.key}
	if ep.RelationScope == charm.ScopeContainer {
		container := u.principalKey
		if container == "" {
			container = u.key
		}
		path = append(path, container)
	}
	return &RelationUnit{
		st:       r.st,
		relation: r,
		unit:     u,
		endpoint: ep,
		scope:    unitScopePath(strings.Join(path, "/")),
	}, nil
}

// RelationUnit holds information about a single unit in a relation, and
// allows clients to conveniently access unit-specific functionality.
type RelationUnit struct {
	st       *State
	relation *Relation
	unit     *Unit
	endpoint RelationEndpoint
	scope    unitScopePath
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

// Join joins the unit to the relation, such that other units watching the
// relation will observe its presence and changes to its settings.
func (ru *RelationUnit) Join() (p *presence.Pinger, err error) {
	defer errorContextf(&err, "cannot join unit %q to relation %q", ru.unit, ru.relation)
	if err = ru.scope.prepareJoin(ru.st.zk, ru.endpoint.RelationRole); err != nil {
		return
	}
	// Private address should be set at agent startup.
	address, err := ru.unit.PrivateAddress()
	if err != nil {
		return
	}
	err = setConfigString(ru.st.zk, ru.scope.settingsPath(ru.unit.key), "private-address", address, "private address of relation unit")
	if err != nil {
		return
	}
	presencePath := ru.scope.presencePath(ru.endpoint.RelationRole, ru.unit.key)
	return presence.StartPinger(ru.st.zk, presencePath, agentPingerPeriod)
}

// Watch returns a watcher that notifies when any other unit in
// the relation joins, departs, or has its settings changed.
func (ru *RelationUnit) Watch() *RelationUnitsWatcher {
	role := ru.endpoint.RelationRole.counterpartRole()
	return newRelationUnitsWatcher(ru.scope, role, ru.unit)
}

// Settings returns a ConfigNode which allows access to the unit's settings
// within the relation.
func (ru *RelationUnit) Settings() (*ConfigNode, error) {
	return readConfigNode(ru.st.zk, ru.scope.settingsPath(ru.unit.key))
}

// ReadSettings returns a map holding the settings of the unit with the
// supplied name. An error will be returned if the relation no longer
// exists, or if the unit's service is not part of the relation, or the
// settings are invalid; but mere non-existence of the unit is not grounds
// for an error, because the unit settings are guaranteed to persist for
// the lifetime of the relation.
func (ru *RelationUnit) ReadSettings(uname string) (settings map[string]interface{}, err error) {
	defer errorContextf(&err, "cannot read settings for unit %q in relation %q", uname, ru.relation)
	topo, err := readTopology(ru.st.zk)
	if err != nil {
		return nil, err
	}
	if !topo.HasRelation(ru.relation.key) {
		// TODO this will be a problem until we have lifecycle management. IE,
		// we expect to occasionally call this method during hook execution;
		// until we can defer relation death until all interested parties have
		// lost interest, we're in danger of the relation disappearing on us.
		return nil, fmt.Errorf("relation broken; settings no longer accessible")
	}
	sname, useq, err := parseUnitName(uname)
	if err != nil {
		return nil, err
	}
	if _, err = ru.relation.Endpoint(sname); err != nil {
		return nil, err
	}
	skey, err := topo.ServiceKey(sname)
	if err != nil {
		return nil, err
	}
	ukey := makeUnitKey(skey, useq)
	path := ru.scope.settingsPath(ukey)
	if stat, err := ru.st.zk.Exists(path); err != nil {
		return nil, err
	} else if stat == nil {
		return nil, fmt.Errorf("unit settings do not exist")
	}
	node, err := readConfigNode(ru.st.zk, path)
	if err != nil {
		return nil, err
	}
	return node.Map(), nil
}

// unitScopePath represents a common zookeeper path used by the set of units
// that can (transitively) affect one another within the context of a
// particular relation. For a globally-scoped relation, every unit is part of
// the same scope; but for a container-scoped relation, each unit is is a
// scope shared only with the units that share a container.
// Thus, unitScopePaths will take one of the following forms:
//
//   /relations/<relation-id>
//   /relations/<relation-id>/<container-id>
type unitScopePath string

// settingsPath returns the path to the relation unit settings node for the
// unit identified by unitKey, or to the relation scope settings node if
// unitKey is empty.
func (s unitScopePath) settingsPath(unitKey string) string {
	return s.subpath("settings", unitKey)
}

// presencePath returns the path to the relation unit presence node for a
// unit (identified by unitKey) of a service acting as role; or to the relation
// scope role node if unitKey is empty.
func (s unitScopePath) presencePath(role RelationRole, unitKey string) string {
	return s.subpath(string(role), unitKey)
}

// prepareJoin ensures that ZooKeeper nodes exist such that a unit of a
// service with the supplied role will be able to join the relation.
func (s unitScopePath) prepareJoin(zk *zookeeper.Conn, role RelationRole) error {
	paths := []string{string(s), s.settingsPath(""), s.presencePath(role, "")}
	for _, path := range paths {
		if _, err := zk.Create(path, "", 0, zkPermAll); err != nil {
			if zookeeper.IsError(err, zookeeper.ZNODEEXISTS) {
				continue
			}
			return err
		}
	}
	return nil
}

// subpath returns an absolute ZooKeeper path to the node whose path relative
// to the scope node is composed of parts. Empty parts will be stripped.
func (s unitScopePath) subpath(parts ...string) string {
	path := string(s)
	for _, part := range parts {
		if part != "" {
			path = path + "/" + part
		}
	}
	return path
}
