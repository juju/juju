// Copyright 2012, 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state

import (
	"fmt"
	"sort"
	"strings"

	"github.com/juju/errors"
	jujutxn "github.com/juju/txn"
	"gopkg.in/juju/charm.v6"
	"gopkg.in/juju/names.v2"
	"gopkg.in/mgo.v2"
	"gopkg.in/mgo.v2/bson"
	"gopkg.in/mgo.v2/txn"

	"github.com/juju/juju/core/status"
	"github.com/juju/juju/permission"
)

// relationKey returns a string describing the relation defined by
// endpoints, for use in various contexts (including error messages).
func relationKey(endpoints []Endpoint) string {
	eps := epSlice{}
	for _, ep := range endpoints {
		eps = append(eps, ep)
	}
	sort.Sort(eps)
	names := []string{}
	for _, ep := range eps {
		names = append(names, ep.String())
	}
	return strings.Join(names, " ")
}

// relationDoc is the internal representation of a Relation in MongoDB.
// Note the correspondence with RelationInfo in apiserver/params.
type relationDoc struct {
	DocID           string     `bson:"_id"`
	Key             string     `bson:"key"`
	ModelUUID       string     `bson:"model-uuid"`
	Id              int        `bson:"id"`
	Endpoints       []Endpoint `bson:"endpoints"`
	Life            Life       `bson:"life"`
	UnitCount       int        `bson:"unitcount"`
	Suspended       bool       `bson:"suspended"`
	SuspendedReason string     `bson:"suspended-reason"`
}

// Relation represents a relation between one or two application endpoints.
type Relation struct {
	st  *State
	doc relationDoc
}

func newRelation(st *State, doc *relationDoc) *Relation {
	return &Relation{
		st:  st,
		doc: *doc,
	}
}

func (r *Relation) String() string {
	return r.doc.Key
}

// Tag returns a name identifying the relation.
func (r *Relation) Tag() names.Tag {
	return names.NewRelationTag(r.doc.Key)
}

// Suspended returns true if the relation is suspended.
func (r *Relation) Suspended() bool {
	return r.doc.Suspended
}

// SuspendedReason returns the reason why the relation is suspended.
func (r *Relation) SuspendedReason() string {
	return r.doc.SuspendedReason
}

// Refresh refreshes the contents of the relation from the underlying
// state. It returns an error that satisfies errors.IsNotFound if the
// relation has been removed.
func (r *Relation) Refresh() error {
	relations, closer := r.st.db().GetCollection(relationsC)
	defer closer()

	doc := relationDoc{}
	err := relations.FindId(r.doc.DocID).One(&doc)
	if err == mgo.ErrNotFound {
		return errors.NotFoundf("relation %v", r)
	}
	if err != nil {
		return errors.Annotatef(err, "cannot refresh relation %v", r)
	}
	if r.doc.Id != doc.Id {
		// The relation has been destroyed and recreated. This is *not* the
		// same relation; if we pretend it is, we run the risk of violating
		// the lifecycle-only-advances guarantee.
		return errors.NotFoundf("relation %v", r)
	}
	r.doc = doc
	return nil
}

// Life returns the relation's current life state.
func (r *Relation) Life() Life {
	return r.doc.Life
}

// Status returns the relation's current status data.
func (r *Relation) Status() (status.StatusInfo, error) {
	rStatus, err := getStatus(r.st.db(), r.globalScope(), "relation")
	if err != nil {
		return rStatus, err
	}
	return rStatus, nil
}

// SetStatus sets the status of the relation.
func (r *Relation) SetStatus(statusInfo status.StatusInfo) error {
	currentStatus, err := r.Status()
	if err != nil {
		return errors.Trace(err)
	}

	if currentStatus.Status != statusInfo.Status {
		validTransition := true
		switch statusInfo.Status {
		case status.Broken:
		case status.Suspending:
			validTransition = currentStatus.Status != status.Broken && currentStatus.Status != status.Suspended
		case status.Joining:
			validTransition = currentStatus.Status != status.Broken && currentStatus.Status != status.Joined
		case status.Joined, status.Suspended:
			validTransition = currentStatus.Status != status.Broken
		case status.Error:
			if statusInfo.Message == "" {
				return errors.Errorf("cannot set status %q without info", statusInfo.Status)
			}
		default:
			return errors.NewNotValid(nil, fmt.Sprintf("cannot set invalid status %q", statusInfo.Status))
		}
		if !validTransition {
			return errors.NewNotValid(nil, fmt.Sprintf(
				"cannot set status %q when relation has status %q", statusInfo.Status, currentStatus.Status))
		}
	}
	return setStatus(r.st.db(), setStatusParams{
		badge:     "relation",
		globalKey: r.globalScope(),
		status:    statusInfo.Status,
		message:   statusInfo.Message,
		rawData:   statusInfo.Data,
		updated:   timeOrNow(statusInfo.Since, r.st.clock()),
	})
}

// SetSuspended sets whether the relation is suspended.
func (r *Relation) SetSuspended(suspended bool, suspendedReason string) error {
	if r.doc.Suspended == suspended {
		return nil
	}
	if !suspended && suspendedReason != "" {
		return errors.New("cannot set suspended reason if not suspended")
	}

	var buildTxn jujutxn.TransactionSource = func(attempt int) ([]txn.Op, error) {

		if attempt > 1 {
			if err := r.Refresh(); err != nil {
				return nil, errors.Trace(err)
			}
		}

		var (
			oc       *OfferConnection
			err      error
			checkOps []txn.Op
		)
		oc, err = r.st.OfferConnectionForRelation(r.Tag().Id())
		if err != nil && !errors.IsNotFound(err) {
			return nil, errors.Trace(err)
		}
		if err == nil {
			checkOps = append(checkOps, txn.Op{
				C:      offerConnectionsC,
				Id:     fmt.Sprintf("%d", r.Id()),
				Assert: txn.DocExists,
			})
		}
		if !suspended && oc != nil {
			// Can only resume a relation when the user of the associated connection has consume access
			// - either via being a model admin or having been granted access.
			isAdmin, err := r.st.isControllerOrModelAdmin(names.NewUserTag(oc.UserName()))
			if err != nil {
				return nil, errors.Trace(err)
			}
			if !isAdmin {
				// Not an admin so check for consume access and add the assert.
				ok, err := r.checkConsumePermission(oc.OfferUUID(), oc.UserName())
				if err != nil {
					return nil, errors.Trace(err)
				}
				if !ok {
					return nil, errors.Errorf(
						"cannot resume relation %q where user %q does not have consume permission",
						r.Tag().Id(), oc.UserName())
				}
				checkOps = append(checkOps, txn.Op{
					C:  permissionsC,
					Id: permissionID(applicationOfferKey(oc.OfferUUID()), userGlobalKey(strings.ToLower(oc.UserName()))),
					Assert: bson.D{
						{"access",
							bson.D{{"$in", []permission.Access{permission.ConsumeAccess, permission.AdminAccess}}}}},
				})
			}
		}
		setOps := []txn.Op{{
			C:      relationsC,
			Id:     r.doc.DocID,
			Assert: bson.D{{"suspended", r.doc.Suspended}},
			Update: bson.D{
				{"$set", bson.D{{"suspended", suspended}}},
				{"$set", bson.D{{"suspended-reason", suspendedReason}}},
			},
		}}
		return append(setOps, checkOps...), nil
	}

	err := r.st.db().Run(buildTxn)
	if err == nil {
		r.doc.Suspended = suspended
	}
	return err
}

func (r *Relation) checkConsumePermission(offerUUID, userId string) (bool, error) {
	perm, err := r.st.GetOfferAccess(offerUUID, names.NewUserTag(userId))
	if err != nil && !errors.IsNotFound(err) {
		return false, errors.Trace(err)
	}
	if perm != permission.ConsumeAccess && perm != permission.AdminAccess {
		return false, nil
	}
	return true, nil
}

// Destroy ensures that the relation will be removed at some point; if no units
// are currently in scope, it will be removed immediately.
func (r *Relation) Destroy() (err error) {
	defer errors.DeferredAnnotatef(&err, "cannot destroy relation %q", r)
	if len(r.doc.Endpoints) == 1 && r.doc.Endpoints[0].Role == charm.RolePeer {
		return errors.Errorf("is a peer relation")
	}
	defer func() {
		if err == nil {
			// This is a white lie; the document might actually be removed.
			r.doc.Life = Dying
		}
	}()
	rel := &Relation{r.st, r.doc}
	// In this context, aborted transactions indicate that the number of units
	// in scope have changed between 0 and not-0. The chances of 5 successive
	// attempts each hitting this change -- which is itself an unlikely one --
	// are considered to be extremely small.
	buildTxn := func(attempt int) ([]txn.Op, error) {
		if attempt > 0 {
			if err := rel.Refresh(); errors.IsNotFound(err) {
				return []txn.Op{}, nil
			} else if err != nil {
				return nil, err
			}
		}
		ops, _, err := rel.destroyOps("")
		if err == errAlreadyDying {
			return nil, jujutxn.ErrNoOperations
		} else if err != nil {
			return nil, err
		}
		return ops, nil
	}
	return rel.st.db().Run(buildTxn)
}

// destroyOps returns the operations necessary to destroy the relation, and
// whether those operations will lead to the relation's removal. These
// operations may include changes to the relation's applications; however, if
// ignoreApplication is not empty, no operations modifying that application will
// be generated.
func (r *Relation) destroyOps(ignoreApplication string) (ops []txn.Op, isRemove bool, err error) {
	if r.doc.Life != Alive {
		return nil, false, errAlreadyDying
	}
	if r.doc.UnitCount == 0 {
		removeOps, err := r.removeOps(ignoreApplication, "")
		if err != nil {
			return nil, false, err
		}
		return removeOps, true, nil
	}
	return []txn.Op{{
		C:      relationsC,
		Id:     r.doc.DocID,
		Assert: bson.D{{"life", Alive}, {"unitcount", bson.D{{"$gt", 0}}}},
		Update: bson.D{{"$set", bson.D{{"life", Dying}}}},
	}}, false, nil
}

// removeOps returns the operations necessary to remove the relation. If
// ignoreApplication is not empty, no operations affecting that application will be
// included; if departingUnitName is non-empty, this implies that the
// relation's applications may be Dying and otherwise unreferenced, and may thus
// require removal themselves.
func (r *Relation) removeOps(ignoreApplication string, departingUnitName string) ([]txn.Op, error) {
	relOp := txn.Op{
		C:      relationsC,
		Id:     r.doc.DocID,
		Remove: true,
	}
	if departingUnitName != "" {
		relOp.Assert = bson.D{{"life", Dying}, {"unitcount", 1}}
	} else {
		relOp.Assert = bson.D{{"life", Alive}, {"unitcount", 0}}
	}
	ops := []txn.Op{relOp}
	for _, ep := range r.doc.Endpoints {
		if ep.ApplicationName == ignoreApplication {
			continue
		}
		app, err := applicationByName(r.st, ep.ApplicationName)
		if err != nil {
			return nil, errors.Trace(err)
		}
		if app.IsRemote() {
			epOps, err := r.removeRemoteEndpointOps(ep, departingUnitName != "")
			if err != nil {
				return nil, errors.Trace(err)
			}
			ops = append(ops, epOps...)
		} else {
			epOps, err := r.removeLocalEndpointOps(ep, departingUnitName)
			if err != nil {
				return nil, errors.Trace(err)
			}
			ops = append(ops, epOps...)
		}
	}
	ops = append(ops, removeStatusOp(r.st, r.globalScope()))
	ops = append(ops, removeRelationNetworksOps(r.st, r.doc.Key)...)
	re := r.st.RemoteEntities()
	tokenOps := re.removeRemoteEntityOps(r.Tag())
	ops = append(ops, tokenOps...)
	offerOps := removeOfferConnectionsForRelationOps(r.Id())
	ops = append(ops, offerOps...)
	cleanupOp := newCleanupOp(cleanupRelationSettings, fmt.Sprintf("r#%d#", r.Id()))
	return append(ops, cleanupOp), nil
}

func (r *Relation) removeLocalEndpointOps(ep Endpoint, departingUnitName string) ([]txn.Op, error) {
	var asserts bson.D
	hasRelation := bson.D{{"relationcount", bson.D{{"$gt", 0}}}}
	departingUnitApplicationMatchesEndpoint := func() bool {
		s, err := names.UnitApplication(departingUnitName)
		return err == nil && s == ep.ApplicationName
	}
	if departingUnitName == "" {
		// We're constructing a destroy operation, either of the relation
		// or one of its applications, and can therefore be assured that both
		// applications are Alive.
		asserts = append(hasRelation, isAliveDoc...)
	} else if departingUnitApplicationMatchesEndpoint() {
		// This application must have at least one unit -- the one that's
		// departing the relation -- so it cannot be ready for removal.
		cannotDieYet := bson.D{{"unitcount", bson.D{{"$gt", 0}}}}
		asserts = append(hasRelation, cannotDieYet...)
	} else {
		// This application may require immediate removal.
		applications, closer := r.st.db().GetCollection(applicationsC)
		defer closer()

		app := &Application{st: r.st}
		hasLastRef := bson.D{{"life", Dying}, {"unitcount", 0}, {"relationcount", 1}}
		removable := append(bson.D{{"_id", ep.ApplicationName}}, hasLastRef...)
		if err := applications.Find(removable).One(&app.doc); err == nil {
			return app.removeOps(hasLastRef)
		} else if err != mgo.ErrNotFound {
			return nil, err
		}
		// If not, we must check that this is still the case when the
		// transaction is applied.
		asserts = bson.D{{"$or", []bson.D{
			{{"life", Alive}},
			{{"unitcount", bson.D{{"$gt", 0}}}},
			{{"relationcount", bson.D{{"$gt", 1}}}},
		}}}
	}
	return []txn.Op{{
		C:      applicationsC,
		Id:     r.st.docID(ep.ApplicationName),
		Assert: asserts,
		Update: bson.D{{"$inc", bson.D{{"relationcount", -1}}}},
	}}, nil
}

func (r *Relation) removeRemoteEndpointOps(ep Endpoint, unitDying bool) ([]txn.Op, error) {
	var asserts bson.D
	hasRelation := bson.D{{"relationcount", bson.D{{"$gt", 0}}}}
	if !unitDying {
		// We're constructing a destroy operation, either of the relation
		// or one of its application, and can therefore be assured that both
		// applications are Alive.
		asserts = append(hasRelation, isAliveDoc...)
	} else {
		// The remote application may require immediate removal.
		applications, closer := r.st.db().GetCollection(remoteApplicationsC)
		defer closer()

		app := &RemoteApplication{st: r.st}
		hasLastRef := bson.D{{"life", Dying}, {"relationcount", 1}}
		removable := append(bson.D{{"_id", ep.ApplicationName}}, hasLastRef...)
		if err := applications.Find(removable).One(&app.doc); err == nil {
			removeOps, err := app.removeOps(hasLastRef)
			if err != nil {
				return nil, err
			}
			return removeOps, nil
		} else if err != mgo.ErrNotFound {
			return nil, err
		}
		// If not, we must check that this is still the case when the
		// transaction is applied.
		asserts = bson.D{{"$or", []bson.D{
			{{"life", Alive}},
			{{"relationcount", bson.D{{"$gt", 1}}}},
		}}}
	}
	return []txn.Op{{
		C:      remoteApplicationsC,
		Id:     r.st.docID(ep.ApplicationName),
		Assert: asserts,
		Update: bson.D{{"$inc", bson.D{{"relationcount", -1}}}},
	}}, nil
}

// Id returns the integer internal relation key. This is exposed
// because the unit agent needs to expose a value derived from this
// (as JUJU_RELATION_ID) to allow relation hooks to differentiate
// between relations with different applications.
func (r *Relation) Id() int {
	return r.doc.Id
}

// Endpoint returns the endpoint of the relation for the named application.
// If the application is not part of the relation, an error will be returned.
func (r *Relation) Endpoint(applicationname string) (Endpoint, error) {
	for _, ep := range r.doc.Endpoints {
		if ep.ApplicationName == applicationname {
			return ep, nil
		}
	}
	msg := fmt.Sprintf("application %q is not a member of %q", applicationname, r)
	return Endpoint{}, errors.NewNotFound(nil, msg)
}

// Endpoints returns the endpoints for the relation.
func (r *Relation) Endpoints() []Endpoint {
	return r.doc.Endpoints
}

// RelatedEndpoints returns the endpoints of the relation r with which
// units of the named application will establish relations. If the service
// is not part of the relation r, an error will be returned.
func (r *Relation) RelatedEndpoints(applicationname string) ([]Endpoint, error) {
	local, err := r.Endpoint(applicationname)
	if err != nil {
		return nil, err
	}
	role := counterpartRole(local.Role)
	var eps []Endpoint
	for _, ep := range r.doc.Endpoints {
		if ep.Role == role {
			eps = append(eps, ep)
		}
	}
	if eps == nil {
		return nil, errors.Errorf("no endpoints of %q relate to application %q", r, applicationname)
	}
	return eps, nil
}

// Unit returns a RelationUnit for the supplied unit.
func (r *Relation) Unit(u *Unit) (*RelationUnit, error) {
	const isLocalUnit = true
	return r.unit(u.Name(), u.doc.Principal, u.IsPrincipal(), isLocalUnit)
}

// RemoteUnit returns a RelationUnit for the supplied unit
// of a remote application.
func (r *Relation) RemoteUnit(unitName string) (*RelationUnit, error) {
	// Verify that the unit belongs to a remote application.
	appName, err := names.UnitApplication(unitName)
	if err != nil {
		return nil, errors.Trace(err)
	}
	if _, err := r.st.RemoteApplication(appName); err != nil {
		return nil, errors.Trace(err)
	}
	// Only non-subordinate applications may be offered for remote
	// relation, so all remote units are principals.
	const principal = ""
	const isPrincipal = true
	const isLocalUnit = false
	return r.unit(unitName, principal, isPrincipal, isLocalUnit)
}

// AllRemoteUnits returns all the RelationUnits for the remote
// application units for a given application.
func (r *Relation) AllRemoteUnits(appName string) ([]*RelationUnit, error) {
	// Verify that the unit belongs to a remote application.
	if _, err := r.st.RemoteApplication(appName); err != nil {
		return nil, errors.Trace(err)
	}

	relationScopes, closer := r.st.db().GetCollection(relationScopesC)
	defer closer()

	ep, err := r.Endpoint(appName)
	if err != nil {
		return nil, err
	}
	scope := r.globalScope()
	parts := []string{"^" + scope, string(ep.Role), appName + "/"}
	ruRegex := strings.Join(parts, "#")

	var docs []relationScopeDoc
	if err := relationScopes.Find(bson.D{{"key", bson.D{{"$regex", ruRegex}}}}).All(&docs); err != nil {
		return nil, errors.Trace(err)
	}
	result := make([]*RelationUnit, len(docs))
	for i, doc := range docs {
		result[i] = &RelationUnit{
			st:          r.st,
			relation:    r,
			unitName:    doc.unitName(),
			isPrincipal: true,
			isLocalUnit: false,
			endpoint:    ep,
			scope:       scope,
		}
	}
	return result, nil
}

// IsCrossModel returns whether this relation is a cross-model
// relation.
func (r *Relation) IsCrossModel() (bool, error) {
	for _, ep := range r.Endpoints() {
		_, err := r.st.RemoteApplication(ep.ApplicationName)
		if err == nil {
			return true, nil
		} else if !errors.IsNotFound(err) {
			return false, errors.Trace(err)
		}
	}
	return false, nil
}

func (r *Relation) unit(
	unitName string,
	principal string,
	isPrincipal bool,
	isLocalUnit bool,
) (*RelationUnit, error) {
	appName, err := names.UnitApplication(unitName)
	if err != nil {
		return nil, err
	}
	ep, err := r.Endpoint(appName)
	if err != nil {
		return nil, err
	}
	scope := r.globalScope()
	if ep.Scope == charm.ScopeContainer {
		container := principal
		if container == "" {
			container = unitName
		}
		scope = fmt.Sprintf("%s#%s", scope, container)
	}
	return &RelationUnit{
		st:          r.st,
		relation:    r,
		unitName:    unitName,
		isPrincipal: isPrincipal,
		isLocalUnit: isLocalUnit,
		endpoint:    ep,
		scope:       scope,
	}, nil
}

// globalScope returns the scope prefix for relation scope document keys
// in the global scope.
func (r *Relation) globalScope() string {
	return relationGlobalScope(r.doc.Id)
}

func relationGlobalScope(id int) string {
	return fmt.Sprintf("r#%d", id)
}

// relationSettingsCleanupChange removes the settings doc.
type relationSettingsCleanupChange struct {
	Prefix string
}

// Prepare is part of the Change interface.
func (change relationSettingsCleanupChange) Prepare(db Database) ([]txn.Op, error) {
	settings, closer := db.GetCollection(settingsC)
	defer closer()
	sel := bson.D{{"_id", bson.D{{"$regex", "^" + change.Prefix}}}}
	var docs []struct {
		DocID string `bson:"_id"`
	}
	err := settings.Find(sel).Select(bson.D{{"_id", 1}}).All(&docs)
	if err != nil {
		return nil, errors.Trace(err)
	}
	if len(docs) == 0 {
		return nil, ErrChangeComplete
	}

	ops := make([]txn.Op, len(docs))
	for i, doc := range docs {
		ops[i] = txn.Op{
			C:      settingsC,
			Id:     doc.DocID,
			Remove: true,
		}
	}
	return ops, nil

}
