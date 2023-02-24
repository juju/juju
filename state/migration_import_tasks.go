// Copyright 2019 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state

import (
	"github.com/juju/description/v4"
	"github.com/juju/errors"
	"github.com/juju/mgo/v3/txn"
	"github.com/juju/names/v4"

	"github.com/juju/juju/core/crossmodel"
	"github.com/juju/juju/core/permission"
	"github.com/juju/juju/core/secrets"
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

func (s stateApplicationOfferDocumentFactoryShim) MakeIncApplicationOffersRefOp(name string) (txn.Op, error) {
	return incApplicationOffersRefOp(s.importer.st, name)
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
	MakeIncApplicationOffersRefOp(string) (txn.Op, error)
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
	ops := make([]txn.Op, 0)
	for _, offer := range offers {
		appDoc, err := src.MakeApplicationOfferDoc(offer)
		if err != nil {
			return errors.Trace(err)
		}
		appOps, err := i.addApplicationOfferOps(src, addApplicationOfferOpsArgs{
			applicationOfferDoc: appDoc,
			acl:                 offer.ACL(),
		})
		if err != nil {
			return errors.Trace(err)
		}
		ops = append(ops, appOps...)
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
	appName := args.applicationOfferDoc.ApplicationName
	incRefOp, err := src.MakeIncApplicationOffersRefOp(appName)
	if err != nil {
		return nil, errors.Trace(err)
	}
	docID := src.DocID(appName)
	ops := []txn.Op{
		{
			C:      applicationOffersC,
			Id:     docID,
			Assert: txn.DocMissing,
			Insert: args.applicationOfferDoc,
		},
		incRefOp,
	}
	for userName, access := range args.acl {
		user := names.NewUserTag(userName)
		ops = append(ops, createPermissionOp(applicationOfferKey(
			args.applicationOfferDoc.OfferUUID), userGlobalKey(userAccessID(user)), permission.Access(access)))
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
// around importing a remoteApplication, from being accessed
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

// RemoteEntitiesInput describes the input used for migrating remote entities.
type RemoteEntitiesInput interface {
	DocModelNamespace
	RemoteEntitiesDescription
}

// ImportRemoteEntities describes a way to import remote entities from a
// description.
type ImportRemoteEntities struct{}

// Execute the import on the remote entities description, carefully modelling
// the dependencies we have.
func (ImportRemoteEntities) Execute(src RemoteEntitiesInput, runner TransactionRunner) error {
	remoteEntities := src.RemoteEntities()
	if len(remoteEntities) == 0 {
		return nil
	}
	ops := make([]txn.Op, len(remoteEntities))
	for i, entity := range remoteEntities {
		docID := src.DocID(entity.ID())
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

// ExternalControllerStateDocumentFactory creates documents that are useful with
// in the state package. In essence this just allows us to model our
// dependencies correctly without having to construct dependencies everywhere.
// Note: we need public methods here because gomock doesn't mock private methods
type ExternalControllerStateDocumentFactory interface {
	ExternalControllerDoc(string) (*externalControllerDoc, error)
	MakeExternalControllerOp(externalControllerDoc, *externalControllerDoc) txn.Op
}

// ExternalControllersDescription defines an in-place usage for reading external
// controllers
type ExternalControllersDescription interface {
	ExternalControllers() []description.ExternalController
}

// ExternalControllersInput describes the input used for migrating external
// controllers.
type ExternalControllersInput interface {
	ExternalControllerStateDocumentFactory
	ExternalControllersDescription
}

// stateExternalControllerDocumentFactoryShim is required to allow the new
// vertical boundary around importing a external controller, from being accessed
// by the existing state package code.
// That way we can keep the importing code clean from the proliferation of state
// code in the juju code base.
type stateExternalControllerDocumentFactoryShim struct {
	stateModelNamspaceShim
	importer *importer
}

func (s stateExternalControllerDocumentFactoryShim) ExternalControllerDoc(uuid string) (*externalControllerDoc, error) {
	service := NewExternalControllers(s.importer.st)
	return service.controller(uuid)
}

func (s stateExternalControllerDocumentFactoryShim) MakeExternalControllerOp(doc externalControllerDoc, existing *externalControllerDoc) txn.Op {
	return upsertExternalControllerOp(&doc, existing, doc.Models)
}

// ImportExternalControllers describes a way to import external controllers
// from a description.
type ImportExternalControllers struct{}

// Execute the import on the external controllers description, carefully
// modelling the dependencies we have.
func (ImportExternalControllers) Execute(src ExternalControllersInput, runner TransactionRunner) error {
	externalControllers := src.ExternalControllers()
	if len(externalControllers) == 0 {
		return nil
	}

	ops := make([]txn.Op, len(externalControllers))
	for i, entity := range externalControllers {
		controllerID := entity.ID().Id()
		doc := externalControllerDoc{
			Id:     controllerID,
			Alias:  entity.Alias(),
			Addrs:  entity.Addrs(),
			CACert: entity.CACert(),
			Models: entity.Models(),
		}
		existing, err := src.ExternalControllerDoc(controllerID)
		if err != nil && !errors.IsNotFound(err) {
			return errors.Trace(err)
		}
		ops[i] = src.MakeExternalControllerOp(doc, existing)
	}

	if err := runner.RunTransaction(ops); err != nil {
		return errors.Trace(err)
	}
	return nil
}

// SecretsDescription defines an in-place usage for reading secrets.
type SecretsDescription interface {
	Secrets() []description.Secret
}

// SecretsInput describes the input used for migrating secrets.
type SecretsInput interface {
	DocModelNamespace
	SecretsDescription
}

// ImportSecrets describes a way to import secrets from a
// description.
type ImportSecrets struct{}

// Execute the import on the secrets description, carefully modelling
// the dependencies we have.
func (ImportSecrets) Execute(src SecretsInput, runner TransactionRunner) error {
	allSecrets := src.Secrets()
	if len(allSecrets) == 0 {
		return nil
	}

	var ops []txn.Op
	for _, secret := range allSecrets {
		uri := &secrets.URI{ID: secret.Id()}
		docID := src.DocID(secret.Id())
		owner, err := secret.Owner()
		if err != nil {
			return errors.Annotatef(err, "invalid owner for secret %q", secret.Id())
		}
		ops = append(ops, txn.Op{
			C:      secretMetadataC,
			Id:     docID,
			Assert: txn.DocMissing,
			Insert: secretMetadataDoc{
				DocID:            docID,
				Version:          secret.Version(),
				OwnerTag:         owner.String(),
				Description:      secret.Description(),
				Label:            secret.Label(),
				LatestRevision:   secret.LatestRevision(),
				LatestExpireTime: secret.LatestExpireTime(),
				RotatePolicy:     secret.RotatePolicy(),
				CreateTime:       secret.Created(),
				UpdateTime:       secret.Updated(),
			},
		})
		if secret.NextRotateTime() != nil {
			nextRotateTime := secret.NextRotateTime()
			ops = append(ops, txn.Op{
				C:      secretRotateC,
				Id:     docID,
				Assert: txn.DocMissing,
				Insert: secretRotationDoc{
					DocID:          docID,
					NextRotateTime: *nextRotateTime,
				},
			})
		}
		for _, rev := range secret.Revisions() {
			key := secretRevisionKey(uri, rev.Number())
			dataCopy := make(secretsDataMap)
			for k, v := range rev.Content() {
				dataCopy[k] = v
			}
			ops = append(ops, txn.Op{
				C:      secretRevisionsC,
				Id:     key,
				Assert: txn.DocMissing,
				Insert: secretRevisionDoc{
					DocID:      key,
					Revision:   rev.Number(),
					CreateTime: rev.Created(),
					UpdateTime: rev.Updated(),
					ExpireTime: rev.ExpireTime(),
					Obsolete:   rev.Obsolete(),
					Data:       dataCopy,
					OwnerTag:   owner.String(),
				},
			})
		}
		for subject, access := range secret.ACL() {
			key := secretConsumerKey(uri.ID, subject)
			ops = append(ops, txn.Op{
				C:      secretPermissionsC,
				Id:     key,
				Assert: txn.DocMissing,
				Insert: secretPermissionDoc{
					DocID:   key,
					Subject: subject,
					Scope:   access.Scope(),
					Role:    access.Role(),
				},
			})
		}
		for _, info := range secret.Consumers() {
			consumer, err := info.Consumer()
			if err != nil {
				return errors.Annotatef(err, "invalid consumer for secret %q", secret.Id())
			}
			key := secretConsumerKey(uri.ID, consumer.String())
			ops = append(ops, txn.Op{
				C:      secretConsumersC,
				Id:     key,
				Assert: txn.DocMissing,
				Insert: secretConsumerDoc{
					DocID:           key,
					ConsumerTag:     consumer.String(),
					Label:           info.Label(),
					CurrentRevision: info.CurrentRevision(),
					LatestRevision:  info.LatestRevision(),
				},
			})
		}
	}

	if err := runner.RunTransaction(ops); err != nil {
		return errors.Trace(err)
	}
	return nil
}
