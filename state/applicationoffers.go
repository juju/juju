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

	// OfferName is the name of the offer.
	OfferName string `bson:"offer-name"`

	// ApplicationName is the name of the application to which an offer pertains.
	ApplicationName string `bson:"application-name"`

	// ApplicationDescription is a description of the application's functionality,
	// typically copied from the charm metadata.
	ApplicationDescription string `bson:"application-description"`

	// Endpoints are the charm endpoints supported by the applicationbob.
	Endpoints map[string]string `bson:"endpoints"`
}

var _ crossmodel.ApplicationOffers = (*applicationOffers)(nil)

type applicationOffers struct {
	st *State
}

// NewApplicationOffers creates a application directory backed by a state instance.
func NewApplicationOffers(st *State) crossmodel.ApplicationOffers {
	return &applicationOffers{st: st}
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

func (s *applicationOffers) offerForName(offerName string) (*applicationOfferDoc, error) {
	applicationOffersCollection, closer := s.st.getCollection(applicationOffersC)
	defer closer()

	var doc applicationOfferDoc
	err := applicationOffersCollection.FindId(offerName).One(&doc)
	if err == mgo.ErrNotFound {
		return nil, errors.NotFoundf("application offer %q", offerName)
	}
	if err != nil {
		return nil, errors.Annotatef(err, "cannot load application offer %q", offerName)
	}
	return &doc, nil
}

// ApplicationOffer returns the named application offer.
func (s *applicationOffers) ApplicationOffer(offerName string) (*crossmodel.ApplicationOffer, error) {
	offerDoc, err := s.offerForName(offerName)
	if err != nil {
		return nil, errors.Trace(err)
	}
	return s.makeApplicationOffer(*offerDoc)
}

// Remove deletes the application offer for offerName immediately.
func (s *applicationOffers) Remove(offerName string) (err error) {
	defer errors.DeferredAnnotatef(&err, "cannot delete application offer %q", offerName)
	err = s.st.runTransaction(s.removeOps(offerName))
	if err == txn.ErrAborted {
		// Already deleted.
		return nil
	}
	return err
}

// removeOps returns the operations required to remove the record for offerName.
func (s *applicationOffers) removeOps(offerName string) []txn.Op {
	return []txn.Op{
		{
			C:      applicationOffersC,
			Id:     offerName,
			Assert: txn.DocExists,
			Remove: true,
		},
	}
}

var errDuplicateApplicationOffer = errors.Errorf("application offer already exists")

func (s *applicationOffers) validateOfferArgs(offer crossmodel.AddApplicationOfferArgs) (err error) {
	// Sanity checks.
	if !names.IsValidApplication(offer.ApplicationName) {
		return errors.NotValidf("application name %q", offer.ApplicationName)
	}
	// Same rules for valid offer names apply as for applications.
	if !names.IsValidApplication(offer.OfferName) {
		return errors.NotValidf("offer name %q", offer.OfferName)
	}
	return nil
}

// AddOffer adds a new application offering to the directory.
func (s *applicationOffers) AddOffer(offerArgs crossmodel.AddApplicationOfferArgs) (_ *crossmodel.ApplicationOffer, err error) {
	defer errors.DeferredAnnotatef(&err, "cannot add application offer %q", offerArgs.OfferName)

	if err := s.validateOfferArgs(offerArgs); err != nil {
		return nil, err
	}
	model, err := s.st.Model()
	if err != nil {
		return nil, errors.Trace(err)
	} else if model.Life() != Alive {
		return nil, errors.Errorf("model is no longer alive")
	}

	doc := s.makeApplicationOfferDoc(offerArgs)
	result, err := s.makeApplicationOffer(doc)
	if err != nil {
		return nil, errors.Trace(err)
	}
	buildTxn := func(attempt int) ([]txn.Op, error) {
		// If we've tried once already and failed, check that
		// environment may have been destroyed.
		if attempt > 0 {
			if err := checkModelActive(s.st); err != nil {
				return nil, errors.Trace(err)
			}
			_, err := s.offerForName(offerArgs.OfferName)
			if err == nil {
				return nil, errDuplicateApplicationOffer
			}
		}
		ops := []txn.Op{
			model.assertActiveOp(),
			{
				C:      applicationOffersC,
				Id:     doc.DocID,
				Assert: txn.DocMissing,
				Insert: doc,
			},
		}
		return ops, nil
	}
	err = s.st.run(buildTxn)
	if err != nil {
		return nil, errors.Trace(err)
	}
	return result, nil
}

// UpdateOffer replaces an existing offer at the same URL.
func (s *applicationOffers) UpdateOffer(offerArgs crossmodel.AddApplicationOfferArgs) (_ *crossmodel.ApplicationOffer, err error) {
	defer errors.DeferredAnnotatef(&err, "cannot update application offer %q", offerArgs.ApplicationName)

	if err := s.validateOfferArgs(offerArgs); err != nil {
		return nil, err
	}
	model, err := s.st.Model()
	if err != nil {
		return nil, errors.Trace(err)
	} else if model.Life() != Alive {
		return nil, errors.Errorf("model is no longer alive")
	}

	doc := s.makeApplicationOfferDoc(offerArgs)
	buildTxn := func(attempt int) ([]txn.Op, error) {
		// If we've tried once already and failed, check that
		// environment may have been destroyed.
		if attempt > 0 {
			if err := checkModelActive(s.st); err != nil {
				return nil, errors.Trace(err)
			}
			_, err := s.offerForName(offerArgs.OfferName)
			if err != nil {
				// This will either be NotFound or some other error.
				// In either case, we return the error.
				return nil, errors.Trace(err)
			}
		}
		ops := []txn.Op{
			model.assertActiveOp(),
			{
				C:      applicationOffersC,
				Id:     doc.DocID,
				Assert: txn.DocExists,
				Update: bson.M{"$set": doc},
			},
		}
		return ops, nil
	}
	err = s.st.run(buildTxn)
	if err != nil {
		return nil, errors.Trace(err)
	}
	return s.makeApplicationOffer(doc)
}

func (s *applicationOffers) makeApplicationOfferDoc(offer crossmodel.AddApplicationOfferArgs) applicationOfferDoc {
	doc := applicationOfferDoc{
		DocID:                  offer.OfferName,
		OfferName:              offer.OfferName,
		ApplicationName:        offer.ApplicationName,
		ApplicationDescription: offer.ApplicationDescription,
		Endpoints:              offer.Endpoints,
	}
	return doc
}

func (s *applicationOffers) makeFilterTerm(filterTerm crossmodel.ApplicationOfferFilter) bson.D {
	var filter bson.D
	if filterTerm.ApplicationName != "" {
		filter = append(filter, bson.DocElem{"application-name", filterTerm.ApplicationName})
	}
	// We match on partial names, eg "-sql"
	if filterTerm.OfferName != "" {
		name := regexp.QuoteMeta(filterTerm.OfferName)
		filter = append(filter, bson.DocElem{"offer-name", bson.D{{"$regex", fmt.Sprintf(".*%s.*", name)}}})
	}
	// We match descriptions by looking for containing terms.
	if filterTerm.ApplicationDescription != "" {
		desc := regexp.QuoteMeta(filterTerm.ApplicationDescription)
		filter = append(filter, bson.DocElem{"application-description", bson.D{{"$regex", fmt.Sprintf(".*%s.*", desc)}}})
	}
	return filter
}

// ListOffers returns the application offers matching any one of the filter terms.
func (s *applicationOffers) ListOffers(filter ...crossmodel.ApplicationOfferFilter) ([]crossmodel.ApplicationOffer, error) {
	applicationOffersCollection, closer := s.st.getCollection(applicationOffersC)
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
		offer, err := s.makeApplicationOffer(doc)
		if err != nil {
			return nil, errors.Trace(err)
		}
		offers[i] = *offer
	}
	return offers, nil
}

func (s *applicationOffers) makeApplicationOffer(doc applicationOfferDoc) (*crossmodel.ApplicationOffer, error) {
	offer := &crossmodel.ApplicationOffer{
		OfferName:              doc.OfferName,
		ApplicationName:        doc.ApplicationName,
		ApplicationDescription: doc.ApplicationDescription,
	}
	app, err := s.st.Application(doc.ApplicationName)
	if err != nil {
		return nil, errors.Trace(err)
	}
	eps, err := getApplicationEndpoints(app, doc.Endpoints)
	if err != nil {
		return nil, errors.Trace(err)
	}
	offer.Endpoints = eps
	return offer, nil
}

func getApplicationEndpoints(application *Application, endpointNames map[string]string) (map[string]charm.Relation, error) {
	result := make(map[string]charm.Relation)
	for alias, endpointName := range endpointNames {
		endpoint, err := application.Endpoint(endpointName)
		if err != nil {
			return nil, errors.Annotatef(err, "getting relation endpoint for relation %q and application %q", endpointName, application.Name())
		}
		result[alias] = endpoint.Relation
	}
	return result, nil
}

type srSlice []applicationOfferDoc

func (sr srSlice) Len() int      { return len(sr) }
func (sr srSlice) Swap(i, j int) { sr[i], sr[j] = sr[j], sr[i] }
func (sr srSlice) Less(i, j int) bool {
	sr1 := sr[i]
	sr2 := sr[j]
	if sr1.OfferName == sr2.OfferName {
		return sr1.ApplicationName < sr2.ApplicationName
	}
	return sr1.OfferName < sr2.OfferName
}
