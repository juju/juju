// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state

import (
	"fmt"
	"regexp"
	"sort"

	"github.com/juju/errors"
	"github.com/juju/names"
	"gopkg.in/mgo.v2/bson"
	"gopkg.in/mgo.v2/txn"

	"github.com/juju/juju/model/crossmodel"
)

// serviceOfferDoc represents the internal state of a service offer in MongoDB.
type offeredServiceDoc struct {
	DocID string `bson:"_id"`

	// URL is the URL used to locate the offer in a directory.
	URL string `bson:"url"`

	// ServiceName is the name of the service.
	ServiceName string `bson:"servicename"`

	// Endpoints is the name of the endpoints offered.
	Endpoints []string `bson:"endpoints"`

	// IsRegistered is set to true when the service in this offer has
	// been recorded in the service directory indicated by the URL.
	IsRegistered bool `bson:"isregistered"`
}

var _ crossmodel.OfferedServices = (*offeredServices)(nil)

type offeredServices struct {
	st *State
}

// NewOfferedServices creates a offered services manager backed by a state instance.
func NewOfferedServices(st *State) crossmodel.OfferedServices {
	return &offeredServices{st: st}
}

func (s *offeredServices) docId(name, url string) string {
	return fmt.Sprintf("%s-%s", name, url)
}

// Remove deletes the service offer at url immediately.
func (s *offeredServices) RemoveOffer(name, url string) (err error) {
	defer errors.DeferredAnnotatef(&err, "cannot delete service offer record %q", url)
	err = s.st.runTransaction(s.removeOps(name, url))
	if err == txn.ErrAborted {
		// Already deleted.
		return nil
	}
	return err
}

// removeOps returns the operations required to remove the record at url.
func (s *offeredServices) removeOps(name, url string) []txn.Op {
	return []txn.Op{
		{
			C:      serviceOffersC,
			Id:     s.docId(name, url),
			Assert: txn.DocExists,
			Remove: true,
		},
	}
}

func (s *offeredServices) validateOffer(offer crossmodel.OfferedService) (err error) {
	// Sanity checks.
	if !names.IsValidService(offer.ServiceName) {
		return errors.NotValidf("service name %q", offer.ServiceName)
	}
	if _, err := crossmodel.ParseServiceURL(offer.ServiceURL); err != nil {
		return errors.NotValidf("service URL %q", offer.ServiceURL)
	}
	return nil
}

// AddOffer adds a new service offering to state.
func (s *offeredServices) AddOffer(offer crossmodel.OfferedService) (err error) {
	defer errors.DeferredAnnotatef(&err, "cannot add service offer %q at %q", offer.ServiceName, offer.ServiceURL)

	if err := s.validateOffer(offer); err != nil {
		return err
	}
	env, err := s.st.Environment()
	if err != nil {
		return errors.Trace(err)
	} else if env.Life() != Alive {
		return errors.Errorf("environment is no longer alive")
	}

	doc := s.makeOfferedServiceDoc(offer)
	buildTxn := func(attempt int) ([]txn.Op, error) {
		// If we've tried once already and failed, check that
		// environment may have been destroyed.
		if attempt > 0 {
			if err := checkEnvLife(s.st); err != nil {
				return nil, errors.Trace(err)
			}
			return nil, errDuplicateServiceOffer
		}
		ops := []txn.Op{
			env.assertAliveOp(),
			{
				C:      serviceOffersC,
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

func (s *offeredServices) makeOfferedServiceDoc(offer crossmodel.OfferedService) offeredServiceDoc {
	doc := offeredServiceDoc{
		DocID:       s.docId(offer.ServiceName, offer.ServiceURL),
		URL:         offer.ServiceURL,
		ServiceName: offer.ServiceName,
	}
	eps := make([]string, len(offer.Endpoints))
	for i, ep := range offer.Endpoints {
		eps[i] = ep
	}
	doc.Endpoints = eps
	return doc
}

func (s *offeredServices) makeFilterTerm(filterTerm crossmodel.OfferedServiceFilter) bson.D {
	var filter bson.D
	if filterTerm.ServiceName != "" {
		filter = append(filter, bson.DocElem{"servicename", filterTerm.ServiceName})
	}
	// We match on partial URLs eg /u/user
	if filterTerm.ServiceURL != "" {
		url := regexp.QuoteMeta(filterTerm.ServiceURL)
		filter = append(filter, bson.DocElem{"url", bson.D{{"$regex", fmt.Sprintf(".*%s.*", url)}}})
	}
	return filter
}

// ListOffers returns the service offers matching any one of the filter terms.
func (s *offeredServices) ListOffers(filter ...crossmodel.OfferedServiceFilter) ([]crossmodel.OfferedService, error) {
	serviceOffersCollection, closer := s.st.getCollection(serviceOffersC)
	defer closer()

	var mgoTerms []bson.D
	for _, term := range filter {
		elems := s.makeFilterTerm(term)
		if len(elems) == 0 {
			continue
		}
		mgoTerms = append(mgoTerms, bson.D{{"$and", []bson.D{elems}}})
	}
	var docs []offeredServiceDoc
	var mgoQuery bson.D
	if len(mgoTerms) > 0 {
		mgoQuery = bson.D{{"$or", mgoTerms}}
	}
	err := serviceOffersCollection.Find(mgoQuery).All(&docs)
	if err != nil {
		return nil, errors.Annotate(err, "cannot find service offers")
	}
	sort.Sort(soSlice(docs))
	offers := make([]crossmodel.OfferedService, len(docs))
	for i, doc := range docs {
		offers[i] = s.makeServiceOffer(doc)
	}
	return offers, nil
}

func (s *offeredServices) makeServiceOffer(doc offeredServiceDoc) crossmodel.OfferedService {
	offer := crossmodel.OfferedService{
		ServiceURL:  doc.URL,
		ServiceName: doc.ServiceName,
	}
	offer.Endpoints = make([]string, len(doc.Endpoints))
	for i, ep := range doc.Endpoints {
		offer.Endpoints[i] = ep
	}
	return offer
}

// RegisterOffer marks a previously saved offer as registered.
func (s *offeredServices) RegisterOffer(name, url string) (err error) {
	defer errors.DeferredAnnotatef(&err, "cannot register service offer")

	env, err := s.st.Environment()
	if err != nil {
		return errors.Trace(err)
	} else if env.Life() != Alive {
		return errors.Errorf("environment is no longer alive")
	}

	buildTxn := func(attempt int) ([]txn.Op, error) {
		// If we've tried once already and failed, check that
		// environment may have been destroyed.
		if attempt > 0 {
			if err := checkEnvLife(s.st); err != nil {
				return nil, errors.Trace(err)
			}
			return nil, errors.NotFoundf("service offer %q at url %q", name, url)
		}
		ops := []txn.Op{
			env.assertAliveOp(),
			{
				C:      serviceOffersC,
				Id:     s.docId(name, url),
				Assert: txn.DocExists,
				Update: bson.M{"$set": bson.M{"isregistered": true}},
			},
		}
		return ops, nil
	}
	err = s.st.run(buildTxn)
	return errors.Trace(err)
}

// UnregisteredOffers returns the service offers not yet registered with a service directory.
func (s *offeredServices) UnregisteredOffers() ([]crossmodel.OfferedService, error) {
	serviceOffersCollection, closer := s.st.getCollection(serviceOffersC)
	defer closer()

	var docs []offeredServiceDoc
	err := serviceOffersCollection.Find(bson.D{{"isregistered", false}}).All(&docs)
	if err != nil {
		return nil, errors.Annotate(err, "cannot find unregistered service offers")
	}
	sort.Sort(soSlice(docs))
	offers := make([]crossmodel.OfferedService, len(docs))
	for i, doc := range docs {
		offers[i] = s.makeServiceOffer(doc)
	}
	return offers, nil
}

type soSlice []offeredServiceDoc

func (so soSlice) Len() int      { return len(so) }
func (so soSlice) Swap(i, j int) { so[i], so[j] = so[j], so[i] }
func (so soSlice) Less(i, j int) bool {
	so1 := so[i]
	so2 := so[j]
	if so1.URL != so2.URL {
		return so1.ServiceName < so2.ServiceName
	}
	return so1.URL < so2.URL
}
