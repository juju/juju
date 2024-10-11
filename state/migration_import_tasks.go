// Copyright 2019 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state

import (
	"strings"

	"github.com/juju/description/v8"
	"github.com/juju/errors"
	"github.com/juju/mgo/v3"
	"github.com/juju/mgo/v3/bson"
	"github.com/juju/mgo/v3/txn"
	"github.com/juju/names/v5"

	"github.com/juju/juju/core/crossmodel"
)

// Migration import tasks provide a boundary of isolation between the
// description package and the state package. Input types are modelled as small
// descrete interfaces, that can be composed to provide more functionality.
// Output types, normally a transaction runner can then take the migrated
// description entity as a txn.Op.
//
// The goal of these input tasks are to be moved out of the state package into
// a similar setup as export migrations. That way we can isolate migrations away
// from state and start creating richer types.
//
// Modelling it this way should provide better test coverage and protection
// around state changes.

// TransactionRunner is an in-place usage for running transactions to a
// persistence store.
type TransactionRunner interface {
	RunTransaction([]txn.Op) error
}

// DocModelNamespace takes a document model ID and ensures it has a model id
// associated with the model.
type DocModelNamespace interface {
	DocID(string) string
}

type stateModelNamspaceShim struct {
	description.Model
	st *State
}

func (s stateModelNamspaceShim) DocID(localID string) string {
	return s.st.docID(localID)
}

// stateApplicationOfferDocumentFactoryShim is required to allow the new
// vertical boundary around importing a applicationOffer, from being accessed by
// the existing state package code.
// That way we can keep the importing code clean from the proliferation of state
// code in the juju code base.
type stateApplicationOfferDocumentFactoryShim struct {
	stateModelNamspaceShim
	importer *importer
}

func (s stateApplicationOfferDocumentFactoryShim) MakeApplicationOfferDoc(app description.ApplicationOffer) (applicationOfferDoc, error) {
	ao := &applicationOffers{st: s.importer.st}
	return ao.makeApplicationOfferDoc(s.importer.st, app.OfferUUID(), crossmodel.AddApplicationOfferArgs{
		OfferName:              app.OfferName(),
		ApplicationName:        app.ApplicationName(),
		ApplicationDescription: app.ApplicationDescription(),
		Endpoints:              app.Endpoints(),
	}), nil
}

func (s stateApplicationOfferDocumentFactoryShim) MakeApplicationOffersRefOp(name string, startCnt int) (txn.Op, error) {
	return newApplicationOffersRefOp(s.importer.st, name, startCnt)
}

type applicationDescriptionShim struct {
	stateApplicationOfferDocumentFactoryShim
	ApplicationDescription
}

// ApplicationDescription is an in-place description of an application
type ApplicationDescription interface {
	Offers() []description.ApplicationOffer
}

// ApplicationOfferStateDocumentFactory creates documents that are useful with
// in the state package. In essence this just allows us to model our
// dependencies correctly without having to construct dependencies everywhere.
// Note: we need public methods here because gomock doesn't mock private methods
type ApplicationOfferStateDocumentFactory interface {
	MakeApplicationOfferDoc(description.ApplicationOffer) (applicationOfferDoc, error)
	MakeApplicationOffersRefOp(string, int) (txn.Op, error)
}

// ApplicationOfferDescription defines an in-place usage for reading
// application offers.
type ApplicationOfferDescription interface {
	Offers() []description.ApplicationOffer
}

// ApplicationOfferInput describes the input used for migrating application
// offers.
type ApplicationOfferInput interface {
	DocModelNamespace
	ApplicationOfferStateDocumentFactory
	ApplicationOfferDescription
}

// ImportApplicationOffer describes a way to import application offers from a
// description.
type ImportApplicationOffer struct {
}

// Execute the import on the application offer description, carefully modelling
// the dependencies we have.
func (i ImportApplicationOffer) Execute(src ApplicationOfferInput,
	runner TransactionRunner,
) error {
	offers := src.Offers()
	if len(offers) == 0 {
		return nil
	}
	refCounts := make(map[string]int, len(offers))
	ops := make([]txn.Op, 0)
	for _, offer := range offers {
		appDoc, err := src.MakeApplicationOfferDoc(offer)
		if err != nil {
			return errors.Trace(err)
		}
		appOps, err := i.addApplicationOfferOps(src,
			addApplicationOfferOpsArgs{
				applicationOfferDoc: appDoc,
				acl:                 offer.ACL(),
			})
		if err != nil {
			return errors.Trace(err)
		}
		ops = append(ops, appOps...)
		appName := offer.ApplicationName()
		if appCnt, ok := refCounts[appName]; ok {
			refCounts[appName] = appCnt + 1
		} else {
			refCounts[appName] = 1
		}
	}
	// range the offers again to create refcount docs, an application
	// may have more than one offer.
	for appName, cnt := range refCounts {
		refCntOpps, err := src.MakeApplicationOffersRefOp(appName, cnt)
		if err != nil {
			return errors.Trace(err)
		}
		ops = append(ops, refCntOpps)
	}
	if err := runner.RunTransaction(ops); err != nil {
		return errors.Trace(err)
	}
	return nil
}

type addApplicationOfferOpsArgs struct {
	applicationOfferDoc applicationOfferDoc
	acl                 map[string]string
}

func (i ImportApplicationOffer) addApplicationOfferOps(src ApplicationOfferInput,
	args addApplicationOfferOpsArgs,
) ([]txn.Op, error) {
	ops := []txn.Op{
		{
			C:      applicationOffersC,
			Id:     src.DocID(args.applicationOfferDoc.OfferName),
			Assert: txn.DocMissing,
			Insert: args.applicationOfferDoc,
		},
	}
	return ops, nil
}

// StateDocumentFactory creates documents that are useful with in the state
// package. In essence this just allows us to model our dependencies correctly
// without having to construct dependencies everywhere.
// Note: we need public methods here because gomock doesn't mock private methods
type StateDocumentFactory interface {
	NewRemoteApplication(*remoteApplicationDoc) *RemoteApplication
	MakeRemoteApplicationDoc(description.RemoteApplication) *remoteApplicationDoc
	MakeStatusDoc(description.Status) statusDoc
	MakeStatusOp(string, statusDoc) txn.Op
}

// stateDocumentFactoryShim is required to allow the new vertical boundary
// around importing a remoteApplication and firewallRules, from being accessed
// by the existing state package code.
// That way we can keep the importing code clean from the proliferation of state
// code in the juju code base.
type stateDocumentFactoryShim struct {
	stateModelNamspaceShim
	importer *importer
}

func (s stateDocumentFactoryShim) NewRemoteApplication(doc *remoteApplicationDoc) *RemoteApplication {
	return newRemoteApplication(s.importer.st, doc)
}

func (s stateDocumentFactoryShim) MakeRemoteApplicationDoc(app description.RemoteApplication) *remoteApplicationDoc {
	return s.importer.makeRemoteApplicationDoc(app)
}

func (s stateDocumentFactoryShim) MakeStatusDoc(status description.Status) statusDoc {
	return s.importer.makeStatusDoc(status)
}

func (s stateDocumentFactoryShim) MakeStatusOp(globalKey string, doc statusDoc) txn.Op {
	return createStatusOp(s.importer.st, globalKey, doc)
}

// RemoteApplicationsDescription defines an in-place usage for reading remote
// applications.
type RemoteApplicationsDescription interface {
	RemoteApplications() []description.RemoteApplication
}

// RemoteApplicationsInput describes the input used for migrating remote
// applications.
type RemoteApplicationsInput interface {
	DocModelNamespace
	StateDocumentFactory
	RemoteApplicationsDescription
}

// ImportRemoteApplications describes a way to import remote applications from a
// description.
type ImportRemoteApplications struct{}

// Execute the import on the remote entities description, carefully modelling
// the dependencies we have.
func (i ImportRemoteApplications) Execute(src RemoteApplicationsInput, runner TransactionRunner) error {
	remoteApplications := src.RemoteApplications()
	if len(remoteApplications) == 0 {
		return nil
	}
	ops := make([]txn.Op, 0)
	for _, app := range remoteApplications {
		appDoc := src.MakeRemoteApplicationDoc(app)

		// Status maybe empty for some remoteApplications. Ensure we handle
		// that correctly by checking if we get one before making a new
		// StatusDoc
		var appStatusDoc *statusDoc
		if status := app.Status(); status != nil {
			doc := src.MakeStatusDoc(status)
			appStatusDoc = &doc
		}
		app := src.NewRemoteApplication(appDoc)

		remoteAppOps, err := i.addRemoteApplicationOps(src, app, addRemoteApplicationOpsArgs{
			remoteApplicationDoc: appDoc,
			statusDoc:            appStatusDoc,
		})
		if err != nil {
			return errors.Trace(err)
		}
		ops = append(ops, remoteAppOps...)
	}
	if err := runner.RunTransaction(ops); err != nil {
		return errors.Trace(err)
	}
	return nil
}

type addRemoteApplicationOpsArgs struct {
	remoteApplicationDoc *remoteApplicationDoc
	statusDoc            *statusDoc
}

func (i ImportRemoteApplications) addRemoteApplicationOps(src RemoteApplicationsInput,
	app *RemoteApplication,
	args addRemoteApplicationOpsArgs,
) ([]txn.Op, error) {
	globalKey := app.globalKey()
	docID := src.DocID(app.Name())

	ops := []txn.Op{
		{
			C:      applicationsC,
			Id:     app.Name(),
			Assert: txn.DocMissing,
		},
		{
			C:      remoteApplicationsC,
			Id:     docID,
			Assert: txn.DocMissing,
			Insert: args.remoteApplicationDoc,
		},
	}
	// The status doc can be optional with a remoteApplication. To ensure that
	// we correctly handle this situation check for it.
	if args.statusDoc != nil {
		ops = append(ops, src.MakeStatusOp(globalKey, *args.statusDoc))
	}

	return ops, nil
}

// RemoteEntitiesDescription defines an in-place usage for reading remote entities.
type RemoteEntitiesDescription interface {
	RemoteEntities() []description.RemoteEntity
}

// ApplicationOffersState is used to look up all application offers.
type ApplicationOffersState interface {
	OfferUUIDForApp(appName string) (string, error)
}

// RemoteEntitiesInput describes the input used for migrating remote entities.
type RemoteEntitiesInput interface {
	DocModelNamespace
	RemoteEntitiesDescription
	ApplicationOffersState

	// OfferUUID returns the uuid for a given offer name.
	OfferUUID(offerName string) (string, bool)
}

// ImportRemoteEntities describes a way to import remote entities from a
// description.
type ImportRemoteEntities struct{}

// Execute the import on the remote entities description, carefully modelling
// the dependencies we have.
func (im *ImportRemoteEntities) Execute(src RemoteEntitiesInput, runner TransactionRunner) error {
	remoteEntities := src.RemoteEntities()
	if len(remoteEntities) == 0 {
		return nil
	}
	ops := make([]txn.Op, len(remoteEntities))
	for i, entity := range remoteEntities {
		var (
			id  string
			ok  bool
			err error
		)
		if id, ok = im.maybeConvertApplicationOffer(src, entity.ID()); !ok {
			id, err = im.legacyAppToOffer(entity.ID(), src.OfferUUIDForApp)
			if err != nil {
				return errors.Trace(err)
			}
		}
		docID := src.DocID(id)
		ops[i] = txn.Op{
			C:      remoteEntitiesC,
			Id:     docID,
			Assert: txn.DocMissing,
			Insert: &remoteEntityDoc{
				DocID: docID,
				Token: entity.Token(),
			},
		}
	}
	if err := runner.RunTransaction(ops); err != nil {
		return errors.Trace(err)
	}
	return nil
}

// maybeConvertApplicationOffer returns the offer uuid if an offer name is passed in.
func (im *ImportRemoteEntities) maybeConvertApplicationOffer(src RemoteEntitiesInput, id string) (string, bool) {
	if !strings.HasPrefix(id, names.ApplicationOfferTagKind+"-") {
		return id, false
	}
	offerName := strings.TrimPrefix(id, names.ApplicationOfferTagKind+"-")
	if uuid, ok := src.OfferUUID(offerName); ok {
		return names.NewApplicationOfferTag(uuid).String(), true
	}
	return id, false
}

func (im *ImportRemoteEntities) legacyAppToOffer(id string, offerUUIDForApp func(string) (string, error)) (string, error) {
	tag, err := names.ParseTag(id)
	if err != nil || tag.Kind() != names.ApplicationTagKind || strings.HasPrefix(tag.Id(), "remote-") {
		return id, err
	}
	offerUUID, err := offerUUIDForApp(tag.Id())
	if errors.Is(err, errors.NotFound) {
		return id, nil
	}

	return names.NewApplicationOfferTag(offerUUID).String(), err
}

type applicationOffersStateShim struct {
	stateModelNamspaceShim

	offerUUIDByName map[string]string
}

func (s *applicationOffersStateShim) OfferUUID(offerName string) (string, bool) {
	uuid, ok := s.offerUUIDByName[offerName]
	return uuid, ok
}

func (a applicationOffersStateShim) OfferUUIDForApp(appName string) (string, error) {
	applicationOffersCollection, closer := a.st.db().GetCollection(applicationOffersC)
	defer closer()

	var doc applicationOfferDoc
	err := applicationOffersCollection.Find(bson.D{{"application-name", appName}}).One(&doc)
	if err == mgo.ErrNotFound {
		return "", errors.NotFoundf("offer for app %q", appName)
	}
	if err != nil {
		return "", errors.Annotate(err, "getting application offer documents")
	}
	return doc.OfferUUID, nil
}

// RelationNetworksDescription defines an in-place usage for reading relation networks.
type RelationNetworksDescription interface {
	RelationNetworks() []description.RelationNetwork
}

// RelationNetworksInput describes the input used for migrating relation
// networks.
type RelationNetworksInput interface {
	DocModelNamespace
	RelationNetworksDescription
}

// ImportRelationNetworks describes a way to import relation networks from a
// description.
type ImportRelationNetworks struct{}

// Execute the import on the relation networks description, carefully modelling
// the dependencies we have.
func (ImportRelationNetworks) Execute(src RelationNetworksInput, runner TransactionRunner) error {
	relationNetworks := src.RelationNetworks()
	if len(relationNetworks) == 0 {
		return nil
	}

	ops := make([]txn.Op, len(relationNetworks))
	for i, entity := range relationNetworks {
		docID := src.DocID(entity.ID())
		ops[i] = txn.Op{
			C:      relationNetworksC,
			Id:     docID,
			Assert: txn.DocMissing,
			Insert: relationNetworksDoc{
				Id:          docID,
				RelationKey: entity.RelationKey(),
				CIDRS:       entity.CIDRS(),
			},
		}
	}

	if err := runner.RunTransaction(ops); err != nil {
		return errors.Trace(err)
	}
	return nil
}
