// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state

import (
	"fmt"
	"sort"

	"github.com/juju/errors"
	"github.com/juju/names"
	jujutxn "github.com/juju/txn"
	"gopkg.in/juju/charm.v6-unstable"
	"gopkg.in/mgo.v2"
	"gopkg.in/mgo.v2/bson"
	"gopkg.in/mgo.v2/txn"
)

// ServiceDirectoryRecord represents the state of a service hosted
// in an external (remote) environment.
type ServiceDirectoryRecord struct {
	st  *State
	doc serviceDirectoryDoc
}

// serviceDirectoryDoc represents the internal state of a service directory record in MongoDB.
type serviceDirectoryDoc struct {
	DocID              string              `bson:"_id"`
	URL                string              `bson:"url"`
	SourceEnvUUID      string              `bson:"sourceuuid"`
	SourceLabel        string              `bson:"sourcelabel"`
	ServiceName        string              `bson:"servicename"`
	ServiceDescription string              `bson:"servicedescription"`
	Endpoints          []remoteEndpointDoc `bson:"endpoints"`
}

func newServiceDirectoryRecord(st *State, doc *serviceDirectoryDoc) *ServiceDirectoryRecord {
	record := &ServiceDirectoryRecord{
		st:  st,
		doc: *doc,
	}
	return record
}

// ServiceName returns the service URL.
func (s *ServiceDirectoryRecord) URL() string {
	return s.doc.URL
}

// ServiceName returns the service name.
func (s *ServiceDirectoryRecord) ServiceName() string {
	return s.doc.ServiceName
}

// ServiceDescription returns the service name.
func (s *ServiceDirectoryRecord) ServiceDescription() string {
	return s.doc.ServiceDescription
}

// SourceLabel returns the label of the source environment.
func (s *ServiceDirectoryRecord) SourceLabel() string {
	return s.doc.SourceLabel
}

// SourceEnvUUID returns the uuid of the source environment.
func (s *ServiceDirectoryRecord) SourceEnvUUID() string {
	return s.doc.SourceEnvUUID
}

// Destroy deletes the service directory record immediately.
func (s *ServiceDirectoryRecord) Destroy() (err error) {
	defer errors.DeferredAnnotatef(&err, "cannot destroy service directory record %q", s)
	record := &ServiceDirectoryRecord{st: s.st, doc: s.doc}
	buildTxn := func(attempt int) ([]txn.Op, error) {
		if attempt > 0 {
			if err := record.Refresh(); errors.IsNotFound(err) {
				return nil, jujutxn.ErrNoOperations
			} else if err != nil {
				return nil, err
			}
		}
		return record.destroyOps()
	}
	return s.st.run(buildTxn)
}

// destroyOps returns the operations required to destroy the record.
func (s *ServiceDirectoryRecord) destroyOps() ([]txn.Op, error) {
	return []txn.Op{
		{
			C:      serviceDirectoryC,
			Id:     s.doc.DocID,
			Assert: txn.DocExists,
			Remove: true,
		},
	}, nil
}

// Endpoints returns the service record's currently available relation endpoints.
func (s *ServiceDirectoryRecord) Endpoints() ([]Endpoint, error) {
	eps := make([]Endpoint, len(s.doc.Endpoints))
	for i, ep := range s.doc.Endpoints {
		eps[i] = Endpoint{
			ServiceName: s.ServiceName(),
			Relation: charm.Relation{
				Name:      ep.Name,
				Role:      ep.Role,
				Interface: ep.Interface,
				Limit:     ep.Limit,
				Scope:     ep.Scope,
			}}
	}
	sort.Sort(epSlice(eps))
	return eps, nil
}

// Endpoint returns the relation endpoint with the supplied name, if it exists.
func (s *ServiceDirectoryRecord) Endpoint(relationName string) (Endpoint, error) {
	eps, err := s.Endpoints()
	if err != nil {
		return Endpoint{}, err
	}
	for _, ep := range eps {
		if ep.Name == relationName {
			return ep, nil
		}
	}
	return Endpoint{}, fmt.Errorf("service directory record %q has no %q relation", s, relationName)
}

// String returns the directory record name.
func (s *ServiceDirectoryRecord) String() string {
	return fmt.Sprintf("%s-%s", s.doc.SourceEnvUUID, s.doc.ServiceName)
}

// Refresh refreshes the contents of the ServiceDirectoryRecord from the underlying
// state. It returns an error that satisfies errors.IsNotFound if the
// record has been removed.
func (s *ServiceDirectoryRecord) Refresh() error {
	serviceDirectoryCollection, closer := s.st.getCollection(serviceDirectoryC)
	defer closer()

	err := serviceDirectoryCollection.FindId(s.doc.DocID).One(&s.doc)
	if err == mgo.ErrNotFound {
		return errors.NotFoundf("service direcotry record %q", s)
	}
	if err != nil {
		return fmt.Errorf("cannot refresh service directory record %q: %v", s, err)
	}
	return nil
}

// AddServiceDirectoryParams defines the parameters used to add a ServiceDirectory record.
type AddServiceDirectoryParams struct {
	ServiceName        string
	ServiceDescription string
	Endpoints          []charm.Relation
	SourceEnvUUID      string
	SourceLabel        string
}

var errDuplicateServiceDirectoryRecord = errors.Errorf("service directory record already exists")

func serviceDirectoryKey(name, url string) string {
	return fmt.Sprintf("%s-%s", name, url)
}

// AddServiceDirectoryRecord creates a new service directory record for the specified URL,
// having the supplied parameters,
func (st *State) AddServiceDirectoryRecord(url string, params AddServiceDirectoryParams) (_ *ServiceDirectoryRecord, err error) {
	defer errors.DeferredAnnotatef(&err, "cannot add service direcotry record %q", params.ServiceName)

	// Sanity checks.
	if params.SourceEnvUUID == "" {
		return nil, errors.Errorf("missing source environment UUID")
	}
	if !names.IsValidService(params.ServiceName) {
		return nil, errors.Errorf("invalid service name")
	}
	env, err := st.Environment()
	if err != nil {
		return nil, errors.Trace(err)
	} else if env.Life() != Alive {
		return nil, errors.Errorf("environment is no longer alive")
	}

	docID := url
	doc := &serviceDirectoryDoc{
		DocID:              docID,
		URL:                url,
		ServiceName:        params.ServiceName,
		ServiceDescription: params.ServiceDescription,
		SourceEnvUUID:      params.SourceEnvUUID,
		SourceLabel:        params.SourceLabel,
	}
	eps := make([]remoteEndpointDoc, len(params.Endpoints))
	for i, ep := range params.Endpoints {
		eps[i] = remoteEndpointDoc{
			Name:      ep.Name,
			Role:      ep.Role,
			Interface: ep.Interface,
			Limit:     ep.Limit,
			Scope:     ep.Scope,
		}
	}
	doc.Endpoints = eps
	record := newServiceDirectoryRecord(st, doc)

	buildTxn := func(attempt int) ([]txn.Op, error) {
		// If we've tried once already and failed, check that
		// environment may have been destroyed.
		if attempt > 0 {
			if err := checkEnvLife(st); err != nil {
				return nil, errors.Trace(err)
			}
			_, err := st.ServiceDirectoryRecord(url)
			if err == nil {
				return nil, errDuplicateServiceDirectoryRecord
			}
		}
		ops := []txn.Op{
			env.assertAliveOp(),
			{
				C:      serviceDirectoryC,
				Id:     docID,
				Assert: txn.DocMissing,
				Insert: doc,
			},
		}
		return ops, nil
	}
	if err = st.run(buildTxn); err != nil {
		return nil, errors.Trace(err)
	}
	return record, nil
}

// ServiceDirectoryRecord returns a service directory record by name.
func (st *State) ServiceDirectoryRecord(url string) (record *ServiceDirectoryRecord, err error) {
	serviceDirectoryCollection, closer := st.getCollection(serviceDirectoryC)
	defer closer()

	doc := &serviceDirectoryDoc{}
	err = serviceDirectoryCollection.FindId(url).One(doc)
	if err == mgo.ErrNotFound {
		return nil, errors.NotFoundf("service directory record %q", url)
	}
	if err != nil {
		return nil, errors.Annotatef(err, "cannot get service directory record %q", url)
	}
	return newServiceDirectoryRecord(st, doc), nil
}

// AllServiceDirectoryEntries returns all the service directory entries used by the environment.
func (st *State) AllServiceDirectoryEntries() (records []*ServiceDirectoryRecord, err error) {
	serviceDirectoryCollection, closer := st.getCollection(serviceDirectoryC)
	defer closer()

	docs := []serviceDirectoryDoc{}
	err = serviceDirectoryCollection.Find(bson.D{}).All(&docs)
	if err != nil {
		return nil, errors.Errorf("cannot get all service directory entries")
	}
	for _, v := range docs {
		records = append(records, newServiceDirectoryRecord(st, &v))
	}
	sort.Sort(srSlice(records))
	return records, nil
}

type srSlice []*ServiceDirectoryRecord

func (sr srSlice) Len() int      { return len(sr) }
func (sr srSlice) Swap(i, j int) { sr[i], sr[j] = sr[j], sr[i] }
func (sr srSlice) Less(i, j int) bool {
	sr1 := sr[i]
	sr2 := sr[j]
	if sr1.doc.URL != sr2.doc.URL {
		return sr1.doc.ServiceName < sr2.doc.ServiceName
	}
	return sr1.doc.URL < sr2.doc.URL
}
