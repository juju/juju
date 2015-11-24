// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state

import (
	"fmt"
	"sort"

	"github.com/juju/errors"
	"github.com/juju/juju/model/crossmodel"
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
	DocID         string              `bson:"_id"`
	Name          string              `bson:"name"`
	URL           string              `bson:"url"`
	Endpoints     []remoteEndpointDoc `bson:"endpoints"`
	Life          Life                `bson:"life"`
	RelationCount int                 `bson:"relationcount"`
}

// remoteEndpointDoc represents the internal state of a remote service endpoint in MongoDB.
type remoteEndpointDoc struct {
	Name      string              `bson:"name"`
	Role      charm.RelationRole  `bson:"role"`
	Interface string              `bson:"interface"`
	Limit     int                 `bson:"limit"`
	Scope     charm.RelationScope `bson:"scope"`
}

func newRemoteService(st *State, doc *remoteServiceDoc) *RemoteService {
	svc := &RemoteService{
		st:  st,
		doc: *doc,
	}
	return svc
}

// IsRemote returns true for a remote service.
func (s *RemoteService) IsRemote() bool {
	return true
}

// Name returns the service name.
func (s *RemoteService) Name() string {
	return s.doc.Name
}

// URL returns the remote service URL.
func (s *RemoteService) URL() string {
	return s.doc.URL
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
func (s *RemoteService) Endpoints() ([]Endpoint, error) {
	return remoteEndpointDocsToEndpoints(s.Name(), s.doc.Endpoints), nil
}

func remoteEndpointDocsToEndpoints(serviceName string, docs []remoteEndpointDoc) []Endpoint {
	eps := make([]Endpoint, len(docs))
	for i, ep := range docs {
		eps[i] = Endpoint{
			ServiceName: serviceName,
			Relation: charm.Relation{
				Name:      ep.Name,
				Role:      ep.Role,
				Interface: ep.Interface,
				Limit:     ep.Limit,
				Scope:     ep.Scope,
			}}
	}
	sort.Sort(epSlice(eps))
	return eps
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

var (
	errSameNameLocalServiceExists = errors.Errorf("local service with same name already exists")
	errRemoteServiceExists        = errors.Errorf("remote service already exists")
)

// AddRemoteService creates a new remote service record, having the supplied relation endpoints,
// with the supplied name (which must be unique across all services, local and remote).
func (st *State) AddRemoteService(name, url string, endpoints []charm.Relation) (service *RemoteService, err error) {
	defer errors.DeferredAnnotatef(&err, "cannot add remote service %q", name)

	// Sanity checks.
	if !names.IsValidService(name) {
		return nil, errors.Errorf("invalid name")
	}
	if _, err := crossmodel.ParseServiceURL(url); err != nil {
		return nil, errors.Annotate(err, "validating service URL")
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
		DocID: serviceID,
		Name:  name,
		URL:   url,
		Life:  Alive,
	}
	eps := make([]remoteEndpointDoc, len(endpoints))
	for i, ep := range endpoints {
		eps[i] = remoteEndpointDoc{
			Name:      ep.Name,
			Role:      ep.Role,
			Interface: ep.Interface,
			Limit:     ep.Limit,
			Scope:     ep.Scope,
		}
	}
	svcDoc.Endpoints = eps
	svc := newRemoteService(st, svcDoc)

	buildTxn := func(attempt int) ([]txn.Op, error) {
		// If we've tried once already and failed, check that
		// environment may have been destroyed.
		if attempt > 0 {
			if err := checkEnvLife(st); err != nil {
				return nil, errors.Trace(err)
			}
			// Ensure a local service with the same name doesn't exist.
			if localExists, err := isNotDead(st, servicesC, name); err != nil {
				return nil, errors.Trace(err)
			} else if localExists {
				return nil, errSameNameLocalServiceExists
			}
			// Ensure a remote service with the same name doesn't exist.
			if exists, err := isNotDead(st, remoteServicesC, name); err != nil {
				return nil, errors.Trace(err)
			} else if exists {
				return nil, errRemoteServiceExists
			}
		}
		ops := []txn.Op{
			env.assertAliveOp(),
			{
				C:      remoteServicesC,
				Id:     serviceID,
				Assert: txn.DocMissing,
				Insert: svcDoc,
			}, {
				C:      servicesC,
				Id:     serviceID,
				Assert: txn.DocMissing,
			},
		}
		return ops, nil
	}
	if err = st.run(buildTxn); err != nil {
		return nil, errors.Trace(err)
	}
	return svc, nil
}

// RemoteService returns a remote service state by name.
func (st *State) RemoteService(name string) (service *RemoteService, err error) {
	services, closer := st.getCollection(remoteServicesC)
	defer closer()

	if !names.IsValidService(name) {
		return nil, errors.NotValidf("remote service name %q", name)
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
