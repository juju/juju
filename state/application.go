// Copyright 2012, 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state

import (
	"context"
	stderrors "errors"
	"fmt"
	"net"
	"sort"
	"strconv"
	"strings"

	"github.com/juju/collections/set"
	"github.com/juju/errors"
	"github.com/juju/mgo/v3"
	"github.com/juju/mgo/v3/bson"
	"github.com/juju/mgo/v3/txn"
	"github.com/juju/names/v6"
	"github.com/juju/schema"
	jujutxn "github.com/juju/txn/v3"

	"github.com/juju/juju/core/arch"
	"github.com/juju/juju/core/config"
	"github.com/juju/juju/core/constraints"
	"github.com/juju/juju/core/network"
	"github.com/juju/juju/core/network/firewall"
	"github.com/juju/juju/core/objectstore"
	"github.com/juju/juju/core/semversion"
	"github.com/juju/juju/core/status"
	"github.com/juju/juju/internal/charm"
	"github.com/juju/juju/internal/configschema"
	mgoutils "github.com/juju/juju/internal/mongo/utils"
	internalpassword "github.com/juju/juju/internal/password"
	"github.com/juju/juju/internal/relation"
	"github.com/juju/juju/internal/tools"
	stateerrors "github.com/juju/juju/state/errors"
)

// ExposedEndpoint encapsulates the expose-related details of a particular
// application endpoint with respect to the sources (CIDRs or space IDs) that
// should be able to access the ports opened by the application charm for an
// endpoint.
type ExposedEndpoint struct {
	// A list of spaces that should be able to reach the opened ports
	// for an exposed application's endpoint.
	ExposeToSpaceIDs []string `bson:"to-space-ids,omitempty"`

	// A list of CIDRs that should be able to reach the opened ports
	// for an exposed application's endpoint.
	ExposeToCIDRs []string `bson:"to-cidrs,omitempty"`
}

// AllowTrafficFromAnyNetwork returns true if the exposed endpoint parameters
// include the 0.0.0.0/0 CIDR.
func (exp ExposedEndpoint) AllowTrafficFromAnyNetwork() bool {
	for _, cidr := range exp.ExposeToCIDRs {
		if cidr == firewall.AllNetworksIPV4CIDR || cidr == firewall.AllNetworksIPV6CIDR {
			return true
		}
	}

	return false
}

// Application represents the state of an application.
type Application struct {
	st  *State
	doc applicationDoc
}

// applicationDoc represents the internal state of an application in MongoDB.
// Note the correspondence with ApplicationInfo in apiserver.
type applicationDoc struct {
	DocID       string `bson:"_id"`
	Name        string `bson:"name"`
	ModelUUID   string `bson:"model-uuid"`
	Subordinate bool   `bson:"subordinate"`
	// CharmURL should be moved to CharmOrigin. Attempting it should
	// be relatively straight forward, but very time consuming.
	// When moving to CharmHub from Juju it should be
	// tackled then.
	CharmURL    *string     `bson:"charmurl"`
	CharmOrigin CharmOrigin `bson:"charm-origin"`
	// CharmModifiedVersion changes will trigger the upgrade-charm hook
	// for units independent of charm url changes.
	CharmModifiedVersion int          `bson:"charmmodifiedversion"`
	ForceCharm           bool         `bson:"forcecharm"`
	Life                 Life         `bson:"life"`
	UnitCount            int          `bson:"unitcount"`
	RelationCount        int          `bson:"relationcount"`
	Tools                *tools.Tools `bson:",omitempty"`
	TxnRevno             int64        `bson:"txn-revno"`

	// Exposed is set to true when the application is exposed.
	Exposed bool `bson:"exposed"`

	// A map for tracking the per-endpoint expose-related parameters for
	// an exposed app where keys are endpoint names or the "" value which
	// represents all application endpoints.
	ExposedEndpoints map[string]ExposedEndpoint `bson:"exposed-endpoints,omitempty"`

	// CAAS related attributes.
	PasswordHash string `bson:"passwordhash"`

	// Placement is the placement directive that should be used allocating units/pods.
	Placement string `bson:"placement,omitempty"`
	// HasResources is set to false after an application has been removed
	// and any k8s cluster resources have been fully cleaned up.
	// Until then, the application must not be removed from the Juju model.
	HasResources bool `bson:"has-resources,omitempty"`
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

// Kind returns a human readable name identifying the application kind.
func (a *Application) Kind() string {
	return a.Tag().Kind()
}

// ApplicationTag returns the more specific ApplicationTag rather than the generic
// Tag.
func (a *Application) ApplicationTag() names.ApplicationTag {
	return names.NewApplicationTag(a.Name())
}

// applicationGlobalKey returns the global database key for the application
// with the given name.
func applicationGlobalKey(appName string) string {
	return appGlobalKeyPrefix + appName
}

// appGlobalKeyPrefix is the string we use to denote application kind.
const appGlobalKeyPrefix = "a#"

// globalKey returns the global database key for the application.
func (a *Application) globalKey() string {
	return applicationGlobalKey(a.doc.Name)
}

func applicationGlobalOperatorKey(appName string) string {
	return applicationGlobalKey(appName) + "#operator"
}

func applicationCharmConfigKey(appName string, curl *string) string {
	return fmt.Sprintf("a#%s#%s", appName, *curl)
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

func applicationStorageConstraintsKey(appName string, curl *string) string {
	return fmt.Sprintf("asc#%s#%s", appName, *curl)
}

// storageConstraintsKey returns the charm-version-specific storage
// constraints collection key for the application.
func (a *Application) storageConstraintsKey() string {
	return applicationStorageConstraintsKey(a.doc.Name, a.doc.CharmURL)
}

func applicationDeviceConstraintsKey(appName string, curl *string) string {
	return fmt.Sprintf("adc#%s#%s", appName, *curl)
}

// deviceConstraintsKey returns the charm-version-specific device
// constraints collection key for the application.
func (a *Application) deviceConstraintsKey() string {
	return applicationDeviceConstraintsKey(a.doc.Name, a.doc.CharmURL)
}

// Base returns the specified base for this charm.
func (a *Application) Base() Base {
	return Base{OS: a.doc.CharmOrigin.Platform.OS, Channel: a.doc.CharmOrigin.Platform.Channel}
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
func (a *Application) SetAgentVersion(v semversion.Binary) (err error) {
	defer errors.DeferredAnnotatef(&err, "setting agent version for application %q", a)
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
		return onAbort(err, stateerrors.ErrDead)
	}
	a.doc.Tools = versionedTool
	return nil
}

var errRefresh = stderrors.New("state seems inconsistent, refresh and try again")

// Destroy ensures that the application and all its relations will be removed at
// some point; if the application has no units, and no relation involving the
// application has any units in scope, they are all removed immediately.
func (a *Application) Destroy(store objectstore.ObjectStore) (err error) {
	op := a.DestroyOperation(store)
	defer func() {
		logger.Tracef(context.TODO(), "Application(%s).Destroy() => %v", a.doc.Name, err)
		if err == nil {
			// After running the destroy ops, app life is either Dying,
			// or it may be set to Dead. If removed, life will also be marked as Dead.
			a.doc.Life = op.PostDestroyAppLife
		}
	}()
	err = a.st.ApplyOperation(op)
	if len(op.Errors) != 0 {
		logger.Warningf(context.TODO(), "operational errors destroying application %v: %v", a.Name(), op.Errors)
	}
	return err
}

// DestroyOperation returns a model operation that will destroy the application.
func (a *Application) DestroyOperation(store objectstore.ObjectStore) *DestroyApplicationOperation {
	return &DestroyApplicationOperation{
		app:   &Application{st: a.st, doc: a.doc},
		store: store,
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

	// CleanupIgnoringResources is true if this operation has been
	// scheduled by a forced cleanup task.
	CleanupIgnoringResources bool

	// Removed is true if the application is removed during destroy.
	Removed bool

	// PostDestroyAppLife is the life of the app if destroy completes without error.
	PostDestroyAppLife Life

	// ForcedOperation stores needed information to force this operation.
	ForcedOperation

	// Store is the object store to use for blob access.
	store objectstore.ObjectStore
}

// Build is part of the ModelOperation interface.
func (op *DestroyApplicationOperation) Build(attempt int) ([]txn.Op, error) {
	if op.store == nil {
		return nil, errors.New("object store not set")
	}

	if attempt > 0 {
		if err := op.app.Refresh(); errors.Is(err, errors.NotFound) {
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
	ops, err := op.destroyOps(op.store)
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
			logger.Debugf(context.TODO(), "forcing application removal")
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
		// Only delete secrets after application is removed.
		if !op.Removed {
			return nil
		}

		// Reimplement in dqlite.
		//if err := op.deleteSecrets(); err != nil {
		//	logger.Errorf(context.TODO(), "cannot delete secrets for application %q: %v", op.app, err)
		//}
		return nil
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
func (op *DestroyApplicationOperation) destroyOps(store objectstore.ObjectStore) ([]txn.Op, error) {
	rels, err := op.app.Relations()
	if op.FatalError(err) {
		return nil, err
	}
	if len(rels) != op.app.doc.RelationCount {
		// This is just an early bail out. The relations obtained may still
		// be wrong, but that situation will be caught by a combination of
		// asserts on relationcount and on each known relation, below.
		logger.Tracef(context.TODO(), "DestroyApplicationOperation(%s).destroyOps mismatched relation count %d != %d",
			op.app.doc.Name, len(rels), op.app.doc.RelationCount)
		return nil, errRefresh
	}
	var ops []txn.Op
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
	op.PostDestroyAppLife = Dying
	if !op.Force && failedRels {
		return nil, op.LastError()
	}

	removeUnitAssignmentOps, err := op.app.removeUnitAssignmentsOps()
	if err != nil {
		return nil, errors.Trace(err)
	}
	ops = append(ops, removeUnitAssignmentOps...)

	// If the application has no units, and all its known relations will be
	// removed, the application can also be removed, so long as there are
	// no other cluster resources, as can be the case for k8s charms.
	if op.app.doc.UnitCount == 0 && op.app.doc.RelationCount == removeCount {
		logger.Tracef(context.TODO(), "DestroyApplicationOperation(%s).destroyOps removing application", op.app.doc.Name)
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
		op.Removed = true
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
		logger.Tracef(context.TODO(), "DestroyApplicationOperation(%s).destroyOps UnitCount == %d, queuing up unitCleanup",
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

func (a *Application) removeUnitAssignmentsOps() (ops []txn.Op, err error) {
	pattern := fmt.Sprintf("^%s:%s/[0-9]+$", a.st.ModelUUID(), a.Name())
	unitAssignments, err := a.st.unitAssignments(bson.D{
		{
			Name: "_id", Value: bson.D{
				{Name: "$regex", Value: pattern},
			},
		},
	})
	if err != nil {
		return nil, errors.Trace(err)
	}
	for _, unitAssignment := range unitAssignments {
		ops = append(ops, removeStagedAssignmentOp(a.st.docID(unitAssignment.Unit)))
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

	// Reimplement in dqlite.
	// Remove secret permissions.
	//secretScopedPermissionsOps, err := a.st.removeScopedSecretPermissionOps(a.Tag())
	//if op.FatalError(err) {
	//	return nil, errors.Trace(err)
	//}
	//ops = append(ops, secretScopedPermissionsOps...)
	//secretConsumerPermissionsOps, err := a.st.removeConsumerSecretPermissionOps(a.Tag())
	//if op.FatalError(err) {
	//	return nil, errors.Trace(err)
	//}
	//ops = append(ops, secretConsumerPermissionsOps...)
	//secretLabelOps, err := a.st.removeOwnerSecretLabelsOps(a.ApplicationTag())
	//if err != nil {
	//	return nil, errors.Trace(err)
	//}
	//ops = append(ops, secretLabelOps...)
	//
	//secretLabelOps, err = a.st.removeConsumerSecretLabelsOps(a.ApplicationTag())
	//if err != nil {
	//	return nil, errors.Trace(err)
	//}
	//ops = append(ops, secretLabelOps...)

	// Note that appCharmDecRefOps might not catch the final decref
	// when run in a transaction that decrefs more than once. So we
	// avoid attempting to do the final cleanup in the ref dec ops and
	// do it explicitly below.
	name := a.doc.Name
	curl := a.doc.CharmURL

	// By the time we get to here, all units and charm refs have been removed,
	// so it's safe to do this additional cleanup.
	ops = append(ops, finalAppCharmRemoveOps(name, curl)...)

	ops = append(ops, a.removeCloudServiceOps()...)
	globalKey := a.globalKey()
	ops = append(ops,
		removeEndpointBindingsOp(globalKey),
		removeConstraintsOp(globalKey),
		removeStatusOp(a.st, globalKey),
		removeStatusOp(a.st, applicationGlobalOperatorKey(name)),
		removeSettingsOp(settingsC, a.applicationConfigKey()),
		removeModelApplicationRefOp(a.st, name),
	)

	cancelCleanupOps, err := a.cancelScheduledCleanupOps()
	if err != nil {
		return nil, errors.Trace(err)
	}
	return append(ops, cancelCleanupOps...), nil
}

func (a *Application) cancelScheduledCleanupOps() ([]txn.Op, error) {
	appOrUnitPattern := bson.DocElem{
		Name: "prefix", Value: bson.D{
			{Name: "$regex", Value: fmt.Sprintf("^%s(/[0-9]+)*$", a.Name())},
		},
	}
	// No unit and app exists now, so cancel the below scheduled cleanup docs to avoid new resources of later deployment
	// getting removed accidentally because we re-use unit numbers for sidecar applications.
	cancelCleanupOpsArgs := []cancelCleanupOpsArg{
		{cleanupForceDestroyedUnit, appOrUnitPattern},
		{cleanupForceRemoveUnit, appOrUnitPattern},
		{cleanupForceApplication, appOrUnitPattern},
	}
	relations, err := a.Relations()
	if err != nil {
		return nil, errors.Trace(err)
	}
	for _, rel := range relations {
		cancelCleanupOpsArgs = append(cancelCleanupOpsArgs, cancelCleanupOpsArg{
			cleanupForceDestroyedRelation,
			bson.DocElem{
				Name: "prefix", Value: relationKey(rel.Endpoints())},
		})
	}

	cancelCleanupOps, err := a.st.cancelCleanupOps(cancelCleanupOpsArgs...)
	if err != nil {
		return nil, errors.Trace(err)
	}
	return cancelCleanupOps, nil
}

// IsExposed returns whether this application is exposed. The explicitly open
// ports (with open-port) for exposed applications may be accessed from machines
// outside of the local deployment network. See MergeExposeSettings and ClearExposed.
func (a *Application) IsExposed() bool {
	return a.doc.Exposed
}

// ExposedEndpoints returns a map where keys are endpoint names (or the ""
// value which represents all endpoints) and values are ExposedEndpoint
// instances that specify which sources (spaces or CIDRs) can access the
// opened ports for each endpoint once the application is exposed.
func (a *Application) ExposedEndpoints() map[string]ExposedEndpoint {
	if len(a.doc.ExposedEndpoints) == 0 {
		return nil
	}
	return a.doc.ExposedEndpoints
}

// UnsetExposeSettings removes the expose settings for the provided list of
// endpoint names. If the resulting exposed endpoints map for the application
// becomes empty after the settings are removed, the application will be
// automatically unexposed.
//
// An error will be returned if an unknown endpoint name is specified or there
// is no existing expose settings entry for any of the provided endpoint names.
//
// See ClearExposed and IsExposed.
func (a *Application) UnsetExposeSettings(exposedEndpoints []string) error {
	bindings, _, err := readEndpointBindings(a.st, a.globalKey())
	if err != nil {
		return errors.Trace(err)
	}

	mergedExposedEndpoints := make(map[string]ExposedEndpoint)
	for endpoint, exposeParams := range a.doc.ExposedEndpoints {
		mergedExposedEndpoints[endpoint] = exposeParams
	}

	for _, endpoint := range exposedEndpoints {
		// The empty endpoint ("") value represents all endpoints.
		if _, found := bindings[endpoint]; !found && endpoint != "" {
			return errors.NotFoundf("endpoint %q", endpoint)
		}

		if _, found := mergedExposedEndpoints[endpoint]; !found {
			return errors.BadRequestf("endpoint %q is not exposed", endpoint)
		}

		delete(mergedExposedEndpoints, endpoint)
	}

	return a.setExposed(
		// retain expose flag if we still have any expose settings left
		len(mergedExposedEndpoints) != 0,
		mergedExposedEndpoints,
	)
}

// MergeExposeSettings marks the application as exposed and merges the provided
// ExposedEndpoint details into the current set of expose settings. The merge
// operation will overwrites expose settings for each existing endpoint name.
//
// See ClearExposed and IsExposed.
func (a *Application) MergeExposeSettings(exposedEndpoints map[string]ExposedEndpoint) error {
	bindings, _, err := readEndpointBindings(a.st, a.globalKey())
	if err != nil {
		return errors.Trace(err)
	}

	mergedExposedEndpoints := make(map[string]ExposedEndpoint)
	for endpoint, exposeParams := range a.doc.ExposedEndpoints {
		mergedExposedEndpoints[endpoint] = exposeParams
	}

	for endpoint, exposeParams := range exposedEndpoints {
		// The empty endpoint ("") value represents all endpoints.
		if _, found := bindings[endpoint]; !found && endpoint != "" {
			return errors.NotFoundf("endpoint %q", endpoint)
		}

		// TODO(nvinuesa): When we move to dqlite we have to make sure
		// that the exposed spaces really exist.
		exposeParams.ExposeToCIDRs = uniqueSortedStrings(exposeParams.ExposeToCIDRs)
		for _, cidr := range exposeParams.ExposeToCIDRs {
			if _, _, err := net.ParseCIDR(cidr); err != nil {
				return errors.Annotatef(err, "unable to parse %q as a CIDR", cidr)
			}
		}

		// If no spaces and CIDRs are provided, assume an implicit
		// 0.0.0.0/0 CIDR. This matches the "expose to the entire
		// world" behavior in juju controllers prior to 2.9.
		if len(exposeParams.ExposeToSpaceIDs)+len(exposeParams.ExposeToCIDRs) == 0 {
			exposeParams.ExposeToCIDRs = []string{firewall.AllNetworksIPV4CIDR, firewall.AllNetworksIPV6CIDR}
		}

		mergedExposedEndpoints[endpoint] = exposeParams
	}

	return a.setExposed(true, mergedExposedEndpoints)
}

func uniqueSortedStrings(in []string) []string {
	if len(in) == 0 {
		return nil
	}

	return set.NewStrings(in...).SortedValues()
}

// ClearExposed removes the exposed flag from the application.
// See MergeExposeSettings and IsExposed.
func (a *Application) ClearExposed() error {
	return a.setExposed(false, nil)
}

func (a *Application) setExposed(exposed bool, exposedEndpoints map[string]ExposedEndpoint) (err error) {
	ops := []txn.Op{{
		C:      applicationsC,
		Id:     a.doc.DocID,
		Assert: isAliveDoc,
		Update: bson.D{{"$set", bson.D{
			{"exposed", exposed},
			{"exposed-endpoints", exposedEndpoints},
		}}},
	}}
	if err := a.st.db().RunTransaction(ops); err != nil {
		return errors.Errorf("cannot set exposed flag for application %q to %v: %v", a, exposed, onAbort(err, applicationNotAliveErr))
	}
	a.doc.Exposed = exposed
	a.doc.ExposedEndpoints = exposedEndpoints
	return nil
}

// Charm returns the application's charm and whether units should upgrade to that
// charm even if they are in an error state.
func (a *Application) Charm() (CharmRefFull, bool, error) {
	if a.doc.CharmURL == nil {
		return nil, false, errors.NotFoundf("charm for application %q", a.doc.Name)
	}
	parsedURL, err := charm.ParseURL(*a.doc.CharmURL)
	if err != nil {
		return nil, false, err
	}
	ch, err := a.st.findCharm(parsedURL)
	if err != nil {
		return nil, false, err
	}
	return ch, a.doc.ForceCharm, nil
}

// CharmOrigin returns the origin of a charm associated with a application.
func (a *Application) CharmOrigin() *CharmOrigin {
	return &a.doc.CharmOrigin
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

// CharmURL returns a string version of the application's charm URL, and
// whether units should upgrade to the charm with that URL even if they are
// in an error state.
func (a *Application) CharmURL() (*string, bool) {
	return a.doc.CharmURL, a.doc.ForceCharm
}

// Endpoints returns the application's currently available relation endpoints.
func (a *Application) Endpoints() (eps []relation.Endpoint, err error) {
	ch, _, err := a.Charm()
	if err != nil {
		return nil, err
	}
	collect := func(role charm.RelationRole, rels map[string]charm.Relation) {
		for _, rel := range rels {
			eps = append(eps, relation.Endpoint{
				ApplicationName: a.doc.Name,
				Relation:        rel,
			})
		}
	}

	meta := ch.Meta()
	if meta == nil {
		return nil, errors.Errorf("nil charm metadata for application %q", a.Name())
	}

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
func (a *Application) Endpoint(relationName string) (relation.Endpoint, error) {
	eps, err := a.Endpoints()
	if err != nil {
		return relation.Endpoint{}, err
	}
	for _, ep := range eps {
		if ep.Name == relationName {
			return ep, nil
		}
	}
	return relation.Endpoint{}, errors.Errorf("application %q has no %q relation", a, relationName)
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

func (a *Application) checkRelationsOps(ch CharmRefFull, relations []*Relation) ([]txn.Op, error) {
	asserts := make([]txn.Op, 0, len(relations))

	// All relations must still exist and their endpoints are implemented by the charm.
	for _, rel := range relations {
		if ep, err := rel.Endpoint(a.doc.Name); err != nil {
			return nil, err
		} else if !ep.ImplementedBy(ch.Meta()) {
			// When switching charms, we should allow peer
			// relations to be broken (e.g. because a newer charm
			// version removes a particular peer relation) even if
			// they are already established as those particular
			// relations will become irrelevant once the upgrade is
			// complete.
			if !isPeer(ep) {
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
	ch CharmRefFull,
	updatedSettings charm.Settings,
	forceUnits bool,
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
	} else if errors.Is(err, errors.NotFound) {
		// No old settings, start with the updated settings.
		newSettings = updatedSettings
	} else {
		return nil, errors.Annotatef(err, "application %q", a.doc.Name)
	}

	cURL := ch.URL()
	// Create or replace application settings.
	var settingsOp txn.Op
	newSettingsKey := applicationCharmConfigKey(a.doc.Name, &cURL)
	if _, err := readSettings(a.st.db(), settingsC, newSettingsKey); errors.Is(err, errors.NotFound) {
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

	// Build the transaction.
	var ops []txn.Op
	if oldKey != nil {
		// Old settings shouldn't change (when they exist).
		ops = append(ops, oldKey.assertUnchangedOp())
	}
	ops = append(ops, unitOps...)
	ops = append(ops, []txn.Op{
		// Create or replace new settings.
		settingsOp,
		// Update the charm URL and force flag (if relevant).
		{
			C:  applicationsC,
			Id: a.doc.DocID,
			Update: bson.D{{"$set", bson.D{
				{"charmurl", cURL},
				{"forcecharm", forceUnits},
			}}},
		},
	}...)
	ops = append(ops, storageConstraintsOps...)
	ops = append(ops, checkStorageOps...)
	ops = append(ops, upgradeStorageOps...)

	ops = append(ops, incCharmModifiedVersionOps(a.doc.DocID)...)

	// Get all relations - we need to check them later.
	relations, err := a.Relations()
	if err != nil {
		return nil, errors.Trace(err)
	}

	// Remove any stale peer relation entries when switching charms
	removeStalePeerOps, err := a.st.removeStalePeerRelationsOps(a.doc.Name, relations, ch.Meta())
	if err != nil {
		return nil, errors.Trace(err)
	}
	ops = append(ops, removeStalePeerOps...)

	// Add any extra peer relations that need creation.
	newPeers := a.extraPeerRelations(ch.Meta())
	addPeerOps, err := a.st.addPeerRelationsOps(a.doc.Name, newPeers)
	if err != nil {
		return nil, errors.Trace(err)
	}
	ops = append(ops, addPeerOps...)

	// Update the relation count as well.
	if len(newPeers) > 0 {
		// Make sure the relation count does not change.
		sameRelCount := bson.D{{"relationcount", len(relations)}}
		ops = append(ops, txn.Op{
			C:      applicationsC,
			Id:     a.doc.DocID,
			Assert: append(notDeadDoc, sameRelCount...),
			Update: bson.D{{"$inc", bson.D{{"relationcount", len(newPeers)}}}},
		})
	}
	// Check relations to ensure no active relations are removed.
	relOps, err := a.checkRelationsOps(ch, relations)
	if err != nil {
		return nil, errors.Trace(err)
	}
	ops = append(ops, relOps...)

	// And finally, decrement the old charm and settings.
	return ops, nil
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
			if errors.Is(err, errors.NotAssigned) {
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
	ch CharmRefFull,
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
	sb, err := NewStorageConfigBackend(a.st)
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
	if err := validateStorageConstraints(sb.storageBackend, newStorageConstraints, ch.Meta()); err != nil {
		return fail(errors.Annotate(err, "validating storage constraints"))
	}
	cURL := ch.URL()
	newStorageConstraintsKey := applicationStorageConstraintsKey(a.doc.Name, &cURL)
	if _, err := readStorageConstraints(sb.mb, newStorageConstraintsKey); errors.Is(err, errors.NotFound) {
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

	sb, err := NewStorageConfigBackend(a.st)
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
				a.st, meta, u, name, cons, countMin,
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

// SetCharmConfig contains the parameters for Application.SetCharm.
type SetCharmConfig struct {
	// Charm is the new charm to use for the application. New units
	// will be started with this charm, and existing units will be
	// upgraded to use it.
	Charm CharmRefFull

	// CharmOrigin is the data for where the charm comes from.  Eventually
	// Channel should be move there.
	CharmOrigin *CharmOrigin

	// ConfigSettings is the charm config settings to apply when upgrading
	// the charm.
	ConfigSettings charm.Settings

	// ForceUnits forces the upgrade on units in an error state.
	ForceUnits bool

	// ForceBase forces the use of the charm even if it is not one of
	// the charm's supported series.
	ForceBase bool

	// Force forces the overriding of the lxd profile validation even if the
	// profile doesn't validate.
	Force bool

	// PendingResourceIDs is a map of resource names to resource IDs to activate during
	// the upgrade.
	PendingResourceIDs map[string]string

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

func (a *Application) validateSetCharmConfig(cfg SetCharmConfig) error {
	if cfg.Charm.Meta().Subordinate != a.doc.Subordinate {
		return errors.Errorf("cannot change an application's subordinacy")
	}
	origin := cfg.CharmOrigin
	if origin == nil {
		return errors.NotValidf("nil charm origin")
	}
	if origin.Platform == nil {
		return errors.BadRequestf("charm origin platform is nil")
	}
	if (origin.ID != "" && origin.Hash == "") || (origin.ID == "" && origin.Hash != "") {
		return errors.BadRequestf("programming error, SetCharm, neither CharmOrigin ID nor Hash can be set before a charm is downloaded. See CharmHubRepository GetDownloadURL.")
	}

	return nil
}

// SetCharm changes the charm for the application.
func (a *Application) SetCharm(
	cfg SetCharmConfig,
	store objectstore.ObjectStore,
) (err error) {
	defer errors.DeferredAnnotatef(
		&err, "cannot upgrade application %q to charm %q", a, cfg.Charm.URL(),
	)

	// Validate the input. ValidateSettings validates and transforms
	// leaving it here.
	if err := a.validateSetCharmConfig(cfg); err != nil {
		return errors.Trace(err)
	}

	updatedSettings, err := cfg.Charm.Config().ValidateSettings(cfg.ConfigSettings)
	if err != nil {
		return errors.Annotate(err, "validating config settings")
	}

	var newCharmModifiedVersion int
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
			return nil, stateerrors.ErrDead
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

		if *a.doc.CharmURL == cfg.Charm.URL() {
			updates := bson.D{
				{"forcecharm", cfg.ForceUnits},
			}
			// Charm URL already set; just update the force flag.
			ops = append(ops, txn.Op{
				C:      applicationsC,
				Id:     a.doc.DocID,
				Assert: txn.DocExists,
				Update: bson.D{{"$set", updates}},
			})
		} else {
			// Check if the new charm specifies a relation max limit
			// that cannot be satisfied by the currently established
			// relation count.
			quotaErr := a.preUpgradeRelationLimitCheck(cfg.Charm)

			// If the operator specified --force, we still allow
			// the upgrade to continue with a warning.
			if errors.Is(quotaErr, errors.QuotaLimitExceeded) && cfg.Force {
				logger.Warningf(context.TODO(), "%v; allowing upgrade to proceed as the operator specified --force", quotaErr)
			} else if quotaErr != nil {
				return nil, errors.Trace(quotaErr)
			}

			chng, err := a.changeCharmOps(
				cfg.Charm,
				updatedSettings,
				cfg.ForceUnits,
				cfg.StorageConstraints,
			)
			if err != nil {
				return nil, errors.Trace(err)
			}
			ops = append(ops, chng...)
			newCharmModifiedVersion++
		}

		// Update the charm origin
		ops = append(ops, txn.Op{
			C:      applicationsC,
			Id:     a.doc.DocID,
			Assert: txn.DocExists,
			Update: bson.D{{"$set", bson.D{
				{"charm-origin", *cfg.CharmOrigin},
			}}},
		})

		// Always update bindings regardless of whether we upgrade to a
		// new version or stay at the previous version.
		currentMap, txnRevno, err := readEndpointBindings(a.st, a.globalKey())
		if err != nil && !errors.Is(err, errors.NotFound) {
			return ops, errors.Trace(err)
		}
		b, err := a.bindingsForOps(currentMap)
		if err != nil {
			return nil, errors.Trace(err)
		}
		endpointBindingsOps, err := b.updateOps(txnRevno, cfg.EndpointBindings, cfg.Charm.Meta(), cfg.Force)
		if err == nil {
			ops = append(ops, endpointBindingsOps...)
		} else if !errors.Is(err, errors.NotFound) && err != jujutxn.ErrNoOperations {
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
	return a.Refresh()
}

// preUpgradeRelationLimitCheck ensures that the already established relation
// counts do not violate the max relation limits specified by the charm version
// we are attempting to upgrade to.
func (a *Application) preUpgradeRelationLimitCheck(newCharm CharmRef) error {
	var (
		existingRels []*Relation
		err          error
	)

	for relName, relSpec := range newCharm.Meta().CombinedRelations() {
		if relSpec.Limit == 0 {
			continue
		}

		// Load and memoize relation list
		if existingRels == nil {
			if existingRels, err = a.Relations(); err != nil {
				return errors.Trace(err)
			}

		}

		establishedCount := establishedRelationCount(existingRels, a.Name(), relSpec)
		if establishedCount > relSpec.Limit {
			return errors.QuotaLimitExceededf("new charm version imposes a maximum relation limit of %d for %s:%s which cannot be satisfied by the number of already established relations (%d)", relSpec.Limit, a.Name(), relName, establishedCount)
		}
	}

	return nil
}

// establishedRelationCount returns the number of already established relations
// for appName and the endpoint specified in the provided relation details.
func establishedRelationCount(existingRelList []*Relation, appName string, rel charm.Relation) int {
	var establishedCount int
	for _, existingRel := range existingRelList {
		// Suspended relations don't count
		if existingRel.Suspended() {
			continue
		}

		for _, existingRelEp := range existingRel.Endpoints() {
			if existingRelEp.ApplicationName == appName &&
				existingRelEp.Relation.Name == rel.Name &&
				existingRelEp.Relation.Interface == rel.Interface {
				establishedCount++
				break
			}
		}
	}

	return establishedCount
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
		if err != nil && !errors.Is(err, errors.NotFound) {
			return nil, errors.Trace(err)
		}
		b, err := a.bindingsForOps(currentMap)
		if err != nil {
			return nil, errors.Trace(err)
		}
		endpointBindingsOps, err := b.updateOps(txnRevno, operatorBindings.Map(), ch.Meta(), force)
		if err != nil && !errors.Is(err, errors.NotFound) && err != jujutxn.ErrNoOperations {
			return nil, errors.Trace(err)
		}

		return endpointBindingsOps, err
	}

	err := a.st.db().Run(buildTxn)
	return errors.Annotatef(err, "merging application bindings")
}

// unitAppName returns the name of the Application, given a Unit's name.
func unitAppName(unitName string) string {
	unitParts := strings.Split(unitName, "/")
	return unitParts[0]
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
		scons, err := a.constraints()
		if errors.Is(err, errors.NotFound) {
			return "", nil, errors.NotFoundf("application %q", a.Name())
		}
		if err != nil {
			return "", nil, errors.Trace(err)
		}
		cons, err = a.st.ResolveConstraints(scons)
		if err != nil {
			return "", nil, errors.Trace(err)
		}
		// If the application is deployed to the controller model and the charm
		// has the special juju- prefix to its name, then bypass the machineID
		// empty check.
		if args.machineID != "" && a.st.IsController() {
			curl, err := charm.ParseURL(*a.doc.CharmURL)
			if err != nil {
				return "", nil, errors.Trace(err)
			}
			if !strings.HasPrefix(curl.Name, "juju-") {
				return "", nil, errors.NotSupportedf("non-empty machineID")
			}
		} else if args.machineID != "" {
			return "", nil, errors.NotSupportedf("non-empty machineID")
		}
	}
	storageCons, err := a.StorageConstraints()
	if err != nil {
		return "", nil, errors.Trace(err)
	}
	uNames, ops, err := a.addUnitOpsWithCons(
		applicationAddUnitOpsArgs{
			cons:               cons,
			principalName:      principalName,
			principalMachineID: args.machineID,
			storageCons:        storageCons,
			attachStorage:      args.AttachStorage,
			providerId:         args.ProviderId,
			address:            args.Address,
			ports:              args.Ports,
			unitName:           args.UnitName,
			passwordHash:       args.PasswordHash,
			charmMeta:          args.CharmMeta,
		},
	)
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
	providerId   *string
	address      *string
	ports        *[]string
	unitName     *string
	passwordHash *string

	// We need charm Meta to add the unit storage and we can't retrieve it
	// from the legacy state so we must pass it here.
	charmMeta *charm.Meta
}

// addUnitOpsWithCons is a helper method for returning addUnitOps.
func (a *Application) addUnitOpsWithCons(
	args applicationAddUnitOpsArgs,
) (string, []txn.Op, error) {
	if a.doc.Subordinate && args.principalName == "" {
		return "", nil, errors.New("application is a subordinate")
	} else if !a.doc.Subordinate && args.principalName != "" {
		return "", nil, errors.New("application is not a subordinate")
	}
	var name string
	if args.unitName != nil {
		name = *args.unitName
	} else {
		newName, err := a.newUnitName()
		if err != nil {
			return "", nil, errors.Trace(err)
		}
		name = newName
	}
	unitTag := names.NewUnitTag(name)

	storageOps, numStorageAttachments, err := a.addUnitStorageOps(
		args, unitTag,
	)
	if err != nil {
		return "", nil, errors.Trace(err)
	}

	docID := a.st.docID(name)
	globalKey := unitGlobalKey(name)
	agentGlobalKey := unitAgentGlobalKey(name)
	platform := a.CharmOrigin().Platform
	base := Base{OS: platform.OS, Channel: platform.Channel}.Normalise()
	udoc := &unitDoc{
		DocID:                  docID,
		Name:                   name,
		Application:            a.doc.Name,
		Base:                   base,
		Life:                   Alive,
		Principal:              args.principalName,
		MachineId:              args.principalMachineID,
		StorageAttachmentCount: numStorageAttachments,
	}
	if args.passwordHash != nil {
		udoc.PasswordHash = *args.passwordHash
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
		StatusInfo: status.MessageInstallingAgent,
		Updated:    now.UnixNano(),
	}

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
				networkAddr := network.NewSpaceAddress(*args.address, network.WithScope(network.ScopeMachineLocal))
				addr := fromNetworkAddress(networkAddr, network.OriginProvider)
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
	return name, ops, nil
}

func (a *Application) addUnitStorageOps(
	args applicationAddUnitOpsArgs,
	unitTag names.UnitTag,
) ([]txn.Op, int, error) {
	sb, err := NewStorageConfigBackend(a.st)
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
	platform := a.CharmOrigin().Platform
	storageOps, storageTags, numStorageAttachments, err := createStorageOps(
		a.st,
		sb,
		unitTag,
		args.charmMeta,
		args.storageCons,
		platform.OS,
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
			a.st,
			si,
			unitTag,
			platform.OS,
			args.charmMeta,
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
		charmStorage := args.charmMeta.Storage[name]
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

	// UnitName is for CAAS models when creating stateful units.
	UnitName *string

	// machineID is only passed in if the unit being created is
	// a subordinate and refers to the machine that is hosting the principal.
	machineID string

	// PasswordHash is only passed for CAAS sidecar units on creation.
	PasswordHash *string

	// We need charm Meta to add the unit storage and we can't retrieve it
	// from the legacy state so we must pass it here.
	CharmMeta *charm.Meta
}

// AddUnit adds a new principal unit to the application.
func (a *Application) AddUnit(
	args AddUnitParams,
) (unit *Unit, err error) {
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

// UpsertCAASUnitParams is passed to UpsertCAASUnit to describe how to create or how to find and
// update an existing unit for sidecar CAAS application.
type UpsertCAASUnitParams struct {
	AddUnitParams

	// OrderedScale is always true. It represents a mapping of OrderedId to Unit ID.
	OrderedScale bool
	// OrderedId is the stable ordinal index of the "pod".
	OrderedId int

	// ObservedAttachedVolumeIDs is the filesystem attachments observed to be attached by the infrastructure,
	// used to map existing attachments.
	ObservedAttachedVolumeIDs []string
}

func (a *Application) UpsertCAASUnit(args UpsertCAASUnitParams) (*Unit, error) {
	if args.PasswordHash == nil {
		return nil, errors.NotValidf("password hash")
	}
	if args.ProviderId == nil {
		return nil, errors.NotValidf("provider id")
	}
	if !args.OrderedScale {
		return nil, errors.NewNotImplemented(nil, "upserting CAAS units not supported without ordered unit IDs")
	}
	if args.UnitName == nil {
		return nil, errors.NotValidf("nil unit name")
	}

	sb, err := NewStorageBackend(a.st)
	if err != nil {
		return nil, errors.Trace(err)
	}

	var unit *Unit
	err = a.st.db().Run(func(attempt int) ([]txn.Op, error) {
		if attempt > 0 {
			err := a.Refresh()
			if err != nil {
				return nil, errors.Trace(err)
			}
		}

		if args.UnitName != nil {
			var err error
			if unit == nil {
				unit, err = a.st.Unit(*args.UnitName)
			} else {
				err = unit.Refresh()
			}
			if errors.Is(err, errors.NotFound) {
				unit = nil
			} else if err != nil {
				return nil, errors.Trace(err)
			}
		}

		// Try to reattach the storage that k8s has observed attached to this pod.
		for _, volumeId := range args.ObservedAttachedVolumeIDs {
			volume, err := sb.volume(bson.D{{"info.volumeid", volumeId}}, "")
			if errors.Is(err, errors.NotFound) {
				continue
			} else if err != nil {
				return nil, errors.Trace(err)
			}

			volumeStorageId, err := volume.StorageInstance()
			if errors.Is(err, errors.NotAssigned) {
				continue
			} else if err != nil {
				return nil, errors.Trace(err)
			}

			args.AddUnitParams.AttachStorage = append(args.AddUnitParams.AttachStorage, volumeStorageId)
		}

		if unit == nil {
			return a.insertCAASUnitOps(args)
		}

		if unit.Life() == Dead {
			return nil, errors.AlreadyExistsf("dead unit %q", unit.Tag().Id())
		}

		updateOps, err := unit.UpdateOperation(UnitUpdateProperties{
			ProviderId: args.ProviderId,
			Address:    args.Address,
			Ports:      args.Ports,
		}).Build(attempt)
		if err != nil {
			return nil, errors.Trace(err)
		}

		var ops []txn.Op
		if args.PasswordHash != nil {
			ops = append(ops, unit.setPasswordHashOps(*args.PasswordHash)...) // setPasswordHashOps asserts notDead
		} else {
			ops = append(ops, txn.Op{
				C:      unitsC,
				Id:     unit.doc.DocID,
				Assert: notDeadDoc,
			})
		}
		ops = append(ops, updateOps...)
		return ops, nil
	})
	if err != nil {
		return nil, err
	}
	if unit == nil {
		unit, err = a.st.Unit(*args.UnitName)
		if err != nil {
			return nil, err
		}
	} else {
		err = unit.Refresh()
		if err != nil {
			return nil, err
		}
	}
	return unit, nil
}

func (a *Application) insertCAASUnitOps(
	args UpsertCAASUnitParams,
) ([]txn.Op, error) {
	if args.UnitName == nil {
		return nil, errors.NotValidf("nil unit name")
	}

	// We write scale info to dqlite now - this check is done there instead.
	// The rest of the code needs to be kept for now.
	//if ps := a.ProvisioningState(); args.OrderedId >= a.GetScale() ||
	//	(ps != nil && ps.Scaling && args.OrderedId >= ps.ScaleTarget) {
	//	return nil, errors.NotAssignedf("unrequired unit %s is", *args.UnitName)
	//}

	_, addOps, err := a.addUnitOps("", args.AddUnitParams, nil)
	if err != nil {
		return nil, errors.Trace(err)
	}

	ops := []txn.Op{{
		C:  applicationsC,
		Id: a.doc.DocID,
		Assert: bson.D{
			{"life", Alive},
		},
	}}
	ops = append(ops, addOps...)
	return ops, nil
}

// removeUnitOps returns the operations necessary to remove the supplied unit,
// assuming the supplied asserts apply to the unit document.
// When 'force' is set, this call will always return some needed operations
// and accumulate all operational errors encountered in the operation.
// If the 'force' is not set, any error will be fatal and no operations will be returned.
func (a *Application) removeUnitOps(store objectstore.ObjectStore, u *Unit, asserts bson.D, op *ForcedOperation, destroyStorage bool) ([]txn.Op, error) {
	hostOps, err := u.destroyHostOps(a, op)
	if op.FatalError(err) {
		return nil, errors.Trace(err)
	}

	// Reimplement in dqlite.
	//secretScopedPermissionsOps, err := a.st.removeScopedSecretPermissionOps(u.Tag())
	//if op.FatalError(err) {
	//	return nil, errors.Trace(err)
	//}
	//secretConsumerPermissionsOps, err := a.st.removeConsumerSecretPermissionOps(u.Tag())
	//if op.FatalError(err) {
	//	return nil, errors.Trace(err)
	//}
	//secretOwnerLabelOps, err := a.st.removeOwnerSecretLabelsOps(u.Tag())
	//if op.FatalError(err) {
	//	return nil, errors.Trace(err)
	//}
	//secretConsumerLabelOps, err := a.st.removeConsumerSecretLabelsOps(u.Tag())
	//if op.FatalError(err) {
	//	return nil, errors.Trace(err)
	//}

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
		removeStatusOp(a.st, u.globalAgentKey()),
		removeStatusOp(a.st, u.globalKey()),
		removeStatusOp(a.st, u.globalWorkloadVersionKey()),
		removeUnitStateOp(a.st, u.globalKey()),
		removeStatusOp(a.st, u.globalCloudContainerKey()),
		removeConstraintsOp(u.globalAgentKey()),
		newCleanupOp(cleanupRemovedUnit, u.doc.Name, op.Force),
	}
	ops = append(ops, hostOps...)

	m, err := a.st.Model()
	if err != nil {
		return nil, errors.Trace(err)
	}
	if m.Type() == ModelTypeCAAS {
		ops = append(ops, u.removeCloudContainerOps()...)
		ops = append(ops, newCleanupOp(cleanupDyingUnitResources, u.doc.Name, op.Force, op.MaxWait))
	}

	sb, err := NewStorageBackend(a.st)
	if err != nil {
		return nil, errors.Trace(err)
	}
	storageInstanceOps, err := removeStorageInstancesOps(sb, u.Tag(), op.Force)
	if op.FatalError(err) {
		return nil, errors.Trace(err)
	}
	ops = append(ops, storageInstanceOps...)

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
	return matchingRelations(a.st, a.doc.Name)
}

// matchingRelations returns all relations matching the application(s)/endpoint(s) provided
// There must be 1 or 2 supplied names, of the form <application>[:<endpoint>]
func matchingRelations(st *State, names ...string) (relations []*Relation, err error) {
	defer errors.DeferredAnnotatef(&err, "can't get relations matching %q", strings.Join(names, " "))
	relationsCollection, closer := st.db().GetCollection(relationsC)
	defer closer()

	var conditions []bson.D
	for _, name := range names {
		appName, relName, err := splitEndpointName(name)
		if err != nil {
			return nil, err
		}
		if relName == "" {
			conditions = append(conditions, bson.D{{"endpoints.applicationname", appName}})
		} else {
			conditions = append(conditions, bson.D{{"endpoints", bson.D{{"$elemMatch", bson.D{
				{"applicationname", appName},
				{"relation.name", relName},
			}}}}})
		}
	}

	docs := []relationDoc{}
	err = relationsCollection.Find(bson.D{{
		"$and", conditions,
	}}).All(&docs)

	if err != nil {
		return nil, err
	}
	for _, v := range docs {
		relations = append(relations, newRelation(st, &v))
	}
	return relations, nil
}

// CharmConfig returns the raw user configuration for the application's charm.
func (a *Application) CharmConfig() (charm.Settings, error) {
	if a.doc.CharmURL == nil {
		return nil, fmt.Errorf("application charm not set")
	}

	s, err := charmSettingsWithDefaults(a.st, a.doc.CharmURL, a.Name())
	return s, errors.Annotatef(err, "charm config for application %q", a.doc.Name)
}

func charmSettingsWithDefaults(st *State, cURL *string, appName string) (charm.Settings, error) {
	key := applicationCharmConfigKey(appName, cURL)
	cfg, err := readSettings(st.db(), settingsC, key)
	if err != nil {
		return nil, errors.Trace(err)
	}

	ch, err := st.Charm(*cURL)
	if err != nil {
		return nil, errors.Trace(err)
	}

	result := ch.Config().DefaultSettings()
	for name, value := range cfg.Map() {
		result[name] = value
	}
	return result, nil
}

// UpdateCharmConfig changes a application's charm config settings. Values set
// to nil will be deleted; unknown and invalid values will return an error.
func (a *Application) UpdateCharmConfig(changes charm.Settings) error {
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

	return errors.Trace(a.updateMasterConfig(current, changes))
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

// ApplicationConfig returns the configuration for the application itself.
func (a *Application) ApplicationConfig() (config.ConfigAttributes, error) {
	cfg, err := readSettings(a.st.db(), settingsC, a.applicationConfigKey())
	if err != nil {
		if errors.Is(err, errors.NotFound) {
			return config.ConfigAttributes{}, nil
		}
		return nil, errors.Annotatef(err, "application config for application %q", a.doc.Name)
	}

	if len(cfg.Keys()) == 0 {
		return config.ConfigAttributes{}, nil
	}
	return cfg.Map(), nil
}

// UpdateApplicationConfig changes an application's config settings.
// Unknown and invalid values will return an error.
func (a *Application) UpdateApplicationConfig(
	changes config.ConfigAttributes,
	reset []string,
	schema configschema.Fields,
	defaults schema.Defaults,
) error {
	node, err := readSettings(a.st.db(), settingsC, a.applicationConfigKey())
	if errors.Is(err, errors.NotFound) {
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
	newConfig, err := config.NewConfig(node.Map(), schema, defaults)
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

var ErrSubordinateConstraints = stderrors.New("constraints do not apply to subordinate applications")

// Constraints returns the current application constraints.
func (a *Application) constraints() (constraints.Value, error) {
	if a.doc.Subordinate {
		return constraints.Value{}, ErrSubordinateConstraints
	}
	return readConstraints(a.st, a.globalKey())
}

// SetConstraints replaces the current application constraints.
func (a *Application) SetConstraints(cons constraints.Value) (err error) {
	unsupported, err := a.st.validateConstraints(cons)
	if len(unsupported) > 0 {
		logger.Warningf(context.TODO(),
			"setting constraints on application %q: unsupported constraints: %v", a.Name(), strings.Join(unsupported, ","))
	} else if err != nil {
		return err
	}

	if a.doc.Subordinate {
		return ErrSubordinateConstraints
	}

	// If the architecture has already been set, do not allow the application
	// architecture to change.
	//
	// If the constraints returns a not found error, we don't actually care,
	// this implies that it's never been set and we want to just take all the
	// valid constraints.
	if current, consErr := a.constraints(); !errors.Is(consErr, errors.NotFound) {
		if consErr != nil {
			return errors.Annotate(consErr, "unable to read constraints")
		}
		// If the incoming arch has a value we only care about that. If the
		// value is empty we can assume that we want the existing current value
		// that is set or not.
		if cons.Arch != nil && *cons.Arch != "" {
			if (current.Arch == nil || *current.Arch == "") && *cons.Arch != arch.DefaultArchitecture {
				return errors.NotSupportedf("changing architecture")
			} else if current.Arch != nil && *current.Arch != "" && *current.Arch != *cons.Arch {
				return errors.NotSupportedf("changing architecture (%s)", *current.Arch)
			}
		}
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
	if err != nil && !errors.Is(err, errors.NotFound) {
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

// StorageConstraints returns the storage constraints for the application.
func (a *Application) StorageConstraints() (map[string]StorageConstraints, error) {
	cons, err := readStorageConstraints(a.st, a.storageConstraintsKey())
	if errors.Is(err, errors.NotFound) {
		return nil, nil
	} else if err != nil {
		return nil, errors.Annotatef(err, "application %q", a.doc.Name)
	}
	return cons, nil
}

// DeviceConstraints returns the device constraints for the application.
func (a *Application) DeviceConstraints() (map[string]DeviceConstraints, error) {
	cons, err := readDeviceConstraints(a.st, a.deviceConstraintsKey())
	if errors.Is(err, errors.NotFound) {
		return nil, nil
	} else if err != nil {
		return nil, errors.Trace(err)
	}
	return cons, nil
}

type addApplicationOpsArgs struct {
	applicationDoc    *applicationDoc
	statusDoc         statusDoc
	constraints       constraints.Value
	storage           map[string]StorageConstraints
	devices           map[string]DeviceConstraints
	applicationConfig map[string]interface{}
	charmConfig       map[string]interface{}
	operatorStatus    *statusDoc
}

// addApplicationOps returns the operations required to add an application to the
// applications collection, along with all the associated expected other application
// entries. This method is used by both the *State.AddApplication method and the
// migration import code.
func addApplicationOps(mb modelBackend, app *Application, args addApplicationOpsArgs) ([]txn.Op, error) {

	globalKey := app.globalKey()
	charmConfigKey := app.charmConfigKey()
	applicationConfigKey := app.applicationConfigKey()
	storageConstraintsKey := app.storageConstraintsKey()
	deviceConstraintsKey := app.deviceConstraintsKey()

	ops := []txn.Op{
		createConstraintsOp(globalKey, args.constraints),
		createStorageConstraintsOp(storageConstraintsKey, args.storage),
		createDeviceConstraintsOp(deviceConstraintsKey, args.devices),
		createSettingsOp(settingsC, charmConfigKey, args.charmConfig),
		createSettingsOp(settingsC, applicationConfigKey, args.applicationConfig),
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

	ops = append(ops, txn.Op{
		C:      applicationsC,
		Id:     app.Name(),
		Assert: txn.DocMissing,
		Insert: args.applicationDoc,
	})
	return ops, nil
}

// SetPassword sets the password for the application's agent.
// TODO(caas) - consider a separate CAAS application entity
func (a *Application) SetPassword(password string) error {
	if len(password) < internalpassword.MinAgentPasswordLength {
		return fmt.Errorf("password is only %d bytes long, and is not a valid Agent password", len(password))
	}
	passwordHash := internalpassword.AgentPasswordHash(password)
	ops := []txn.Op{{
		C:      applicationsC,
		Id:     a.doc.DocID,
		Assert: notDeadDoc,
		Update: bson.D{{"$set", bson.D{{"passwordhash", passwordHash}}}},
	}}
	err := a.st.db().RunTransaction(ops)
	if err != nil {
		return fmt.Errorf("cannot set password of application %q: %v", a, onAbort(err, stateerrors.ErrDead))
	}
	a.doc.PasswordHash = passwordHash
	return nil
}

// PasswordValid returns whether the given password is valid
// for the given application.
func (a *Application) PasswordValid(password string) bool {
	agentHash := internalpassword.AgentPasswordHash(password)
	return agentHash == a.doc.PasswordHash
}

// UnitUpdateProperties holds information used to update
// the state model for the unit.
type UnitUpdateProperties struct {
	ProviderId           *string
	Address              *string
	Ports                *[]string
	UnitName             *string
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
	for _, txnOp := range all {
		switch nextOps, err := txnOp.Build(attempt); err {
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
func (a *Application) AddOperation(
	props UnitUpdateProperties,
) *AddUnitOperation {
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
		UnitName:   op.props.UnitName,
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

// finalAppCharmRemoveOps returns operations to delete the settings
// and storage, device constraints documents and queue a charm cleanup.
func finalAppCharmRemoveOps(appName string, curl *string) []txn.Op {
	settingsKey := applicationCharmConfigKey(appName, curl)
	removeSettingsOp := txn.Op{
		C:      settingsC,
		Id:     settingsKey,
		Remove: true,
	}
	// ensure removing storage constraints doc
	storageConstraintsKey := applicationStorageConstraintsKey(appName, curl)
	removeStorageConstraintsOp := removeStorageConstraintsOp(storageConstraintsKey)
	// ensure removing device constraints doc
	deviceConstraintsKey := applicationDeviceConstraintsKey(appName, curl)
	removeDeviceConstraintsOp := removeDeviceConstraintsOp(deviceConstraintsKey)

	return []txn.Op{removeSettingsOp, removeStorageConstraintsOp, removeDeviceConstraintsOp}
}
