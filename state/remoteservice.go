// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state

import (
	"fmt"
	"sort"
	"strings"

	"github.com/juju/errors"
	"github.com/juju/names"
	jujutxn "github.com/juju/txn"
	"gopkg.in/juju/charm.v6-unstable"
	"gopkg.in/mgo.v2"
	"gopkg.in/mgo.v2/bson"
	"gopkg.in/mgo.v2/txn"
)

// RemoteService represents the state of a service hosted
// in an external (remote) environment.
type RemoteService struct {
	st  *State
	doc remoteServiceDoc
}

// remoteServiceDoc represents the internal state of a remote service in MongoDB.
type remoteServiceDoc struct {
	DocID         string     `bson:"_id"`
	Name          string     `bson:"name"`
	Endpoints     []Endpoint `bson:"endpoints"`
	Life          Life       `bson:"life"`
	RelationCount int        `bson:"relationcount"`
}

func newRemoteService(st *State, doc *remoteServiceDoc) *RemoteService {
	svc := &RemoteService{
		st:  st,
		doc: *doc,
	}
	return svc
}

// Name returns the service name.
func (s *RemoteService) Name() string {
	return s.doc.Name
}

// Tag returns a name identifying the service.
func (s *RemoteService) Tag() names.Tag {
	return names.NewServiceTag(s.Name())
}

// Life returns whether the service is Alive, Dying or Dead.
func (s *RemoteService) Life() Life {
	return s.doc.Life
}

// Destroy ensures that this remote service reference and all its relations
// will be removed at some point; if no relation involving the
// service has any units in scope, they are all removed immediately.
func (s *RemoteService) Destroy() (err error) {
	defer errors.DeferredAnnotatef(&err, "cannot destroy remote service %q", s)
	defer func() {
		if err == nil {
			s.doc.Life = Dying
		}
	}()
	svc := &RemoteService{st: s.st, doc: s.doc}
	buildTxn := func(attempt int) ([]txn.Op, error) {
		if attempt > 0 {
			if err := svc.Refresh(); errors.IsNotFound(err) {
				return nil, jujutxn.ErrNoOperations
			} else if err != nil {
				return nil, err
			}
		}
		switch ops, err := svc.destroyOps(); err {
		case errRefresh:
		case errAlreadyDying:
			return nil, jujutxn.ErrNoOperations
		case nil:
			return ops, nil
		default:
			return nil, err
		}
		return nil, jujutxn.ErrTransientFailure
	}
	return s.st.run(buildTxn)
}

// destroyOps returns the operations required to destroy the service. If it
// returns errRefresh, the service should be refreshed and the destruction
// operations recalculated.
func (s *RemoteService) destroyOps() ([]txn.Op, error) {
	if s.doc.Life == Dying {
		return nil, errAlreadyDying
	}
	rels, err := s.Relations()
	if err != nil {
		return nil, err
	}
	if len(rels) != s.doc.RelationCount {
		// This is just an early bail out. The relations obtained may still
		// be wrong, but that situation will be caught by a combination of
		// asserts on relationcount and on each known relation, below.
		return nil, errRefresh
	}
	var ops []txn.Op
	removeCount := 0
	for _, rel := range rels {
		relOps, isRemove, err := rel.destroyOps(s.doc.Name)
		if err == errAlreadyDying {
			relOps = []txn.Op{{
				C:      relationsC,
				Id:     rel.doc.DocID,
				Assert: bson.D{{"life", Dying}},
			}}
		} else if err != nil {
			return nil, err
		}
		if isRemove {
			removeCount++
		}
		ops = append(ops, relOps...)
	}
	// If all of the service's known relations will be
	// removed, the service can also be removed.
	if s.doc.RelationCount == removeCount {
		hasLastRefs := bson.D{{"life", Alive}, {"relationcount", removeCount}}
		return append(ops, s.removeOps(hasLastRefs)...), nil
	}
	// In all other cases, service removal will be handled as a consequence
	// of the removal of the relation referencing it. If any  relations have
	// been removed, they'll be caught by the operations collected above;
	// but if any has been added, we need to abort and add  a destroy op for
	// that relation too.
	// In combination, it's enough to check for count equality:
	// an add/remove will not touch the count, but  will be caught by
	// virtue of being a remove.
	notLastRefs := bson.D{
		{"life", Alive},
		{"relationcount", s.doc.RelationCount},
	}
	update := bson.D{{"$set", bson.D{{"life", Dying}}}}
	if removeCount != 0 {
		decref := bson.D{{"$inc", bson.D{{"relationcount", -removeCount}}}}
		update = append(update, decref...)
	}
	return append(ops, txn.Op{
		C:      remoteServicesC,
		Id:     s.doc.DocID,
		Assert: notLastRefs,
		Update: update,
	}), nil
}

// removeOps returns the operations required to remove the service. Supplied
// asserts will be included in the operation on the service document.
func (s *RemoteService) removeOps(asserts bson.D) []txn.Op {
	ops := []txn.Op{
		{
			C:      remoteServicesC,
			Id:     s.doc.DocID,
			Assert: asserts,
			Remove: true,
		},
	}
	return ops
}

// Endpoints returns the service's currently available relation endpoints.
func (s *RemoteService) Endpoints() (eps []Endpoint, err error) {
	eps = s.doc.Endpoints
	sort.Sort(epSlice(eps))
	return eps, nil
}

// Endpoint returns the relation endpoint with the supplied name, if it exists.
func (s *RemoteService) Endpoint(relationName string) (Endpoint, error) {
	eps, err := s.Endpoints()
	if err != nil {
		return Endpoint{}, err
	}
	for _, ep := range eps {
		if ep.Name == relationName {
			return ep, nil
		}
	}
	return Endpoint{}, fmt.Errorf("remote service %q has no %q relation", s, relationName)
}

// String returns the service name.
func (s *RemoteService) String() string {
	return s.doc.Name
}

// Refresh refreshes the contents of the Service from the underlying
// state. It returns an error that satisfies errors.IsNotFound if the
// service has been removed.
func (s *RemoteService) Refresh() error {
	services, closer := s.st.getCollection(remoteServicesC)
	defer closer()

	err := services.FindId(s.doc.DocID).One(&s.doc)
	if err == mgo.ErrNotFound {
		return errors.NotFoundf("remote service %q", s)
	}
	if err != nil {
		return fmt.Errorf("cannot refresh service %q: %v", s, err)
	}
	return nil
}

// Relations returns a Relation for every relation the service is in.
func (s *RemoteService) Relations() (relations []*Relation, err error) {
	return serviceRelations(s.st, s.doc.Name)
}

// AddRemoteService creates a new remote service record, running the supplied charm, with the
// supplied name (which must be unique). If the charm defines peer relations,
// they will be created automatically.
func (st *State) AddRemoteService(name string, endpoints []Endpoint) (service *RemoteService, err error) {
	defer errors.DeferredAnnotatef(&err, "cannot add remote service %q", name)
	// Sanity checks.
	if !names.IsValidService(name) {
		return nil, errors.Errorf("invalid name")
	}
	if exists, err := isNotDead(st, remoteServicesC, name); err != nil {
		return nil, errors.Trace(err)
	} else if exists {
		return nil, errors.Errorf("remote service already exists")
	}
	env, err := st.Environment()
	if err != nil {
		return nil, errors.Trace(err)
	} else if env.Life() != Alive {
		return nil, errors.Errorf("environment is no longer alive")
	}
	serviceID := st.docID(name)
	// Create the service addition operations.
	svcDoc := &remoteServiceDoc{
		DocID:     serviceID,
		Name:      name,
		Life:      Alive,
		Endpoints: endpoints,
	}
	// Mark the endpoints for this remote service.
	for i, ep := range svcDoc.Endpoints {
		ep.IsRemote = true
		svcDoc.Endpoints[i] = ep
	}
	svc := newRemoteService(st, svcDoc)

	ops := []txn.Op{
		env.assertAliveOp(),
		{
			C:      remoteServicesC,
			Id:     serviceID,
			Assert: txn.DocMissing,
			Insert: svcDoc,
		},
	}

	if err := st.runTransaction(ops); err == txn.ErrAborted {
		if err := checkEnvLife(st); err != nil {
			return nil, errors.Trace(err)
		}
		return nil, errors.Errorf("remote service already exists")
	} else if err != nil {
		return nil, errors.Trace(err)
	}
	// Refresh to pick the txn-revno.
	if err = svc.Refresh(); err != nil {
		return nil, errors.Trace(err)
	}
	return svc, nil
}

// RemoteService returns a remote service state by name.
func (st *State) RemoteService(name string) (service *RemoteService, err error) {
	services, closer := st.getCollection(remoteServicesC)
	defer closer()

	if !names.IsValidService(name) {
		return nil, errors.Errorf("%q is not a valid service name", name)
	}
	sdoc := &remoteServiceDoc{}
	err = services.FindId(name).One(sdoc)
	if err == mgo.ErrNotFound {
		return nil, errors.NotFoundf("remote service %q", name)
	}
	if err != nil {
		return nil, errors.Annotatef(err, "cannot get remote service %q", name)
	}
	return newRemoteService(st, sdoc), nil
}

// AllRemoteServices returns all the remote services used by the environment.
func (st *State) AllRemoteServices() (services []*RemoteService, err error) {
	servicesCollection, closer := st.getCollection(remoteServicesC)
	defer closer()

	sdocs := []remoteServiceDoc{}
	err = servicesCollection.Find(bson.D{}).All(&sdocs)
	if err != nil {
		return nil, errors.Errorf("cannot get all remote services")
	}
	for _, v := range sdocs {
		services = append(services, newRemoteService(st, &v))
	}
	return services, nil
}

func remoteServiceEndpointsFunc(st *State, name string) (EndpointsEntity, error) {
	s, err := st.RemoteService(name)
	if err != nil {
		return nil, err
	}
	return s, nil
}

// RelatedEndpoints returns the endpoints corresponding to the supplied
// local and remote service names. Names are of the form <service>[:<relation>].
// If the supplied names uniquely specify a possible relation, or if they
// uniquely specify a possible relation once all implicit relations have been
// filtered, the endpoints corresponding to that relation will be returned.
func (st *State) RelatedEndpoints(serviceName, remoteServiceName string) ([]Endpoint, error) {
	// Collect all possible sane endpoint lists.
	var candidates [][]Endpoint
	eps1, err := st.endpoints(serviceName, serviceEndpointsFunc, notPeer)
	if err != nil {
		return nil, errors.Trace(err)
	}
	eps2, err := st.endpoints(remoteServiceName, remoteServiceEndpointsFunc, notPeer)
	if err != nil {
		return nil, errors.Trace(err)
	}
	for _, ep1 := range eps1 {
		for _, ep2 := range eps2 {
			if ep1.CanRelateTo(ep2) && ep1.Scope != charm.ScopeContainer {
				ep2.IsRemote = true
				candidates = append(candidates, []Endpoint{ep1, ep2})
			}
		}
	}

	// We want exactly on candidate, or else we error.
	switch len(candidates) {
	case 0:
		return nil, errors.Errorf("no relations found")
	case 1:
		return candidates[0], nil
	}
	keys := []string{}
	for _, cand := range candidates {
		keys = append(keys, fmt.Sprintf("%q", relationKey(cand)))
	}
	sort.Strings(keys)
	return nil, errors.Errorf("ambiguous relation: [%q %q] could refer to %s",
		serviceName, remoteServiceName, strings.Join(keys, "; "))
}

// AddRemoteRelation creates a new relation with the given local and remote endpoints.
func (st *State) AddRemoteRelation(localEp, remoteEp Endpoint) (r *Relation, err error) {
	key := relationKey([]Endpoint{localEp, remoteEp})
	defer errors.DeferredAnnotatef(&err, "cannot add relation %q", key)
	if !localEp.CanRelateTo(remoteEp) {
		return nil, errors.Errorf("endpoints do not relate")
	}
	if localEp.IsRemote {
		return nil, errors.Errorf("expecting endpoint %q to be for a local service", localEp.Name)
	}
	if !remoteEp.IsRemote {
		return nil, errors.Errorf("expecting endpoint %q to be for a remote service", remoteEp.Name)
	}
	// Santity check - neither endpoint can be container scoped.
	if localEp.Scope == charm.ScopeContainer || remoteEp.Scope == charm.ScopeContainer {
		return nil, errors.Errorf("both endpoints must be globally scoped")
	}
	// We only get a unique relation id once, to save on roundtrips. If it's
	// -1, we haven't got it yet (we don't get it at this stage, because we
	// still don't know whether it's sane to even attempt creation).
	id := -1
	// If a local service's charm is upgraded while we're trying to add a relation,
	// we'll need to re-validate service sanity.
	var doc *relationDoc
	buildTxn := func(attempt int) ([]txn.Op, error) {
		// Perform initial relation sanity check.
		if exists, err := isNotDead(st, relationsC, key); err != nil {
			return nil, errors.Trace(err)
		} else if exists {
			return nil, errors.Errorf("relation already exists")
		}
		// Collect per-service operations, checking sanity as we go.
		var ops []txn.Op
		// Increment relation count for local service.
		localSvc, err := st.Service(localEp.ServiceName)
		if errors.IsNotFound(err) {
			return nil, errors.Errorf("service %q does not exist", localEp.ServiceName)
		} else if err != nil {
			return nil, errors.Trace(err)
		} else if localSvc.doc.Life != Alive {
			return nil, errors.Errorf("service %q is not alive", localEp.ServiceName)
		}
		if localSvc.doc.Subordinate {
			return nil, errors.Errorf("cannot relate subordinate %q to remote service %q", localEp.ServiceName, remoteEp.ServiceName)
		}
		ch, _, err := localSvc.Charm()
		if err != nil {
			return nil, errors.Trace(err)
		}
		if !localEp.ImplementedBy(ch) {
			return nil, errors.Errorf("%q does not implement %q", localEp.ServiceName, localEp)
		}
		ops = append(ops, txn.Op{
			C:      servicesC,
			Id:     st.docID(localEp.ServiceName),
			Assert: bson.D{{"life", Alive}, {"charmurl", ch.URL()}},
			Update: bson.D{{"$inc", bson.D{{"relationcount", 1}}}},
		})
		// Increment relation count for remote service.
		remoteSvc, err := st.RemoteService(remoteEp.ServiceName)
		if errors.IsNotFound(err) {
			return nil, errors.Errorf("remote service %q does not exist", remoteEp.ServiceName)
		} else if err != nil {
			return nil, errors.Trace(err)
		} else if remoteSvc.doc.Life != Alive {
			return nil, errors.Errorf("remote service %q is not alive", remoteEp.ServiceName)
		}
		ops = append(ops, txn.Op{
			C:      remoteServicesC,
			Id:     st.docID(remoteEp.ServiceName),
			Assert: bson.D{{"life", Alive}},
			Update: bson.D{{"$inc", bson.D{{"relationcount", 1}}}},
		})

		// Create a new unique id if that has not already been done, and add
		// an operation to create the relation document.
		if id == -1 {
			var err error
			if id, err = st.sequence("relation"); err != nil {
				return nil, errors.Trace(err)
			}
		}
		docID := st.docID(key)
		doc = &relationDoc{
			DocID:     docID,
			Key:       key,
			EnvUUID:   st.EnvironUUID(),
			Id:        id,
			Endpoints: []Endpoint{localEp, remoteEp},
			Life:      Alive,
		}
		ops = append(ops, txn.Op{
			C:      relationsC,
			Id:     docID,
			Assert: txn.DocMissing,
			Insert: doc,
		})
		return ops, nil
	}
	if err = st.run(buildTxn); err == nil {
		return &Relation{st, *doc}, nil
	}
	return nil, errors.Trace(err)
}
