// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state

import (
	"fmt"
	"regexp"
	"sort"

	"github.com/juju/errors"
	"github.com/juju/names"
	jujutxn "github.com/juju/txn"
	"gopkg.in/juju/charm.v6-unstable"
	"gopkg.in/mgo.v2"
	"gopkg.in/mgo.v2/bson"
	"gopkg.in/mgo.v2/txn"

	"github.com/juju/juju/model/crossmodel"
)

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

var _ crossmodel.ServiceDirectory = (*serviceDirectory)(nil)

type serviceDirectory struct {
	st *State
}

// NewServiceDirectory creates a service directory backed by a state instance.
func NewServiceDirectory(st *State) crossmodel.ServiceDirectory {
	return &serviceDirectory{st: st}
}

// ServiceOfferEndpoint returns from the specified offer, the relation endpoint
// with the supplied name, if it exists.
func ServiceOfferEndpoint(offer crossmodel.ServiceOffer, relationName string) (Endpoint, error) {
	for _, ep := range offer.Endpoints {
		if ep.Name == relationName {
			return Endpoint{
				ServiceName: offer.ServiceName,
				Relation:    ep,
			}, nil
		}
	}
	return Endpoint{}, errors.Errorf("service offer %q has no %q relation", offer.String(), relationName)
}

func (s *serviceDirectory) offerAtURL(url string) (*serviceDirectoryDoc, error) {
	serviceDirectoryCollection, closer := s.st.getCollection(serviceDirectoryC)
	defer closer()

	var doc serviceDirectoryDoc
	err := serviceDirectoryCollection.FindId(url).One(&doc)
	if err == mgo.ErrNotFound {
		return nil, errors.NotFoundf("service offer %q", url)
	}
	if err != nil {
		return nil, errors.Annotatef(err, "cannot count service offers %q", url)
	}
	return &doc, nil
}

// Delete deletes the service directory record at url immediately.
func (s *serviceDirectory) Delete(url string) (err error) {
	defer errors.DeferredAnnotatef(&err, "cannot delete service offer %q", url)
	buildTxn := func(attempt int) ([]txn.Op, error) {
		if attempt > 0 {
			_, err := s.offerAtURL(url)
			if err == mgo.ErrNotFound {
				return nil, jujutxn.ErrNoOperations
			} else if err != nil {
				return nil, err
			}
		}
		return s.destroyOps(url)
	}
	return s.st.run(buildTxn)
}

// destroyOps returns the operations required to destroy the record at url.
func (s *serviceDirectory) destroyOps(url string) ([]txn.Op, error) {
	return []txn.Op{
		{
			C:      serviceDirectoryC,
			Id:     url,
			Assert: txn.DocExists,
			Remove: true,
		},
	}, nil
}

var errDuplicateServiceDirectoryRecord = errors.Errorf("service offer already exists")

func (s *serviceDirectory) validateOffer(offer crossmodel.ServiceOffer) (err error) {
	// Sanity checks.
	if offer.SourceEnvUUID == "" {
		return errors.Errorf("missing source environment UUID")
	}
	if !names.IsValidService(offer.ServiceName) {
		return errors.Errorf("invalid service name")
	}
	// TODO(wallyworld) - validate service URL
	return nil
}

// AddOffer adds a new service offering to the directory.
func (s *serviceDirectory) AddOffer(offer crossmodel.ServiceOffer) (err error) {
	defer errors.DeferredAnnotatef(&err, "cannot add service offer %q", offer.ServiceName)

	if err := s.validateOffer(offer); err != nil {
		return err
	}
	env, err := s.st.Environment()
	if err != nil {
		return errors.Trace(err)
	} else if env.Life() != Alive {
		return errors.Errorf("environment is no longer alive")
	}

	doc := s.makeServiceDirectoryDoc(offer)
	buildTxn := func(attempt int) ([]txn.Op, error) {
		// If we've tried once already and failed, check that
		// environment may have been destroyed.
		if attempt > 0 {
			if err := checkEnvLife(s.st); err != nil {
				return nil, errors.Trace(err)
			}
			_, err := s.offerAtURL(offer.ServiceURL)
			if err == nil {
				return nil, errDuplicateServiceDirectoryRecord
			}
		}
		ops := []txn.Op{
			env.assertAliveOp(),
			{
				C:      serviceDirectoryC,
				Id:     doc.DocID,
				Assert: txn.DocMissing,
				Insert: doc,
			},
		}
		return ops, nil
	}
	err = s.st.run(buildTxn)
	return errors.Trace(err)
}

// UpdateOffer replaces an existing offer at the same URL.
func (s *serviceDirectory) UpdateOffer(offer crossmodel.ServiceOffer) (err error) {
	defer errors.DeferredAnnotatef(&err, "cannot update service offer %q", offer.ServiceName)

	if err := s.validateOffer(offer); err != nil {
		return err
	}
	env, err := s.st.Environment()
	if err != nil {
		return errors.Trace(err)
	} else if env.Life() != Alive {
		return errors.Errorf("environment is no longer alive")
	}

	doc := s.makeServiceDirectoryDoc(offer)
	buildTxn := func(attempt int) ([]txn.Op, error) {
		// If we've tried once already and failed, check that
		// environment may have been destroyed.
		if attempt > 0 {
			if err := checkEnvLife(s.st); err != nil {
				return nil, errors.Trace(err)
			}
			_, err := s.offerAtURL(offer.ServiceURL)
			if err != nil {
				// This will either be NotFound or some other error.
				// In either case, we return the error.
				return nil, errors.Trace(err)
			}
		}
		ops := []txn.Op{
			env.assertAliveOp(),
			{
				C:      serviceDirectoryC,
				Id:     doc.DocID,
				Assert: txn.DocExists,
				Update: doc,
			},
		}
		return ops, nil
	}
	err = s.st.run(buildTxn)
	return errors.Trace(err)
}

func (s *serviceDirectory) makeServiceDirectoryDoc(offer crossmodel.ServiceOffer) serviceDirectoryDoc {
	doc := serviceDirectoryDoc{
		DocID:              offer.ServiceURL,
		URL:                offer.ServiceURL,
		ServiceName:        offer.ServiceName,
		ServiceDescription: offer.ServiceDescription,
		SourceEnvUUID:      offer.SourceEnvUUID,
		SourceLabel:        offer.SourceLabel,
	}
	eps := make([]remoteEndpointDoc, len(offer.Endpoints))
	for i, ep := range offer.Endpoints {
		eps[i] = remoteEndpointDoc{
			Name:      ep.Name,
			Role:      ep.Role,
			Interface: ep.Interface,
			Limit:     ep.Limit,
			Scope:     ep.Scope,
		}
	}
	doc.Endpoints = eps
	return doc
}

func (s *serviceDirectory) makeFilterTerm(filterTerm crossmodel.OfferFilter) bson.D {
	var filter bson.D
	if filterTerm.ServiceName != "" {
		filter = append(filter, bson.DocElem{"servicename", filterTerm.ServiceName})
	}
	if filterTerm.SourceLabel != "" {
		filter = append(filter, bson.DocElem{"sourcelabel", filterTerm.SourceLabel})
	}
	if filterTerm.SourceEnvUUID != "" {
		filter = append(filter, bson.DocElem{"sourceuuid", filterTerm.SourceEnvUUID})
	}
	// We match on partial URLs eg /u/user
	if filterTerm.ServiceURL != "" {
		url := regexp.QuoteMeta(filterTerm.ServiceURL)
		filter = append(filter, bson.DocElem{"url", bson.D{{"$regex", fmt.Sprintf(".*%s.*", url)}}})
	}
	// We match descriptions by looking for containing terms.
	if filterTerm.ServiceDescription != "" {
		desc := regexp.QuoteMeta(filterTerm.ServiceDescription)
		filter = append(filter, bson.DocElem{"servicedescription", bson.D{{"$regex", fmt.Sprintf(".*%s.*", desc)}}})
	}
	return filter
}

// ListOffers returns the service offers matching any one of the filter terms.
func (s *serviceDirectory) ListOffers(filter ...crossmodel.OfferFilter) ([]crossmodel.ServiceOffer, error) {
	serviceDirectoryCollection, closer := s.st.getCollection(serviceDirectoryC)
	defer closer()

	var mgoTerms []bson.D
	for _, term := range filter {
		elems := s.makeFilterTerm(term)
		if len(elems) == 0 {
			continue
		}
		mgoTerms = append(mgoTerms, bson.D{{"$and", []bson.D{elems}}})
	}
	var docs []serviceDirectoryDoc
	var mgoQuery bson.D
	if len(mgoTerms) > 0 {
		mgoQuery = bson.D{{"$or", mgoTerms}}
	}
	err := serviceDirectoryCollection.Find(mgoQuery).All(&docs)
	if err != nil {
		return nil, errors.Annotate(err, "cannot find service offers")
	}
	sort.Sort(srSlice(docs))
	offers := make([]crossmodel.ServiceOffer, len(docs))
	for i, doc := range docs {
		offers[i] = s.makeServiceOffer(doc)
	}
	return offers, nil
}

func (s *serviceDirectory) makeServiceOffer(doc serviceDirectoryDoc) crossmodel.ServiceOffer {
	offer := crossmodel.ServiceOffer{
		ServiceURL:         doc.URL,
		ServiceName:        doc.ServiceName,
		ServiceDescription: doc.ServiceDescription,
		SourceEnvUUID:      doc.SourceEnvUUID,
		SourceLabel:        doc.SourceLabel,
	}
	offer.Endpoints = make([]charm.Relation, len(doc.Endpoints))
	for i, ep := range doc.Endpoints {
		offer.Endpoints[i] = charm.Relation{
			Name:      ep.Name,
			Role:      ep.Role,
			Interface: ep.Interface,
			Limit:     ep.Limit,
			Scope:     ep.Scope,
		}
	}
	return offer
}

type srSlice []serviceDirectoryDoc

func (sr srSlice) Len() int      { return len(sr) }
func (sr srSlice) Swap(i, j int) { sr[i], sr[j] = sr[j], sr[i] }
func (sr srSlice) Less(i, j int) bool {
	sr1 := sr[i]
	sr2 := sr[j]
	if sr1.URL != sr2.URL {
		return sr1.ServiceName < sr2.ServiceName
	}
	return sr1.URL < sr2.URL
}
