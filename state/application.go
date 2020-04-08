// Copyright 2012, 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state

import (
	stderrors "errors"
	"fmt"
	"sort"
	"strconv"
	"strings"

	"github.com/juju/collections/set"
	"github.com/juju/errors"
	"github.com/juju/os/series"
	"github.com/juju/schema"
	jujutxn "github.com/juju/txn"
	"github.com/juju/utils"
	"github.com/juju/version"
	"gopkg.in/juju/charm.v6"
	csparams "gopkg.in/juju/charmrepo.v4/csclient/params"
	"gopkg.in/juju/environschema.v1"
	"gopkg.in/juju/names.v3"
	"gopkg.in/mgo.v2"
	"gopkg.in/mgo.v2/bson"
	"gopkg.in/mgo.v2/txn"

	"github.com/juju/juju/core/application"
	"github.com/juju/juju/core/constraints"
	"github.com/juju/juju/core/leadership"
	"github.com/juju/juju/core/model"
	"github.com/juju/juju/core/network"
	"github.com/juju/juju/core/status"
	mgoutils "github.com/juju/juju/mongo/utils"
	"github.com/juju/juju/tools"
)

// Application represents the state of an application.
type Application struct {
	st  *State
	doc applicationDoc
}

// applicationDoc represents the internal state of an application in MongoDB.
// Note the correspondence with ApplicationInfo in apiserver.
type applicationDoc struct {
	DocID                string       `bson:"_id"`
	Name                 string       `bson:"name"`
	ModelUUID            string       `bson:"model-uuid"`
	Series               string       `bson:"series"`
	Subordinate          bool         `bson:"subordinate"`
	CharmURL             *charm.URL   `bson:"charmurl"`
	Channel              string       `bson:"cs-channel"`
	CharmModifiedVersion int          `bson:"charmmodifiedversion"`
	ForceCharm           bool         `bson:"forcecharm"`
	Life                 Life         `bson:"life"`
	UnitCount            int          `bson:"unitcount"`
	RelationCount        int          `bson:"relationcount"`
	Exposed              bool         `bson:"exposed"`
	MinUnits             int          `bson:"minunits"`
	Tools                *tools.Tools `bson:",omitempty"`
	TxnRevno             int64        `bson:"txn-revno"`
	MetricCredentials    []byte       `bson:"metric-credentials"`

	// CAAS related attributes.
	DesiredScale int    `bson:"scale"`
	PasswordHash string `bson:"passwordhash"`
	// Placement is the placement directive that should be used allocating units/pods.
	Placement string `bson:"placement,omitempty"`
}

func newApplication(st *State, doc *applicationDoc) *Application {
	app := &Application{
		st:  st,
		doc: *doc,
	}
	return app
}

// IsRemote returns false for a local application.
func (a *Application) IsRemote() bool {
	return false
}

// Name returns the application name.
func (a *Application) Name() string {
	return a.doc.Name
}

// Tag returns a name identifying the application.
// The returned name will be different from other Tag values returned by any
// other entities from the same state.
func (a *Application) Tag() names.Tag {
	return a.ApplicationTag()
}

// ApplicationTag returns the more specific ApplicationTag rather than the generic
// Tag.
func (a *Application) ApplicationTag() names.ApplicationTag {
	return names.NewApplicationTag(a.Name())
}

// applicationGlobalKey returns the global database key for the application
// with the given name.
func applicationGlobalKey(appName string) string {
	return "a#" + appName
}

// globalKey returns the global database key for the application.
func (a *Application) globalKey() string {
	return applicationGlobalKey(a.doc.Name)
}

func applicationGlobalOperatorKey(appName string) string {
	return applicationGlobalKey(appName) + "#operator"
}

func applicationCharmConfigKey(appName string, curl *charm.URL) string {
	return fmt.Sprintf("a#%s#%s", appName, curl)
}

// charmConfigKey returns the charm-version-specific settings collection
// key for the application.
func (a *Application) charmConfigKey() string {
	return applicationCharmConfigKey(a.doc.Name, a.doc.CharmURL)
}

func applicationConfigKey(appName string) string {
	return fmt.Sprintf("a#%s#application", appName)
}

// applicationConfigKey returns the charm-version-specific settings collection
// key for the application.
func (a *Application) applicationConfigKey() string {
	return applicationConfigKey(a.doc.Name)
}

func applicationStorageConstraintsKey(appName string, curl *charm.URL) string {
	return fmt.Sprintf("asc#%s#%s", appName, curl)
}

// storageConstraintsKey returns the charm-version-specific storage
// constraints collection key for the application.
func (a *Application) storageConstraintsKey() string {
	return applicationStorageConstraintsKey(a.doc.Name, a.doc.CharmURL)
}

func applicationDeviceConstraintsKey(appName string, curl *charm.URL) string {
	return fmt.Sprintf("adc#%s#%s", appName, curl)
}

// deviceConstraintsKey returns the charm-version-specific device
// constraints collection key for the application.
func (a *Application) deviceConstraintsKey() string {
	return applicationDeviceConstraintsKey(a.doc.Name, a.doc.CharmURL)
}

// Series returns the specified series for this charm.
func (a *Application) Series() string {
	return a.doc.Series
}

// Life returns whether the application is Alive, Dying or Dead.
func (a *Application) Life() Life {
	return a.doc.Life
}

// AgentTools returns the tools that the operator is currently running.
// It an error that satisfies errors.IsNotFound if the tools have not
// yet been set.
func (a *Application) AgentTools() (*tools.Tools, error) {
	if a.doc.Tools == nil {
		return nil, errors.NotFoundf("operator image metadata for application %q", a)
	}
	result := *a.doc.Tools
	return &result, nil
}

// SetAgentVersion sets the Tools value in applicationDoc.
func (a *Application) SetAgentVersion(v version.Binary) (err error) {
	defer errors.DeferredAnnotatef(&err, "cannot set agent version for application %q", a)
	if err = checkVersionValidity(v); err != nil {
		return errors.Trace(err)
	}
	versionedTool := &tools.Tools{Version: v}
	ops := []txn.Op{{
		C:      applicationsC,
		Id:     a.doc.DocID,
		Assert: notDeadDoc,
		Update: bson.D{{"$set", bson.D{{"tools", versionedTool}}}},
	}}
	if err := a.st.db().RunTransaction(ops); err != nil {
		return onAbort(err, ErrDead)
	}
	a.doc.Tools = versionedTool
	return nil

}

var errRefresh = stderrors.New("state seems inconsistent, refresh and try again")

// Destroy ensures that the application and all its relations will be removed at
// some point; if the application has no units, and no relation involving the
// application has any units in scope, they are all removed immediately.
func (a *Application) Destroy() (err error) {
	defer func() {
		logger.Tracef("Application(%s).Destroy() => %v", a.doc.Name, err)
		if err == nil {
			// This is a white lie; the document might actually be removed.
			a.doc.Life = Dying
		}
	}()
	op := a.DestroyOperation()
	err = a.st.ApplyOperation(op)
	if len(op.Errors) != 0 {
		logger.Warningf("operational errors destroying application %v: %v", a.Name(), op.Errors)
	}
	return err
}

// DestroyOperation returns a model operation that will destroy the application.
func (a *Application) DestroyOperation() *DestroyApplicationOperation {
	return &DestroyApplicationOperation{
		app: &Application{st: a.st, doc: a.doc},
	}
}

// DestroyApplicationOperation is a model operation for destroying an
// application.
type DestroyApplicationOperation struct {
	// app holds the application to destroy.
	app *Application

	// DestroyStorage controls whether or not storage attached
	// to units of the application are destroyed. If this is false,
	// then detachable storage will be detached and left in the model.
	DestroyStorage bool

	// RemoveOffers controls whether or not application offers
	// are removed. If this is false, then the operation will
	// fail if there are any offers remaining.
	RemoveOffers bool

	// ForcedOperation stores needed information to force this operation.
	ForcedOperation
}

// Build is part of the ModelOperation interface.
func (op *DestroyApplicationOperation) Build(attempt int) ([]txn.Op, error) {
	if attempt > 0 {
		if err := op.app.Refresh(); errors.IsNotFound(err) {
			return nil, jujutxn.ErrNoOperations
		} else if err != nil {
			return nil, err
		}
	}
	// This call returns needed operations to destroy an application.
	// All operational errors are added to 'op' struct
	// and may be of interest to the user. Without 'force', these errors are considered fatal.
	// If 'force' is specified, they are treated as non-fatal - they will not prevent further
	// processing: we'll still try to remove application.
	ops, err := op.destroyOps()
	switch errors.Cause(err) {
	case errRefresh:
		return nil, jujutxn.ErrTransientFailure
	case errAlreadyDying:
		return nil, jujutxn.ErrNoOperations
	case nil:
		if len(op.Errors) == 0 {
			return ops, nil
		}
		if op.Force {
			logger.Debugf("forcing application removal")
			return ops, nil
		}
		// Should be impossible to reach as--by convention--we return an error and
		// an empty ops slice when a force-able error occurs and we're running !op.Force
		err = errors.Errorf("errors encountered: %q", op.Errors)
	}
	return nil, err
}

// Done is part of the ModelOperation interface.
func (op *DestroyApplicationOperation) Done(err error) error {
	if err == nil {
		return err
	}
	connected, err2 := applicationHasConnectedOffers(op.app.st, op.app.Name())
	if err2 != nil {
		err = errors.Trace(err2)
	} else if connected {
		rels, err2 := op.app.st.AllRelations()
		if err2 != nil {
			err = errors.Trace(err2)
		} else {
			err = errors.Errorf("application is used by %d offer%s", len(rels), plural(len(rels)))
		}
	} else {
		err = errors.NewNotSupported(err, "change to the application detected")
	}

	return errors.Annotatef(err, "cannot destroy application %q", op.app)
}

// destroyOps returns the operations required to destroy the application. If it
// returns errRefresh, the application should be refreshed and the destruction
// operations recalculated.
//
// When this operation has 'force' set, all operational errors are considered non-fatal
// and are accumulated on the operation.
// This method will return all operations we can construct despite errors.
//
// When the 'force' is not set, any operational errors will be considered fatal. All operations
// constructed up until the error will be discarded and the error will be returned.
func (op *DestroyApplicationOperation) destroyOps() ([]txn.Op, error) {
	rels, err := op.app.Relations()
	if op.FatalError(err) {
		return nil, err
	}
	if len(rels) != op.app.doc.RelationCount {
		// This is just an early bail out. The relations obtained may still
		// be wrong, but that situation will be caught by a combination of
		// asserts on relationcount and on each known relation, below.
		logger.Tracef("DestroyApplicationOperation(%s).destroyOps mismatched relation count %d != %d",
			op.app.doc.Name, len(rels), op.app.doc.RelationCount)
		return nil, errRefresh
	}
	ops := []txn.Op{minUnitsRemoveOp(op.app.st, op.app.doc.Name)}
	removeCount := 0
	failedRels := false
	for _, rel := range rels {
		// When forced, this call will return both operations to remove this
		// relation as well as all operational errors encountered.
		// If the 'force' is not set and the call came across some errors,
		// these errors will be fatal and no operations will be returned.
		relOps, isRemove, err := rel.destroyOps(op.app.doc.Name, &op.ForcedOperation)
		if errors.Cause(err) == errAlreadyDying {
			relOps = []txn.Op{{
				C:      relationsC,
				Id:     rel.doc.DocID,
				Assert: bson.D{{"life", Dying}},
			}}
		} else if err != nil {
			op.AddError(err)
			failedRels = true
			continue
		}
		if isRemove {
			removeCount++
		}
		ops = append(ops, relOps...)
	}
	if !op.Force && failedRels {
		return nil, op.LastError()
	}
	resOps, err := removeResourcesOps(op.app.st, op.app.doc.Name)
	if op.FatalError(err) {
		return nil, errors.Trace(err)
	}
	ops = append(ops, resOps...)

	// We can't delete an application if it is being offered,
	// unless those offers have no relations.
	if !op.RemoveOffers {
		countOp, n, err := countApplicationOffersRefOp(op.app.st, op.app.Name())
		if err != nil {
			return nil, errors.Trace(err)
		}
		if n == 0 {
			ops = append(ops, countOp)
		} else {
			connected, err := applicationHasConnectedOffers(op.app.st, op.app.Name())
			if err != nil {
				return nil, errors.Trace(err)
			}
			if connected {
				return nil, errors.Errorf("application is used by %d offer%s", n, plural(n))
			}
			// None of our offers are connected,
			// it's safe to remove them.
			removeOfferOps, err := removeApplicationOffersOps(op.app.st, op.app.Name())
			if err != nil {
				return nil, errors.Trace(err)
			}
			ops = append(ops, removeOfferOps...)
			ops = append(ops, txn.Op{
				C:  applicationsC,
				Id: op.app.doc.DocID,
				Assert: bson.D{
					// We're using the txn-revno here because relationcount is too
					// coarse-grained for what we need. Using the revno will
					// create false positives during concurrent updates of the
					// model, but eliminates the possibility of it entering
					// an inconsistent state.
					{"txn-revno", op.app.doc.TxnRevno},
				},
			})
		}
	}

	branchOps, err := op.unassignBranchOps()
	if err != nil {
		if !op.Force {
			return nil, errors.Trace(err)
		}
		op.AddError(err)
	}
	ops = append(ops, branchOps...)

	// If the application has no units, and all its known relations will be
	// removed, the application can also be removed.
	if op.app.doc.UnitCount == 0 && op.app.doc.RelationCount == removeCount {
		logger.Tracef("DestroyApplicationOperation(%s).destroyOps removing application", op.app.doc.Name)
		// If we're forcing destruction the assertion shouldn't be that
		// life is alive, but that it's what we think it is now.
		assertion := bson.D{
			{"life", op.app.doc.Life},
			{"unitcount", 0},
			{"relationcount", removeCount},
		}
		// When forced, this call will return operations to remove this
		// application and accumulate all operational errors encountered in the operation.
		// If the 'force' is not set and the call came across some errors,
		// these errors will be fatal and no operations will be returned.
		removeOps, err := op.app.removeOps(assertion, &op.ForcedOperation)
		if err != nil {
			if !op.Force || errors.Cause(err) == errRefresh {
				return nil, errors.Trace(err)
			}
			op.AddError(err)
			return ops, nil
		}
		return append(ops, removeOps...), nil
	}
	// In all other cases, application removal will be handled as a consequence
	// of the removal of the last unit or relation referencing it. If any
	// relations have been removed, they'll be caught by the operations
	// collected above; but if any has been added, we need to abort and add
	// a destroy op for that relation too. In combination, it's enough to
	// check for count equality: an add/remove will not touch the count, but
	// will be caught by virtue of being a remove.
	notLastRefs := bson.D{
		{"life", op.app.doc.Life},
		{"relationcount", op.app.doc.RelationCount},
	}
	// With respect to unit count, a changing value doesn't matter, so long
	// as the count's equality with zero does not change, because all we care
	// about is that *some* unit is, or is not, keeping the application from
	// being removed: the difference between 1 unit and 1000 is irrelevant.
	if op.app.doc.UnitCount > 0 {
		logger.Tracef("DestroyApplicationOperation(%s).destroyOps UnitCount == %d, queuing up unitCleanup",
			op.app.doc.Name, op.app.doc.UnitCount)
		cleanupOp := newCleanupOp(
			cleanupUnitsForDyingApplication,
			op.app.doc.Name,
			op.DestroyStorage,
			op.Force,
			op.MaxWait,
		)
		ops = append(ops, cleanupOp)
		notLastRefs = append(notLastRefs, bson.D{{"unitcount", bson.D{{"$gt", 0}}}}...)
	} else {
		notLastRefs = append(notLastRefs, bson.D{{"unitcount", 0}}...)
	}
	update := bson.D{{"$set", bson.D{{"life", Dying}}}}
	if removeCount != 0 {
		decref := bson.D{{"$inc", bson.D{{"relationcount", -removeCount}}}}
		update = append(update, decref...)
	}
	ops = append(ops, txn.Op{
		C:      applicationsC,
		Id:     op.app.doc.DocID,
		Assert: notLastRefs,
		Update: update,
	})
	return ops, nil
}

func (op *DestroyApplicationOperation) unassignBranchOps() ([]txn.Op, error) {
	m, err := op.app.st.Model()
	if err != nil {
		return nil, errors.Trace(err)
	}
	appName := op.app.doc.Name
	branches, err := m.applicationBranches(appName)
	if err != nil {
		return nil, errors.Trace(err)
	}
	if len(branches) == 0 {
		return nil, nil
	}
	ops := []txn.Op{}
	for _, b := range branches {
		// assumption: branches from applicationBranches will
		// ALWAYS have the appName in assigned-units, but not
		// always in config.
		ops = append(ops, b.unassignAppOps(appName)...)
	}
	return ops, nil
}

func removeResourcesOps(st *State, applicationID string) ([]txn.Op, error) {
	persist, err := st.ResourcesPersistence()
	if errors.IsNotSupported(err) {
		// Nothing to see here, move along.
		return nil, nil
	}
	if err != nil {
		return nil, errors.Trace(err)
	}
	ops, err := persist.NewRemoveResourcesOps(applicationID)
	if err != nil {
		return nil, errors.Trace(err)
	}
	return ops, nil
}

// removeOps returns the operations required to remove the application. Supplied
// asserts will be included in the operation on the application document.
// When force is set, the operation will proceed regardless of the errors,
// and if any errors are encountered, all possible accumulated operations
// as well as all encountered errors will be returned.
// When 'force' is set, this call will return operations to remove this
// application and will accumulate all operational errors encountered in the operation.
// If the 'force' is not set, any error will be fatal and no operations will be returned.
func (a *Application) removeOps(asserts bson.D, op *ForcedOperation) ([]txn.Op, error) {
	ops := []txn.Op{{
		C:      applicationsC,
		Id:     a.doc.DocID,
		Assert: asserts,
		Remove: true,
	}}

	// Remove application offers.
	removeOfferOps, err := removeApplicationOffersOps(a.st, a.doc.Name)
	if op.FatalError(err) {
		return nil, errors.Trace(err)
	}
	ops = append(ops, removeOfferOps...)

	// Note that appCharmDecRefOps might not catch the final decref
	// when run in a transaction that decrefs more than once. So we
	// avoid attempting to do the final cleanup in the ref dec ops and
	// do it explicitly below.
	name := a.doc.Name
	curl := a.doc.CharmURL
	// When 'force' is set, this call will return operations to delete application references
	// to this charm as well as accumulate all operational errors encountered in the operation.
	// If the 'force' is not set, any error will be fatal and no operations will be returned.
	charmOps, err := appCharmDecRefOps(a.st, name, curl, false, op)
	if err != nil {
		if errors.Cause(err) == errRefcountAlreadyZero {
			// We have already removed the reference to the charm, this indicates
			// the application is already removed, reload yourself and try again
			return nil, errRefresh
		}
		if op.FatalError(err) {
			return nil, errors.Trace(err)
		}
	}
	ops = append(ops, charmOps...)

	// By the time we get to here, all units and charm refs have been removed,
	// so it's safe to do this additional cleanup.
	ops = append(ops, finalAppCharmRemoveOps(name, curl)...)

	ops = append(ops, a.removeCloudServiceOps()...)
	globalKey := a.globalKey()
	ops = append(ops,
		removeEndpointBindingsOp(globalKey),
		removeConstraintsOp(globalKey),
		annotationRemoveOp(a.st, globalKey),
		removeLeadershipSettingsOp(name),
		removeStatusOp(a.st, globalKey),
		removeStatusOp(a.st, applicationGlobalOperatorKey(name)),
		removeSettingsOp(settingsC, a.applicationConfigKey()),
		removeModelApplicationRefOp(a.st, name),
		removePodSpecOp(a.ApplicationTag()),
	)
	return ops, nil
}

// IsExposed returns whether this application is exposed. The explicitly open
// ports (with open-port) for exposed applications may be accessed from machines
// outside of the local deployment network. See SetExposed and ClearExposed.
func (a *Application) IsExposed() bool {
	return a.doc.Exposed
}

// SetExposed marks the application as exposed.
// See ClearExposed and IsExposed.
func (a *Application) SetExposed() error {
	return a.setExposed(true)
}

// ClearExposed removes the exposed flag from the application.
// See SetExposed and IsExposed.
func (a *Application) ClearExposed() error {
	return a.setExposed(false)
}

func (a *Application) setExposed(exposed bool) (err error) {
	ops := []txn.Op{{
		C:      applicationsC,
		Id:     a.doc.DocID,
		Assert: isAliveDoc,
		Update: bson.D{{"$set", bson.D{{"exposed", exposed}}}},
	}}
	if err := a.st.db().RunTransaction(ops); err != nil {
		return errors.Errorf("cannot set exposed flag for application %q to %v: %v", a, exposed, onAbort(err, applicationNotAliveErr))
	}
	a.doc.Exposed = exposed
	return nil
}

// Charm returns the application's charm and whether units should upgrade to that
// charm even if they are in an error state.
func (a *Application) Charm() (ch *Charm, force bool, err error) {
	// We don't worry about the channel since we aren't interacting
	// with the charm store here.
	ch, err = a.st.Charm(a.doc.CharmURL)
	if err != nil {
		return nil, false, err
	}
	return ch, a.doc.ForceCharm, nil
}

// IsPrincipal returns whether units of the application can
// have subordinate units.
func (a *Application) IsPrincipal() bool {
	return !a.doc.Subordinate
}

// CharmModifiedVersion increases whenever the application's charm is changed in any
// way.
func (a *Application) CharmModifiedVersion() int {
	return a.doc.CharmModifiedVersion
}

// CharmURL returns the application's charm URL, and whether units should upgrade
// to the charm with that URL even if they are in an error state.
func (a *Application) CharmURL() (curl *charm.URL, force bool) {
	return a.doc.CharmURL, a.doc.ForceCharm
}

// Channel identifies the charm store channel from which the application's
// charm was deployed. It is only needed when interacting with the charm
// store.
func (a *Application) Channel() csparams.Channel {
	return csparams.Channel(a.doc.Channel)
}

// Endpoints returns the application's currently available relation endpoints.
func (a *Application) Endpoints() (eps []Endpoint, err error) {
	ch, _, err := a.Charm()
	if err != nil {
		return nil, err
	}
	collect := func(role charm.RelationRole, rels map[string]charm.Relation) {
		for _, rel := range rels {
			eps = append(eps, Endpoint{
				ApplicationName: a.doc.Name,
				Relation:        rel,
			})
		}
	}
	meta := ch.Meta()
	collect(charm.RolePeer, meta.Peers)
	collect(charm.RoleProvider, meta.Provides)
	collect(charm.RoleRequirer, meta.Requires)
	collect(charm.RoleProvider, map[string]charm.Relation{
		"juju-info": {
			Name:      "juju-info",
			Role:      charm.RoleProvider,
			Interface: "juju-info",
			Scope:     charm.ScopeGlobal,
		},
	})
	sort.Sort(epSlice(eps))
	return eps, nil
}

// Endpoint returns the relation endpoint with the supplied name, if it exists.
func (a *Application) Endpoint(relationName string) (Endpoint, error) {
	eps, err := a.Endpoints()
	if err != nil {
		return Endpoint{}, err
	}
	for _, ep := range eps {
		if ep.Name == relationName {
			return ep, nil
		}
	}
	return Endpoint{}, errors.Errorf("application %q has no %q relation", a, relationName)
}

// extraPeerRelations returns only the peer relations in newMeta not
// present in the application's current charm meta data.
func (a *Application) extraPeerRelations(newMeta *charm.Meta) map[string]charm.Relation {
	if newMeta == nil {
		// This should never happen, since we're checking the charm in SetCharm already.
		panic("newMeta is nil")
	}
	ch, _, err := a.Charm()
	if err != nil {
		return nil
	}
	newPeers := newMeta.Peers
	oldPeers := ch.Meta().Peers
	extraPeers := make(map[string]charm.Relation)
	for relName, rel := range newPeers {
		if _, ok := oldPeers[relName]; !ok {
			extraPeers[relName] = rel
		}
	}
	return extraPeers
}

func (a *Application) checkRelationsOps(ch *Charm, relations []*Relation) ([]txn.Op, error) {
	asserts := make([]txn.Op, 0, len(relations))

	isPeerToItself := func(ep Endpoint) bool {
		// We do not want to prevent charm upgrade when endpoint relation is
		// peer-scoped and there is only one unit of this application.
		// Essentially, this is the corner case when a unit relates to itself.
		// For example, in this case, we want to allow charm upgrade, for e.g.
		// interface name change does not affect anything.
		units, err := a.AllUnits()
		if err != nil {
			// Whether we could get application units does not matter.
			// We are only interested in thinking further if we can get units.
			return false
		}
		return len(units) == 1 && isPeer(ep)
	}

	// All relations must still exist and their endpoints are implemented by the charm.
	for _, rel := range relations {
		if ep, err := rel.Endpoint(a.doc.Name); err != nil {
			return nil, err
		} else if !ep.ImplementedBy(ch) {
			if !isPeerToItself(ep) {
				return nil, errors.Errorf("would break relation %q", rel)
			}
		}
		asserts = append(asserts, txn.Op{
			C:      relationsC,
			Id:     rel.doc.DocID,
			Assert: txn.DocExists,
		})
	}
	return asserts, nil
}

func (a *Application) checkStorageUpgrade(newMeta, oldMeta *charm.Meta, units []*Unit) (_ []txn.Op, err error) {
	// Make sure no storage instances are added or removed.

	sb, err := NewStorageBackend(a.st)
	if err != nil {
		return nil, errors.Trace(err)
	}

	var ops []txn.Op
	for name, oldStorageMeta := range oldMeta.Storage {
		if _, ok := newMeta.Storage[name]; ok {
			continue
		}
		if oldStorageMeta.CountMin > 0 {
			return nil, errors.Errorf("required storage %q removed", name)
		}
		// Optional storage has been removed. So long as there
		// are no instances of the store, it can safely be
		// removed.
		if oldStorageMeta.Shared {
			op, n, err := sb.countEntityStorageInstances(a.Tag(), name)
			if err != nil {
				return nil, errors.Trace(err)
			}
			if n > 0 {
				return nil, errors.Errorf("in-use storage %q removed", name)
			}
			ops = append(ops, op)
		} else {
			for _, u := range units {
				op, n, err := sb.countEntityStorageInstances(u.Tag(), name)
				if err != nil {
					return nil, errors.Trace(err)
				}
				if n > 0 {
					return nil, errors.Errorf("in-use storage %q removed", name)
				}
				ops = append(ops, op)
			}
		}
	}
	less := func(a, b int) bool {
		return a != -1 && (b == -1 || a < b)
	}
	for name, newStorageMeta := range newMeta.Storage {
		oldStorageMeta, ok := oldMeta.Storage[name]
		if !ok {
			continue
		}
		if newStorageMeta.Type != oldStorageMeta.Type {
			return nil, errors.Errorf(
				"existing storage %q type changed from %q to %q",
				name, oldStorageMeta.Type, newStorageMeta.Type,
			)
		}
		if newStorageMeta.Shared != oldStorageMeta.Shared {
			return nil, errors.Errorf(
				"existing storage %q shared changed from %v to %v",
				name, oldStorageMeta.Shared, newStorageMeta.Shared,
			)
		}
		if newStorageMeta.ReadOnly != oldStorageMeta.ReadOnly {
			return nil, errors.Errorf(
				"existing storage %q read-only changed from %v to %v",
				name, oldStorageMeta.ReadOnly, newStorageMeta.ReadOnly,
			)
		}
		if newStorageMeta.Location != oldStorageMeta.Location {
			return nil, errors.Errorf(
				"existing storage %q location changed from %q to %q",
				name, oldStorageMeta.Location, newStorageMeta.Location,
			)
		}
		if less(newStorageMeta.CountMax, oldStorageMeta.CountMax) {
			var oldCountMax interface{} = oldStorageMeta.CountMax
			if oldStorageMeta.CountMax == -1 {
				oldCountMax = "<unbounded>"
			}
			return nil, errors.Errorf(
				"existing storage %q range contracted: max decreased from %v to %d",
				name, oldCountMax, newStorageMeta.CountMax,
			)
		}
		if oldStorageMeta.Location != "" && oldStorageMeta.CountMax == 1 && newStorageMeta.CountMax != 1 {
			// If a location is specified, the store may not go
			// from being a singleton to multiple, since then the
			// location has a different meaning.
			return nil, errors.Errorf(
				"existing storage %q with location changed from single to multiple",
				name,
			)
		}
	}
	return ops, nil
}

// changeCharmOps returns the operations necessary to set a application's
// charm URL to a new value.
func (a *Application) changeCharmOps(
	ch *Charm,
	channel string,
	updatedSettings charm.Settings,
	forceUnits bool,
	resourceIDs map[string]string,
	updatedStorageConstraints map[string]StorageConstraints,
) ([]txn.Op, error) {
	// Build the new application config from what can be used of the old one.
	var newSettings charm.Settings
	oldKey, err := readSettings(a.st.db(), settingsC, a.charmConfigKey())
	if err == nil {
		// Filter the old settings through to get the new settings.
		newSettings = ch.Config().FilterSettings(oldKey.Map())
		for k, v := range updatedSettings {
			newSettings[k] = v
		}
	} else if errors.IsNotFound(err) {
		// No old settings, start with the updated settings.
		newSettings = updatedSettings
	} else {
		return nil, errors.Annotatef(err, "application %q", a.doc.Name)
	}

	// Create or replace application settings.
	var settingsOp txn.Op
	newSettingsKey := applicationCharmConfigKey(a.doc.Name, ch.URL())
	if _, err := readSettings(a.st.db(), settingsC, newSettingsKey); errors.IsNotFound(err) {
		// No settings for this key yet, create it.
		settingsOp = createSettingsOp(settingsC, newSettingsKey, newSettings)
	} else if err != nil {
		return nil, errors.Annotatef(err, "application %q", a.doc.Name)
	} else {
		// Settings exist, just replace them with the new ones.
		settingsOp, _, err = replaceSettingsOp(a.st.db(), settingsC, newSettingsKey, newSettings)
		if err != nil {
			return nil, errors.Annotatef(err, "application %q", a.doc.Name)
		}
	}

	// Make sure no units are added or removed while the upgrade
	// transaction is being executed. This allows us to make
	// changes to units during the upgrade, e.g. add storage
	// to existing units, or remove optional storage so long as
	// it is unreferenced.
	units, err := a.AllUnits()
	if err != nil {
		return nil, errors.Trace(err)
	}
	unitOps := make([]txn.Op, len(units))
	for i, u := range units {
		unitOps[i] = txn.Op{
			C:      unitsC,
			Id:     u.doc.DocID,
			Assert: txn.DocExists,
		}
	}
	unitOps = append(unitOps, txn.Op{
		C:      applicationsC,
		Id:     a.doc.DocID,
		Assert: bson.D{{"unitcount", len(units)}},
	})

	checkStorageOps, upgradeStorageOps, storageConstraintsOps, err := a.newCharmStorageOps(ch, units, updatedStorageConstraints)
	if err != nil {
		return nil, errors.Trace(err)
	}

	// Add or create a reference to the new charm, settings,
	// and storage constraints docs.
	incOps, err := appCharmIncRefOps(a.st, a.doc.Name, ch.URL(), true)
	if err != nil {
		return nil, errors.Trace(err)
	}
	var decOps []txn.Op
	// Drop the references to the old settings, storage constraints,
	// and charm docs (if the refs actually exist yet).
	if oldKey != nil {
		// Since we can force this now, let's.. There is no point hanging on
		// to the old key.
		op := &ForcedOperation{Force: true}
		decOps, err = appCharmDecRefOps(a.st, a.doc.Name, a.doc.CharmURL, true, op) // current charm
		if err != nil {
			return nil, errors.Annotatef(err, "could not remove old charm references for %v", oldKey)
		}
		if len(op.Errors) != 0 {
			logger.Errorf("could not remove old charm references for %v:%v", oldKey, op.Errors)
		}
	}

	// Build the transaction.
	var ops []txn.Op
	if oldKey != nil {
		// Old settings shouldn't change (when they exist).
		ops = append(ops, oldKey.assertUnchangedOp())
	}
	ops = append(ops, unitOps...)
	ops = append(ops, incOps...)
	ops = append(ops, []txn.Op{
		// Create or replace new settings.
		settingsOp,
		// Update the charm URL and force flag (if relevant).
		{
			C:  applicationsC,
			Id: a.doc.DocID,
			Update: bson.D{{"$set", bson.D{
				{"charmurl", ch.URL()},
				{"cs-channel", channel},
				{"forcecharm", forceUnits},
			}}},
		},
	}...)
	ops = append(ops, storageConstraintsOps...)
	ops = append(ops, checkStorageOps...)
	ops = append(ops, upgradeStorageOps...)

	ops = append(ops, incCharmModifiedVersionOps(a.doc.DocID)...)

	// Add any extra peer relations that need creation.
	newPeers := a.extraPeerRelations(ch.Meta())
	peerOps, err := a.st.addPeerRelationsOps(a.doc.Name, newPeers)
	if err != nil {
		return nil, errors.Trace(err)
	}

	if len(resourceIDs) > 0 {
		// Collect pending resource resolution operations.
		resOps, err := a.resolveResourceOps(resourceIDs)
		if err != nil {
			return nil, errors.Trace(err)
		}
		ops = append(ops, resOps...)
	}

	// Get all relations - we need to check them later.
	relations, err := a.Relations()
	if err != nil {
		return nil, errors.Trace(err)
	}
	// Make sure the relation count does not change.
	sameRelCount := bson.D{{"relationcount", len(relations)}}

	ops = append(ops, peerOps...)
	// Update the relation count as well.
	ops = append(ops, txn.Op{
		C:      applicationsC,
		Id:     a.doc.DocID,
		Assert: append(notDeadDoc, sameRelCount...),
		Update: bson.D{{"$inc", bson.D{{"relationcount", len(newPeers)}}}},
	})
	// Check relations to ensure no active relations are removed.
	relOps, err := a.checkRelationsOps(ch, relations)
	if err != nil {
		return nil, errors.Trace(err)
	}
	ops = append(ops, relOps...)

	// And finally, decrement the old charm and settings.
	return append(ops, decOps...), nil
}

// bindingsForOps returns a Bindings object intended for createOps and updateOps
// only.
func (a *Application) bindingsForOps(bindings map[string]string) (*Bindings, error) {
	// Call NewBindings first to ensure this map contains space ids
	b, err := NewBindings(a.st, bindings)
	if err != nil {
		return nil, err
	}
	b.app = a
	return b, nil
}

// Deployed machines returns the collection of machines
// that this application has units deployed to.
func (a *Application) DeployedMachines() ([]*Machine, error) {
	units, err := a.AllUnits()
	if err != nil {
		return nil, errors.Trace(err)
	}

	machineIds := set.NewStrings()
	var machines []*Machine
	for _, u := range units {
		// AssignedMachineId returns the correct machine
		// whether principal or subordinate.
		id, err := u.AssignedMachineId()
		if err != nil {
			if errors.IsNotAssigned(err) {
				// We aren't interested in this unit at this time.
				continue
			}
			return nil, errors.Trace(err)
		}
		if machineIds.Contains(id) {
			continue
		}

		m, err := a.st.Machine(id)
		if err != nil {
			return nil, errors.Trace(err)
		}
		machineIds.Add(id)
		machines = append(machines, m)
	}
	return machines, nil
}

func (a *Application) newCharmStorageOps(
	ch *Charm,
	units []*Unit,
	updatedStorageConstraints map[string]StorageConstraints,
) ([]txn.Op, []txn.Op, []txn.Op, error) {

	fail := func(err error) ([]txn.Op, []txn.Op, []txn.Op, error) {
		return nil, nil, nil, errors.Trace(err)
	}

	// Check storage to ensure no referenced storage is removed, or changed
	// in an incompatible way. We do this before computing the new storage
	// constraints, as incompatible charm changes will otherwise yield
	// confusing error messages that would suggest the user has supplied
	// invalid constraints.
	sb, err := NewStorageBackend(a.st)
	if err != nil {
		return fail(err)
	}
	oldCharm, _, err := a.Charm()
	if err != nil {
		return fail(err)
	}
	oldMeta := oldCharm.Meta()
	checkStorageOps, err := a.checkStorageUpgrade(ch.Meta(), oldMeta, units)
	if err != nil {
		return fail(err)
	}

	// Create or replace storage constraints. We take the existing storage
	// constraints, remove any keys that are no longer referenced by the
	// charm, and update the constraints that the user has specified.
	var storageConstraintsOp txn.Op
	oldStorageConstraints, err := a.StorageConstraints()
	if err != nil {
		return fail(err)
	}
	newStorageConstraints := oldStorageConstraints
	for name, cons := range updatedStorageConstraints {
		newStorageConstraints[name] = cons
	}
	for name := range newStorageConstraints {
		if _, ok := ch.Meta().Storage[name]; !ok {
			delete(newStorageConstraints, name)
		}
	}
	if err := addDefaultStorageConstraints(sb, newStorageConstraints, ch.Meta()); err != nil {
		return fail(errors.Annotate(err, "adding default storage constraints"))
	}
	if err := validateStorageConstraints(sb, newStorageConstraints, ch.Meta()); err != nil {
		return fail(errors.Annotate(err, "validating storage constraints"))
	}
	newStorageConstraintsKey := applicationStorageConstraintsKey(a.doc.Name, ch.URL())
	if _, err := readStorageConstraints(sb.mb, newStorageConstraintsKey); errors.IsNotFound(err) {
		storageConstraintsOp = createStorageConstraintsOp(
			newStorageConstraintsKey, newStorageConstraints,
		)
	} else if err != nil {
		return fail(err)
	} else {
		storageConstraintsOp = replaceStorageConstraintsOp(
			newStorageConstraintsKey, newStorageConstraints,
		)
	}

	// Upgrade charm storage.
	upgradeStorageOps, err := a.upgradeStorageOps(ch.Meta(), oldMeta, units, newStorageConstraints)
	if err != nil {
		return fail(err)
	}
	return checkStorageOps, upgradeStorageOps, []txn.Op{storageConstraintsOp}, nil
}

func (a *Application) upgradeStorageOps(
	meta, oldMeta *charm.Meta,
	units []*Unit,
	allStorageCons map[string]StorageConstraints,
) (_ []txn.Op, err error) {

	sb, err := NewStorageBackend(a.st)
	if err != nil {
		return nil, errors.Trace(err)
	}

	// For each store, ensure that every unit has the minimum requirements.
	// If a unit has an existing store, but its minimum count has been
	// increased, we only add the shortfall; we do not necessarily add as
	// many instances as are specified in the storage constraints.
	var ops []txn.Op
	for name, cons := range allStorageCons {
		for _, u := range units {
			countMin := meta.Storage[name].CountMin
			if _, ok := oldMeta.Storage[name]; !ok {
				// The store did not exist previously, so we
				// create the full amount specified in the
				// constraints.
				countMin = int(cons.Count)
			}
			_, unitOps, err := sb.addUnitStorageOps(
				meta, u, name, cons, countMin,
			)
			if err != nil {
				return nil, errors.Trace(err)
			}
			ops = append(ops, unitOps...)
		}
	}
	return ops, nil
}

// incCharmModifiedVersionOps returns the operations necessary to increment
// the CharmModifiedVersion field for the given application.
func incCharmModifiedVersionOps(applicationID string) []txn.Op {
	return []txn.Op{{
		C:      applicationsC,
		Id:     applicationID,
		Assert: txn.DocExists,
		Update: bson.D{{"$inc", bson.D{{"charmmodifiedversion", 1}}}},
	}}
}

func (a *Application) resolveResourceOps(resourceIDs map[string]string) ([]txn.Op, error) {
	// Collect pending resource resolution operations.
	resources, err := a.st.Resources()
	if err != nil {
		return nil, errors.Trace(err)
	}
	return resources.NewResolvePendingResourcesOps(a.doc.Name, resourceIDs)
}

// SetCharmConfig contains the parameters for Application.SetCharm.
type SetCharmConfig struct {
	// Charm is the new charm to use for the application. New units
	// will be started with this charm, and existing units will be
	// upgraded to use it.
	Charm *Charm

	// Channel is the charm store channel from which charm was pulled.
	Channel csparams.Channel

	// ConfigSettings is the charm config settings to apply when upgrading
	// the charm.
	ConfigSettings charm.Settings

	// ForceUnits forces the upgrade on units in an error state.
	ForceUnits bool

	// ForceSeries forces the use of the charm even if it is not one of
	// the charm's supported series.
	ForceSeries bool

	// Force forces the overriding of the lxd profile validation even if the
	// profile doesn't validate.
	Force bool

	// ResourceIDs is a map of resource names to resource IDs to activate during
	// the upgrade.
	ResourceIDs map[string]string

	// StorageConstraints contains the storage constraints to add or update when
	// upgrading the charm.
	//
	// Any existing storage instances for the named stores will be
	// unaffected; the storage constraints will only be used for
	// provisioning new storage instances.
	StorageConstraints map[string]StorageConstraints

	// EndpointBindings is an operator-defined map of endpoint names to
	// space names that should be merged with any existing bindings.
	EndpointBindings map[string]string
}

// SetCharm changes the charm for the application.
func (a *Application) SetCharm(cfg SetCharmConfig) (err error) {
	defer errors.DeferredAnnotatef(
		&err, "cannot upgrade application %q to charm %q", a, cfg.Charm,
	)
	if cfg.Charm.Meta().Subordinate != a.doc.Subordinate {
		return errors.Errorf("cannot change an application's subordinacy")
	}
	currentCharm, err := a.st.Charm(a.doc.CharmURL)
	if err != nil {
		return errors.Trace(err)
	}
	if cfg.Charm.Meta().Deployment != currentCharm.Meta().Deployment {
		if currentCharm.Meta().Deployment == nil || currentCharm.Meta().Deployment == nil {
			return errors.New("cannot change a charm's deployment info")
		}
		if cfg.Charm.Meta().Deployment.DeploymentType != currentCharm.Meta().Deployment.DeploymentType {
			return errors.New("cannot change a charm's deployment type")
		}
		if cfg.Charm.Meta().Deployment.DeploymentMode != currentCharm.Meta().Deployment.DeploymentMode {
			return errors.New("cannot change a charm's deployment mode")
		}
	}
	// For old style charms written for only one series, we still retain
	// this check. Newer charms written for multi-series have a URL
	// with series = "".
	if cfg.Charm.URL().Series != "" {
		if cfg.Charm.URL().Series != a.doc.Series {
			return errors.Errorf("cannot change an application's series")
		}
	} else if !cfg.ForceSeries {
		supported := false
		for _, oneSeries := range cfg.Charm.Meta().Series {
			if oneSeries == a.doc.Series {
				supported = true
				break
			}
		}
		if !supported {
			supportedSeries := "no series"
			if len(cfg.Charm.Meta().Series) > 0 {
				supportedSeries = strings.Join(cfg.Charm.Meta().Series, ", ")
			}
			return errors.Errorf("only these series are supported: %v", supportedSeries)
		}
	} else {
		// Even with forceSeries=true, we do not allow a charm to be used which is for
		// a different OS. This assumes the charm declares it has supported series which
		// we can check for OS compatibility. Otherwise, we just accept the series supplied.
		currentOS, err := series.GetOSFromSeries(a.doc.Series)
		if err != nil {
			// We don't expect an error here but there's not much we can
			// do to recover.
			return err
		}
		supportedOS := false
		supportedSeries := cfg.Charm.Meta().Series
		for _, chSeries := range supportedSeries {
			charmSeriesOS, err := series.GetOSFromSeries(chSeries)
			if err != nil {
				return nil
			}
			if currentOS == charmSeriesOS {
				supportedOS = true
				break
			}
		}
		if !supportedOS && len(supportedSeries) > 0 {
			return errors.Errorf("OS %q not supported by charm", currentOS)
		}
	}

	updatedSettings, err := cfg.Charm.Config().ValidateSettings(cfg.ConfigSettings)
	if err != nil {
		return errors.Annotate(err, "validating config settings")
	}

	// we don't need to check that this is a charm.LXDProfiler, as we can
	// state that the function exists.
	if profile := cfg.Charm.LXDProfile(); profile != nil {
		// Validate the config devices, to ensure we don't apply an invalid
		// profile, if we know it's never going to work.
		if err := profile.ValidateConfigDevices(); err != nil && !cfg.Force {
			return errors.Annotate(err, "validating lxd profile")
		}
	}

	var newCharmModifiedVersion int
	channel := string(cfg.Channel)
	acopy := &Application{a.st, a.doc}
	buildTxn := func(attempt int) ([]txn.Op, error) {
		a := acopy
		if attempt > 0 {
			if err := a.Refresh(); err != nil {
				return nil, errors.Trace(err)
			}
		}

		// NOTE: We're explicitly allowing SetCharm to succeed
		// when the application is Dying, because application/charm
		// upgrades should still be allowed to apply to dying
		// applications and units, so that bugs in departed/broken
		// hooks can be addressed at runtime.
		if a.Life() == Dead {
			return nil, ErrDead
		}

		// Record the current value of charmModifiedVersion, so we can
		// set the value on the method receiver's in-memory document
		// structure. We increment the version only when we change the
		// charm URL.
		newCharmModifiedVersion = a.doc.CharmModifiedVersion

		ops := []txn.Op{{
			C:  applicationsC,
			Id: a.doc.DocID,
			Assert: append(notDeadDoc, bson.DocElem{
				"charmmodifiedversion", a.doc.CharmModifiedVersion,
			}),
		}}

		if a.doc.CharmURL.String() == cfg.Charm.URL().String() {
			// Charm URL already set; just update the force flag and channel.
			ops = append(ops, txn.Op{
				C:  applicationsC,
				Id: a.doc.DocID,
				Update: bson.D{{"$set", bson.D{
					{"cs-channel", channel},
					{"forcecharm", cfg.ForceUnits},
				}}},
			})
		} else {
			chng, err := a.changeCharmOps(
				cfg.Charm,
				channel,
				updatedSettings,
				cfg.ForceUnits,
				cfg.ResourceIDs,
				cfg.StorageConstraints,
			)
			if err != nil {
				return nil, errors.Trace(err)
			}
			ops = append(ops, chng...)
			newCharmModifiedVersion++
		}

		// Always update bindings regardless of whether we upgrade to a
		// new version or stay at the previous version.
		currentMap, txnRevno, err := readEndpointBindings(a.st, a.globalKey())
		if err != nil && !errors.IsNotFound(err) {
			return ops, errors.Trace(err)
		}
		b, err := a.bindingsForOps(currentMap)
		if err != nil {
			return nil, errors.Trace(err)
		}
		endpointBindingsOps, err := b.updateOps(txnRevno, cfg.EndpointBindings, cfg.Charm.Meta(), cfg.Force)
		if err == nil {
			ops = append(ops, endpointBindingsOps...)
		} else if !errors.IsNotFound(err) && err != jujutxn.ErrNoOperations {
			// If endpoint bindings do not exist this most likely means the application
			// itself no longer exists, which will be caught soon enough anyway.
			// ErrNoOperations on the other hand means there's nothing to update.
			return nil, errors.Trace(err)
		}

		return ops, nil
	}

	if err := a.st.db().Run(buildTxn); err != nil {
		return err
	}
	a.doc.CharmURL = cfg.Charm.URL()
	a.doc.Channel = channel
	a.doc.ForceCharm = cfg.ForceUnits
	a.doc.CharmModifiedVersion = newCharmModifiedVersion
	return nil
}

// MergeBindings merges the provided bindings map with the existing application
// bindings.
func (a *Application) MergeBindings(operatorBindings *Bindings, force bool) error {
	buildTxn := func(attempt int) ([]txn.Op, error) {
		if attempt > 0 {
			if err := a.Refresh(); err != nil {
				return nil, errors.Trace(err)
			}
		}

		ch, _, err := a.Charm()
		if err != nil {
			return nil, errors.Trace(err)
		}

		currentMap, txnRevno, err := readEndpointBindings(a.st, a.globalKey())
		if err != nil && !errors.IsNotFound(err) {
			return nil, errors.Trace(err)
		}
		b, err := a.bindingsForOps(currentMap)
		if err != nil {
			return nil, errors.Trace(err)
		}
		endpointBindingsOps, err := b.updateOps(txnRevno, operatorBindings.Map(), ch.Meta(), force)
		if err != nil && !errors.IsNotFound(err) && err != jujutxn.ErrNoOperations {
			return nil, errors.Trace(err)
		}

		return endpointBindingsOps, err
	}

	err := a.st.db().Run(buildTxn)
	return errors.Annotatef(err, "merging application bindings")
}

// UpdateApplicationSeries updates the series for the Application.
func (a *Application) UpdateApplicationSeries(series string, force bool) (err error) {
	buildTxn := func(attempt int) ([]txn.Op, error) {
		if attempt > 0 {
			// If we've tried once already and failed, re-evaluate the criteria.
			if err := a.Refresh(); err != nil {
				return nil, errors.Trace(err)
			}
		}
		// Exit early if the Application series doesn't need to change.
		if a.Series() == series {
			return nil, jujutxn.ErrNoOperations
		}

		// Verify and gather data for the transaction operations.
		err := a.VerifySupportedSeries(series, force)
		if err != nil {
			return nil, err
		}
		units, err := a.AllUnits()
		if err != nil {
			return nil, errors.Trace(err)
		}
		var subApps []*Application
		var unit *Unit

		if len(units) > 0 {
			// All units have the same subordinates...
			unit = units[0]
			for _, n := range unit.SubordinateNames() {
				app, err := a.st.Application(unitAppName(n))
				if err != nil {
					return nil, err
				}
				err = app.VerifySupportedSeries(series, force)
				if err != nil {
					return nil, err
				}
				subApps = append(subApps, app)
			}
		}

		//Create the transaction operations
		ops := []txn.Op{{
			C:  applicationsC,
			Id: a.doc.DocID,
			Assert: bson.D{{"life", Alive},
				{"charmurl", a.doc.CharmURL},
				{"unitcount", a.doc.UnitCount}},
			Update: bson.D{{"$set", bson.D{{"series", series}}}},
		}}

		if unit != nil {
			ops = append(ops, txn.Op{
				C:  unitsC,
				Id: unit.doc.DocID,
				Assert: bson.D{{"life", Alive},
					{"subordinates", unit.SubordinateNames()}},
			})
		}

		for _, sub := range subApps {
			ops = append(ops, txn.Op{
				C:  applicationsC,
				Id: sub.doc.DocID,
				Assert: bson.D{{"life", Alive},
					{"charmurl", sub.doc.CharmURL},
					{"unitcount", sub.doc.UnitCount}},
				Update: bson.D{{"$set", bson.D{{"series", series}}}},
			})
		}
		return ops, nil
	}

	err = a.st.db().Run(buildTxn)
	return errors.Annotatef(err, "updating application series")
}

// VerifySupportedSeries verifies if the given series is supported by the
// application.
func (a *Application) VerifySupportedSeries(series string, force bool) error {
	ch, _, err := a.Charm()
	if err != nil {
		return err
	}
	supportedSeries := ch.Meta().Series
	if len(supportedSeries) == 0 {
		supportedSeries = append(supportedSeries, ch.URL().Series)
	}
	_, seriesSupportedErr := charm.SeriesForCharm(series, supportedSeries)
	if seriesSupportedErr != nil && !force {
		return &ErrIncompatibleSeries{
			SeriesList: supportedSeries,
			Series:     series,
			CharmName:  ch.String(),
		}
	}
	return nil
}

// String returns the application name.
func (a *Application) String() string {
	return a.doc.Name
}

// Refresh refreshes the contents of the Application from the underlying
// state. It returns an error that satisfies errors.IsNotFound if the
// application has been removed.
func (a *Application) Refresh() error {
	applications, closer := a.st.db().GetCollection(applicationsC)
	defer closer()

	err := applications.FindId(a.doc.DocID).One(&a.doc)
	if err == mgo.ErrNotFound {
		return errors.NotFoundf("application %q", a)
	}
	if err != nil {
		return errors.Errorf("cannot refresh application %q: %v", a, err)
	}
	return nil
}

// GetPlacement returns the application's placement directive.
// This is used on CAAS models.
func (a *Application) GetPlacement() string {
	return a.doc.Placement
}

// GetScale returns the application's desired scale value.
// This is used on CAAS models.
func (a *Application) GetScale() int {
	return a.doc.DesiredScale
}

// ChangeScale alters the existing scale by the provided change amount, returning the new amount.
// This is used on CAAS models.
func (a *Application) ChangeScale(scaleChange int) (int, error) {
	newScale := a.doc.DesiredScale + scaleChange
	logger.Tracef("ChangeScale DesiredScale %v, scaleChange %v, newScale %v", a.doc.DesiredScale, scaleChange, newScale)
	if newScale < 0 {
		return a.doc.DesiredScale, errors.NotValidf("cannot remove more units than currently exist")
	}
	buildTxn := func(attempt int) ([]txn.Op, error) {
		if attempt > 0 {
			if err := a.Refresh(); err != nil {
				return nil, errors.Trace(err)
			}
			alive, err := isAlive(a.st, applicationsC, a.doc.DocID)
			if err != nil {
				return nil, errors.Trace(err)
			} else if !alive {
				return nil, applicationNotAliveErr
			}
			newScale = a.doc.DesiredScale + scaleChange
			if newScale < 0 {
				return nil, errors.NotValidf("cannot remove more units than currently exist")
			}
		}
		ops := []txn.Op{{
			C:  applicationsC,
			Id: a.doc.DocID,
			Assert: bson.D{
				{"life", Alive},
				{"charmurl", a.doc.CharmURL},
				{"unitcount", a.doc.UnitCount},
				{"scale", a.doc.DesiredScale},
			},
			Update: bson.D{{"$set", bson.D{{"scale", newScale}}}},
		}}

		cloudSvcDoc := cloudServiceDoc{
			DocID:                 a.globalKey(),
			DesiredScaleProtected: true,
		}
		cloudSvcOp, err := buildCloudServiceOps(a.st, cloudSvcDoc)
		if err != nil {
			return nil, errors.Trace(err)
		}
		ops = append(ops, cloudSvcOp...)
		return ops, nil
	}
	if err := a.st.db().Run(buildTxn); err != nil {
		return a.doc.DesiredScale, errors.Errorf("cannot set scale for application %q to %v: %v", a, newScale, onAbort(err, applicationNotAliveErr))
	}
	a.doc.DesiredScale = newScale
	return newScale, nil
}

// SetScale sets the application's desired scale value.
// This is used on CAAS models.
func (a *Application) SetScale(scale int, generation int64, force bool) error {
	if scale < 0 {
		return errors.NotValidf("application scale %d", scale)
	}
	svcInfo, err := a.ServiceInfo()
	if err != nil && !errors.IsNotFound(err) {
		return errors.Trace(err)
	}
	if err == nil {
		logger.Tracef(
			"SetScale DesiredScaleProtected %v, DesiredScale %v -> %v, Generation %v -> %v",
			svcInfo.DesiredScaleProtected(), a.doc.DesiredScale, scale, svcInfo.Generation(), generation,
		)
		if svcInfo.DesiredScaleProtected() && !force && scale != a.doc.DesiredScale {
			return errors.Forbiddenf("SetScale(%d) without force while desired scale %d is not applied yet", scale, a.doc.DesiredScale)
		}
		if !force && generation < svcInfo.Generation() {
			return errors.Forbiddenf(
				"application generation %d can not be reverted to %d", svcInfo.Generation(), generation,
			)
		}
	}

	buildTxn := func(attempt int) ([]txn.Op, error) {
		if attempt > 0 {
			if err := a.Refresh(); err != nil {
				return nil, errors.Trace(err)
			}
			alive, err := isAlive(a.st, applicationsC, a.doc.DocID)
			if err != nil {
				return nil, errors.Trace(err)
			} else if !alive {
				return nil, applicationNotAliveErr
			}
		}
		ops := []txn.Op{{
			C:  applicationsC,
			Id: a.doc.DocID,
			Assert: bson.D{
				{"life", Alive},
				{"charmurl", a.doc.CharmURL},
				{"unitcount", a.doc.UnitCount},
			},
			Update: bson.D{{"$set", bson.D{{"scale", scale}}}},
		}}
		cloudSvcDoc := cloudServiceDoc{
			DocID: a.globalKey(),
		}
		if force {
			// scale from cli.
			cloudSvcDoc.DesiredScaleProtected = true
		} else {
			// scale from cluster always has a valid generation (>= current generation).
			cloudSvcDoc.Generation = generation
		}
		cloudSvcOp, err := buildCloudServiceOps(a.st, cloudSvcDoc)
		if err != nil {
			return nil, errors.Trace(err)
		}
		ops = append(ops, cloudSvcOp...)
		return ops, nil
	}
	if err := a.st.db().Run(buildTxn); err != nil {
		return errors.Errorf("cannot set scale for application %q to %v: %v", a, scale, onAbort(err, applicationNotAliveErr))
	}
	a.doc.DesiredScale = scale
	return nil
}

// newUnitName returns the next unit name.
func (a *Application) newUnitName() (string, error) {
	unitSeq, err := sequence(a.st, a.Tag().String())
	if err != nil {
		return "", errors.Trace(err)
	}
	name := a.doc.Name + "/" + strconv.Itoa(unitSeq)
	return name, nil
}

// addUnitOps returns a unique name for a new unit, and a list of txn operations
// necessary to create that unit. The principalName param must be non-empty if
// and only if s is a subordinate application. Only one subordinate of a given
// application will be assigned to a given principal. The asserts param can be used
// to include additional assertions for the application document.  This method
// assumes that the application already exists in the db.
func (a *Application) addUnitOps(
	principalName string,
	args AddUnitParams,
	asserts bson.D,
) (string, []txn.Op, error) {
	var cons constraints.Value
	if !a.doc.Subordinate {
		scons, err := a.Constraints()
		if errors.IsNotFound(err) {
			return "", nil, errors.NotFoundf("application %q", a.Name())
		}
		if err != nil {
			return "", nil, errors.Trace(err)
		}
		cons, err = a.st.ResolveConstraints(scons)
		if err != nil {
			return "", nil, errors.Trace(err)
		}
		if args.machineID != "" {
			return "", nil, errors.NotSupportedf("non-empty machineID")
		}
	}
	storageCons, err := a.StorageConstraints()
	if err != nil {
		return "", nil, errors.Trace(err)
	}
	uNames, ops, err := a.addUnitOpsWithCons(applicationAddUnitOpsArgs{
		cons:               cons,
		principalName:      principalName,
		principalMachineID: args.machineID,
		storageCons:        storageCons,
		attachStorage:      args.AttachStorage,
		providerId:         args.ProviderId,
		address:            args.Address,
		ports:              args.Ports,
	})
	if err != nil {
		return uNames, ops, errors.Trace(err)
	}
	// we verify the application is alive
	asserts = append(isAliveDoc, asserts...)
	ops = append(ops, a.incUnitCountOp(asserts))
	return uNames, ops, nil
}

type applicationAddUnitOpsArgs struct {
	principalName      string
	principalMachineID string

	cons          constraints.Value
	storageCons   map[string]StorageConstraints
	attachStorage []names.StorageTag

	// These optional attributes are relevant to CAAS models.
	providerId *string
	address    *string
	ports      *[]string
}

// addApplicationUnitOps is just like addUnitOps but explicitly takes a
// constraints value (this is used at application creation time).
func (a *Application) addApplicationUnitOps(args applicationAddUnitOpsArgs) (string, []txn.Op, error) {
	result, ops, err := a.addUnitOpsWithCons(args)
	if err == nil {
		ops = append(ops, a.incUnitCountOp(nil))
	}
	return result, ops, err
}

// addUnitOpsWithCons is a helper method for returning addUnitOps.
func (a *Application) addUnitOpsWithCons(args applicationAddUnitOpsArgs) (string, []txn.Op, error) {
	if a.doc.Subordinate && args.principalName == "" {
		return "", nil, errors.New("application is a subordinate")
	} else if !a.doc.Subordinate && args.principalName != "" {
		return "", nil, errors.New("application is not a subordinate")
	}
	name, err := a.newUnitName()
	if err != nil {
		return "", nil, errors.Trace(err)
	}
	unitTag := names.NewUnitTag(name)

	appCharm, _, err := a.Charm()
	if err != nil {
		return "", nil, errors.Trace(err)
	}
	storageOps, numStorageAttachments, err := a.addUnitStorageOps(
		args, unitTag, appCharm,
	)
	if err != nil {
		return "", nil, errors.Trace(err)
	}

	docID := a.st.docID(name)
	globalKey := unitGlobalKey(name)
	agentGlobalKey := unitAgentGlobalKey(name)
	udoc := &unitDoc{
		DocID:                  docID,
		Name:                   name,
		Application:            a.doc.Name,
		Series:                 a.doc.Series,
		Life:                   Alive,
		Principal:              args.principalName,
		MachineId:              args.principalMachineID,
		StorageAttachmentCount: numStorageAttachments,
	}
	now := a.st.clock().Now()
	agentStatusDoc := statusDoc{
		Status:  status.Allocating,
		Updated: now.UnixNano(),
	}

	m, err := a.st.Model()
	if err != nil {
		return "", nil, errors.Trace(err)
	}
	unitStatusDoc := &statusDoc{
		Status:     status.Waiting,
		StatusInfo: status.MessageInitializingAgent,
		Updated:    now.UnixNano(),
	}
	meterStatus := &meterStatusDoc{Code: MeterNotSet.String()}

	workloadVersionDoc := &statusDoc{
		Status:  status.Unknown,
		Updated: now.UnixNano(),
	}
	if m.Type() != ModelTypeCAAS {
		unitStatusDoc.StatusInfo = status.MessageWaitForMachine
	}
	var containerDoc *cloudContainerDoc
	if m.Type() == ModelTypeCAAS {
		if args.providerId != nil || args.address != nil || args.ports != nil {
			containerDoc = &cloudContainerDoc{
				Id: globalKey,
			}
			if args.providerId != nil {
				containerDoc.ProviderId = *args.providerId
			}
			if args.address != nil {
				networkAddr := network.NewScopedSpaceAddress(*args.address, network.ScopeMachineLocal)
				addr := fromNetworkAddress(networkAddr, OriginProvider)
				containerDoc.Address = &addr
			}
			if args.ports != nil {
				containerDoc.Ports = *args.ports
			}
		}
	}

	ops, err := addUnitOps(a.st, addUnitOpsArgs{
		unitDoc:            udoc,
		containerDoc:       containerDoc,
		agentStatusDoc:     agentStatusDoc,
		workloadStatusDoc:  unitStatusDoc,
		workloadVersionDoc: workloadVersionDoc,
		meterStatusDoc:     meterStatus,
	})
	if err != nil {
		return "", nil, errors.Trace(err)
	}

	ops = append(ops, storageOps...)

	if a.doc.Subordinate {
		ops = append(ops, txn.Op{
			C:  unitsC,
			Id: a.st.docID(args.principalName),
			Assert: append(isAliveDoc, bson.DocElem{
				"subordinates", bson.D{{"$not", bson.RegEx{Pattern: "^" + a.doc.Name + "/"}}},
			}),
			Update: bson.D{{"$addToSet", bson.D{{"subordinates", name}}}},
		})
	} else {
		ops = append(ops, createConstraintsOp(agentGlobalKey, args.cons))
	}

	// At the last moment we still have the statusDocs in scope, set the initial
	// history entries. This is risky, and may lead to extra entries, but that's
	// an intrinsic problem with mixing txn and non-txn ops -- we can't sync
	// them cleanly.
	_, _ = probablyUpdateStatusHistory(a.st.db(), globalKey, *unitStatusDoc)
	_, _ = probablyUpdateStatusHistory(a.st.db(), globalWorkloadVersionKey(name), *workloadVersionDoc)
	_, _ = probablyUpdateStatusHistory(a.st.db(), agentGlobalKey, agentStatusDoc)
	return name, ops, nil
}

func (a *Application) addUnitStorageOps(
	args applicationAddUnitOpsArgs,
	unitTag names.UnitTag,
	charm *Charm,
) ([]txn.Op, int, error) {

	sb, err := NewStorageBackend(a.st)
	if err != nil {
		return nil, -1, errors.Trace(err)
	}

	// Reduce the count of new storage created for each existing storage
	// being attached.
	var storageCons map[string]StorageConstraints
	for _, tag := range args.attachStorage {
		storageName, err := names.StorageName(tag.Id())
		if err != nil {
			return nil, -1, errors.Trace(err)
		}
		if cons, ok := args.storageCons[storageName]; ok && cons.Count > 0 {
			if storageCons == nil {
				// We must not modify the contents of the original
				// args.storageCons map, as it comes from the
				// user. Make a copy and modify that.
				storageCons = make(map[string]StorageConstraints)
				for name, cons := range args.storageCons {
					storageCons[name] = cons
				}
				args.storageCons = storageCons
			}
			cons.Count--
			storageCons[storageName] = cons
		}
	}

	// Add storage instances/attachments for the unit. If the
	// application is subordinate, we'll add the machine storage
	// if the principal is assigned to a machine. Otherwise, we
	// will add the subordinate's storage along with the principal's
	// when the principal is assigned to a machine.
	var machineAssignable machineAssignable
	if a.doc.Subordinate {
		pu, err := a.st.Unit(args.principalName)
		if err != nil {
			return nil, -1, errors.Trace(err)
		}
		machineAssignable = pu
	}
	storageOps, storageTags, numStorageAttachments, err := createStorageOps(
		sb,
		unitTag,
		charm.Meta(),
		args.storageCons,
		a.doc.Series,
		machineAssignable,
	)
	if err != nil {
		return nil, -1, errors.Trace(err)
	}
	for _, storageTag := range args.attachStorage {
		si, err := sb.storageInstance(storageTag)
		if err != nil {
			return nil, -1, errors.Annotatef(
				err, "attaching %s",
				names.ReadableString(storageTag),
			)
		}
		ops, err := sb.attachStorageOps(
			si,
			unitTag,
			a.doc.Series,
			charm,
			machineAssignable,
		)
		if err != nil {
			return nil, -1, errors.Trace(err)
		}
		storageOps = append(storageOps, ops...)
		numStorageAttachments++
		storageTags[si.StorageName()] = append(storageTags[si.StorageName()], storageTag)
	}
	for name, tags := range storageTags {
		count := len(tags)
		charmStorage := charm.Meta().Storage[name]
		if err := validateCharmStorageCountChange(charmStorage, 0, count); err != nil {
			return nil, -1, errors.Trace(err)
		}
		incRefOp, err := increfEntityStorageOp(a.st, unitTag, name, count)
		if err != nil {
			return nil, -1, errors.Trace(err)
		}
		storageOps = append(storageOps, incRefOp)
	}
	return storageOps, numStorageAttachments, nil
}

// applicationOffersRefCountKey returns a key for refcounting offers
// for the specified application. Each time an offer is created, the
// refcount is incremented, and the opposite happens on removal.
func applicationOffersRefCountKey(appName string) string {
	return fmt.Sprintf("offer#%s", appName)
}

// incApplicationOffersRefOp returns a txn.Op that increments the reference
// count for an application offer.
func incApplicationOffersRefOp(mb modelBackend, appName string) (txn.Op, error) {
	refcounts, closer := mb.db().GetCollection(refcountsC)
	defer closer()
	offerRefCountKey := applicationOffersRefCountKey(appName)
	incRefOp, err := nsRefcounts.CreateOrIncRefOp(refcounts, offerRefCountKey, 1)
	return incRefOp, errors.Trace(err)
}

// countApplicationOffersRefOp returns the number of offers for an application,
// along with a txn.Op that ensures that that does not change.
func countApplicationOffersRefOp(mb modelBackend, appName string) (txn.Op, int, error) {
	refcounts, closer := mb.db().GetCollection(refcountsC)
	defer closer()
	key := applicationOffersRefCountKey(appName)
	return nsRefcounts.CurrentOp(refcounts, key)
}

// decApplicationOffersRefOp returns a txn.Op that decrements the reference
// count for an application offer.
func decApplicationOffersRefOp(mb modelBackend, appName string) (txn.Op, error) {
	refcounts, closer := mb.db().GetCollection(refcountsC)
	defer closer()
	offerRefCountKey := applicationOffersRefCountKey(appName)
	decRefOp, _, err := nsRefcounts.DyingDecRefOp(refcounts, offerRefCountKey)
	if err != nil {
		return txn.Op{}, errors.Trace(err)
	}
	return decRefOp, nil
}

// incUnitCountOp returns the operation to increment the application's unit count.
func (a *Application) incUnitCountOp(asserts bson.D) txn.Op {
	op := txn.Op{
		C:      applicationsC,
		Id:     a.doc.DocID,
		Update: bson.D{{"$inc", bson.D{{"unitcount", 1}}}},
	}
	if len(asserts) > 0 {
		op.Assert = asserts
	}
	return op
}

// AddUnitParams contains parameters for the Application.AddUnit method.
type AddUnitParams struct {
	// AttachStorage identifies storage instances to attach to the unit.
	AttachStorage []names.StorageTag

	// These attributes are relevant to CAAS models.

	// ProviderId identifies the unit for a given provider.
	ProviderId *string

	// Address is the container address.
	Address *string

	// Ports are the open ports on the container.
	Ports *[]string

	// machineID is only passed in if the unit being created is
	// a subordinate and refers to the machine that is hosting the principal.
	machineID string
}

// AddUnit adds a new principal unit to the application.
func (a *Application) AddUnit(args AddUnitParams) (unit *Unit, err error) {
	defer errors.DeferredAnnotatef(&err, "cannot add unit to application %q", a)
	name, ops, err := a.addUnitOps("", args, nil)
	if err != nil {
		return nil, err
	}

	if err := a.st.db().RunTransaction(ops); err == txn.ErrAborted {
		if alive, err := isAlive(a.st, applicationsC, a.doc.DocID); err != nil {
			return nil, err
		} else if !alive {
			return nil, applicationNotAliveErr
		}
		return nil, errors.New("inconsistent state")
	} else if err != nil {
		return nil, err
	}
	return a.st.Unit(name)
}

// removeUnitOps returns the operations necessary to remove the supplied unit,
// assuming the supplied asserts apply to the unit document.
// When 'force' is set, this call will always return some needed operations
// and accumulate all operational errors encountered in the operation.
// If the 'force' is not set, any error will be fatal and no operations will be returned.
func (a *Application) removeUnitOps(u *Unit, asserts bson.D, op *ForcedOperation, destroyStorage bool) ([]txn.Op, error) {
	hostOps, err := u.destroyHostOps(a, op)
	if op.FatalError(err) {
		return nil, errors.Trace(err)
	}
	portsOps, err := removePortsForUnitOps(a.st, u)
	if op.FatalError(err) {
		return nil, errors.Trace(err)
	}
	resOps, err := removeUnitResourcesOps(a.st, u.doc.Name)
	if op.FatalError(err) {
		return nil, errors.Trace(err)
	}

	observedFieldsMatch := bson.D{
		{"charmurl", u.doc.CharmURL},
		{"machineid", u.doc.MachineId},
	}
	ops := []txn.Op{
		{
			C:      unitsC,
			Id:     u.doc.DocID,
			Assert: append(observedFieldsMatch, asserts...),
			Remove: true,
		},
		removeMeterStatusOp(a.st, u.globalMeterStatusKey()),
		removeStatusOp(a.st, u.globalAgentKey()),
		removeStatusOp(a.st, u.globalKey()),
		removeUnitStateOp(a.st, u.globalKey()),
		removeStatusOp(a.st, u.globalCloudContainerKey()),
		removeConstraintsOp(u.globalAgentKey()),
		annotationRemoveOp(a.st, u.globalKey()),
		newCleanupOp(cleanupRemovedUnit, u.doc.Name, op.Force),
	}
	ops = append(ops, portsOps...)
	ops = append(ops, resOps...)
	ops = append(ops, hostOps...)

	m, err := a.st.Model()
	if op.FatalError(err) {
		return nil, errors.Trace(err)
	} else {
		if m.Type() == ModelTypeCAAS {
			ops = append(ops, u.removeCloudContainerOps()...)
		}
		branchOps, err := unassignUnitFromBranchOp(u.doc.Name, a.doc.Name, m)
		if err != nil {
			if !op.Force {
				return nil, errors.Trace(err)
			}
			op.AddError(err)
		}
		ops = append(ops, branchOps...)
	}

	sb, err := NewStorageBackend(a.st)
	if op.FatalError(err) {
		return nil, errors.Trace(err)
	} else {
		storageInstanceOps, err := removeStorageInstancesOps(sb, u.Tag(), op.Force)
		if op.FatalError(err) {
			return nil, errors.Trace(err)
		}
		ops = append(ops, storageInstanceOps...)
	}

	if u.doc.CharmURL != nil {
		// If the unit has a different URL to the application, allow any final
		// cleanup to happen; otherwise we just do it when the app itself is removed.
		maybeDoFinal := u.doc.CharmURL != a.doc.CharmURL
		// When 'force' is set, this call will return both needed operations
		// as well as all operational errors encountered.
		// If the 'force' is not set, any error will be fatal and no operations will be returned.
		decOps, err := appCharmDecRefOps(a.st, a.doc.Name, u.doc.CharmURL, maybeDoFinal, op)
		if errors.IsNotFound(err) {
			return nil, errRefresh
		} else if op.FatalError(err) {
			return nil, errors.Trace(err)
		}
		ops = append(ops, decOps...)
	}
	appOp := txn.Op{
		C:      applicationsC,
		Id:     a.doc.DocID,
		Assert: bson.D{{"life", a.doc.Life}, {"unitcount", bson.D{{"$gt", 0}}}},
		Update: bson.D{{"$inc", bson.D{{"unitcount", -1}}}},
	}
	ops = append(ops, appOp)
	if a.doc.Life == Dying {
		// Create a cleanup for this application as this might be the last reference.
		cleanupOp := newCleanupOp(
			cleanupApplication,
			a.doc.Name,
			destroyStorage,
			op.Force,
		)
		ops = append(ops, cleanupOp)
	}
	return ops, nil
}

func removeUnitResourcesOps(st *State, unitID string) ([]txn.Op, error) {
	persist, err := st.ResourcesPersistence()
	if errors.IsNotSupported(err) {
		// Nothing to see here, move along.
		return nil, nil
	}
	if err != nil {
		return nil, errors.Trace(err)
	}
	ops, err := persist.NewRemoveUnitResourcesOps(unitID)
	if err != nil {
		return nil, errors.Trace(err)
	}
	return ops, nil
}

func unassignUnitFromBranchOp(unitName, appName string, m *Model) ([]txn.Op, error) {
	branch, err := m.unitBranch(unitName)
	if err != nil {
		return nil, errors.Trace(err)
	}
	if branch == nil {
		// Nothing to see here, move along.
		return nil, nil
	}
	return branch.unassignUnitOps(unitName, appName), nil
}

// AllUnits returns all units of the application.
func (a *Application) AllUnits() (units []*Unit, err error) {
	return allUnits(a.st, a.doc.Name)
}

func allUnits(st *State, application string) (units []*Unit, err error) {
	unitsCollection, closer := st.db().GetCollection(unitsC)
	defer closer()

	docs := []unitDoc{}
	err = unitsCollection.Find(bson.D{{"application", application}}).All(&docs)
	if err != nil {
		return nil, errors.Annotatef(err, "cannot get all units from application %q", application)
	}
	m, err := st.Model()
	if err != nil {
		return nil, errors.Trace(err)
	}
	for i := range docs {
		units = append(units, newUnit(st, m.Type(), &docs[i]))
	}
	return units, nil
}

// Relations returns a Relation for every relation the application is in.
func (a *Application) Relations() (relations []*Relation, err error) {
	return applicationRelations(a.st, a.doc.Name)
}

func applicationRelations(st *State, name string) (relations []*Relation, err error) {
	defer errors.DeferredAnnotatef(&err, "can't get relations for application %q", name)
	relationsCollection, closer := st.db().GetCollection(relationsC)
	defer closer()

	docs := []relationDoc{}
	err = relationsCollection.Find(bson.D{{"endpoints.applicationname", name}}).All(&docs)
	if err != nil {
		return nil, err
	}
	for _, v := range docs {
		relations = append(relations, newRelation(st, &v))
	}
	return relations, nil
}

// CharmConfig returns the raw user configuration for the application's charm.
func (a *Application) CharmConfig(branchName string) (charm.Settings, error) {
	if a.doc.CharmURL == nil {
		return nil, fmt.Errorf("application charm not set")
	}

	s, err := charmSettingsWithDefaults(a.st, a.doc.CharmURL, a.Name(), branchName)
	return s, errors.Annotatef(err, "charm config for application %q", a.doc.Name)
}

func charmSettingsWithDefaults(st *State, cURL *charm.URL, appName, branchName string) (charm.Settings, error) {
	cfg, err := branchCharmSettings(st, cURL, appName, branchName)
	if err != nil {
		return nil, errors.Trace(err)
	}

	ch, err := st.Charm(cURL)
	if err != nil {
		return nil, errors.Trace(err)
	}

	result := ch.Config().DefaultSettings()
	for name, value := range cfg.Map() {
		result[name] = value
	}
	return result, nil
}

func branchCharmSettings(st *State, cURL *charm.URL, appName, branchName string) (*Settings, error) {
	key := applicationCharmConfigKey(appName, cURL)
	cfg, err := readSettings(st.db(), settingsC, key)
	if err != nil {
		return nil, errors.Trace(err)
	}

	if branchName != model.GenerationMaster {
		branch, err := st.Branch(branchName)
		if err != nil {
			return nil, errors.Trace(err)
		}
		cfg.applyChanges(branch.Config()[appName])
	}

	return cfg, nil
}

// UpdateCharmConfig changes a application's charm config settings. Values set
// to nil will be deleted; unknown and invalid values will return an error.
func (a *Application) UpdateCharmConfig(branchName string, changes charm.Settings) error {
	ch, _, err := a.Charm()
	if err != nil {
		return errors.Trace(err)
	}
	changes, err = ch.Config().ValidateSettings(changes)
	if err != nil {
		return errors.Trace(err)
	}

	// TODO(fwereade) state.Settings is itself really problematic in just
	// about every use case. This needs to be resolved some time; but at
	// least the settings docs are keyed by charm url as well as application
	// name, so the actual impact of a race is non-threatening.
	current, err := readSettings(a.st.db(), settingsC, a.charmConfigKey())
	if err != nil {
		return errors.Annotatef(err, "charm config for application %q", a.doc.Name)
	}

	if branchName == model.GenerationMaster {
		return errors.Trace(a.updateMasterConfig(current, changes))
	}
	return errors.Trace(a.updateBranchConfig(branchName, current, changes))
}

// TODO (manadart 2019-04-03): Implement master config changes as
// instantly committed branches.
func (a *Application) updateMasterConfig(current *Settings, validChanges charm.Settings) error {
	for name, value := range validChanges {
		if value == nil {
			current.Delete(name)
		} else {
			current.Set(name, value)
		}
	}
	_, err := current.Write()
	return errors.Trace(err)
}

// updateBranchConfig compares the incoming charm settings to the current
// settings to generate a collection of changes, which is used to update the
// branch with the input name.
func (a *Application) updateBranchConfig(branchName string, current *Settings, validChanges charm.Settings) error {
	branch, err := a.st.Branch(branchName)
	if err != nil {
		return errors.Trace(err)
	}

	return errors.Trace(branch.UpdateCharmConfig(a.Name(), current, validChanges))
}

// ApplicationConfig returns the configuration for the application itself.
func (a *Application) ApplicationConfig() (application.ConfigAttributes, error) {
	config, err := readSettings(a.st.db(), settingsC, a.applicationConfigKey())
	if errors.IsNotFound(err) || len(config.Keys()) == 0 {
		return application.ConfigAttributes(nil), nil
	} else if err != nil {
		return nil, errors.Annotatef(err, "application config for application %q", a.doc.Name)
	}
	return application.ConfigAttributes(config.Map()), nil
}

// UpdateApplicationConfig changes an application's config settings.
// Unknown and invalid values will return an error.
func (a *Application) UpdateApplicationConfig(
	changes application.ConfigAttributes,
	reset []string,
	schema environschema.Fields,
	defaults schema.Defaults,
) error {
	node, err := readSettings(a.st.db(), settingsC, a.applicationConfigKey())
	if errors.IsNotFound(err) {
		return errors.Errorf("cannot update application config since no config exists for application %v", a.doc.Name)
	} else if err != nil {
		return errors.Annotatef(err, "application config for application %q", a.doc.Name)
	}
	resetKeys := set.NewStrings(reset...)
	for name, value := range changes {
		if resetKeys.Contains(name) {
			continue
		}
		node.Set(name, value)
	}
	for _, name := range reset {
		node.Delete(name)
	}
	newConfig, err := application.NewConfig(node.Map(), schema, defaults)
	if err != nil {
		return errors.Trace(err)
	}
	if err := newConfig.Validate(); err != nil {
		return errors.Trace(err)
	}
	// Update node so it gets coerced values with correct types.
	coerced := newConfig.Attributes()
	for _, key := range node.Keys() {
		node.Set(key, coerced[key])
	}
	_, err = node.Write()
	return err
}

// LeaderSettings returns a application's leader settings. If nothing has been set
// yet, it will return an empty map; this is not an error.
func (a *Application) LeaderSettings() (map[string]string, error) {
	// There's no compelling reason to have these methods on Application -- and
	// thus require an extra db read to access them -- but it stops the State
	// type getting even more cluttered.

	doc, err := readSettingsDoc(a.st.db(), settingsC, leadershipSettingsKey(a.doc.Name))
	if errors.IsNotFound(err) {
		return nil, errors.NotFoundf("application %q", a.doc.Name)
	} else if err != nil {
		return nil, errors.Annotatef(err, "application %q", a.doc.Name)
	}
	result := make(map[string]string)
	for escapedKey, interfaceValue := range doc.Settings {
		key := mgoutils.UnescapeKey(escapedKey)
		if value, _ := interfaceValue.(string); value != "" {
			// Empty strings are technically bad data -- when set, they clear.
			result[key] = value
		} else {
			// Some bad data isn't reason enough to obscure the good data.
			logger.Warningf("unexpected leader settings value for %s: %#v", key, interfaceValue)
		}
	}
	return result, nil
}

// UpdateLeaderSettings updates the application's leader settings with the supplied
// values, but will fail (with a suitable error) if the supplied Token loses
// validity. Empty values in the supplied map will be cleared in the database.
func (a *Application) UpdateLeaderSettings(token leadership.Token, updates map[string]string) error {
	// There's no compelling reason to have these methods on Application -- and
	// thus require an extra db read to access them -- but it stops the State
	// type getting even more cluttered.
	key := leadershipSettingsKey(a.doc.Name)
	converted := make(map[string]interface{}, len(updates))
	for k, v := range updates {
		converted[k] = v
	}

	modelOp := newUpdateLeaderSettingsOperation(a.st.db(), token, key, converted)
	err := a.st.ApplyOperation(modelOp)
	if errors.IsNotFound(err) {
		return errors.NotFoundf("application %q", a.doc.Name)
	} else if err != nil {
		return errors.Annotatef(err, "application %q", a.doc.Name)
	}
	return nil
}

var ErrSubordinateConstraints = stderrors.New("constraints do not apply to subordinate applications")

// Constraints returns the current application constraints.
func (a *Application) Constraints() (constraints.Value, error) {
	if a.doc.Subordinate {
		return constraints.Value{}, ErrSubordinateConstraints
	}
	return readConstraints(a.st, a.globalKey())
}

// SetConstraints replaces the current application constraints.
func (a *Application) SetConstraints(cons constraints.Value) (err error) {
	unsupported, err := a.st.validateConstraints(cons)
	if len(unsupported) > 0 {
		logger.Warningf(
			"setting constraints on application %q: unsupported constraints: %v", a.Name(), strings.Join(unsupported, ","))
	} else if err != nil {
		return err
	}
	if a.doc.Subordinate {
		return ErrSubordinateConstraints
	}
	defer errors.DeferredAnnotatef(&err, "cannot set constraints")
	if a.doc.Life != Alive {
		return applicationNotAliveErr
	}
	ops := []txn.Op{{
		C:      applicationsC,
		Id:     a.doc.DocID,
		Assert: isAliveDoc,
	}}
	ops = append(ops, setConstraintsOp(a.globalKey(), cons))
	return onAbort(a.st.db().RunTransaction(ops), applicationNotAliveErr)
}

// EndpointBindings returns the mapping for each endpoint name and the space
// ID it is bound to (or empty if unspecified). When no bindings are stored
// for the application, defaults are returned.
func (a *Application) EndpointBindings() (*Bindings, error) {
	// We don't need the TxnRevno below.
	bindings, _, err := readEndpointBindings(a.st, a.globalKey())
	if err != nil && !errors.IsNotFound(err) {
		return nil, errors.Trace(err)
	}
	if bindings == nil {
		bindings, err = a.defaultEndpointBindings()
		if err != nil {
			return nil, errors.Trace(err)
		}
	}
	return &Bindings{st: a.st, bindingsMap: bindings}, nil
}

// defaultEndpointBindings returns a map with each endpoint from the current
// charm metadata bound to an empty space. If no charm URL is set yet, it
// returns an empty map.
func (a *Application) defaultEndpointBindings() (map[string]string, error) {
	if a.doc.CharmURL == nil {
		return map[string]string{}, nil
	}

	appCharm, _, err := a.Charm()
	if err != nil {
		return nil, errors.Trace(err)
	}

	return DefaultEndpointBindingsForCharm(a.st, appCharm.Meta())
}

// MetricCredentials returns any metric credentials associated with this application.
func (a *Application) MetricCredentials() []byte {
	return a.doc.MetricCredentials
}

// SetMetricCredentials updates the metric credentials associated with this application.
func (a *Application) SetMetricCredentials(b []byte) error {
	buildTxn := func(attempt int) ([]txn.Op, error) {
		if attempt > 0 {
			alive, err := isAlive(a.st, applicationsC, a.doc.DocID)
			if err != nil {
				return nil, errors.Trace(err)
			} else if !alive {
				return nil, applicationNotAliveErr
			}
		}
		ops := []txn.Op{
			{
				C:      applicationsC,
				Id:     a.doc.DocID,
				Assert: isAliveDoc,
				Update: bson.M{"$set": bson.M{"metric-credentials": b}},
			},
		}
		return ops, nil
	}
	if err := a.st.db().Run(buildTxn); err != nil {
		return errors.Annotatef(err, "cannot update metric credentials")
	}
	a.doc.MetricCredentials = b
	return nil
}

// StorageConstraints returns the storage constraints for the application.
func (a *Application) StorageConstraints() (map[string]StorageConstraints, error) {
	cons, err := readStorageConstraints(a.st, a.storageConstraintsKey())
	if errors.IsNotFound(err) {
		return nil, nil
	} else if err != nil {
		return nil, errors.Annotatef(err, "application %q", a.doc.Name)
	}
	return cons, nil
}

// DeviceConstraints returns the device constraints for the application.
func (a *Application) DeviceConstraints() (map[string]DeviceConstraints, error) {
	cons, err := readDeviceConstraints(a.st, a.deviceConstraintsKey())
	if errors.IsNotFound(err) {
		return nil, nil
	} else if err != nil {
		return nil, errors.Trace(err)
	}
	return cons, nil
}

// Status returns the status of the application.
// Only unit leaders are allowed to set the status of the application.
// If no status is recorded, then there are no unit leaders and the
// status is derived from the unit status values.
func (a *Application) Status() (status.StatusInfo, error) {
	statuses, closer := a.st.db().GetCollection(statusesC)
	defer closer()
	query := statuses.Find(bson.D{{"_id", a.globalKey()}, {"neverset", true}})
	if count, err := query.Count(); err != nil {
		return status.StatusInfo{}, errors.Trace(err)
	} else if count != 0 {
		// This indicates that SetStatus has never been called on this application.
		// This in turn implies the application status document is likely to be
		// inaccurate, so we return aggregated unit statuses instead.
		//
		// TODO(thumper) 2019-12-20: bug 1857075
		// TODO(fwereade): this is completely wrong and will produce bad results
		// in not-very-challenging scenarios. The leader unit remains responsible
		// for setting the application status in a timely way, *whether or not the
		// charm's hooks exists or sets an application status*. This logic should be
		// removed as soon as possible, and the responsibilities implemented in
		// the right places rather than being applied at seeming random.
		units, err := a.AllUnits()
		if err != nil {
			return status.StatusInfo{}, err
		}
		logger.Tracef("application %q has %d units", a.Name(), len(units))
		var unitStatuses []status.StatusInfo
		for _, unit := range units {
			unitStatus, err := unit.Status()
			if err != nil {
				// Sometimes as units are being removed, we may hit a not found error here.
				if errors.IsNotFound(err) {
					continue
				}
				return status.StatusInfo{}, errors.Annotatef(err, "deriving application status from %q", unit.Name())
			}
			unitStatuses = append(unitStatuses, unitStatus)
		}
		if len(unitStatuses) > 0 {
			return deriveApplicationStatus(unitStatuses), nil
		}
	}
	return getStatus(a.st.db(), a.globalKey(), "application")
}

// CheckApplicationExpectsWorkload checks if the application expects workload or not.
func CheckApplicationExpectsWorkload(m *Model, appName string) (bool, error) {
	cm, err := m.CAASModel()
	if err != nil {
		// IAAS models alway have a unit workload.
		return true, nil
	}
	_, err = cm.PodSpec(names.NewApplicationTag(appName))
	if err != nil && !errors.IsNotFound(err) {
		return false, errors.Trace(err)
	}
	return err == nil, nil
}

// SetStatus sets the status for the application.
func (a *Application) SetStatus(statusInfo status.StatusInfo) error {
	if !status.ValidWorkloadStatus(statusInfo.Status) {
		return errors.Errorf("cannot set invalid status %q", statusInfo.Status)
	}

	var newHistory *statusDoc
	m, err := a.st.Model()
	if err != nil {
		return errors.Trace(err)
	}
	if m.Type() == ModelTypeCAAS {
		// Application status for a caas model needs to consider status
		// info coming from the operator pod as well; It may need to
		// override what is set here.
		expectWorkload, err := CheckApplicationExpectsWorkload(m, a.Name())
		if err != nil {
			return errors.Trace(err)
		}
		operatorStatus, err := getStatus(a.st.db(), applicationGlobalOperatorKey(a.Name()), "operator")
		if err == nil {
			newHistory, err = caasHistoryRewriteDoc(statusInfo, operatorStatus, expectWorkload, caasApplicationDisplayStatus, a.st.clock())
			if err != nil {
				return errors.Trace(err)
			}
		} else if !errors.IsNotFound(err) {
			return errors.Trace(err)
		}
	}

	return setStatus(a.st.db(), setStatusParams{
		badge:            "application",
		globalKey:        a.globalKey(),
		status:           statusInfo.Status,
		message:          statusInfo.Message,
		rawData:          statusInfo.Data,
		updated:          timeOrNow(statusInfo.Since, a.st.clock()),
		historyOverwrite: newHistory,
	})
}

// SetOperatorStatus sets the operator status for an application.
// This is used on CAAS models.
func (a *Application) SetOperatorStatus(sInfo status.StatusInfo) error {
	m, err := a.st.Model()
	if err != nil {
		return errors.Trace(err)
	}
	if m.Type() != ModelTypeCAAS {
		return errors.NotSupportedf("caas operation on non-caas model")
	}

	err = setStatus(a.st.db(), setStatusParams{
		badge:     "operator",
		globalKey: applicationGlobalOperatorKey(a.Name()),
		status:    sInfo.Status,
		message:   sInfo.Message,
		rawData:   sInfo.Data,
		updated:   timeOrNow(sInfo.Since, a.st.clock()),
	})
	if err != nil {
		return errors.Trace(err)
	}
	appStatus, err := a.Status()
	if err != nil {
		return errors.Trace(err)
	}
	expectWorkload, err := CheckApplicationExpectsWorkload(m, a.Name())
	if err != nil {
		return errors.Trace(err)
	}
	historyDoc, err := caasHistoryRewriteDoc(appStatus, sInfo, expectWorkload, caasApplicationDisplayStatus, a.st.clock())
	if err != nil {
		return errors.Trace(err)
	}
	if historyDoc != nil {
		// rewriting application status history
		_, err = probablyUpdateStatusHistory(a.st.db(), a.globalKey(), *historyDoc)
		if err != nil {
			return errors.Trace(err)
		}
	}
	return nil
}

// StatusHistory returns a slice of at most filter.Size StatusInfo items
// or items as old as filter.Date or items newer than now - filter.Delta time
// representing past statuses for this application.
func (a *Application) StatusHistory(filter status.StatusHistoryFilter) ([]status.StatusInfo, error) {
	args := &statusHistoryArgs{
		db:        a.st.db(),
		globalKey: a.globalKey(),
		filter:    filter,
	}
	return statusHistory(args)
}

// ApplicationAndUnitsStatus returns the status for this application and all its units.
func (a *Application) ApplicationAndUnitsStatus() (status.StatusInfo, map[string]status.StatusInfo, error) {
	applicationStatus, err := a.Status()
	if err != nil {
		return status.StatusInfo{}, nil, errors.Trace(err)
	}
	units, err := a.AllUnits()
	if err != nil {
		return status.StatusInfo{}, nil, err
	}
	results := make(map[string]status.StatusInfo, len(units))
	for _, unit := range units {
		unitStatus, err := unit.Status()
		if err != nil {
			return status.StatusInfo{}, nil, err
		}
		results[unit.Name()] = unitStatus
	}
	return applicationStatus, results, nil

}

func deriveApplicationStatus(statuses []status.StatusInfo) status.StatusInfo {
	var result status.StatusInfo
	for _, unitStatus := range statuses {
		currentSeverity := statusServerities[result.Status]
		unitSeverity := statusServerities[unitStatus.Status]
		if unitSeverity > currentSeverity {
			result.Status = unitStatus.Status
			result.Message = unitStatus.Message
			result.Data = unitStatus.Data
			result.Since = unitStatus.Since
		}
	}
	return result
}

// statusSeverities holds status values with a severity measure.
// Status values with higher severity are used in preference to others.
var statusServerities = map[status.Status]int{
	status.Error:       100,
	status.Blocked:     90,
	status.Waiting:     80,
	status.Maintenance: 70,
	status.Active:      60,
	status.Terminated:  50,
	status.Unknown:     40,
}

type addApplicationOpsArgs struct {
	applicationDoc    *applicationDoc
	statusDoc         statusDoc
	constraints       constraints.Value
	storage           map[string]StorageConstraints
	devices           map[string]DeviceConstraints
	applicationConfig map[string]interface{}
	charmConfig       map[string]interface{}
	// These are nil when adding a new application, and most likely
	// non-nil when migrating.
	leadershipSettings map[string]interface{}
	operatorStatus     *statusDoc
}

// addApplicationOps returns the operations required to add an application to the
// applications collection, along with all the associated expected other application
// entries. This method is used by both the *State.AddApplication method and the
// migration import code.
func addApplicationOps(mb modelBackend, app *Application, args addApplicationOpsArgs) ([]txn.Op, error) {
	charmRefOps, err := appCharmIncRefOps(mb, args.applicationDoc.Name, args.applicationDoc.CharmURL, true)
	if err != nil {
		return nil, errors.Trace(err)
	}

	globalKey := app.globalKey()
	charmConfigKey := app.charmConfigKey()
	applicationConfigKey := app.applicationConfigKey()
	storageConstraintsKey := app.storageConstraintsKey()
	deviceConstraintsKey := app.deviceConstraintsKey()
	leadershipKey := leadershipSettingsKey(app.Name())

	ops := []txn.Op{
		createConstraintsOp(globalKey, args.constraints),
		createStorageConstraintsOp(storageConstraintsKey, args.storage),
		createDeviceConstraintsOp(deviceConstraintsKey, args.devices),
		createSettingsOp(settingsC, charmConfigKey, args.charmConfig),
		createSettingsOp(settingsC, applicationConfigKey, args.applicationConfig),
		createSettingsOp(settingsC, leadershipKey, args.leadershipSettings),
		createStatusOp(mb, globalKey, args.statusDoc),
		addModelApplicationRefOp(mb, app.Name()),
	}
	m, err := app.st.Model()
	if err != nil {
		return nil, errors.Trace(err)
	}
	if m.Type() == ModelTypeCAAS {
		operatorStatusDoc := args.statusDoc
		if args.operatorStatus != nil {
			operatorStatusDoc = *args.operatorStatus
		}
		ops = append(ops, createStatusOp(mb, applicationGlobalOperatorKey(app.Name()), operatorStatusDoc))
	}

	ops = append(ops, charmRefOps...)
	ops = append(ops, txn.Op{
		C:      applicationsC,
		Id:     app.Name(),
		Assert: txn.DocMissing,
		Insert: args.applicationDoc,
	})
	ops = append(ops, txn.Op{
		C:      remoteApplicationsC,
		Id:     app.Name(),
		Assert: txn.DocMissing,
	})
	return ops, nil
}

// SetPassword sets the password for the application's agent.
// TODO(caas) - consider a separate CAAS application entity
func (a *Application) SetPassword(password string) error {
	if len(password) < utils.MinAgentPasswordLength {
		return fmt.Errorf("password is only %d bytes long, and is not a valid Agent password", len(password))
	}
	passwordHash := utils.AgentPasswordHash(password)
	ops := []txn.Op{{
		C:      applicationsC,
		Id:     a.doc.DocID,
		Assert: notDeadDoc,
		Update: bson.D{{"$set", bson.D{{"passwordhash", passwordHash}}}},
	}}
	err := a.st.db().RunTransaction(ops)
	if err != nil {
		return fmt.Errorf("cannot set password of application %q: %v", a, onAbort(err, ErrDead))
	}
	a.doc.PasswordHash = passwordHash
	return nil
}

// PasswordValid returns whether the given password is valid
// for the given application.
func (a *Application) PasswordValid(password string) bool {
	agentHash := utils.AgentPasswordHash(password)
	if agentHash == a.doc.PasswordHash {
		return true
	}
	return false
}

// UnitUpdateProperties holds information used to update
// the state model for the unit.
type UnitUpdateProperties struct {
	ProviderId           *string
	Address              *string
	Ports                *[]string
	AgentStatus          *status.StatusInfo
	UnitStatus           *status.StatusInfo
	CloudContainerStatus *status.StatusInfo
}

// UpdateUnits applies the given application unit update operations.
func (a *Application) UpdateUnits(unitsOp *UpdateUnitsOperation) error {
	return a.st.ApplyOperation(unitsOp)
}

// UpdateUnitsOperation is a model operation for updating
// some units of an application.
type UpdateUnitsOperation struct {
	Adds    []*AddUnitOperation
	Deletes []*DestroyUnitOperation
	Updates []*UpdateUnitOperation
}

func (op *UpdateUnitsOperation) allOps() []ModelOperation {
	var all []ModelOperation
	for _, mop := range op.Adds {
		all = append(all, mop)
	}
	for _, mop := range op.Updates {
		all = append(all, mop)
	}
	for _, mop := range op.Deletes {
		all = append(all, mop)
	}
	return all
}

// Build is part of the ModelOperation interface.
func (op *UpdateUnitsOperation) Build(attempt int) ([]txn.Op, error) {
	var ops []txn.Op

	all := op.allOps()
	for _, op := range all {
		switch nextOps, err := op.Build(attempt); err {
		case jujutxn.ErrNoOperations:
			continue
		case nil:
			ops = append(ops, nextOps...)
		default:
			return nil, errors.Trace(err)
		}
	}
	return ops, nil
}

// Done is part of the ModelOperation interface.
func (op *UpdateUnitsOperation) Done(err error) error {
	if err != nil {
		return errors.Annotate(err, "updating units")
	}
	all := op.allOps()
	for _, op := range all {
		if err := op.Done(nil); err != nil {
			return errors.Trace(err)
		}
	}
	return nil
}

// AddOperation returns a model operation that will add a unit.
func (a *Application) AddOperation(props UnitUpdateProperties) *AddUnitOperation {
	return &AddUnitOperation{
		application: &Application{st: a.st, doc: a.doc},
		props:       props,
	}
}

// AddUnitOperation is a model operation that will add a unit.
type AddUnitOperation struct {
	application *Application
	props       UnitUpdateProperties

	unitName string
}

// Build is part of the ModelOperation interface.
func (op *AddUnitOperation) Build(attempt int) ([]txn.Op, error) {
	if alive, err := isAlive(op.application.st, applicationsC, op.application.doc.DocID); err != nil {
		return nil, err
	} else if !alive {
		return nil, applicationNotAliveErr
	}

	var ops []txn.Op

	addUnitArgs := AddUnitParams{
		ProviderId: op.props.ProviderId,
		Address:    op.props.Address,
		Ports:      op.props.Ports,
	}
	name, addOps, err := op.application.addUnitOps("", addUnitArgs, nil)
	if err != nil {
		return nil, errors.Trace(err)
	}

	op.unitName = name
	ops = append(ops, addOps...)

	if op.props.CloudContainerStatus != nil {
		now := op.application.st.clock().Now()
		doc := statusDoc{
			Status:     op.props.CloudContainerStatus.Status,
			StatusInfo: op.props.CloudContainerStatus.Message,
			StatusData: mgoutils.EscapeKeys(op.props.CloudContainerStatus.Data),
			Updated:    now.UnixNano(),
		}

		newStatusOps := createStatusOp(op.application.st, globalCloudContainerKey(name), doc)
		ops = append(ops, newStatusOps)
	}

	return ops, nil
}

// Done is part of the ModelOperation interface.
func (op *AddUnitOperation) Done(err error) error {
	if err != nil {
		return errors.Annotatef(err, "adding unit to %q", op.application.Name())
	}
	if op.props.AgentStatus == nil && op.props.CloudContainerStatus == nil {
		return nil
	}
	// We do a separate status update here because we require all units to be
	// created as "allocating". If the add operation specifies a status,
	// that status is used to update the initial "allocating" status. We then
	// get the expected 2 status entries in history. This is done in a separate
	// transaction; a failure here will effectively be retried because the worker
	// which has made the API call will restart and then perform the necessary update..
	u, err := op.application.st.Unit(op.unitName)
	if err != nil {
		return errors.Trace(err)
	}
	if op.props.AgentStatus != nil {
		now := op.application.st.clock().Now()
		if err := u.Agent().SetStatus(status.StatusInfo{
			Status:  op.props.AgentStatus.Status,
			Message: op.props.AgentStatus.Message,
			Data:    op.props.AgentStatus.Data,
			Since:   &now,
		}); err != nil {
			return errors.Trace(err)
		}
	}
	if op.props.CloudContainerStatus != nil {
		doc := statusDoc{
			Status:     op.props.CloudContainerStatus.Status,
			StatusInfo: op.props.CloudContainerStatus.Message,
			StatusData: mgoutils.EscapeKeys(op.props.CloudContainerStatus.Data),
			Updated:    timeOrNow(op.props.CloudContainerStatus.Since, u.st.clock()).UnixNano(),
		}
		_, err := probablyUpdateStatusHistory(op.application.st.db(), globalCloudContainerKey(op.unitName), doc)
		if err != nil {
			return errors.Trace(err)
		}

		// Ensure unit history is updated correctly
		unitStatus, err := getStatus(op.application.st.db(), unitGlobalKey(op.unitName), "unit")
		if err != nil {
			return errors.Trace(err)
		}
		newHistory, err := caasHistoryRewriteDoc(unitStatus, *op.props.CloudContainerStatus, true, caasUnitDisplayStatus, op.application.st.clock())
		if err != nil {
			return errors.Trace(err)
		}
		if newHistory != nil {
			err = setStatus(op.application.st.db(), setStatusParams{
				badge:            "unit",
				globalKey:        unitGlobalKey(op.unitName),
				status:           unitStatus.Status,
				message:          unitStatus.Message,
				rawData:          unitStatus.Data,
				updated:          timeOrNow(unitStatus.Since, u.st.clock()),
				historyOverwrite: newHistory,
			})
			if err != nil {
				return errors.Trace(err)
			}
		}
	}

	return nil
}

// UpdateCloudService updates the cloud service details for the application.
func (a *Application) UpdateCloudService(providerId string, addresses []network.SpaceAddress) error {
	_, err := a.st.SaveCloudService(SaveCloudServiceArgs{
		Id:         a.Name(),
		ProviderId: providerId,
		Addresses:  addresses,
	})
	return errors.Trace(err)
}

// ServiceInfo returns information about this application's cloud service.
// This is only used for CAAS models.
func (a *Application) ServiceInfo() (CloudServicer, error) {
	svc, err := a.st.CloudService(a.Name())
	if err != nil {
		return nil, errors.Trace(err)
	}
	return svc, nil
}

// UnitCount returns the of number of units for this application.
func (a *Application) UnitCount() int {
	return a.doc.UnitCount
}

// RelationCount returns the of number of active relations for this application.
func (a *Application) RelationCount() int {
	return a.doc.RelationCount

}

// UnitNames returns the of this application's units.
func (a *Application) UnitNames() ([]string, error) {
	u, err := appUnitNames(a.st, a.Name())
	return u, errors.Trace(err)
}

func appUnitNames(st *State, appName string) ([]string, error) {
	unitsCollection, closer := st.db().GetCollection(unitsC)
	defer closer()

	var docs []struct {
		Name string `bson:"name"`
	}
	err := unitsCollection.Find(bson.D{{"application", appName}}).Select(bson.D{{"name", 1}}).All(&docs)
	if err != nil {
		return nil, errors.Trace(err)
	}

	unitNames := make([]string, len(docs))
	for i, doc := range docs {
		unitNames[i] = doc.Name
	}
	return unitNames, nil
}
