// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state

import (
	"encoding/json"
	"fmt"
	"sort"
	"time"

	"github.com/juju/charm/v13"
	"github.com/juju/collections/set"
	"github.com/juju/errors"
	"github.com/juju/mgo/v3"
	"github.com/juju/mgo/v3/bson"
	"github.com/juju/mgo/v3/txn"
	"github.com/juju/names/v5"
	jujutxn "github.com/juju/txn/v3"
	"gopkg.in/macaroon.v2"

	"github.com/juju/juju/core/crossmodel"
	"github.com/juju/juju/core/status"
)

// RemoteApplication represents the state of an application hosted
// in an external (remote) model.
type RemoteApplication struct {
	st  *State
	doc remoteApplicationDoc
}

// remoteApplicationDoc represents the internal state of a remote application in MongoDB.
type remoteApplicationDoc struct {
	DocID                string              `bson:"_id"`
	Name                 string              `bson:"name"`
	OfferUUID            string              `bson:"offer-uuid"`
	URL                  string              `bson:"url,omitempty"`
	SourceControllerUUID string              `bson:"source-controller-uuid"`
	SourceModelUUID      string              `bson:"source-model-uuid"`
	Endpoints            []remoteEndpointDoc `bson:"endpoints"`
	Life                 Life                `bson:"life"`
	RelationCount        int                 `bson:"relationcount"`
	IsConsumerProxy      bool                `bson:"is-consumer-proxy"`
	Version              int                 `bson:"version"`
	Macaroon             string              `bson:"macaroon,omitempty"`
}

// remoteEndpointDoc represents the internal state of a remote application endpoint in MongoDB.
type remoteEndpointDoc struct {
	Name      string              `bson:"name"`
	Role      charm.RelationRole  `bson:"role"`
	Interface string              `bson:"interface"`
	Limit     int                 `bson:"limit"`
	Scope     charm.RelationScope `bson:"scope"`
}

func newRemoteApplication(st *State, doc *remoteApplicationDoc) *RemoteApplication {
	app := &RemoteApplication{
		st:  st,
		doc: *doc,
	}
	return app
}

// remoteAppGlobalKeyPrefix is the string we use to denote
// remoteApplication kind.
const remoteAppGlobalKeyPrefix = "c#"

// remoteApplicationGlobalKey returns the global database key for the
// remote application with the given name.
//
// This seems like an aggressively cryptic prefix, but apparently the
// all-watcher requires that global keys have single letter prefixes
// and r and a were taken.
// TODO(babbageclunk): check whether this is still the case.
func remoteApplicationGlobalKey(appName string) string {
	return remoteAppGlobalKeyPrefix + appName
}

// globalKey returns the global database key for the remote application.
func (a *RemoteApplication) globalKey() string {
	return remoteApplicationGlobalKey(a.doc.Name)
}

// IsRemote returns true for a remote application.
func (a *RemoteApplication) IsRemote() bool {
	return true
}

// SourceModel returns the tag of the model to which the application belongs.
func (a *RemoteApplication) SourceModel() names.ModelTag {
	return names.NewModelTag(a.doc.SourceModelUUID)
}

// SourceController returns the UUID of the controller hosting the application.
func (a *RemoteApplication) SourceController() string {
	return a.doc.SourceControllerUUID
}

// IsConsumerProxy returns the application is created
// from a registration operation by a consuming model.
func (a *RemoteApplication) IsConsumerProxy() bool {
	return a.doc.IsConsumerProxy
}

// ConsumeVersion is incremented each time a new consumer proxy
// is created for an offer.
func (a *RemoteApplication) ConsumeVersion() int {
	return a.doc.Version
}

// Name returns the application name.
func (a *RemoteApplication) Name() string {
	return a.doc.Name
}

// OfferUUID returns the offer UUID.
func (a *RemoteApplication) OfferUUID() string {
	return a.doc.OfferUUID
}

// URL returns the remote application URL, and a boolean indicating whether or not
// a URL is known for the remote application. A URL will only be available for the
// consumer of an offered application.
func (a *RemoteApplication) URL() (string, bool) {
	return a.doc.URL, a.doc.URL != ""
}

// Token returns the token for the remote application, provided by the remote
// model to identify the application in future communications.
func (a *RemoteApplication) Token() (string, error) {
	r := a.st.RemoteEntities()
	return r.GetToken(a.Tag())
}

// Tag returns a name identifying the application.
func (a *RemoteApplication) Tag() names.Tag {
	return names.NewApplicationTag(a.Name())
}

// Kind returns a human readable name identifying the remote application kind.
func (a *RemoteApplication) Kind() string {
	return "remote-application"
}

// Life returns whether the application is Alive, Dying or Dead.
func (a *RemoteApplication) Life() Life {
	return a.doc.Life
}

// StatusHistory returns a slice of at most filter.Size StatusInfo items
// or items as old as filter.Date or items newer than now - filter.Delta time
// representing past statuses for this remote application.
func (a *RemoteApplication) StatusHistory(filter status.StatusHistoryFilter) ([]status.StatusInfo, error) {
	args := &statusHistoryArgs{
		db:        a.st.db(),
		globalKey: a.globalKey(),
		filter:    filter,
		clock:     a.st.clock(),
	}
	return statusHistory(args)
}

// DestroyOperation returns a model operation to destroy remote application.
func (a *RemoteApplication) DestroyOperation(force bool) *DestroyRemoteApplicationOperation {
	return &DestroyRemoteApplicationOperation{
		app:             &RemoteApplication{st: a.st, doc: a.doc},
		ForcedOperation: ForcedOperation{Force: force},
	}
}

// DestroyRemoteApplicationOperation is a model operation to destroy a remote application.
type DestroyRemoteApplicationOperation struct {
	// ForcedOperation stores needed information to force this operation.
	ForcedOperation

	// app holds the remote application to destroy.
	app *RemoteApplication
}

// Build is part of the ModelOperation interface.
func (op *DestroyRemoteApplicationOperation) Build(attempt int) ([]txn.Op, error) {
	if attempt > 0 {
		if err := op.app.Refresh(); errors.Is(err, errors.NotFound) {
			return nil, jujutxn.ErrNoOperations
		} else if err != nil {
			return nil, err
		}
	}
	// When 'force' is set on the operation, this call will return needed operations
	// and accumulate all operational errors encountered in the operation.
	// If the 'force' is not set, any error will be fatal and no operations will be returned.
	switch ops, err := op.destroyOps(); err {
	case errRefresh:
	case errAlreadyDying:
		return nil, jujutxn.ErrNoOperations
	case nil:
		return ops, nil
	default:
		if op.Force {
			logger.Warningf("force destroy saas application %v despite error %v", op.app, err)
			return ops, nil
		}
		return nil, err
	}
	return nil, jujutxn.ErrNoOperations
}

// Done is part of the ModelOperation interface.
func (op *DestroyRemoteApplicationOperation) Done(err error) error {
	// NOTE(tsm): if you change the business logic here, check
	//            that RemoveOfferOperation is modified to suit
	if err != nil {
		if !op.Force {
			return errors.Annotatef(err, "cannot destroy saas application %q", op.app)
		}
		op.AddError(errors.Errorf("force destroy of saas application %v failed but proceeded despite encountering ERROR %v", op.app, err))
	}
	if err := op.eraseHistory(); err != nil {
		if !op.Force {
			logger.Errorf("cannot delete history for saas application %q: %v", op.app, err)
		}
		op.AddError(errors.Errorf("force erase saas application %q history proceeded despite encountering ERROR %v", op.app, err))
	}
	if err := op.deleteSecretReferences(); err != nil {
		logger.Errorf("cannot delete secret references for saas application %q: %v", op.app, err)
	}
	return nil
}

func (op *DestroyRemoteApplicationOperation) eraseHistory() error {
	var stop <-chan struct{} // stop not used here yet.
	if err := eraseStatusHistory(stop, op.app.st, op.app.globalKey()); err != nil {
		one := errors.Annotate(err, "saas application")
		if op.FatalError(one) {
			return one
		}
	}
	return nil
}

func (op *DestroyRemoteApplicationOperation) deleteSecretReferences() error {
	if err := op.app.st.removeRemoteSecretConsumer(op.app.Name()); err != nil {
		return errors.Annotatef(err, "deleting secret consumer records for %q", op.app.Name())
	}
	return nil
}

// DestroyWithForce in addition to doing what Destroy() does,
// when force is passed in as 'true', forces th destruction of remote application,
// ignoring errors.
func (a *RemoteApplication) DestroyWithForce(force bool, maxWait time.Duration) (opErrs []error, err error) {
	defer func() {
		if err == nil {
			a.doc.Life = Dying
		}
	}()
	op := a.DestroyOperation(force)
	op.MaxWait = maxWait
	err = a.st.ApplyOperation(op)
	return op.Errors, err
}

// Destroy ensures that this remote application reference and all its relations
// will be removed at some point; if no relation involving the
// application has any units in scope, they are all removed immediately.
func (a *RemoteApplication) Destroy() error {
	errs, err := a.DestroyWithForce(false, time.Duration(0))
	if len(errs) != 0 {
		logger.Warningf("operational errors destroying saas application %v: %v", a.Name(), errs)
	}
	return err
}

// destroyOps returns the operations required to destroy the application. If it
// returns errRefresh, the application should be refreshed and the destruction
// operations recalculated.
// When 'force' is set, this call will return needed operations
// and accumulate all operational errors encountered in the operation.
// If the 'force' is not set, any error will be fatal and no operations will be returned.
func (op *DestroyRemoteApplicationOperation) destroyOps() (ops []txn.Op, err error) {
	if op.app.doc.Life == Dying {
		if !op.Force {
			return nil, errAlreadyDying
		}
	}
	haveRels := true
	rels, err := op.app.Relations()
	if op.FatalError(err) {
		return nil, errors.Trace(err)
	}
	if err != nil {
		haveRels = false
	}

	// We'll need status below when processing relations.
	statusInfo, statusErr := op.app.Status()
	if op.FatalError(statusErr) && !errors.Is(statusErr, errors.NotFound) {
		return nil, statusErr
	}
	// If the application is already terminated and dead, the removal
	// can be short circuited.
	forceTerminate := op.Force || statusInfo.Status == status.Terminated

	if !forceTerminate && haveRels && len(rels) != op.app.doc.RelationCount {
		// This is just an early bail out. The relations obtained may still
		// be wrong, but that situation will be caught by a combination of
		// asserts on relationcount and on each known relation, below.
		return nil, errRefresh
	}

	op.ForcedOperation.Force = forceTerminate
	removeCount := 0
	if haveRels {
		failRels := false
		for _, rel := range rels {
			// If the remote app has been terminated, we may have been offline
			// and not noticed so need to clean up any exiting relation units.
			destroyRelUnitOps, err := destroyCrossModelRelationUnitsOps(&op.ForcedOperation, op.app, rel, true)
			if err != nil && err != jujutxn.ErrNoOperations {
				return nil, errors.Trace(err)
			}
			ops = append(ops, destroyRelUnitOps...)
			// When 'force' is set, this call will return both needed operations
			// as well as all operational errors encountered.
			// If the 'force' is not set, any error will be fatal and no operations will be returned.
			relOps, isRemove, err := rel.destroyOps(op.app.doc.Name, &op.ForcedOperation)
			if err == errAlreadyDying {
				relOps = []txn.Op{{
					C:      relationsC,
					Id:     rel.doc.DocID,
					Assert: bson.D{{"life", Dying}},
				}}
			} else if err != nil {
				op.AddError(err)
				failRels = true
				continue
			}
			if isRemove {
				removeCount++
			}
			ops = append(ops, relOps...)
		}
		if !op.Force && failRels {
			return nil, errors.Trace(op.LastError())
		}
	}
	// If all of the application's known relations will be
	// removed, the application can also be removed.
	if forceTerminate || op.app.doc.RelationCount == removeCount {
		var hasLastRefs bson.D
		if !forceTerminate {
			hasLastRefs = bson.D{{"life", Alive}, {"relationcount", removeCount}}
		}
		removeOps, err := op.app.removeOps(hasLastRefs)
		if err != nil {
			return nil, errors.Trace(err)
		}
		ops = append(ops, removeOps...)
		return ops, nil
	}
	// In all other cases, application removal will be handled as a consequence
	// of the removal of the relation referencing it. If any  relations have
	// been removed, they'll be caught by the operations collected above;
	// but if any has been added, we need to abort and add  a destroy op for
	// that relation too.
	// In combination, it's enough to check for count equality:
	// an add/remove will not touch the count, but  will be caught by
	// virtue of being a remove.
	notLastRefs := bson.D{
		{"life", Alive},
		{"relationcount", op.app.doc.RelationCount},
	}
	update := bson.D{{"$set", bson.D{{"life", Dying}}}}
	if removeCount != 0 {
		decref := bson.D{{"$inc", bson.D{{"relationcount", -removeCount}}}}
		update = append(update, decref...)
	}
	ops = append(ops, txn.Op{
		C:      remoteApplicationsC,
		Id:     op.app.doc.DocID,
		Assert: notLastRefs,
		Update: update,
	})
	return ops, nil
}

// removeOps returns the operations required to remove the application. Supplied
// asserts will be included in the operation on the application document.
func (a *RemoteApplication) removeOps(asserts bson.D) ([]txn.Op, error) {
	r := a.st.RemoteEntities()
	ops := []txn.Op{
		{
			C:      remoteApplicationsC,
			Id:     a.doc.DocID,
			Assert: asserts,
			Remove: true,
		},
		removeStatusOp(a.st, a.globalKey()),
	}
	tokenOps := r.removeRemoteEntityOps(a.Tag())
	ops = append(ops, tokenOps...)

	secretConsumerPermissionsOps, err := a.st.removeConsumerSecretPermissionOps(a.Tag())
	if err != nil {
		return nil, errors.Annotatef(err, "deleting secret consumer records for %q", a.Name())
	}
	ops = append(ops, secretConsumerPermissionsOps...)

	return ops, nil
}

// Status returns the status of the remote application.
func (a *RemoteApplication) Status() (status.StatusInfo, error) {
	return getStatus(a.st.db(), a.globalKey(), fmt.Sprintf("saas application %q", a.doc.Name))
}

// SetStatus sets the status for the application.
func (a *RemoteApplication) SetStatus(info status.StatusInfo, recorder status.StatusHistoryRecorder) error {
	// We only care about status for alive apps; we want to
	// avoid stray updates from the other model.
	if a.Life() != Alive {
		return nil
	}
	if !info.Status.KnownWorkloadStatus() {
		return errors.Errorf("cannot set invalid status %q", info.Status)
	}

	return setStatus(a.st.db(), setStatusParams{
		badge:      fmt.Sprintf("saas application %q", a.doc.Name),
		statusKind: a.Kind(),
		statusId:   a.Name(),
		globalKey:  a.globalKey(),
		status:     info.Status,
		message:    info.Message,
		rawData:    info.Data,
		updated:    timeOrNow(info.Since, a.st.clock()),
	}, recorder)
}

// TerminateOperation returns a ModelOperation that will terminate this
// remote application when applied, ensuring that all units have left
// scope as well.
func (a *RemoteApplication) TerminateOperation(message string) ModelOperation {
	return &terminateRemoteApplicationOperation{
		app: a,
		doc: statusDoc{
			Status:     status.Terminated,
			StatusInfo: message,
			Updated:    a.st.clock().Now().UnixNano(),
		},
		recorder: status.NoopStatusHistoryRecorder,
	}
}

type terminateRemoteApplicationOperation struct {
	app      *RemoteApplication
	doc      statusDoc
	recorder status.StatusHistoryRecorder
}

// Build is part of ModelOperation.
func (op *terminateRemoteApplicationOperation) Build(attempt int) ([]txn.Op, error) {
	if attempt > 0 {
		err := op.app.Refresh()
		if err != nil && !errors.Is(err, errors.NotFound) {
			return nil, errors.Trace(err)
		}
		if err != nil || op.app.Life() == Dead {
			return nil, jujutxn.ErrNoOperations
		}
	}
	ops, err := statusSetOps(op.app.st.db(), op.doc, op.app.globalKey())
	if err != nil {
		return nil, errors.Annotate(err, "setting status")
	}
	// Strictly speaking, we should transition through Dying state.
	ops = append(ops, txn.Op{
		C:      remoteApplicationsC,
		Id:     op.app.doc.DocID,
		Assert: notDeadDoc,
		Update: bson.D{{"$set", bson.D{{"life", Dying}}}},
	})
	name := op.app.Name()
	logger.Debugf("leaving scope on all %q relation units", name)
	rels, err := op.app.Relations()
	if err != nil {
		return nil, errors.Annotatef(err, "getting relations for %q", name)
	}
	// Termination happens when the offer has disappeared so we can force destroy any
	// relations on the consuming side.
	// Destroying each relation also forces remote units to leave scope.
	for _, rel := range rels {
		relOps, err := destroyCrossModelRelationUnitsOps(&ForcedOperation{Force: true}, op.app, rel, false)
		if err != nil && err != jujutxn.ErrNoOperations {
			return nil, errors.Annotatef(err, "removing relation %q", rel)
		}
		ops = append(ops, relOps...)
	}
	return ops, nil
}

// Done is part of ModelOperation.
func (op *terminateRemoteApplicationOperation) Done(err error) error {
	if err != nil {
		return errors.Annotatef(err, "terminating saas application %q", op.app.Name())
	}
	_, _ = probablyUpdateStatusHistory(op.app.st.db(),
		op.app.Kind(), op.app.Name(), op.app.globalKey(), op.doc, op.recorder)
	// Set the life to Dead so that the lifecycle watcher will trigger to inform the
	// relevant workers that this application is gone.
	ops := []txn.Op{{
		C:      remoteApplicationsC,
		Id:     op.app.doc.DocID,
		Update: bson.D{{"$set", bson.D{{"life", Dead}}}},
	}}
	return op.app.st.db().RunTransaction(ops)
}

// Endpoints returns the application's currently available relation endpoints.
func (a *RemoteApplication) Endpoints() ([]Endpoint, error) {
	return remoteEndpointDocsToEndpoints(a.Name(), a.doc.Endpoints), nil
}

func remoteEndpointDocsToEndpoints(applicationName string, docs []remoteEndpointDoc) []Endpoint {
	eps := make([]Endpoint, len(docs))
	for i, ep := range docs {
		eps[i] = Endpoint{
			ApplicationName: applicationName,
			Relation: charm.Relation{
				Name:      ep.Name,
				Role:      ep.Role,
				Interface: ep.Interface,
				Limit:     ep.Limit,
				Scope:     ep.Scope,
			}}
	}
	sort.Sort(epSlice(eps))
	return eps
}

// Endpoint returns the relation endpoint with the supplied name, if it exists.
func (a *RemoteApplication) Endpoint(relationName string) (Endpoint, error) {
	eps, err := a.Endpoints()
	if err != nil {
		return Endpoint{}, err
	}
	for _, ep := range eps {
		if ep.Name == relationName {
			return ep, nil
		}
	}
	return Endpoint{}, fmt.Errorf("saas application %q has no %q relation", a, relationName)
}

// AddEndpoints adds the specified endpoints to the remote application.
// If an endpoint with the same name already exists, an error is returned.
// If the endpoints change during the update, the operation is retried.
func (a *RemoteApplication) AddEndpoints(eps []charm.Relation) error {
	newEps := make([]remoteEndpointDoc, len(eps))
	for i, ep := range eps {
		newEps[i] = remoteEndpointDoc{
			Name:      ep.Name,
			Role:      ep.Role,
			Interface: ep.Interface,
			Limit:     ep.Limit,
			Scope:     ep.Scope,
		}
	}

	model, err := a.st.Model()
	if err != nil {
		return errors.Trace(err)
	} else if model.Life() != Alive {
		return errors.Errorf("model is no longer alive")
	}

	checkCompatibleEndpoints := func(currentEndpoints []Endpoint) error {
		// Ensure there are no current endpoints with the same name as
		// any of those we want to update.
		currentEndpointNames := set.NewStrings()
		for _, ep := range currentEndpoints {
			currentEndpointNames.Add(ep.Name)
		}
		for _, r := range eps {
			if currentEndpointNames.Contains(r.Name) {
				return errors.AlreadyExistsf("endpoint %v", r.Name)
			}
		}
		return nil
	}

	currentEndpoints, err := a.Endpoints()
	if err != nil {
		return errors.Trace(err)
	}
	if err := checkCompatibleEndpoints(currentEndpoints); err != nil {
		return err
	}
	applicationID := a.st.docID(a.Name())
	buildTxn := func(attempt int) ([]txn.Op, error) {
		// If we've tried once already and failed, check that
		// model may have been destroyed.
		if attempt > 0 {
			if err := checkModelActive(a.st); err != nil {
				return nil, errors.Trace(err)
			}
			if err = a.Refresh(); err != nil {
				return nil, errors.Trace(err)
			}
			currentEndpoints, err = a.Endpoints()
			if err != nil {
				return nil, errors.Trace(err)
			}
			if err := checkCompatibleEndpoints(currentEndpoints); err != nil {
				return nil, err
			}
		}
		ops := []txn.Op{
			model.assertActiveOp(),
			{
				C:  remoteApplicationsC,
				Id: applicationID,
				Assert: bson.D{
					{"endpoints", bson.D{{
						"$not", bson.D{{
							"$elemMatch", bson.D{{
								"$in", newEps}},
						}},
					}}},
				},
				Update: bson.D{
					{"$addToSet", bson.D{{"endpoints", bson.D{{"$each", newEps}}}}},
				},
			},
		}
		return ops, nil
	}
	if err := a.st.db().Run(buildTxn); err != nil {
		return errors.Trace(err)
	}
	return a.Refresh()
}

// SetSourceController updates the source controller attribute.
func (a *RemoteApplication) SetSourceController(sourceControllerUUID string) error {
	model, err := a.st.Model()
	if err != nil {
		return errors.Trace(err)
	} else if model.Life() != Alive {
		return errors.Errorf("model is no longer alive")
	}

	applicationID := a.st.docID(a.Name())
	buildTxn := func(attempt int) ([]txn.Op, error) {
		// If we've tried once already and failed, check that
		// model may have been destroyed.
		if attempt > 0 {
			if model.Life() == Dead {
				return nil, errors.Errorf("model %q is %s", model.Name(), model.Life().String())
			}
			if err = a.Refresh(); err != nil {
				return nil, errors.Trace(err)
			}
		}
		ops := []txn.Op{
			{
				C:      modelsC,
				Id:     model.UUID(),
				Assert: notDeadDoc,
			}, {
				C:      remoteApplicationsC,
				Id:     applicationID,
				Assert: txn.DocExists,
				Update: bson.D{
					{"$set", bson.D{{"source-controller-uuid", sourceControllerUUID}}},
				},
			},
		}
		return ops, nil
	}
	if err := a.st.db().Run(buildTxn); err != nil {
		return errors.Trace(err)
	}
	return a.Refresh()
}

func (a *RemoteApplication) Macaroon() (*macaroon.Macaroon, error) {
	if a.doc.Macaroon == "" {
		return nil, nil
	}
	var mac macaroon.Macaroon
	err := json.Unmarshal([]byte(a.doc.Macaroon), &mac)
	if err != nil {
		return nil, errors.Trace(err)
	}
	return &mac, nil
}

// String returns the application name.
func (a *RemoteApplication) String() string {
	return a.doc.Name
}

// Refresh refreshes the contents of the RemoteApplication from the underlying
// state. It returns an error that satisfies errors.IsNotFound if the
// application has been removed.
func (a *RemoteApplication) Refresh() error {
	applications, closer := a.st.db().GetCollection(remoteApplicationsC)
	defer closer()

	err := applications.FindId(a.doc.DocID).One(&a.doc)
	if err == mgo.ErrNotFound {
		return errors.NotFoundf("saas application %q", a)
	}
	if err != nil {
		return fmt.Errorf("cannot refresh application %q: %v", a, err)
	}
	return nil
}

// Relations returns a Relation for every relation the application is in.
func (a *RemoteApplication) Relations() (relations []*Relation, err error) {
	return matchingRelations(a.st, a.doc.Name)
}

// AddRemoteApplicationParams contains the parameters for adding a remote application
// to the model.
type AddRemoteApplicationParams struct {
	// Name is the name to give the remote application. This does not have to
	// match the application name in the URL, or the name in the remote model.
	Name string

	// OfferUUID is the UUID of the offer.
	OfferUUID string

	// URL is either empty, or the URL that the remote application was offered
	// with on the hosting model.
	URL string

	// ExternalControllerUUID, if set, is the UUID of the controller other
	// than this one, which is hosting the offer.
	ExternalControllerUUID string

	// SourceModel is the tag of the model to which the remote application belongs.
	SourceModel names.ModelTag

	// Token is an opaque string that identifies the remote application in the
	// source model.
	Token string

	// Endpoints describes the endpoints that the remote application implements.
	Endpoints []charm.Relation

	// IsConsumerProxy is true when a remote application is created as a result
	// of a registration operation from a remote model.
	IsConsumerProxy bool

	// ConsumeVersion is incremented each time a new consumer proxy
	// is created for an offer.
	ConsumeVersion int

	// Macaroon is used for authentication on the offering side.
	Macaroon *macaroon.Macaroon
}

// Validate returns an error if there's a problem with the
// parameters being used to create a remote application.
func (p AddRemoteApplicationParams) Validate() error {
	if !names.IsValidApplication(p.Name) {
		return errors.NotValidf("name %q", p.Name)
	}
	if p.URL != "" {
		// URL may be empty, to represent remote applications corresponding
		// to consumers of an offered application.
		if _, err := crossmodel.ParseOfferURL(p.URL); err != nil {
			return errors.Annotate(err, "validating offer URL")
		}
	}
	if p.SourceModel == (names.ModelTag{}) {
		return errors.NotValidf("empty source model tag")
	}
	return nil
}

// AddRemoteApplication creates a new remote application record,
// having the supplied relation endpoints, with the supplied name,
// which must be unique across all applications, local and remote.
func (st *State) AddRemoteApplication(args AddRemoteApplicationParams) (_ *RemoteApplication, err error) {
	defer errors.DeferredAnnotatef(&err, "cannot add saas application %q", args.Name)

	// Sanity checks.
	if err := args.Validate(); err != nil {
		return nil, errors.Trace(err)
	}
	model, err := st.Model()
	if err != nil {
		return nil, errors.Trace(err)
	} else if model.Life() != Alive {
		return nil, errors.Errorf("model is no longer alive")
	}

	var macJSON string
	if args.Macaroon != nil {
		b, err := json.Marshal(args.Macaroon)
		if err != nil {
			return nil, errors.Trace(err)
		}
		macJSON = string(b)
	}
	applicationID := st.docID(args.Name)
	// Create the application addition operations.
	appDoc := &remoteApplicationDoc{
		DocID:                applicationID,
		Name:                 args.Name,
		SourceControllerUUID: args.ExternalControllerUUID,
		SourceModelUUID:      args.SourceModel.Id(),
		URL:                  args.URL,
		Life:                 Alive,
		IsConsumerProxy:      args.IsConsumerProxy,
		Version:              args.ConsumeVersion,
		Macaroon:             macJSON,
	}
	if !args.IsConsumerProxy {
		if appDoc.Version, err = sequenceWithMin(st, args.OfferUUID, 1); err != nil {
			return nil, errors.Trace(err)
		}
		appDoc.OfferUUID = args.OfferUUID
	}
	eps := make([]remoteEndpointDoc, len(args.Endpoints))
	for i, ep := range args.Endpoints {
		eps[i] = remoteEndpointDoc{
			Name:      ep.Name,
			Role:      ep.Role,
			Interface: ep.Interface,
			Limit:     ep.Limit,
			Scope:     ep.Scope,
		}
	}
	appDoc.Endpoints = eps
	app := newRemoteApplication(st, appDoc)

	buildTxn := func(attempt int) ([]txn.Op, error) {
		// If we've tried once already and failed, check that
		// model may have been destroyed.
		if attempt > 0 {
			if err := checkModelActive(st); err != nil {
				return nil, errors.Trace(err)
			}
			// Ensure a local application with the same name doesn't exist.
			if localExists, err := isNotDead(st, applicationsC, args.Name); err != nil {
				return nil, errors.Trace(err)
			} else if localExists {
				return nil, errors.AlreadyExistsf("local application with same name")
			}
			// Ensure a remote application with the same name doesn't exist.
			if exists, err := isNotDead(st, remoteApplicationsC, args.Name); err != nil {
				return nil, errors.Trace(err)
			} else if exists {
				return nil, errors.AlreadyExistsf("saas application")
			}
		}
		ops := []txn.Op{
			model.assertActiveOp(),
			{
				C:      remoteApplicationsC,
				Id:     appDoc.Name,
				Assert: txn.DocMissing,
				Insert: appDoc,
			}, {
				C:      applicationsC,
				Id:     appDoc.Name,
				Assert: txn.DocMissing,
			},
		}
		if !args.IsConsumerProxy {
			statusDoc := statusDoc{
				ModelUUID: st.ModelUUID(),
				Status:    status.Unknown,
				Updated:   st.clock().Now().UnixNano(),
			}
			ops = append(ops, createStatusOp(st, app.globalKey(), statusDoc))
		}
		// If we know the token, import it.
		if args.Token != "" {
			importRemoteEntityOps := st.RemoteEntities().importRemoteEntityOps(app.Tag(), args.Token)
			ops = append(ops, importRemoteEntityOps...)
		}
		return ops, nil
	}
	if err = st.db().Run(buildTxn); err != nil {
		return nil, errors.Trace(err)
	}
	return app, nil
}

// RemoteApplication returns a remote application state by name.
func (st *State) RemoteApplication(name string) (_ *RemoteApplication, err error) {
	if !names.IsValidApplication(name) {
		return nil, errors.NotValidf("saas application name %q", name)
	}

	applications, closer := st.db().GetCollection(remoteApplicationsC)
	defer closer()

	appDoc := &remoteApplicationDoc{}
	err = applications.FindId(name).One(appDoc)
	if err == mgo.ErrNotFound {
		return nil, errors.NotFoundf("saas application %q", name)
	}
	if err != nil {
		return nil, errors.Annotatef(err, "cannot get saas application %q", name)
	}
	return newRemoteApplication(st, appDoc), nil
}

// AllRemoteApplications returns all the remote applications used by the model.
func (st *State) AllRemoteApplications() (applications []*RemoteApplication, err error) {
	applicationsCollection, closer := st.db().GetCollection(remoteApplicationsC)
	defer closer()

	appDocs := []remoteApplicationDoc{}
	err = applicationsCollection.Find(bson.D{}).All(&appDocs)
	if err != nil {
		return nil, errors.Errorf("cannot get all saas applications")
	}
	for _, v := range appDocs {
		applications = append(applications, newRemoteApplication(st, &v))
	}
	return applications, nil
}
