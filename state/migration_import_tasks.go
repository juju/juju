// Copyright 2019 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state

import (
	"strings"

	"github.com/juju/collections/set"
	"github.com/juju/description/v9"
	"github.com/juju/errors"
	"github.com/juju/mgo/v3"
	"github.com/juju/mgo/v3/bson"
	"github.com/juju/mgo/v3/txn"
	"github.com/juju/names/v5"

	"github.com/juju/juju/core/crossmodel"
	"github.com/juju/juju/core/network/firewall"
	"github.com/juju/juju/core/permission"
	"github.com/juju/juju/core/secrets"
	"github.com/juju/juju/environs/config"
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
	for userName, access := range args.acl {
		user := names.NewUserTag(userName)
		h := createPermissionOp(applicationOfferKey(
			args.applicationOfferDoc.OfferUUID), userGlobalKey(userAccessID(user)), permission.Access(access))
		ops = append(ops, h)
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

// FirewallRulesDescription defines an in-place usage for reading firewall
// rules.
type FirewallRulesDescription interface {
	FirewallRules() []description.FirewallRule
}

// FirewallRulesInput describes the input used for migrating firewall rules.
type FirewallRulesInput interface {
	FirewallRulesDescription
}

// FirewallRulesOutput describes the methods used to set firewall rules
// on the dest model
type FirewallRulesOutput interface {
	UpdateModelConfig(map[string]interface{}, []string, ...ValidateConfigFunc) error
}

// ImportFirewallRules describes a way to import firewallRules from a
// description.
type ImportFirewallRules struct{}

// Execute the import on the firewall rules description, carefully modelling
// the dependencies we have.
func (rules ImportFirewallRules) Execute(src FirewallRulesInput, dst FirewallRulesOutput) error {
	firewallRules := src.FirewallRules()
	if len(firewallRules) == 0 {
		return nil
	}

	for _, rule := range firewallRules {
		var err error
		cidrs := strings.Join(rule.WhitelistCIDRs(), ",")
		switch firewall.WellKnownServiceType(rule.WellKnownService()) {
		case firewall.SSHRule:
			err = dst.UpdateModelConfig(map[string]interface{}{
				config.SSHAllowKey: cidrs,
			}, nil)
		case firewall.JujuApplicationOfferRule:
			// SAASIngressAllow cannot be empty. If it is, leave as it's default value
			if cidrs != "" {
				err = dst.UpdateModelConfig(map[string]interface{}{
					config.SAASIngressAllowKey: cidrs,
				}, nil)
			}
		}
		if err != nil {
			return errors.Trace(err)
		}
	}
	return nil
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
	RemoteSecrets() []description.RemoteSecret
}

// SecretConsumersState is used to create secret consumer keys
// for use in the state model.
type SecretConsumersState interface {
	SecretConsumerKey(uri *secrets.URI, subject string) string
}

// BackendRevisionCountProcesser is used to create a backend revision reference count.
type BackendRevisionCountProcesser interface {
	IncBackendRevisionCountOps(backendID string) ([]txn.Op, error)
}

// SecretsInput describes the input used for migrating secrets.
type SecretsInput interface {
	DocModelNamespace
	SecretConsumersState
	BackendRevisionCountProcesser
	SecretsDescription
}

type secretStateShim struct {
	stateModelNamspaceShim
}

func (s *secretStateShim) SecretConsumerKey(uri *secrets.URI, subject string) string {
	return s.st.secretConsumerKey(uri, subject)
}

func (s *secretStateShim) IncBackendRevisionCountOps(backendID string) ([]txn.Op, error) {
	return s.st.incBackendRevisionCountOps(backendID, 1)
}

// ImportSecrets describes a way to import secrets from a
// description.
type ImportSecrets struct{}

// Execute the import on the secrets description, carefully modelling
// the dependencies we have.
func (ImportSecrets) Execute(src SecretsInput, runner TransactionRunner, knownSecretBackends set.Strings) error {
	allSecrets := src.Secrets()
	if len(allSecrets) == 0 {
		return nil
	}

	seenBackendIds := set.NewStrings()
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
				DocID:                  docID,
				Version:                secret.Version(),
				OwnerTag:               owner.String(),
				Description:            secret.Description(),
				Label:                  secret.Label(),
				LatestRevision:         secret.LatestRevision(),
				LatestRevisionChecksum: secret.LatestRevisionChecksum(),
				LatestExpireTime:       secret.LatestExpireTime(),
				RotatePolicy:           secret.RotatePolicy(),
				AutoPrune:              secret.AutoPrune(),
				CreateTime:             secret.Created(),
				UpdateTime:             secret.Updated(),
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
			var valueRef *valueRefDoc
			if len(dataCopy) == 0 {
				valueRef = &valueRefDoc{
					BackendID:  rev.ValueRef().BackendID(),
					RevisionID: rev.ValueRef().RevisionID(),
				}
				if !secrets.IsInternalSecretBackendID(valueRef.BackendID) && !seenBackendIds.Contains(valueRef.BackendID) {
					if !knownSecretBackends.Contains(valueRef.BackendID) {
						return errors.New("target controller does not have all required secret backends set up")
					}
					ops = append(ops, txn.Op{
						C:      secretBackendsC,
						Id:     valueRef.BackendID,
						Assert: txn.DocExists,
					})
				}
				seenBackendIds.Add(valueRef.BackendID)
			}
			ops = append(ops, txn.Op{
				C:      secretRevisionsC,
				Id:     key,
				Assert: txn.DocMissing,
				Insert: secretRevisionDoc{
					DocID:         key,
					Revision:      rev.Number(),
					CreateTime:    rev.Created(),
					UpdateTime:    rev.Updated(),
					ExpireTime:    rev.ExpireTime(),
					Obsolete:      rev.Obsolete(),
					PendingDelete: rev.PendingDelete(),
					Data:          dataCopy,
					ValueRef:      valueRef,
					OwnerTag:      owner.String(),
				},
			})
			if valueRef != nil {
				refOps, err := src.IncBackendRevisionCountOps(valueRef.BackendID)
				if err != nil {
					return errors.Trace(err)
				}
				ops = append(ops, refOps...)
			}
		}
		for subject, access := range secret.ACL() {
			key := src.SecretConsumerKey(uri, subject)
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
			key := src.SecretConsumerKey(uri, consumer.String())
			currentRev := info.CurrentRevision()
			latestRev := info.LatestRevision()
			// Older models may have set the consumed rev info to 0 (assuming the latest revision always).
			// So set the latest values explicitly.
			if currentRev == 0 {
				currentRev = secret.LatestRevision()
				latestRev = secret.LatestRevision()
			}
			ops = append(ops, txn.Op{
				C:      secretConsumersC,
				Id:     key,
				Assert: txn.DocMissing,
				Insert: secretConsumerDoc{
					DocID:           key,
					ConsumerTag:     consumer.String(),
					Label:           info.Label(),
					CurrentRevision: currentRev,
					LatestRevision:  latestRev,
				},
			})
		}
		for _, info := range secret.RemoteConsumers() {
			consumer, err := info.Consumer()
			if err != nil {
				return errors.Annotatef(err, "invalid consumer for secret %q", secret.Id())
			}
			key := src.SecretConsumerKey(uri, consumer.String())
			ops = append(ops, txn.Op{
				C:      secretRemoteConsumersC,
				Id:     key,
				Assert: txn.DocMissing,
				Insert: secretRemoteConsumerDoc{
					DocID:           key,
					ConsumerTag:     consumer.String(),
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

// RemoteSecretsInput describes the input used for migrating remote secret consumer info.
type RemoteSecretsInput interface {
	DocModelNamespace
	SecretConsumersState
	SecretsDescription
}

// ImportRemoteSecrets describes a way to import remote
// secrets from a description.
type ImportRemoteSecrets struct{}

// Execute the import on the remote secrets description.
func (ImportRemoteSecrets) Execute(src RemoteSecretsInput, runner TransactionRunner) error {
	allRemoteSecrets := src.RemoteSecrets()
	if len(allRemoteSecrets) == 0 {
		return nil
	}

	var ops []txn.Op
	for _, info := range allRemoteSecrets {
		uri := &secrets.URI{ID: info.ID(), SourceUUID: info.SourceUUID()}
		consumer, err := info.Consumer()
		if err != nil {
			return errors.Annotatef(err, "invalid consumer for remote secret %q", uri)
		}
		key := src.SecretConsumerKey(uri, consumer.String())
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

	if err := runner.RunTransaction(ops); err != nil {
		return errors.Trace(err)
	}
	return nil
}

// VirtualHostKeysDescription defines an in-place usage for reading virtual host keys.
type VirtualHostKeysDescription interface {
	VirtualHostKeys() []description.VirtualHostKey
}

// VirtualHostKeysInput describes the input used for migrating virtual host keys info.
type VirtualHostKeysInput interface {
	DocModelNamespace
	VirtualHostKeysDescription
}

// ImportVirtualHostKeys describes a way to import
// virtual host keys from a description.
type ImportVirtualHostKeys struct{}

// Execute the import on the virtual host keys description.
func (ImportVirtualHostKeys) Execute(src VirtualHostKeysInput, runner TransactionRunner) error {
	allVirtualHostKeys := src.VirtualHostKeys()
	if len(allVirtualHostKeys) == 0 {
		return nil
	}

	var ops []txn.Op
	for _, info := range allVirtualHostKeys {
		docID := src.DocID(info.ID())
		ops = append(ops, txn.Op{
			C:      virtualHostKeysC,
			Id:     docID,
			Assert: txn.DocMissing,
			Insert: virtualHostKeyDoc{
				DocId:   docID,
				HostKey: info.HostKey(),
			},
		})
	}

	if err := runner.RunTransaction(ops); err != nil {
		return errors.Trace(err)
	}
	return nil
}
