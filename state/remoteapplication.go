// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state

import (
	"fmt"
	"sort"
	"time"

	"github.com/juju/errors"
	jujutxn "github.com/juju/txn"
	"gopkg.in/juju/charm.v6-unstable"
	"gopkg.in/juju/names.v2"
	"gopkg.in/mgo.v2"
	"gopkg.in/mgo.v2/bson"
	"gopkg.in/mgo.v2/txn"

	"github.com/juju/juju/core/crossmodel"
	"github.com/juju/juju/status"
)

// RemoteApplication represents the state of an application hosted
// in an external (remote) model.
type RemoteApplication struct {
	st  *State
	doc remoteApplicationDoc
}

// remoteApplicationDoc represents the internal state of a remote application in MongoDB.
type remoteApplicationDoc struct {
	DocID           string              `bson:"_id"`
	Name            string              `bson:"name"`
	URL             string              `bson:"url,omitempty"`
	SourceModelUUID string              `bson:"source-model-uuid"`
	Endpoints       []remoteEndpointDoc `bson:"endpoints"`
	Life            Life                `bson:"life"`
	RelationCount   int                 `bson:"relationcount"`
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

// remoteApplicationGlobalKey returns the global database key for the
// remote application with the given name.
func remoteApplicationGlobalKey(appName string) string {
	return "c#" + appName
}

// globalKey returns the global database key for the remote application.
func (s *RemoteApplication) globalKey() string {
	return remoteApplicationGlobalKey(s.doc.Name)
}

// IsRemote returns true for a remote application.
func (s *RemoteApplication) IsRemote() bool {
	return true
}

// SourceModel returns the tag of the model to which the application belongs.
func (s *RemoteApplication) SourceModel() names.ModelTag {
	return names.NewModelTag(s.doc.SourceModelUUID)
}

// Name returns the application name.
func (s *RemoteApplication) Name() string {
	return s.doc.Name
}

// URL returns the remote service URL, and a boolean indicating whether or not
// a URL is known for the remote service. A URL will only be available for the
// consumer of an offered service.
func (s *RemoteApplication) URL() (string, bool) {
	return s.doc.URL, s.doc.URL != ""
}

// Token returns the token for the remote application, provided by the remote
// model to identify the service in future communications.
func (s *RemoteApplication) Token() (string, error) {
	r := s.st.RemoteEntities()
	return r.GetToken(s.SourceModel(), s.Tag())
}

// Tag returns a name identifying the application.
func (s *RemoteApplication) Tag() names.Tag {
	return names.NewApplicationTag(s.Name())
}

// Life returns whether the application is Alive, Dying or Dead.
func (s *RemoteApplication) Life() Life {
	return s.doc.Life
}

// Destroy ensures that this remote application reference and all its relations
// will be removed at some point; if no relation involving the
// application has any units in scope, they are all removed immediately.
func (s *RemoteApplication) Destroy() (err error) {
	defer errors.DeferredAnnotatef(&err, "cannot destroy remote application %q", s)
	defer func() {
		if err == nil {
			s.doc.Life = Dying
		}
	}()
	app := &RemoteApplication{st: s.st, doc: s.doc}
	buildTxn := func(attempt int) ([]txn.Op, error) {
		if attempt > 0 {
			if err := app.Refresh(); errors.IsNotFound(err) {
				return nil, jujutxn.ErrNoOperations
			} else if err != nil {
				return nil, err
			}
		}
		switch ops, err := app.destroyOps(); err {
		case errRefresh:
		case errAlreadyDying:
			return nil, jujutxn.ErrNoOperations
		case nil:
			return ops, nil
		default:
			return nil, err
		}
		return nil, jujutxn.ErrTransientFailure
	}
	return s.st.run(buildTxn)
}

// destroyOps returns the operations required to destroy the application. If it
// returns errRefresh, the application should be refreshed and the destruction
// operations recalculated.
func (s *RemoteApplication) destroyOps() ([]txn.Op, error) {
	if s.doc.Life == Dying {
		return nil, errAlreadyDying
	}
	rels, err := s.Relations()
	if err != nil {
		return nil, err
	}
	if len(rels) != s.doc.RelationCount {
		// This is just an early bail out. The relations obtained may still
		// be wrong, but that situation will be caught by a combination of
		// asserts on relationcount and on each known relation, below.
		return nil, errRefresh
	}
	var ops []txn.Op
	removeCount := 0
	for _, rel := range rels {
		relOps, isRemove, err := rel.destroyOps(s.doc.Name)
		if err == errAlreadyDying {
			relOps = []txn.Op{{
				C:      relationsC,
				Id:     rel.doc.DocID,
				Assert: bson.D{{"life", Dying}},
			}}
		} else if err != nil {
			return nil, err
		}
		if isRemove {
			removeCount++
		}
		ops = append(ops, relOps...)
	}
	// If all of the application's known relations will be
	// removed, the application can also be removed.
	if s.doc.RelationCount == removeCount {
		hasLastRefs := bson.D{{"life", Alive}, {"relationcount", removeCount}}
		return append(ops, s.removeOps(hasLastRefs)...), nil
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
		{"relationcount", s.doc.RelationCount},
	}
	update := bson.D{{"$set", bson.D{{"life", Dying}}}}
	if removeCount != 0 {
		decref := bson.D{{"$inc", bson.D{{"relationcount", -removeCount}}}}
		update = append(update, decref...)
	}
	return append(ops, txn.Op{
		C:      remoteApplicationsC,
		Id:     s.doc.DocID,
		Assert: notLastRefs,
		Update: update,
	}), nil
}

// removeOps returns the operations required to remove the application. Supplied
// asserts will be included in the operation on the application document.
func (s *RemoteApplication) removeOps(asserts bson.D) []txn.Op {
	r := s.st.RemoteEntities()
	ops := []txn.Op{
		{
			C:      remoteApplicationsC,
			Id:     s.doc.DocID,
			Assert: asserts,
			Remove: true,
		},
		removeStatusOp(s.st, s.globalKey()),
		r.removeRemoteEntityOp(s.SourceModel(), s.Tag()),
	}
	return ops
}

// Status returns the status of the remote application.
func (s *RemoteApplication) Status() (status.StatusInfo, error) {
	return getStatus(s.st, s.globalKey(), "remote application")
}

// SetStatus sets the status for the application.
func (s *RemoteApplication) SetStatus(info status.StatusInfo) error {
	if !info.Status.KnownWorkloadStatus() {
		return errors.Errorf("cannot set invalid status %q", info.Status)
	}
	return setStatus(s.st, setStatusParams{
		badge:     "remote application",
		globalKey: s.globalKey(),
		status:    info.Status,
		message:   info.Message,
		rawData:   info.Data,
		updated:   info.Since,
	})
}

// Endpoints returns the application's currently available relation endpoints.
func (s *RemoteApplication) Endpoints() ([]Endpoint, error) {
	return remoteEndpointDocsToEndpoints(s.Name(), s.doc.Endpoints), nil
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
func (s *RemoteApplication) Endpoint(relationName string) (Endpoint, error) {
	eps, err := s.Endpoints()
	if err != nil {
		return Endpoint{}, err
	}
	for _, ep := range eps {
		if ep.Name == relationName {
			return ep, nil
		}
	}
	return Endpoint{}, fmt.Errorf("remote application %q has no %q relation", s, relationName)
}

// String returns the application name.
func (s *RemoteApplication) String() string {
	return s.doc.Name
}

// Refresh refreshes the contents of the RemoteApplication from the underlying
// state. It returns an error that satisfies errors.IsNotFound if the
// application has been removed.
func (s *RemoteApplication) Refresh() error {
	applications, closer := s.st.getCollection(remoteApplicationsC)
	defer closer()

	err := applications.FindId(s.doc.DocID).One(&s.doc)
	if err == mgo.ErrNotFound {
		return errors.NotFoundf("remote application %q", s)
	}
	if err != nil {
		return fmt.Errorf("cannot refresh application %q: %v", s, err)
	}
	return nil
}

// Relations returns a Relation for every relation the application is in.
func (s *RemoteApplication) Relations() (relations []*Relation, err error) {
	return applicationRelations(s.st, s.doc.Name)
}

// AddRemoteApplicationParams contains the parameters for adding a remote service
// to the environment.
type AddRemoteApplicationParams struct {
	// Name is the name to give the remote application. This does not have to
	// match the application name in the URL, or the name in the remote model.
	Name string

	// URL is either empty, or the URL that the remote application was offered
	// with.
	URL string

	// SourceModel is the tag of the model to which the remote application belongs.
	SourceModel names.ModelTag

	// Token is an opaque string that identifies the remote application in the
	// source model.
	Token string

	// Endpoints describes the endpoints that the remote application implements.
	Endpoints []charm.Relation
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
		if _, err := crossmodel.ParseApplicationURL(p.URL); err != nil {
			return errors.Annotate(err, "validating application URL")
		}
	}
	if p.SourceModel == (names.ModelTag{}) {
		return errors.NotValidf("empty source model tag")
	}
	return nil
}

// AddRemoteApplication creates a new remote application record, having the supplied relation endpoints,
// with the supplied name (which must be unique across all applications, local and remote).
func (st *State) AddRemoteApplication(args AddRemoteApplicationParams) (_ *RemoteApplication, err error) {
	defer errors.DeferredAnnotatef(&err, "cannot add remote application %q", args.Name)

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

	applicationID := st.docID(args.Name)
	// Create the application addition operations.
	appDoc := &remoteApplicationDoc{
		DocID:           applicationID,
		Name:            args.Name,
		SourceModelUUID: args.SourceModel.Id(),
		URL:             args.URL,
		Life:            Alive,
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
	statusDoc := statusDoc{
		ModelUUID:  st.ModelUUID(),
		Status:     status.Unknown,
		StatusInfo: "waiting for remote connection",
		Updated:    time.Now().UnixNano(),
	}

	importRemoteEntityOps := st.RemoteEntities().importRemoteEntityOps(
		args.SourceModel, app.Tag(), args.Token,
	)
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
				return nil, errors.AlreadyExistsf("remote application")
			}
		}
		ops := []txn.Op{
			model.assertActiveOp(),
			createStatusOp(st, app.globalKey(), statusDoc),
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
		ops = append(ops, importRemoteEntityOps...)
		return ops, nil
	}
	if err = st.run(buildTxn); err != nil {
		return nil, errors.Trace(err)
	}
	return app, nil
}

// RemoteApplication returns a remote application state by name.
func (st *State) RemoteApplication(name string) (_ *RemoteApplication, err error) {
	if !names.IsValidApplication(name) {
		return nil, errors.NotValidf("remote application name %q", name)
	}

	applications, closer := st.getCollection(remoteApplicationsC)
	defer closer()

	appDoc := &remoteApplicationDoc{}
	err = applications.FindId(name).One(appDoc)
	if err == mgo.ErrNotFound {
		return nil, errors.NotFoundf("remote application %q", name)
	}
	if err != nil {
		return nil, errors.Annotatef(err, "cannot get remote application %q", name)
	}
	return newRemoteApplication(st, appDoc), nil
}

// RemoteApplicationByToken returns a remote application state by token.
func (st *State) RemoteApplicationByToken(token string) (_ *RemoteApplication, err error) {
	apps, closer := st.getCollection(remoteApplicationsC)
	defer closer()

	appDoc := &remoteApplicationDoc{}
	err = apps.Find(bson.D{{"token", token}}).One(appDoc)
	if err == mgo.ErrNotFound {
		return nil, errors.NotFoundf("remote application with token %q", token)
	}
	if err != nil {
		return nil, errors.Annotatef(err, "cannot get remote application with token %q", token)
	}
	return newRemoteApplication(st, appDoc), nil
}

// AllRemoteApplications returns all the remote applications used by the model.
func (st *State) AllRemoteApplications() (applications []*RemoteApplication, err error) {
	applicationsCollection, closer := st.getCollection(remoteApplicationsC)
	defer closer()

	appDocs := []remoteApplicationDoc{}
	err = applicationsCollection.Find(bson.D{}).All(&appDocs)
	if err != nil {
		return nil, errors.Errorf("cannot get all remote applications")
	}
	for _, v := range appDocs {
		applications = append(applications, newRemoteApplication(st, &v))
	}
	return applications, nil
}
