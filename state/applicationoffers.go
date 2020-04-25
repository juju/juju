// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state

import (
	"fmt"
	"regexp"
	"sort"
	"strings"

	"github.com/juju/charm/v7"
	"github.com/juju/collections/set"
	"github.com/juju/errors"
	"github.com/juju/names/v4"
	jujutxn "github.com/juju/txn"
	"github.com/juju/utils"
	"gopkg.in/mgo.v2"
	"gopkg.in/mgo.v2/bson"
	"gopkg.in/mgo.v2/txn"

	"github.com/juju/juju/core/crossmodel"
	"github.com/juju/juju/core/permission"
)

const (
	// applicationOfferGlobalKey is the key for an application offer.
	applicationOfferGlobalKey = "ao"
)

// applicationOfferKey will return the key for a given offer using the
// offer uuid and the applicationOfferGlobalKey.
func applicationOfferKey(offerUUID string) string {
	return fmt.Sprintf("%s#%s", applicationOfferGlobalKey, offerUUID)
}

// applicationOfferDoc represents the internal state of a application offer in MongoDB.
type applicationOfferDoc struct {
	DocID string `bson:"_id"`

	// OfferUUID is the UUID of the offer.
	OfferUUID string `bson:"offer-uuid"`

	// OfferName is the name of the offer.
	OfferName string `bson:"offer-name"`

	// ApplicationName is the name of the application to which an offer pertains.
	ApplicationName string `bson:"application-name"`

	// ApplicationDescription is a description of the application's functionality,
	// typically copied from the charm metadata.
	ApplicationDescription string `bson:"application-description"`

	// Endpoints are the charm endpoints supported by the application.
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

// TODO(wallyworld) - remove when we use UUID everywhere
func applicationOfferUUID(st *State, offerName string) (string, error) {
	appOffers := &applicationOffers{st: st}
	offer, err := appOffers.ApplicationOffer(offerName)
	if err != nil {
		return "", errors.Trace(err)
	}
	return offer.OfferUUID, nil
}

func (s *applicationOffers) offerQuery(query bson.D) (*applicationOfferDoc, error) {
	applicationOffersCollection, closer := s.st.db().GetCollection(applicationOffersC)
	defer closer()

	var doc applicationOfferDoc
	err := applicationOffersCollection.Find(query).One(&doc)
	return &doc, err
}

// ApplicationOffer returns the named application offer.
func (s *applicationOffers) ApplicationOffer(offerName string) (*crossmodel.ApplicationOffer, error) {
	offerDoc, err := s.offerQuery(bson.D{{"_id", offerName}})
	if err != nil {
		if err == mgo.ErrNotFound {
			return nil, errors.NotFoundf("application offer %q", offerName)
		}
		return nil, errors.Annotatef(err, "cannot load application offer %q", offerName)
	}
	return s.makeApplicationOffer(*offerDoc)
}

// ApplicationOfferForUUID returns the application offer for the UUID.
func (s *applicationOffers) ApplicationOfferForUUID(offerUUID string) (*crossmodel.ApplicationOffer, error) {
	offerDoc, err := s.offerQuery(bson.D{{"offer-uuid", offerUUID}})
	if err != nil {
		if err == mgo.ErrNotFound {
			return nil, errors.NotFoundf("application offer %q", offerUUID)
		}
		return nil, errors.Annotatef(err, "cannot load application offer %q", offerUUID)
	}
	return s.makeApplicationOffer(*offerDoc)
}

// AllApplicationOffers returns all application offers in the model.
func (s *applicationOffers) AllApplicationOffers() (offers []*crossmodel.ApplicationOffer, _ error) {
	applicationOffersCollection, closer := s.st.db().GetCollection(applicationOffersC)
	defer closer()

	var docs []applicationOfferDoc
	err := applicationOffersCollection.Find(bson.D{}).All(&docs)
	if err != nil {
		return nil, errors.Errorf("cannot get all application offers")
	}
	for _, doc := range docs {
		offer, err := s.makeApplicationOffer(doc)
		if err != nil {
			return nil, errors.Trace(err)
		}
		offers = append(offers, offer)
	}
	return offers, nil
}

// RemoveOfferOperation returns a model operation that will allow relation to leave scope.
func (s *applicationOffers) RemoveOfferOperation(offerName string, force bool) *RemoveOfferOperation {
	return &RemoveOfferOperation{
		offers:          &applicationOffers{s.st},
		offerName:       offerName,
		ForcedOperation: ForcedOperation{Force: force},
	}
}

// RemoveOfferOperation is a model operation to remove application offer.
type RemoveOfferOperation struct {
	// ForcedOperation stores needed information to force this operation.
	ForcedOperation

	// offers holds the application offers to remove.
	offers *applicationOffers

	// offerName is the offer name to remove.
	offerName string
}

// Build is part of the ModelOperation interface.
func (op *RemoveOfferOperation) Build(attempt int) ([]txn.Op, error) {
	offer, err := op.offers.ApplicationOffer(op.offerName)
	if errors.IsNotFound(err) {
		return nil, jujutxn.ErrNoOperations
	}
	if err != nil {
		return nil, errors.Trace(err)
	}
	// When 'force' is set on the operation, this call will return needed operations
	// and accumulate all operational errors encountered in the operation.
	// If the 'force' is not set, any error will be fatal and no operations will be returned.
	switch ops, err := op.internalRemove(offer, attempt); err {
	case errRefresh:
	case errAlreadyDying:
		return nil, jujutxn.ErrNoOperations
	case nil:
		return ops, nil
	default:
		if op.Force {
			logger.Warningf("force removing offer %v despite error %v", op.offerName, err)
			return ops, nil
		}
		return nil, err
	}
	return nil, jujutxn.ErrNoOperations
}

// Done is part of the ModelOperation interface.
func (op *RemoveOfferOperation) Done(err error) error {
	if err != nil {
		if !op.Force {
			return errors.Annotatef(err, "cannot delete application offer %q", op.offerName)
		}
		op.AddError(errors.Errorf("forced offer %v removal but proceeded despite encountering ERROR %v", op.offerName, err))
	}
	return nil
}

// Remove deletes the application offer for offerName immediately.
func (s *applicationOffers) Remove(offerName string, force bool) error {
	op := s.RemoveOfferOperation(offerName, force)
	err := s.st.ApplyOperation(op)
	if len(op.Errors) != 0 {
		logger.Warningf("operational errors removing offer %v: %v", offerName, op.Errors)
	}
	return err
}

func (op *RemoveOfferOperation) internalRemove(offer *crossmodel.ApplicationOffer, attempt int) ([]txn.Op, error) {
	// Load the application before counting the connections
	// so we can do a consistency check on relation count.
	app, err := op.offers.st.Application(offer.ApplicationName)
	if err != nil {
		return nil, errors.Trace(err)
	}
	conns, err := op.offers.st.OfferConnections(offer.OfferUUID)
	if err != nil {
		return nil, errors.Trace(err)
	}
	if len(conns) > 0 && !op.Force {
		return nil, errors.Errorf("offer has %d relation%s", len(conns), plural(len(conns)))
	}
	// Because we don't refcount offer connections, we instead either
	// assert here that the relation count doesn't change, and that the
	// specific relations that make up that count aren't removed, or we
	// remove the relations, depending on whether force=true.
	rels, err := app.Relations()
	if err != nil {
		return nil, errors.Trace(err)
	}
	if len(rels) != app.doc.RelationCount {
		return nil, jujutxn.ErrTransientFailure
	}
	ops := []txn.Op{{
		C:      applicationsC,
		Id:     offer.ApplicationName,
		Assert: bson.D{{"relationcount", app.doc.RelationCount}},
	}}
	for _, rel := range rels {
		remoteApp, isCrossModel, err := rel.RemoteApplication()
		if err != nil {
			return nil, errors.Trace(err)
		}
		if isCrossModel && !op.Force {
			return nil, jujutxn.ErrTransientFailure
		}
		if op.Force {
			// We only force delete cross model relations (connections).
			if !isCrossModel {
				continue
			}
			if err := rel.Refresh(); errors.IsNotFound(err) {
				continue
			} else if err != nil {
				return nil, err
			}

			logger.Debugf("forcing cleanup of remote application %v", remoteApp.Name())
			remoteAppOps, err := remoteApp.DestroyOperation(op.Force).Build(attempt)
			if err != nil && err != jujutxn.ErrNoOperations {
				op.AddError(err)
			}
			ops = append(ops, remoteAppOps...)

			// Force any remote units to leave scope so the offer
			// can be cleaned up.
			logger.Debugf("forcing cleanup of units for %v", remoteApp.Name())
			remoteUnits, err := rel.AllRemoteUnits(remoteApp.Name())
			if err != nil {
				return nil, errors.Trace(err)
			}
			logger.Debugf("got %v relation units to clean", len(remoteUnits))
			for _, ru := range remoteUnits {
				leaveScopeOps, err := ru.leaveScopeForcedOps(&op.ForcedOperation)
				if err != nil && err != jujutxn.ErrNoOperations {
					op.AddError(err)
				}
				ops = append(ops, leaveScopeOps...)
			}

			// When 'force' is set, this call will return needed operations
			// and accumulate all operational errors encountered in the operation.
			// If the 'force' is not set, any error will be fatal and no operations will be returned.
			relOps, _, err := rel.destroyOps("", &op.ForcedOperation)
			if err == errAlreadyDying {
				continue
			} else if err != nil {
				return nil, errors.Trace(err)
			}
			ops = append(ops, relOps...)
		} else {
			ops = append(ops, txn.Op{
				C:      relationsC,
				Id:     rel.doc.DocID,
				Assert: txn.DocExists,
			})
		}
	}
	decRefOp, err := decApplicationOffersRefOp(op.offers.st, offer.ApplicationName)
	if err != nil {
		return nil, errors.Trace(err)
	}
	ops = append(ops, txn.Op{
		C:      applicationOffersC,
		Id:     offer.OfferName,
		Assert: txn.DocExists,
		Remove: true,
	}, decRefOp)
	return ops, nil
}

// applicationOffersDocs returns the offer docs for the given application
func applicationOffersDocs(st *State, application string) ([]applicationOfferDoc, error) {
	applicationOffersCollection, closer := st.db().GetCollection(applicationOffersC)
	defer closer()
	query := bson.D{{"application-name", application}}
	var docs []applicationOfferDoc
	if err := applicationOffersCollection.Find(query).All(&docs); err != nil {
		return nil, errors.Annotatef(err, "reading application %q offers", application)
	}
	return docs, nil
}

// applicationHasConnectedOffers returns true when any of the the application's
// offers have connections
func applicationHasConnectedOffers(st *State, application string) (bool, error) {
	offers, err := applicationOffersDocs(st, application)
	if err != nil {
		return false, errors.Trace(err)
	}
	for _, offer := range offers {
		connections, err := st.OfferConnections(offer.OfferUUID)
		if err != nil {
			return false, errors.Trace(err)
		}
		if len(connections) > 0 {
			return true, nil
		}
	}
	return false, nil
}

// removeApplicationOffersOps returns txn.Ops that will remove all offers for
// the specified application. No assertions on the application or the offer
// connections are made; the caller is responsible for ensuring that offer
// connections have already been removed, or will be removed along with the
// offers.
func removeApplicationOffersOps(st *State, application string) ([]txn.Op, error) {
	// Check how many offer refs there are. If there are none, there's
	// nothing more to do. If there are refs, we assume that the number
	// if refs is the same as what we find below. If there is a
	// discrepancy, then it'll result in a transaction failure, and
	// we'll retry.
	countRefsOp, n, err := countApplicationOffersRefOp(st, application)
	if err != nil {
		return nil, errors.Trace(err)
	}
	if n == 0 {
		return []txn.Op{countRefsOp}, nil
	}
	docs, err := applicationOffersDocs(st, application)
	if err != nil {
		return nil, errors.Trace(err)
	}
	var ops []txn.Op
	for _, doc := range docs {
		ops = append(ops, txn.Op{
			C:      applicationOffersC,
			Id:     doc.DocID,
			Assert: txn.DocExists,
			Remove: true,
		})
	}
	offerRefCountKey := applicationOffersRefCountKey(application)
	removeRefsOp := nsRefcounts.JustRemoveOp(refcountsC, offerRefCountKey, len(docs))
	return append(ops, removeRefsOp), nil
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
	if !names.IsValidUser(offer.Owner) {
		return errors.NotValidf("offer owner %q", offer.Owner)
	}
	for _, readUser := range offer.HasRead {
		if !names.IsValidUser(readUser) {
			return errors.NotValidf("offer reader %q", readUser)
		}
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
	uuid, err := utils.NewUUID()
	if err != nil {
		return nil, errors.Trace(err)
	}
	doc := s.makeApplicationOfferDoc(s.st, uuid.String(), offerArgs)
	result, err := s.makeApplicationOffer(doc)
	if err != nil {
		return nil, errors.Trace(err)
	}
	buildTxn := func(attempt int) ([]txn.Op, error) {
		// If we've tried once already and failed, check that
		// model may have been destroyed.
		if attempt > 0 {
			if err := checkModelActive(s.st); err != nil {
				return nil, errors.Trace(err)
			}
			_, err := s.ApplicationOffer(offerArgs.OfferName)
			if err == nil {
				return nil, errDuplicateApplicationOffer
			}
		}
		incRefOp, err := incApplicationOffersRefOp(s.st, offerArgs.ApplicationName)
		if err != nil {
			return nil, errors.Trace(err)
		}
		ops := []txn.Op{
			model.assertActiveOp(),
			{
				C:      applicationOffersC,
				Id:     doc.DocID,
				Assert: txn.DocMissing,
				Insert: doc,
			},
			incRefOp,
		}
		return ops, nil
	}
	err = s.st.db().Run(buildTxn)
	if err != nil {
		return nil, errors.Trace(err)
	}

	// Ensure the owner has admin access to the offer.
	offerTag := names.NewApplicationOfferTag(doc.OfferName)
	owner := names.NewUserTag(offerArgs.Owner)
	err = s.st.CreateOfferAccess(offerTag, owner, permission.AdminAccess)
	if err != nil {
		return nil, errors.Annotate(err, "granting admin permission to the offer owner")
	}
	// Add in any read access permissions.
	for _, user := range offerArgs.HasRead {
		readerTag := names.NewUserTag(user)
		err = s.st.CreateOfferAccess(offerTag, readerTag, permission.ReadAccess)
		if err != nil {
			return nil, errors.Annotatef(err, "granting read permission to %q", user)
		}
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

	offer, err := s.ApplicationOffer(offerArgs.OfferName)
	if err != nil {
		// This will either be NotFound or some other error.
		// In either case, we return the error.
		return nil, errors.Trace(err)
	}
	doc := s.makeApplicationOfferDoc(s.st, offer.OfferUUID, offerArgs)
	var refOps []txn.Op
	if offerArgs.ApplicationName != offer.ApplicationName {
		incRefOp, err := incApplicationOffersRefOp(s.st, offerArgs.ApplicationName)
		if err != nil {
			return nil, errors.Trace(err)
		}
		decRefOp, err := decApplicationOffersRefOp(s.st, offer.ApplicationName)
		if err != nil {
			return nil, errors.Trace(err)
		}
		refOps = append(refOps, incRefOp, decRefOp)
	}
	buildTxn := func(attempt int) ([]txn.Op, error) {
		// If we've tried once already and failed, check that
		// model may have been destroyed.
		if attempt > 0 {
			if err := checkModelActive(s.st); err != nil {
				return nil, errors.Trace(err)
			}
			_, err := s.ApplicationOffer(offerArgs.OfferName)
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
		ops = append(ops, refOps...)
		return ops, nil
	}
	err = s.st.db().Run(buildTxn)
	if err != nil {
		return nil, errors.Trace(err)
	}
	return s.makeApplicationOffer(doc)
}

func (s *applicationOffers) makeApplicationOfferDoc(mb modelBackend, uuid string, offer crossmodel.AddApplicationOfferArgs) applicationOfferDoc {
	doc := applicationOfferDoc{
		DocID:                  mb.docID(offer.OfferName),
		OfferUUID:              uuid,
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
		filter = append(filter, bson.DocElem{"offer-name", bson.D{{"$regex", fmt.Sprintf(".*%s.*", filterTerm.OfferName)}}})
	}
	// We match descriptions by looking for containing terms.
	if filterTerm.ApplicationDescription != "" {
		desc := regexp.QuoteMeta(filterTerm.ApplicationDescription)
		filter = append(filter, bson.DocElem{"application-description", bson.D{{"$regex", fmt.Sprintf(".*%s.*", desc)}}})
	}
	return filter
}

// ListOffers returns the application offers matching any one of the filter terms.
func (s *applicationOffers) ListOffers(filters ...crossmodel.ApplicationOfferFilter) ([]crossmodel.ApplicationOffer, error) {
	applicationOffersCollection, closer := s.st.db().GetCollection(applicationOffersC)
	defer closer()

	var offerDocs []applicationOfferDoc
	if len(filters) == 0 {
		err := applicationOffersCollection.Find(nil).All(&offerDocs)
		if err != nil {
			return nil, errors.Trace(err)
		}
	}
	for _, filter := range filters {
		var mgoQuery bson.D
		elems := s.makeFilterTerm(filter)
		mgoQuery = append(mgoQuery, elems...)

		var docs []applicationOfferDoc
		err := applicationOffersCollection.Find(mgoQuery).All(&docs)
		if err != nil {
			return nil, errors.Trace(err)
		}

		docs, err = s.filterOffers(docs, filter)
		if err != nil {
			return nil, errors.Trace(err)
		}
		offerDocs = append(offerDocs, docs...)
	}
	sort.Sort(offerSlice(offerDocs))

	offers := make([]crossmodel.ApplicationOffer, len(offerDocs))
	for i, doc := range offerDocs {
		offer, err := s.makeApplicationOffer(doc)
		if err != nil {
			return nil, errors.Trace(err)
		}
		offers[i] = *offer
	}
	return offers, nil
}

// filterOffers takes a list of offers resulting from a db query
// and performs additional filtering which cannot be done via mongo.
func (s *applicationOffers) filterOffers(
	in []applicationOfferDoc,
	filter crossmodel.ApplicationOfferFilter,
) ([]applicationOfferDoc, error) {

	out, err := s.filterOffersByEndpoint(in, filter.Endpoints)
	if err != nil {
		return nil, errors.Trace(err)
	}
	out, err = s.filterOffersByConnectedUser(out, filter.ConnectedUsers)
	if err != nil {
		return nil, errors.Trace(err)
	}
	return s.filterOffersByAllowedConsumer(out, filter.AllowedConsumers)
}

func (s *applicationOffers) filterOffersByEndpoint(
	in []applicationOfferDoc,
	endpoints []crossmodel.EndpointFilterTerm,
) ([]applicationOfferDoc, error) {

	if len(endpoints) == 0 {
		return in, nil
	}

	match := func(ep Endpoint) bool {
		for _, fep := range endpoints {
			if fep.Interface != "" && fep.Interface == ep.Interface {
				continue
			}
			if fep.Name != "" && fep.Name == ep.Name {
				continue
			}
			if fep.Role != "" && fep.Role == ep.Role {
				continue
			}
			return false
		}
		return true
	}

	var out []applicationOfferDoc
	for _, doc := range in {
		app, err := s.st.Application(doc.ApplicationName)
		if err != nil {
			return nil, errors.Trace(err)
		}
		for _, epName := range doc.Endpoints {
			ep, err := app.Endpoint(epName)
			if err != nil {
				return nil, errors.Trace(err)
			}
			if match(ep) {
				out = append(out, doc)
			}
		}
	}
	return out, nil
}

func (s *applicationOffers) filterOffersByConnectedUser(
	in []applicationOfferDoc,
	users []string,
) ([]applicationOfferDoc, error) {

	if len(users) == 0 {
		return in, nil
	}

	offerUUIDS := make(set.Strings)
	for _, username := range users {
		conns, err := s.st.OfferConnectionsForUser(username)
		if err != nil {
			return nil, errors.Trace(err)
		}
		for _, oc := range conns {
			offerUUIDS.Add(oc.OfferUUID())
		}
	}
	var out []applicationOfferDoc
	for _, doc := range in {
		if offerUUIDS.Contains(doc.OfferUUID) {
			out = append(out, doc)
		}
	}
	return out, nil
}

func (s *applicationOffers) filterOffersByAllowedConsumer(
	in []applicationOfferDoc,
	users []string,
) ([]applicationOfferDoc, error) {

	if len(users) == 0 {
		return in, nil
	}

	var out []applicationOfferDoc
	for _, doc := range in {
		offerUsers, err := s.st.GetOfferUsers(doc.OfferUUID)
		if err != nil {
			return nil, errors.Trace(err)
		}
		for _, username := range users {
			if offerUsers[username].EqualOrGreaterOfferAccessThan(permission.ConsumeAccess) {
				out = append(out, doc)
				break
			}
		}
	}
	return out, nil
}

func (s *applicationOffers) makeApplicationOffer(doc applicationOfferDoc) (*crossmodel.ApplicationOffer, error) {
	offer := &crossmodel.ApplicationOffer{
		OfferName:              doc.OfferName,
		OfferUUID:              doc.OfferUUID,
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

type offerSlice []applicationOfferDoc

func (sr offerSlice) Len() int      { return len(sr) }
func (sr offerSlice) Swap(i, j int) { sr[i], sr[j] = sr[j], sr[i] }
func (sr offerSlice) Less(i, j int) bool {
	sr1 := sr[i]
	sr2 := sr[j]
	if sr1.OfferName == sr2.OfferName {
		return sr1.ApplicationName < sr2.ApplicationName
	}
	return sr1.OfferName < sr2.OfferName
}

// WatchOfferStatus returns a NotifyWatcher that notifies of changes
// to the offer's status.
func (st *State) WatchOfferStatus(offerUUID string) (NotifyWatcher, error) {
	oa := NewApplicationOffers(st)
	offer, err := oa.ApplicationOfferForUUID(offerUUID)
	if err != nil {
		return nil, errors.Trace(err)
	}
	filter := func(id interface{}) bool {
		k, err := st.strictLocalID(id.(string))
		if err != nil {
			return false
		}

		// Does the app name match?
		if strings.HasPrefix(k, "a#") && k[2:] == offer.ApplicationName {
			return true
		}

		// Maybe it is a status change for a unit of the app.
		if !strings.HasPrefix(k, "u#") && !strings.HasSuffix(k, "#charm") {
			return false
		}
		k = strings.TrimRight(k[2:], "#charm")
		if !names.IsValidUnit(k) {
			return false
		}

		unitApp, _ := names.UnitApplication(k)
		return unitApp == offer.ApplicationName
	}
	return newNotifyCollWatcher(st, statusesC, filter), nil
}
