// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state

import (
	"fmt"
	"sort"

	"github.com/juju/errors"
	"github.com/juju/names"
	jujutxn "github.com/juju/txn"
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
	DocID         string     `bson:"_id"`
	SourceEnvUUID string     `bson:"sourceuuid"`
	SourceLabel   string     `bson:"sourcelabel"`
	ServiceName   string     `bson:"servicename"`
	Endpoints     []Endpoint `bson:"endpoints"`
}

func newServiceDirectoryRecord(st *State, doc *serviceDirectoryDoc) *ServiceDirectoryRecord {
	record := &ServiceDirectoryRecord{
		st:  st,
		doc: *doc,
	}
	return record
}

// ServiceName returns the service name.
func (s *ServiceDirectoryRecord) ServiceName() string {
	return s.doc.ServiceName
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
		eps[i] = ep
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
	ServiceName   string
	Endpoints     []Endpoint
	SourceEnvUUID string
	SourceLabel   string
}

var errDuplicateServiceDirectoryRecord = errors.Errorf("service directory record already exists")

// AddServiceDirectoryRecord creates a new service directory record, having the supplied parameters,
func (st *State) AddServiceDirectoryRecord(params AddServiceDirectoryParams) (_ *ServiceDirectoryRecord, err error) {
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

	if _, err := st.ServiceDirectoryRecord(params.ServiceName); err == nil {
		return nil, errDuplicateServiceDirectoryRecord
	} else if !errors.IsNotFound(err) {
		return nil, errors.Trace(err)
	}

	docID := st.docID(params.ServiceName)
	doc := &serviceDirectoryDoc{
		DocID:         docID,
		ServiceName:   params.ServiceName,
		Endpoints:     params.Endpoints,
		SourceEnvUUID: params.SourceEnvUUID,
		SourceLabel:   params.SourceLabel,
	}
	record := newServiceDirectoryRecord(st, doc)

	buildTxn := func(attempt int) ([]txn.Op, error) {
		// If we've tried once already and failed, check that
		// environment may have been destroyed.
		if attempt > 0 {
			if err := checkEnvLife(st); err != nil {
				return nil, errors.Trace(err)
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
	if err = st.run(buildTxn); err == nil {
		return record, nil
	}
	if err != jujutxn.ErrExcessiveContention {
		return nil, err
	}
	// Check the reason for failure - may be because of a name conflict.
	if _, err = st.ServiceDirectoryRecord(params.ServiceName); err == nil {
		return nil, errDuplicateServiceDirectoryRecord
	} else if !errors.IsNotFound(err) {
		return nil, errors.Trace(err)
	}
	return nil, errors.Trace(err)
}

// ServiceDirectoryRecord returns a service directory record by name.
func (st *State) ServiceDirectoryRecord(serviceName string) (record *ServiceDirectoryRecord, err error) {
	serviceDirectoryCollection, closer := st.getCollection(serviceDirectoryC)
	defer closer()

	if !names.IsValidService(serviceName) {
		return nil, errors.Errorf("%q is not a valid service name", serviceName)
	}
	doc := &serviceDirectoryDoc{}
	err = serviceDirectoryCollection.FindId(serviceName).One(doc)
	if err == mgo.ErrNotFound {
		return nil, errors.NotFoundf("service directory record %q", serviceName)
	}
	if err != nil {
		return nil, errors.Annotatef(err, "cannot get service directory record %q", serviceName)
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
	return records, nil
}
