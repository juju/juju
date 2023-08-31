// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state

import (
	"fmt"
	"regexp"
	"sort"
	"strings"

	"github.com/juju/charm/v11"
	"github.com/juju/collections/set"
	"github.com/juju/errors"
	"github.com/juju/mgo/v3"
	"github.com/juju/mgo/v3/bson"
	"github.com/juju/mgo/v3/txn"
	"github.com/juju/names/v4"
	jujutxn "github.com/juju/txn/v3"
	"github.com/juju/utils/v3"

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

	// TxnRevno is used to assert the collection have not changed since this
	// document was fetched.
	TxnRevno int64 `bson:"txn-revno,omitempty"`
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
	offerDoc, err := s.applicationOfferDoc(offerName)
	if err != nil {
		return nil, errors.Trace(err)
	}
	return s.makeApplicationOffer(*offerDoc)
}

func (s *applicationOffers) applicationOfferDoc(offerName string) (*applicationOfferDoc, error) {
	offerDoc, err := s.offerQuery(bson.D{{"_id", offerName}})
	if err != nil {
		if err == mgo.ErrNotFound {
			return nil, errors.NotFoundf("offer %q", offerName)
		}
		return nil, errors.Annotatef(err, "cannot load application offer %q", offerName)
	}
	return offerDoc, nil
}

// ApplicationOfferForUUID returns the application offer for the UUID.
func (s *applicationOffers) ApplicationOfferForUUID(offerUUID string) (*crossmodel.ApplicationOffer, error) {
	offerDoc, err := s.offerQuery(bson.D{{"offer-uuid", offerUUID}})
	if err != nil {
		if err == mgo.ErrNotFound {
			return nil, errors.NotFoundf("offer %q", offerUUID)
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
		return nil, errors.Annotate(err, "getting application offer documents")
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

// maybeConsumerProxyForOffer returns the remote app consumer proxy related to
// the offer on the specified relation if one exists.
func (st *State) maybeConsumerProxyForOffer(offer *crossmodel.ApplicationOffer, rel *Relation) (*RemoteApplication, bool, error) {
	// Is the relation for an offer connection
	offConn, err := st.OfferConnectionForRelation(rel.String())
	if err != nil && !errors.IsNotFound(err) {
		return nil, false, errors.Trace(err)
	}
	if err != nil || offConn.OfferUUID() != offer.OfferUUID {
		return nil, false, nil
	}

	// Get the remote app proxy for the connection.
	remoteApp, isCrossModel, err := rel.RemoteApplication()
	if err != nil {
		return nil, false, errors.Trace(err)
	}
	// Sanity check - we expect a cross model relation at this stage.
	if !isCrossModel {
		return nil, false, nil
	}

	// We have a remote app proxy, is it related to the offer in question.
	_, err = rel.Endpoint(offer.ApplicationName)
	if err != nil {
		if !errors.IsNotFound(err) {
			return nil, false, errors.Trace(err)
		}
		return nil, false, nil
	}
	return remoteApp, true, nil
}

// RemoveOfferOperation returns a model operation that will allow relation to leave scope.
func (s *applicationOffers) RemoveOfferOperation(offerName string, force bool) (*RemoveOfferOperation, error) {
	offerStore := &applicationOffers{s.st}

	// Any proxies for applications on the consuming side also need to be removed.
	offer, err := offerStore.ApplicationOffer(offerName)
	if err != nil {
		return nil, errors.Trace(err)
	}
	var associatedAppProxies []*DestroyRemoteApplicationOperation
	if err == nil {
		// Look at relations to the offer and if it is a cross model relation,
		// record the associated remote app proxy in the remove operation.
		rels, err := s.st.AllRelations()
		if err != nil {
			return nil, errors.Trace(err)
		}
		for _, rel := range rels {
			remoteApp, isCrossModel, err := s.st.maybeConsumerProxyForOffer(offer, rel)
			if err != nil {
				return nil, errors.Trace(err)
			}
			if !isCrossModel {
				continue
			}
			logger.Debugf("destroy consumer proxy %v for offer %v", remoteApp.Name(), offerName)
			associatedAppProxies = append(associatedAppProxies, remoteApp.DestroyOperation(force))
		}
	}
	return &RemoveOfferOperation{
		offerStore:           offerStore,
		offerName:            offerName,
		associatedAppProxies: associatedAppProxies,
		ForcedOperation:      ForcedOperation{Force: force},
	}, nil
}

// RemoveOfferOperation is a model operation to remove application offer.
type RemoveOfferOperation struct {
	offerStore *applicationOffers

	// ForcedOperation stores needed information to force this operation.
	ForcedOperation
	// offerName is the offer name to remove.
	offerName string
	// offer is the offer itself, set as the operation runs.
	offer *crossmodel.ApplicationOffer

	// associatedAppProxies are consuming model references that need
	// to be removed along with the offer.
	associatedAppProxies []*DestroyRemoteApplicationOperation
}

// Build is part of the ModelOperation interface.
func (op *RemoveOfferOperation) Build(attempt int) (ops []txn.Op, err error) {
	op.offer, err = op.offerStore.ApplicationOffer(op.offerName)
	if errors.IsNotFound(err) {
		return nil, jujutxn.ErrNoOperations
	}
	if err != nil {
		return nil, errors.Trace(err)
	}
	// When 'force' is set on the operation, this call will return needed operations
	// and accumulate all operational errors encountered in the operation.
	// If the 'force' is not set, any error will be fatal and no operations will be returned.
	switch ops, err = op.internalRemove(op.offer); err {
	case errRefresh:
	case errAlreadyDying:
		return nil, jujutxn.ErrNoOperations
	}
	if err != nil {
		if op.Force {
			logger.Warningf("force removing offer %v despite error %v", op.offerName, err)
		} else {
			return nil, err
		}
	}
	// If the offer is being removed, then any proxies for applications on the
	// consuming side also need to be removed.
	for _, remoteProxyOp := range op.associatedAppProxies {
		proxyOps, err := remoteProxyOp.Build(attempt)
		if err == jujutxn.ErrNoOperations {
			continue
		}
		if err != nil {
			if remoteProxyOp.Force {
				logger.Warningf("force removing consuming proxy %v despite error %v", remoteProxyOp.app.Name(), err)
			} else {
				return nil, err
			}
		}
		ops = append(ops, proxyOps...)
	}
	return ops, nil
}

// Done is part of the ModelOperation interface.
func (op *RemoveOfferOperation) Done(err error) error {
	if err != nil {
		if !op.Force {
			if errors.Cause(err) == jujutxn.ErrExcessiveContention {
				relCount, err := op.countOfferRelations(op.offer)
				if err != nil {
					return errors.Annotatef(err, "cannot delete application offer %q", op.offerName)
				}
				if relCount > 0 {
					return errors.Errorf("cannot delete application offer %q since its underlying application still has %d relations", op.offerName, relCount)
				}
			}
			return errors.Annotatef(err, "cannot delete application offer %q", op.offerName)
		}
		op.AddError(errors.Errorf("forced offer %v removal but proceeded despite encountering ERROR %v", op.offerName, err))
	}
	for _, remoteProxyOp := range op.associatedAppProxies {
		// Final cleanup of consuming app proxy is best effort.
		if err := remoteProxyOp.Done(nil); err != nil {
			op.AddError(errors.Errorf("error finalising removal of consuming proxy %q: %v", remoteProxyOp.app.Name(), err))
		}
	}
	// Now the offer is removed, delete any user permissions.
	userPerms, err := op.offerStore.st.GetOfferUsers(op.offer.OfferUUID)
	if err != nil {
		op.AddError(errors.Errorf("error removing offer permissions: %v", err))
		return nil
	}
	var removeOps []txn.Op
	for userName := range userPerms {
		user := names.NewUserTag(userName)
		removeOps = append(removeOps,
			removePermissionOp(applicationOfferKey(op.offer.OfferUUID), userGlobalKey(userAccessID(user))))
	}
	err = op.offerStore.st.db().RunTransaction(removeOps)
	if err != nil {
		op.AddError(errors.Errorf("error removing offer permissions: %v", err))
	}
	return nil
}

// Remove deletes the application offer for offerName immediately.
func (s *applicationOffers) Remove(offerName string, force bool) error {
	op, err := s.RemoveOfferOperation(offerName, force)
	if err != nil {
		return errors.Trace(err)
	}
	err = s.st.ApplyOperation(op)
	if len(op.Errors) != 0 {
		logger.Warningf("operational errors removing offer %v: %v", offerName, op.Errors)
	}
	return err
}

func (op *RemoveOfferOperation) countOfferRelations(offer *crossmodel.ApplicationOffer) (int, error) {
	if offer == nil {
		return 0, nil
	}
	app, err := op.offerStore.st.Application(offer.ApplicationName)
	if err != nil {
		return 0, errors.Trace(err)
	}
	rels, err := app.Relations()
	if err != nil {
		return 0, errors.Trace(err)
	}
	var count int
	for _, rel := range rels {
		remoteApp, isCrossModel, err := op.offerStore.st.maybeConsumerProxyForOffer(offer, rel)
		if err != nil {
			return 0, errors.Trace(err)
		}
		if !isCrossModel || remoteApp == nil {
			continue
		}
		count++
	}
	return count, nil
}

func (op *RemoveOfferOperation) internalRemove(offer *crossmodel.ApplicationOffer) ([]txn.Op, error) {
	// Load the application before counting the connections
	// so we can do a consistency check on relation count.
	app, err := op.offerStore.st.Application(offer.ApplicationName)
	if err != nil {
		return nil, errors.Trace(err)
	}
	conns, err := op.offerStore.st.OfferConnections(offer.OfferUUID)
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
		remoteApp, isCrossModel, err := op.offerStore.st.maybeConsumerProxyForOffer(offer, rel)
		if err != nil {
			return nil, errors.Trace(err)
		}
		if isCrossModel && !op.Force {
			logger.Debugf("aborting removal of offer %q due to relation %q", offer.OfferName, rel)
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

			// Force any remote units to leave scope so the offer can be cleaned up.
			destroyRelUnitOps, err := destroyCrossModelRelationUnitsOps(&op.ForcedOperation, remoteApp, rel, false)
			if err != nil && err != jujutxn.ErrNoOperations {
				return nil, errors.Trace(err)
			}
			ops = append(ops, destroyRelUnitOps...)

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
	decRefOp, err := decApplicationOffersRefOp(op.offerStore.st, offer.ApplicationName)
	if err != nil {
		return nil, errors.Trace(err)
	}
	ops = append(ops, txn.Op{
		C:      applicationOffersC,
		Id:     offer.OfferName,
		Assert: txn.DocExists,
		Remove: true,
	}, decRefOp)
	r := op.offerStore.st.RemoteEntities()
	tokenOps := r.removeRemoteEntityOps(names.NewApplicationTag(offer.OfferName))
	ops = append(ops, tokenOps...)
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

	// Check application and endpoints exist in state.
	app, err := s.st.Application(offer.ApplicationName)
	if err != nil {
		return errors.Trace(err)
	}
	_, err = getApplicationEndpoints(app, offer.Endpoints)
	if err != nil {
		return errors.Trace(err)
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
	defer errors.DeferredAnnotatef(&err, "cannot update application offer %q", offerArgs.OfferName)

	if err := s.validateOfferArgs(offerArgs); err != nil {
		return nil, err
	}
	model, err := s.st.Model()
	if err != nil {
		return nil, errors.Trace(err)
	} else if model.Life() != Alive {
		return nil, errors.Errorf("model is no longer alive")
	}

	var doc applicationOfferDoc
	buildTxn := func(attempt int) ([]txn.Op, error) {
		// If we've tried once already and failed, check that
		// model may have been destroyed.
		if attempt > 0 {
			if err := checkModelActive(s.st); err != nil {
				return nil, errors.Trace(err)
			}
		}

		// Load fresh copy of the offer and setup the update document.
		curOfferDoc, err := s.applicationOfferDoc(offerArgs.OfferName)
		if err != nil {
			// This will either be NotFound or some other error.
			// In either case, we return the error.
			return nil, errors.Trace(err)
		}
		doc = s.makeApplicationOfferDoc(s.st, curOfferDoc.OfferUUID, offerArgs)

		var ops []txn.Op
		if offerArgs.ApplicationName != curOfferDoc.ApplicationName {
			incRefOp, err := incApplicationOffersRefOp(s.st, offerArgs.ApplicationName)
			if err != nil {
				return nil, errors.Trace(err)
			}
			decRefOp, err := decApplicationOffersRefOp(s.st, curOfferDoc.ApplicationName)
			if err != nil {
				return nil, errors.Trace(err)
			}
			ops = append(ops, incRefOp, decRefOp)
		} else {
			// Figure out if we are trying to remove any endpoints from the
			// current offer instance.
			existingEndpoints := set.NewStrings()
			for _, ep := range curOfferDoc.Endpoints {
				existingEndpoints.Add(ep)
			}

			updatedEndpoints := set.NewStrings()
			for _, ep := range offerArgs.Endpoints {
				updatedEndpoints.Add(ep)
			}

			// If the update removes any existing endpoints ensure that they
			// are not currently in use and return an error if that's the
			// case. This prevents users from accidentally breaking saas
			// consumers.
			goneEndpoints := existingEndpoints.Difference(updatedEndpoints)
			if err := s.ensureEndpointsNotInUse(curOfferDoc.ApplicationName, curOfferDoc.OfferUUID, goneEndpoints); err != nil {
				return nil, err
			}
		}

		return append(ops,
			model.assertActiveOp(),
			txn.Op{
				C:      applicationOffersC,
				Id:     doc.DocID,
				Assert: bson.D{{"txn-revno", curOfferDoc.TxnRevno}},
				Update: bson.M{"$set": doc},
			},
		), nil
	}
	err = s.st.db().Run(buildTxn)
	if err != nil {
		return nil, errors.Trace(err)
	}
	return s.makeApplicationOffer(doc)
}

func (s *applicationOffers) ensureEndpointsNotInUse(appName, offerUUID string, endpoints set.Strings) error {
	if len(endpoints) == 0 {
		return nil
	}

	connections, err := s.st.OfferConnections(offerUUID)
	if err != nil {
		return errors.Trace(err)
	}

	inUse := set.NewStrings()
	for _, conn := range connections {
		for _, part := range strings.Fields(conn.RelationKey()) {
			tokens := strings.Split(part, ":")
			if len(tokens) != 2 {
				return errors.New("malformed relation key")
			}

			if tokens[0] == appName && endpoints.Contains(tokens[1]) {
				inUse.Add(tokens[1])
			}
		}
	}

	switch len(inUse) {
	case 0:
		return nil
	case 1:
		return errors.Errorf("application endpoint %q has active consumers", inUse.Values()[0])
	default:
		return errors.Errorf("application endpoints %q have active consumers", strings.Join(inUse.SortedValues(), ", "))
	}
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

// WatchOffer returns a new NotifyWatcher watching for
// changes to the specified offer.
func (st *State) WatchOffer(offerName string) NotifyWatcher {
	filter := func(rawId interface{}) bool {
		id, err := st.strictLocalID(rawId.(string))
		if err != nil {
			return false
		}
		return offerName == id
	}
	return newNotifyCollWatcher(st, applicationOffersC, filter)
}
