// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state

import (
	"fmt"
	"regexp"
	"sort"

	"github.com/juju/errors"
	"gopkg.in/juju/names.v2"
	"gopkg.in/mgo.v2/bson"
	"gopkg.in/mgo.v2/txn"

	"github.com/juju/juju/core/crossmodel"
)

// applicationOfferDoc represents the internal state of a application offer in MongoDB.
type offeredApplicationDoc struct {
	DocID string `bson:"_id"`

	// URL is the URL used to locate the offer in a directory.
	URL string `bson:"url"`

	// ApplicationName is the name of the application.
	ApplicationName string `bson:"application-name"`

	// CharmName is the name of the charm usd to deploy the service.
	CharmName string `bson:"charm-name"`

	// Description is the description of the service.
	Description string `bson:"description"`

	// Endpoints is the collection of endpoint names offered (internal->published).
	// The map allows for advertised endpoint names to be aliased.
	Endpoints map[string]string `bson:"endpoints"`

	// Registered is set to true when the application in this offer is
	// to be recorded in the application directory indicated by the URL.
	Registered bool `bson:"registered"`
}

type offeredApplications struct {
	st *State
}

// NewOfferedApplications creates a offered applications manager backed by a state instance.
func NewOfferedApplications(st *State) crossmodel.OfferedApplications {
	return &offeredApplications{st: st}
}

// Remove deletes the application offer at url immediately.
func (s *offeredApplications) RemoveOffer(url string) (err error) {
	defer errors.DeferredAnnotatef(&err, "cannot delete application offer at %q", url)
	err = s.st.runTransaction(s.removeOps(url))
	if err == txn.ErrAborted {
		// Already deleted.
		return nil
	}
	return err
}

// removeOps returns the operations required to remove the record at url.
func (s *offeredApplications) removeOps(url string) []txn.Op {
	return []txn.Op{
		{
			C:      applicationOffersC,
			Id:     url,
			Assert: txn.DocExists,
			Remove: true,
		},
	}
}

func (s *offeredApplications) validateOffer(offer crossmodel.OfferedApplication) (err error) {
	// Sanity checks.
	if !names.IsValidApplication(offer.ApplicationName) {
		return errors.NotValidf("application name %q", offer.ApplicationName)
	}
	if _, err := crossmodel.ParseApplicationURL(offer.ApplicationURL); err != nil {
		return errors.NotValidf("application URL %q", offer.ApplicationURL)
	}
	return nil
}

// AddOffer adds a new application offering to state.
func (s *offeredApplications) AddOffer(offer crossmodel.OfferedApplication) (err error) {
	defer errors.DeferredAnnotatef(&err, "cannot add application offer %q at %q", offer.ApplicationName, offer.ApplicationURL)

	if err := s.validateOffer(offer); err != nil {
		return err
	}

	doc := s.makeOfferedApplicationDoc(offer)
	ops := []txn.Op{
		assertModelActiveOp(s.st.ModelUUID()),
		{
			C:      applicationOffersC,
			Id:     doc.DocID,
			Assert: txn.DocMissing,
			Insert: doc,
		},
	}
	err = s.st.runTransaction(ops)
	if err == txn.ErrAborted {
		if err := checkModelActive(s.st); err != nil {
			return err
		}
		return errDuplicateApplicationOffer
	}
	return errors.Trace(err)
}

// UpdateOffer updates an existing application offering to state.
func (s *offeredApplications) UpdateOffer(url string, endpoints map[string]string) (err error) {
	defer errors.DeferredAnnotatef(&err, "cannot update application offer")

	if _, err := crossmodel.ParseApplicationURL(url); err != nil {
		return errors.NotValidf("application URL %q", url)
	}
	if len(endpoints) == 0 {
		return errors.New("no endpoints specified")
	}
	ops := []txn.Op{
		assertModelActiveOp(s.st.ModelUUID()),
		{
			C:      applicationOffersC,
			Id:     url,
			Assert: txn.DocExists,
			Update: bson.D{
				{"$set", bson.D{{"endpoints", endpoints}}},
			},
		},
	}
	err = s.st.runTransaction(ops)
	if err == txn.ErrAborted {
		if err := checkModelActive(s.st); err != nil {
			return err
		}
		return errors.NotFoundf("application offer at %q", url)
	}
	return errors.Trace(err)
}

func (s *offeredApplications) makeOfferedApplicationDoc(offer crossmodel.OfferedApplication) offeredApplicationDoc {
	doc := offeredApplicationDoc{
		DocID:           offer.ApplicationURL,
		URL:             offer.ApplicationURL,
		ApplicationName: offer.ApplicationName,
		CharmName:       offer.CharmName,
		Description:     offer.Description,
		Registered:      true,
	}
	eps := make(map[string]string, len(offer.Endpoints))
	for internal, published := range offer.Endpoints {
		eps[internal] = published
	}
	doc.Endpoints = eps
	return doc
}

func (s *offeredApplications) makeFilterTerm(filterTerm crossmodel.OfferedApplicationFilter) bson.D {
	var filter bson.D
	if filterTerm.ApplicationName != "" {
		filter = append(filter, bson.DocElem{"application-name", filterTerm.ApplicationName})
	}
	if filterTerm.CharmName != "" {
		filter = append(filter, bson.DocElem{"charm-name", filterTerm.CharmName})
	}
	if filterTerm.Registered != nil {
		filter = append(filter, bson.DocElem{"registered", *filterTerm.Registered})
	}
	// We match on partial URLs eg /u/user
	if filterTerm.ApplicationURL != "" {
		url := regexp.QuoteMeta(filterTerm.ApplicationURL)
		filter = append(filter, bson.DocElem{"url", bson.D{{"$regex", fmt.Sprintf(".*%s.*", url)}}})
	}
	return filter
}

// ListOffers returns the application offers matching any one of the filter terms.
func (s *offeredApplications) ListOffers(filter ...crossmodel.OfferedApplicationFilter) ([]crossmodel.OfferedApplication, error) {
	applicationOffersCollection, closer := s.st.getCollection(applicationOffersC)
	defer closer()

	var mgoTerms []bson.D
	for _, term := range filter {
		elems := s.makeFilterTerm(term)
		if len(elems) == 0 {
			continue
		}
		mgoTerms = append(mgoTerms, bson.D{{"$and", []bson.D{elems}}})
	}
	var docs []offeredApplicationDoc
	var mgoQuery bson.D
	if len(mgoTerms) > 0 {
		mgoQuery = bson.D{{"$or", mgoTerms}}
	}
	err := applicationOffersCollection.Find(mgoQuery).All(&docs)
	if err != nil {
		return nil, errors.Annotate(err, "cannot find application offers")
	}
	sort.Sort(soSlice(docs))
	offers := make([]crossmodel.OfferedApplication, len(docs))
	for i, doc := range docs {
		offers[i] = s.makeApplicationOffer(doc)
	}
	return offers, nil
}

func (s *offeredApplications) makeApplicationOffer(doc offeredApplicationDoc) crossmodel.OfferedApplication {
	offer := crossmodel.OfferedApplication{
		ApplicationURL:  doc.URL,
		ApplicationName: doc.ApplicationName,
		CharmName:       doc.CharmName,
		Description:     doc.Description,
		Registered:      doc.Registered,
	}
	offer.Endpoints = make(map[string]string, len(doc.Endpoints))
	for internal, published := range doc.Endpoints {
		offer.Endpoints[internal] = published
	}
	return offer
}

// SetOfferRegistered marks a previously saved offer as registered or not.
func (s *offeredApplications) SetOfferRegistered(url string, registered bool) (err error) {
	defer errors.DeferredAnnotatef(&err, "cannot register application offer")

	model, err := s.st.Model()
	if err != nil {
		return errors.Trace(err)
	} else if model.Life() != Alive {
		return errors.Errorf("model is no longer alive")
	}

	buildTxn := func(attempt int) ([]txn.Op, error) {
		// If we've tried once already and failed, check that
		// model may have been destroyed.
		if attempt > 0 {
			if err := checkModelActive(s.st); err != nil {
				return nil, errors.Trace(err)
			}
			return nil, errors.NotFoundf("application offer at %q", url)
		}
		ops := []txn.Op{
			model.assertActiveOp(),
			{
				C:      applicationOffersC,
				Id:     url,
				Assert: txn.DocExists,
				Update: bson.M{"$set": bson.M{"registered": registered}},
			},
		}
		return ops, nil
	}
	err = s.st.run(buildTxn)
	return errors.Trace(err)
}

type soSlice []offeredApplicationDoc

func (so soSlice) Len() int      { return len(so) }
func (so soSlice) Swap(i, j int) { so[i], so[j] = so[j], so[i] }
func (so soSlice) Less(i, j int) bool {
	so1 := so[i]
	so2 := so[j]
	if so1.URL != so2.URL {
		return so1.ApplicationName < so2.ApplicationName
	}
	return so1.URL < so2.URL
}
