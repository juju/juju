// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state

import (
	"fmt"
	"regexp"
	"sort"

	"github.com/juju/errors"
	"gopkg.in/juju/charm.v6-unstable"
	"gopkg.in/juju/names.v2"
	"gopkg.in/mgo.v2"
	"gopkg.in/mgo.v2/bson"
	"gopkg.in/mgo.v2/txn"

	"github.com/juju/juju/core/crossmodel"
)

// applicationOfferDoc represents the internal state of a application offer in MongoDB.
type applicationOfferDoc struct {
	DocID string `bson:"_id"`

	// URL is the URL used to locate the offer in a directory.
	URL string `bson:"url"`

	// SourceModelUUID is the UUID of the environment hosting the application.
	SourceModelUUID string `bson:"source-model-uuid"`

	// SourceLabel is a user friendly name for the source environment.
	SourceLabel string `bson:"source-label"`

	// ApplicationName is the name of the application.
	ApplicationName string `bson:"application-name"`

	// ApplicationDescription is a description of the application's functionality,
	// typically copied from the charm metadata.
	ApplicationDescription string `bson:"application-description"`

	// Endpoints are the charm endpoints supported by the applicationbob.
	Endpoints []remoteEndpointDoc `bson:"endpoints"`
}

var _ crossmodel.ApplicationDirectory = (*applicationDirectory)(nil)

type applicationDirectory struct {
	st *State
}

// NewApplicationDirectory creates a application directory backed by a state instance.
func NewApplicationDirectory(st *State) crossmodel.ApplicationDirectory {
	return &applicationDirectory{st: st}
}

// ApplicationOfferEndpoint returns from the specified offer, the relation endpoint
// with the supplied name, if it exists.
func ApplicationOfferEndpoint(offer crossmodel.ApplicationOffer, relationName string) (Endpoint, error) {
	for _, ep := range offer.Endpoints {
		if ep.Name == relationName {
			return Endpoint{
				ApplicationName: offer.ApplicationName,
				Relation:        ep,
			}, nil
		}
	}
	return Endpoint{}, errors.NotFoundf("relation %q on application offer %q", relationName, offer.String())
}

func (s *applicationDirectory) offerAtURL(url string) (*applicationOfferDoc, error) {
	applicationOffersCollection, closer := s.st.getCollection(localApplicationDirectoryC)
	defer closer()

	var doc applicationOfferDoc
	err := applicationOffersCollection.FindId(url).One(&doc)
	if err == mgo.ErrNotFound {
		return nil, errors.NotFoundf("application offer %q", url)
	}
	if err != nil {
		return nil, errors.Annotatef(err, "cannot count application offers %q", url)
	}
	return &doc, nil
}

// Remove deletes the application offer at url immediately.
func (s *applicationDirectory) Remove(url string) (err error) {
	defer errors.DeferredAnnotatef(&err, "cannot delete application offer %q", url)
	err = s.st.runTransaction(s.removeOps(url))
	if err == txn.ErrAborted {
		// Already deleted.
		return nil
	}
	return err
}

// removeOps returns the operations required to remove the record at url.
func (s *applicationDirectory) removeOps(url string) []txn.Op {
	return []txn.Op{
		{
			C:      localApplicationDirectoryC,
			Id:     url,
			Assert: txn.DocExists,
			Remove: true,
		},
	}
}

var errDuplicateApplicationOffer = errors.Errorf("application offer already exists")

func (s *applicationDirectory) validateOffer(offer crossmodel.ApplicationOffer) (err error) {
	// Sanity checks.
	if offer.SourceModelUUID == "" {
		return errors.Errorf("missing source model UUID")
	}
	if !names.IsValidApplication(offer.ApplicationName) {
		return errors.Errorf("invalid application name")
	}
	// TODO(wallyworld) - validate application URL
	return nil
}

// AddOffer adds a new application offering to the directory.
func (s *applicationDirectory) AddOffer(offer crossmodel.ApplicationOffer) (err error) {
	defer errors.DeferredAnnotatef(&err, "cannot add application offer %q at %q", offer.ApplicationName, offer.ApplicationURL)

	if err := s.validateOffer(offer); err != nil {
		return err
	}
	model, err := s.st.Model()
	if err != nil {
		return errors.Trace(err)
	} else if model.Life() != Alive {
		return errors.Errorf("model is no longer alive")
	}

	doc := s.makeApplicationOfferDoc(offer)
	buildTxn := func(attempt int) ([]txn.Op, error) {
		// If we've tried once already and failed, check that
		// environment may have been destroyed.
		if attempt > 0 {
			if err := checkModelActive(s.st); err != nil {
				return nil, errors.Trace(err)
			}
			_, err := s.offerAtURL(offer.ApplicationURL)
			if err == nil {
				return nil, errDuplicateApplicationOffer
			}
		}
		ops := []txn.Op{
			model.assertActiveOp(),
			{
				C:      localApplicationDirectoryC,
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
func (s *applicationDirectory) UpdateOffer(offer crossmodel.ApplicationOffer) (err error) {
	defer errors.DeferredAnnotatef(&err, "cannot update application offer %q", offer.ApplicationName)

	if err := s.validateOffer(offer); err != nil {
		return err
	}
	model, err := s.st.Model()
	if err != nil {
		return errors.Trace(err)
	} else if model.Life() != Alive {
		return errors.Errorf("model is no longer alive")
	}

	doc := s.makeApplicationOfferDoc(offer)
	buildTxn := func(attempt int) ([]txn.Op, error) {
		// If we've tried once already and failed, check that
		// environment may have been destroyed.
		if attempt > 0 {
			if err := checkModelActive(s.st); err != nil {
				return nil, errors.Trace(err)
			}
			_, err := s.offerAtURL(offer.ApplicationURL)
			if err != nil {
				// This will either be NotFound or some other error.
				// In either case, we return the error.
				return nil, errors.Trace(err)
			}
		}
		ops := []txn.Op{
			model.assertActiveOp(),
			{
				C:      localApplicationDirectoryC,
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

func (s *applicationDirectory) makeApplicationOfferDoc(offer crossmodel.ApplicationOffer) applicationOfferDoc {
	doc := applicationOfferDoc{
		DocID:                  offer.ApplicationURL,
		URL:                    offer.ApplicationURL,
		ApplicationName:        offer.ApplicationName,
		ApplicationDescription: offer.ApplicationDescription,
		SourceModelUUID:        offer.SourceModelUUID,
		SourceLabel:            offer.SourceLabel,
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

func (s *applicationDirectory) makeFilterTerm(filterTerm crossmodel.ApplicationOfferFilter) bson.D {
	var filter bson.D
	if filterTerm.ApplicationName != "" {
		filter = append(filter, bson.DocElem{"application-name", filterTerm.ApplicationName})
	}
	if filterTerm.SourceLabel != "" {
		filter = append(filter, bson.DocElem{"source-label", filterTerm.SourceLabel})
	}
	if filterTerm.SourceModelUUID != "" {
		filter = append(filter, bson.DocElem{"source-model-uuid", filterTerm.SourceModelUUID})
	}
	// We match on partial URLs eg /u/user
	if filterTerm.ApplicationURL != "" {
		url := regexp.QuoteMeta(filterTerm.ApplicationURL)
		filter = append(filter, bson.DocElem{"url", bson.D{{"$regex", fmt.Sprintf(".*%s.*", url)}}})
	}
	// We match descriptions by looking for containing terms.
	if filterTerm.ApplicationDescription != "" {
		desc := regexp.QuoteMeta(filterTerm.ApplicationDescription)
		filter = append(filter, bson.DocElem{"application-description", bson.D{{"$regex", fmt.Sprintf(".*%s.*", desc)}}})
	}
	return filter
}

// ListOffers returns the application offers matching any one of the filter terms.
func (s *applicationDirectory) ListOffers(filter ...crossmodel.ApplicationOfferFilter) ([]crossmodel.ApplicationOffer, error) {
	applicationOffersCollection, closer := s.st.getCollection(localApplicationDirectoryC)
	defer closer()

	// TODO(wallyworld) - add support for filtering on endpoints
	var mgoTerms []bson.D
	for _, term := range filter {
		elems := s.makeFilterTerm(term)
		if len(elems) == 0 {
			continue
		}
		mgoTerms = append(mgoTerms, bson.D{{"$and", []bson.D{elems}}})
	}
	var docs []applicationOfferDoc
	var mgoQuery bson.D
	if len(mgoTerms) > 0 {
		mgoQuery = bson.D{{"$or", mgoTerms}}
	}
	err := applicationOffersCollection.Find(mgoQuery).All(&docs)
	if err != nil {
		return nil, errors.Annotate(err, "cannot find application offers")
	}
	sort.Sort(srSlice(docs))
	offers := make([]crossmodel.ApplicationOffer, len(docs))
	for i, doc := range docs {
		offers[i] = s.makeApplicationOffer(doc)
	}
	return offers, nil
}

func (s *applicationDirectory) makeApplicationOffer(doc applicationOfferDoc) crossmodel.ApplicationOffer {
	offer := crossmodel.ApplicationOffer{
		ApplicationURL:         doc.URL,
		ApplicationName:        doc.ApplicationName,
		ApplicationDescription: doc.ApplicationDescription,
		SourceModelUUID:        doc.SourceModelUUID,
		SourceLabel:            doc.SourceLabel,
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

type srSlice []applicationOfferDoc

func (sr srSlice) Len() int      { return len(sr) }
func (sr srSlice) Swap(i, j int) { sr[i], sr[j] = sr[j], sr[i] }
func (sr srSlice) Less(i, j int) bool {
	sr1 := sr[i]
	sr2 := sr[j]
	if sr1.URL != sr2.URL {
		return sr1.ApplicationName < sr2.ApplicationName
	}
	return sr1.URL < sr2.URL
}
